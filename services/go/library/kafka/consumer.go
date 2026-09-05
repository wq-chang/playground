package kafka

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"

	"go-services/library/kafka/internal/consumer"
)

// AckMode determines how the consumer acknowledges records.
type AckMode = consumer.AckMode

const (
	// AckModeAtLeastOnce ensures records are processed at least once.
	AckModeAtLeastOnce = consumer.AckModeAtLeastOnce

	// AckModeAtMostOnce ensures records are processed at most once.
	AckModeAtMostOnce = consumer.AckModeAtMostOnce
)

// DLQProducer is the narrow interface Consumer needs for dead-letter publishing.
// Client.Producer satisfies this interface.
type DLQProducer interface {
	ProduceSync(ctx context.Context, record *kgo.Record) error
}

// Consumer is the topic subscription and consumption API.
type Consumer struct {
	cfg          *config
	kgoClient    *kgo.Client
	dlqProducer  DLQProducer
	log          *slog.Logger
	router       *consumer.Router
	pauses       *consumer.PauseRegistry
	runState     *consumer.RunState
	registry     *consumer.PartitionRegistry
	committer    *consumer.Committer
	dispatcher   *consumer.Dispatcher
	workerRunner *consumer.WorkerRunner
	// fetchResumer clears stale partition-level pauses in the kgo client when
	// partitions are revoked. The full client satisfies this interface; it is
	// an interface so rebalance behavior is unit-testable.
	fetchResumer fetchResumer
}

// fetchResumer is the subset of *kgo.Client used to clear stale
// partition-level pauses when partitions are revoked. kgo's pause set is
// sticky and survives rebalances, so a leftover pause must be cleared
// explicitly. The full client satisfies this interface.
type fetchResumer interface {
	ResumeFetchPartitions(topicPartitions map[string][]int32)
}

// commitOffsetsSync wraps kgo.Client.CommitOffsetsSync's callback-based API
// into a synchronous error-returning function that satisfies consumer.OffsetClient.
func commitOffsetsSync(kcl *kgo.Client) consumer.OffsetClient {
	return consumer.OffsetClientFunc(func(ctx context.Context, offsets map[string]map[int32]kgo.EpochOffset) error {
		var commitErr error
		kcl.CommitOffsetsSync(ctx, offsets, func(
			_ *kgo.Client,
			_ *kmsg.OffsetCommitRequest,
			resp *kmsg.OffsetCommitResponse,
			err error,
		) {
			commitErr = errors.Join(err, commitResponseError(resp))
		})
		return commitErr
	})
}

// commitResponseError folds per-partition OffsetCommit response error codes
// into a single error. kgo surfaces only transport errors through the sync
// callback's error; per-partition codes (e.g. IllegalGeneration, which occur
// when a commit races a rebalance) live in the response and are documented to
// be checked by the caller.
//
// RebalanceInProgress is intentionally skipped: kgo synthesizes that code for
// partitions it filtered out of the wire request when a commit crossed a
// generation change, and at this layer it is a benign "rejoin and retry"
// signal, not a failure of the offsets we still own.
func commitResponseError(resp *kmsg.OffsetCommitResponse) error {
	if resp == nil {
		return nil
	}
	var errs []error
	for _, topic := range resp.Topics {
		for _, p := range topic.Partitions {
			if p.ErrorCode == 0 || p.ErrorCode == kerr.RebalanceInProgress.Code {
				continue
			}
			errs = append(errs, fmt.Errorf(
				"offset commit failed for topic %q partition %d: %w",
				topic.Topic, p.Partition, kerr.ErrorForCode(p.ErrorCode),
			))
		}
	}
	return errors.Join(errs...)
}

