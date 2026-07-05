package kafka

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"go-services/library/kafka/internal/consumer"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
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
	fetchClient  consumer.FetchControlClient
	workerClient consumer.WorkerClient
	drainTimeout time.Duration
}

// v2RegisterClient adapts Consumer's Client to consumer.RegisterClient.
type v2RegisterClient struct {
	kcl *kgo.Client
}

func (a v2RegisterClient) AddConsumeTopics(topics ...string) {
	a.kcl.AddConsumeTopics(topics...)
}

// v2FetchControlClient adapts Consumer's Client to consumer.FetchControlClient.
type v2FetchControlClient struct {
	kcl *kgo.Client
}

func (a v2FetchControlClient) PauseFetchPartitions(partitions map[string][]int32) {
	a.kcl.PauseFetchPartitions(partitions)
}

func (a v2FetchControlClient) ResumeFetchPartitions(partitions map[string][]int32) {
	a.kcl.ResumeFetchPartitions(partitions)
}

// v2WorkerClient adapts Consumer's Client to consumer.WorkerClient.
type v2WorkerClient struct {
	kcl *kgo.Client
}

func (a v2WorkerClient) CommitOffsetsSync(ctx context.Context, offsets map[string]map[int32]kgo.EpochOffset) error {
	var commitErr error
	a.kcl.CommitOffsetsSync(ctx, offsets, func(
		_ *kgo.Client,
		_ *kmsg.OffsetCommitRequest,
		_ *kmsg.OffsetCommitResponse,
		err error,
	) {
		commitErr = err
	})
	return commitErr
}

func (a v2WorkerClient) PauseFetchTopics(topics ...string) {
	a.kcl.PauseFetchTopics(topics...)
}

func (a v2WorkerClient) ResumeFetchPartitions(partitions map[string][]int32) {
	a.kcl.ResumeFetchPartitions(partitions)
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
		fetchClient:  nil,
		workerClient: nil,
		drainTimeout: 30 * time.Second,
	}
	v2.fetchClient = v2FetchControlClient{kcl: kgoClient}
	v2.workerClient = v2WorkerClient{kcl: kgoClient}

	regClient := v2RegisterClient{kcl: kgoClient}
	v2.router = consumer.NewRouter(regClient)
	v2.pauses = consumer.NewPauseRegistry(time.Now)
	v2.runState = consumer.NewRunState()

	v2.registry = consumer.NewPartitionRegistry(v2.log)
	v2.committer = consumer.NewCommitter(
		v2.log,
		v2.registry,
		v2.pauses,
		v2.workerClient,
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
	)

	v2.dispatcher = consumer.NewDispatcher(
		v2.router, v2.pauses, v2.registry,
		v2.workerRunner.Start,
		capacityCh,
	)

	subs := make([]Subscription, 0, len(cfg.subscriptions))
	for _, sub := range cfg.subscriptions {
		subs = append(subs, sub)
	}
	if len(subs) > 0 {
		if err := v2.router.RegisterQuietBatch(subs); err != nil {
			return nil, fmt.Errorf("v2 init: %w", err)
		}
	}

	return v2, nil
}

// AddSubscription registers a new topic subscription.
func (v2 *Consumer) AddSubscription(subscription Subscription) error {
	normalized, err := subscription.Normalize()
	if err != nil {
		return err
	}
	return v2.router.Register(normalized)
}

// AddTopic registers a single-record handler using the default ack mode.
func (v2 *Consumer) AddTopic(topic string, handler Handler) error {
	return v2.AddSubscription(newDefaultSubscription(topic, handler, v2.cfg.defaultAckMode))
}

// AddBatchTopic registers a batch handler using the default ack mode.
func (v2 *Consumer) AddBatchTopic(topic string, handler BatchHandler) error {
	return v2.AddSubscription(newDefaultBatchSubscription(topic, handler, v2.cfg.defaultAckMode))
}

// Run starts the consumer loop.
func (v2 *Consumer) Run(ctx context.Context) error {
	runCtx, err := v2.runState.Begin()
	if err != nil {
		return err
	}

	pollCtx, stopPolling := mergeRunContexts(ctx, runCtx)
	defer stopPolling()

	if v2.kgoClient == nil {
		return fmt.Errorf("consumer client is not initialized")
	}

	// Start commit loop.
	v2.runState.Go(func() {
		if commitErr := v2.committer.Run(runCtx); commitErr != nil {
			v2.runState.Fail(commitErr)
		}
	})

	// Dispatch loop.
	err = v2.runDispatch(pollCtx)

	// Graceful shutdown (only if no fatal error).
	if runErr := v2.runState.Err(); runErr == nil {
		states := v2.registry.BeginClosingAll()
		drainCtx, drainCancel := context.WithTimeout(context.Background(), v2.drainTimeout)
		defer drainCancel()
		if shutdownErr := v2.committer.Finalize(drainCtx, nil, states, "failed to commit processed offsets on shutdown"); shutdownErr != nil {
			if err == nil {
				err = shutdownErr
			}
		}
	}

	v2.runState.Stop()
	v2.runState.Wait()

	fatalErr := v2.runState.Err()
	v2.runState.Reset()

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

func (v2 *Consumer) runDispatch(ctx context.Context) error {
	cl := v2.kgoClient
	maxRecords := v2.cfg.fetchMaxRecords
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
			v2.log.WarnContext(ctx, "kafka poll error", "err", err)
			continue
		}
		records := fetches.Records()
		if len(records) > 0 {
			if err := v2.dispatcher.Dispatch(
				ctx,
				records,
				v2.fetchClient,
				v2.workerClient,
			); err != nil {
				return err
			}
		}
		cl.AllowRebalance()
	}
}

// onPartitionsRevoked handles partition revocation.
func (v2 *Consumer) onPartitionsRevoked(
	ctx context.Context,
	_ *kgo.Client,
	partitions map[string][]int32,
) {
	if len(partitions) == 0 {
		return
	}

	if v2.fetchClient != nil {
		v2.fetchClient.PauseFetchPartitions(partitions)
	}

	states := v2.registry.BeginClosing(partitions)
	if err := v2.committer.Finalize(ctx, ctx, states,
		"failed to commit processed offsets on revoke"); err != nil {
		v2.log.ErrorContext(ctx, "failed to commit processed offsets on revoke", "err", err)
		v2.runState.Fail(err)
	}
}

// onPartitionsLost handles lost partitions by dropping in-memory commit progress.
func (v2 *Consumer) onPartitionsLost(
	ctx context.Context,
	partitions map[string][]int32,
) {
	v2.log.WarnContext(ctx, "partitions lost; dropping in-memory commit progress", "partitions", partitions)
	v2.registry.DropLost(partitions)
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
