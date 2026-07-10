package kafka

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

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
	drainTimeout time.Duration
}

// commitOffsetsSync wraps kgo.Client.CommitOffsetsSync's callback-based API
// into a synchronous error-returning function that satisfies consumer.OffsetClient.
func commitOffsetsSync(kcl *kgo.Client) consumer.OffsetClient {
	return offsetCommitter{kcl: kcl}
}

type offsetCommitter struct {
	kcl *kgo.Client
}

func (o offsetCommitter) CommitOffsetsSync(ctx context.Context, offsets map[string]map[int32]kgo.EpochOffset) error {
	var commitErr error
	o.kcl.CommitOffsetsSync(ctx, offsets, func(
		_ *kgo.Client,
		_ *kmsg.OffsetCommitRequest,
		_ *kmsg.OffsetCommitResponse,
		err error,
	) {
		commitErr = err
	})
	return commitErr
}

// newConsumer creates a Consumer from the shared config, kgo client, and
// optional DLQ producer.
func newConsumer(cfg *config, kgoClient *kgo.Client, dlqProducer DLQProducer) (*Consumer, error) {
	v2 := &Consumer{
		cfg:          cfg,
		kgoClient:    kgoClient,
		dlqProducer:  dlqProducer,
		log:          cfg.logger,
		router:       nil,
		pauses:       nil,
		runState:     nil,
		registry:     nil,
		committer:    nil,
		dispatcher:   nil,
		workerRunner: nil,
		drainTimeout: cfg.drainTimeout,
	}

	v2.router = consumer.NewRouter(kgoClient.AddConsumeTopics)
	v2.pauses = consumer.NewPauseRegistry(time.Now)
	v2.runState = consumer.NewRunState()

	v2.registry = consumer.NewPartitionRegistry(v2.log)
	v2.committer = consumer.NewCommitter(
		v2.registry,
		commitOffsetsSync(kgoClient),
		consumer.CommitConfig{},
	)

	executor := consumer.NewRecordExecutor(v2.log)

	dlqWriter := func(ctx context.Context, record *kgo.Record) error {
		if dlqProducer != nil {
			return dlqProducer.ProduceSync(ctx, record)
		}
		return fmt.Errorf("dlq publishing requires a producer-enabled client")
	}

	capacityCh := make(chan struct{}, 1)

	v2.workerRunner = consumer.NewWorkerRunner(
		v2.log,
		v2.runState,
		v2.committer,
		executor,
		v2.registry,
		v2.pauses,
		v2.cfg.workers,
		capacityCh,
		dlqWriter,
		kgoClient,
	)

	v2.dispatcher = consumer.NewDispatcher(
		v2.router,
		v2.pauses,
		v2.registry,
		kgoClient,
		v2.workerRunner.Start,
		cfg.queueCapacity,
		capacityCh,
	)

	subs := make([]Subscription, 0, len(cfg.subscriptions))
	for _, sub := range cfg.subscriptions {
		subs = append(subs, sub)
	}
	if len(subs) > 0 {
		if err := v2.router.RegisterQuietBatch(subs); err != nil {
			return nil, fmt.Errorf("subscription init: %w", err)
		}
	}

	return v2, nil
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
	runCtx, err := c.runState.Begin()
	if err != nil {
		return err
	}

	pollCtx, stopPolling := mergeRunContexts(ctx, runCtx)
	defer stopPolling()

	if c.kgoClient == nil {
		return fmt.Errorf("consumer client is not initialized")
	}

	// Start commit loop.
	c.runState.Go(func() {
		if commitErr := c.committer.Run(runCtx); commitErr != nil {
			c.runState.Fail(commitErr)
		}
	})

	// Dispatch loop.
	err = c.runDispatch(pollCtx)

	// Graceful shutdown (only if no fatal error).
	if runErr := c.runState.Err(); runErr == nil {
		states := c.registry.BeginClosingAll()
		drainCtx, drainCancel := context.WithTimeout(context.Background(), c.drainTimeout)
		defer drainCancel()
		if shutdownErr := c.committer.Finalize(drainCtx, nil, states, "failed to commit processed offsets on shutdown"); shutdownErr != nil {
			if err == nil {
				err = shutdownErr
			}
		}
	}

	c.runState.Stop()
	c.runState.Wait()

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
			if err := c.dispatcher.Dispatch(ctx, records); err != nil {
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

	if cl != nil {
		cl.PauseFetchPartitions(partitions)
	}

	states := c.registry.BeginClosing(partitions)
	if err := c.committer.Finalize(ctx, ctx, states,
		"failed to commit processed offsets on revoke"); err != nil {
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