// newConsumer creates a Consumer from the shared config, kgo client, and
// optional DLQ producer.
func newConsumer(cfg *config, kgoClient *kgo.Client, dlqProducer DLQProducer) (*Consumer, error) {
	logger := cfg.logger

	router := consumer.NewRouter(kgoClient.AddConsumeTopics)
	pauses := consumer.NewPauseRegistry(time.Now, kgoClient)
	runState := consumer.NewRunState()

	registry := consumer.NewPartitionRegistry(logger)
	committer := consumer.NewCommitter(
		registry,
		commitOffsetsSync(kgoClient),
		consumer.CommitConfig{
			FlushInterval:    cfg.flushInterval,
			DebounceInterval: cfg.debounceInterval,
		},
	)

	executor := consumer.NewRecordExecutor(logger)

	dlqWriter := func(ctx context.Context, record *kgo.Record) error {
		if dlqProducer != nil {
			return dlqProducer.ProduceSync(ctx, record)
		}
		return fmt.Errorf("dlq publishing requires a producer-enabled client")
	}

	capacityCh := make(chan struct{}, 1)

	workerRunner := consumer.NewWorkerRunner(
		logger,
		runState,
		committer,
		executor,
		registry,
		pauses,
		cfg.workers,
		capacityCh,
		dlqWriter,
		kgoClient,
	)

	dispatcher := consumer.NewDispatcher(
		router,
		pauses,
		registry,
		kgoClient,
		workerRunner.Start,
		cfg.queueCapacity,
		capacityCh,
	)

	subs := make([]Subscription, 0, len(cfg.subscriptions))
	for _, sub := range cfg.subscriptions {
		subs = append(subs, sub)
	}
	if len(subs) > 0 {
		if err := router.RegisterQuietBatch(subs); err != nil {
			return nil, fmt.Errorf("subscription init: %w", err)
		}
	}

	return &Consumer{
		cfg:          cfg,
		kgoClient:    kgoClient,
		dlqProducer:  dlqProducer,
		log:          logger,
		router:       router,
		pauses:       pauses,
		runState:     runState,
		registry:     registry,
		committer:    committer,
		dispatcher:   dispatcher,
		workerRunner: workerRunner,
		fetchResumer: kgoClient,
	}, nil
}

// AddSubscription registers a new topic subscription.
func (c *Consumer) AddSubscription(subscription Subscription) error {
	normalized, err := subscription.Normalize()
	if err != nil {
		return err
	}
	return c.router.Register(normalized)
}

// AddTopic registers a single-record handler using the default ack mode.
func (c *Consumer) AddTopic(topic string, handler Handler) error {
	return c.AddSubscription(newDefaultSubscription(topic, handler, c.cfg.defaultAckMode))
}

// AddBatchTopic registers a batch handler using the default ack mode.
func (c *Consumer) AddBatchTopic(topic string, handler BatchHandler) error {
	return c.AddSubscription(newDefaultBatchSubscription(topic, handler, c.cfg.defaultAckMode))
}

// Run starts the consumer loop.
func (c *Consumer) Run(ctx context.Context) error {
	if c.kgoClient == nil {
		return fmt.Errorf("consumer client is not initialized")
	}

	runCtx, err := c.runState.Begin()
	if err != nil {
		return err
	}

	pollCtx, stopPolling := mergeRunContexts(ctx, runCtx)
	defer stopPolling()

	// Start commit loop.
	c.runState.Go(func() {
		if commitErr := c.committer.Run(runCtx); commitErr != nil {
			c.runState.Fail(commitErr)
		}
	})

	// Dispatch loop.
	err = c.runDispatch(pollCtx)

	// Shut down all partition workers unconditionally so that Wait() can
	// complete even when a fatal error has been recorded.
	states := c.registry.BeginClosingAll()

	if runErr := c.runState.Err(); runErr == nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), c.cfg.shutdownTimeout)
		defer cancel()
		if shutdownErr := c.committer.Finalize(
			shutdownCtx,
			states,
			"failed to commit processed offsets on shutdown"); shutdownErr != nil {
			if err == nil {
				err = shutdownErr
			}
		}
	} else {
		c.registry.Cleanup(states)
	}

	c.runState.Stop()

	// Bound the worker wait with the shutdown timeout. Finalize honors the
	// timeout, but a handler that ignores context cancellation can pin a
	// partition worker forever, and an unbounded wait would hang Run
	// regardless. On timeout the consumer is left in an unusable state
	// (no Reset), so it cannot be recycled.
	if !c.waitForWorkersToStop(ctx) {
		if cause := c.runState.Err(); cause != nil {
			return fmt.Errorf(
				"consumer shutdown exceeded %s while handling fatal error: %w; consumer left in unusable state",
				c.cfg.shutdownTimeout,
				cause,
			)
		}
		return fmt.Errorf("consumer shutdown exceeded %s; consumer left in unusable state", c.cfg.shutdownTimeout)
	}

	fatalErr := c.runState.Err()
	c.runState.Reset()

	// Error priority.
	switch {
	case fatalErr != nil:
		return fatalErr
	case err == nil || errors.Is(err, context.Canceled):
		return ctx.Err()
	default:
		return err
	}
}

// waitForWorkersToStop waits for all pipeline goroutines to stop, bounded by
// the configured shutdown timeout. Returns true when all goroutines exited;
// false when the timeout elapsed first (the remaining goroutines are
// abandoned and logged as an error).
func (c *Consumer) waitForWorkersToStop(ctx context.Context) bool {
	done := make(chan struct{})
	go func() {
		c.runState.Wait()
		close(done)
	}()

	timer := time.NewTimer(c.cfg.shutdownTimeout)
	defer timer.Stop()

	select {
	case <-done:
		return true
	case <-timer.C:
		c.log.ErrorContext(
			ctx,
			"consumer shutdown exceeded shutdownTimeout; abandoning pipeline goroutines",
			"timeout", c.cfg.shutdownTimeout,
		)
		return false
	}
}

func (c *Consumer) runDispatch(ctx context.Context) error {
	cl := c.kgoClient
	maxRecords := c.cfg.fetchMaxRecords
	for {
		fetches := cl.PollRecords(ctx, maxRecords)
		if fetches.IsClientClosed() {
			return nil
		}
		if err := fetches.Err(); err != nil {
			cl.AllowRebalance()
			if ctx.Err() != nil {
				return nil
			}
			c.log.WarnContext(ctx, "kafka poll error", "err", err)
			continue
		}
		records := fetches.Records()
		if len(records) > 0 {
			if err := c.dispatcher.Dispatch(ctx, c.runState.Context(), records); err != nil {
				return err
			}
		}
		cl.AllowRebalance()
	}
}

// onPartitionsRevoked handles partition revocation.
func (c *Consumer) onPartitionsRevoked(
	ctx context.Context,
	cl *kgo.Client,
	partitions map[string][]int32,
) {
	if len(partitions) == 0 {
		return
	}

	// Why resume and not pause: kgo's pause set is sticky — revoking a
	// partition does NOT clear a partition-level pause (verified in
	// franz-go v1.21.1: the paused set is only mutated by the public
	// Pause*/Resume* methods; the join/sync/revoke machinery never touches
	// it), and fetch-request building skips any cursor where
	// paused.has(topic, partition). A leftover pause — from the dispatcher's
	// backpressure pause, or a previous revoke — would therefore silently
	// starve the partition if it is ever re-assigned to this member.
	// Pausing revoked partitions is also unnecessary: with
	// BlockRebalanceOnPoll, franz-go guarantees that no subsequent poll
	// returns records for revoked partitions once this callback completes.
	// So instead of pausing, we resume — clearing stale partition-level
	// pauses exactly when the partition leaves us. Topic-level pauses from
	// the stop-on-exhausted policy (PauseFetchTopics) are a separate kgo
	// pause set and are not affected.
	if c.fetchResumer != nil {
		c.fetchResumer.ResumeFetchPartitions(partitions)
	} else if cl != nil {
		cl.ResumeFetchPartitions(partitions)
	}

	states := c.registry.BeginClosing(partitions)

	// The rebalance hook's context is the client's context with no deadline,
	// so bound the drain + final commit ourselves: draining past the group's
	// rebalance timeout gets this member kicked and stalls the whole group's
	// rebalance. A bounded timeout converts that hang into runState.Fail and
	// a restart, mirroring the graceful-shutdown path. Deriving from the
	// hook context (not context.Background) preserves the existing
	// "callback context cancellation aborts the revoke" semantics.
	revokeCtx, cancel := context.WithTimeout(ctx, c.cfg.shutdownTimeout)
	defer cancel()

	if err := c.committer.Finalize(revokeCtx, states, "failed to commit processed offsets on revoke"); err != nil {
		c.log.ErrorContext(ctx, "failed to commit processed offsets on revoke", "err", err)
		c.runState.Fail(err)
	}
}

// onPartitionsLost handles lost partitions by dropping in-memory commit progress.
func (c *Consumer) onPartitionsLost(
	ctx context.Context,
	partitions map[string][]int32,
) {
	c.log.WarnContext(ctx, "partitions lost; dropping in-memory commit progress", "partitions", partitions)
	c.registry.DropLost(partitions)
}

// mergeRunContexts combines the parent context with the run context so that
// cancellation of either stops the poll loop.
func mergeRunContexts(parent, run context.Context) (context.Context, context.CancelFunc) {
	mergedCtx, mergedCancel := context.WithCancel(context.Background())
	go func() {
		select {
		case <-parent.Done():
			mergedCancel()
		case <-run.Done():
			mergedCancel()
		case <-mergedCtx.Done():
		}
	}()
	return mergedCtx, mergedCancel
}
