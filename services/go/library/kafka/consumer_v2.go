// services/go/library/kafka/consumer_v2.go
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

// consumerV2 is the temporary v2 consumer façade.
type consumerV2 struct {
	cfg          *config
	client       *Client
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

// v2RegisterClient adapts consumerV2's Client to consumer.RegisterClient.
type v2RegisterClient struct {
	v2 *consumerV2
}

func (a v2RegisterClient) IsClosed() bool {
	return a.v2.client != nil && a.v2.client.isClosed()
}

func (a v2RegisterClient) AddConsumeTopics(topics ...string) {
	if a.v2.client != nil && a.v2.client.kgoClient != nil {
		a.v2.client.kgoClient.AddConsumeTopics(topics...)
	}
}

// v2FetchControlClient adapts consumerV2's Client to consumer.FetchControlClient.
type v2FetchControlClient struct {
	v2 *consumerV2
}

func (a v2FetchControlClient) PauseFetchPartitions(partitions map[string][]int32) {
	if a.v2.client != nil && a.v2.client.kgoClient != nil {
		a.v2.client.kgoClient.PauseFetchPartitions(partitions)
	}
}

func (a v2FetchControlClient) ResumeFetchPartitions(partitions map[string][]int32) {
	if a.v2.client != nil && a.v2.client.kgoClient != nil {
		a.v2.client.kgoClient.ResumeFetchPartitions(partitions)
	}
}

// v2WorkerClient adapts consumerV2's Client to consumer.WorkerClient.
type v2WorkerClient struct {
	v2 *consumerV2
}

func (a v2WorkerClient) CommitOffsetsSync(ctx context.Context, offsets map[string]map[int32]kgo.EpochOffset) error {
	if a.v2.client == nil || a.v2.client.kgoClient == nil {
		return fmt.Errorf("kafka client is not initialized")
	}
	var commitErr error
	a.v2.client.kgoClient.CommitOffsetsSync(ctx, offsets, func(
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
	if a.v2.client != nil && a.v2.client.kgoClient != nil {
		a.v2.client.kgoClient.PauseFetchTopics(topics...)
	}
}

func (a v2WorkerClient) ResumeFetchPartitions(partitions map[string][]int32) {
	if a.v2.client != nil && a.v2.client.kgoClient != nil {
		a.v2.client.kgoClient.ResumeFetchPartitions(partitions)
	}
}

// newConsumerV2 creates a v2 consumer from the shared config and client.
func newConsumerV2(cfg *config, client *Client) (*consumerV2, error) {
	v2 := &consumerV2{
		cfg:          cfg,
		client:       client,
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
	v2.fetchClient = v2FetchControlClient{v2: v2}
	v2.workerClient = v2WorkerClient{v2: v2}

	regClient := v2RegisterClient{v2: v2}
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

	// DLQ writer closure — publishes enriched records to the DLQ topic.
	dlqWriter := func(ctx context.Context, record *kgo.Record) error {
		if v2.client == nil || v2.client.kgoClient == nil {
			return fmt.Errorf("kafka client is not initialized")
		}
		// A real implementation would use the producer. For now, return an error
		// if no producer is available. This will be properly wired when the
		// producer is refactored.
		if v2.client.Producer != nil {
			return v2.client.Producer.ProduceSync(ctx, record)
		}
		return fmt.Errorf("dlq publishing requires a producer-enabled client")
	}

	// Create worker runner with placeholder notify capacity.
	v2.workerRunner = consumer.NewWorkerRunner(
		v2.log,
		v2.runState,
		v2.committer,
		executor,
		v2.registry,
		v2.pauses,
		v2.cfg.workers,
		func() {},
		dlqWriter,
	)

	// Dispatcher needs worker runner for Start().
	v2.dispatcher = consumer.NewDispatcher(v2.router, v2.pauses, v2.registry, v2.workerRunner)

	// Update worker runner's notify capacity now that Dispatcher exists.
	v2.workerRunner.SetNotifyCapacity(v2.dispatcher.NotifyCapacity)

	// Register startup subscriptions from config.
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
func (v2 *consumerV2) AddSubscription(subscription Subscription) error {
	normalized, err := subscription.Normalize()
	if err != nil {
		return err
	}
	return v2.router.Register(normalized)
}

// AddTopic registers a single-record handler using the default ack mode.
func (v2 *consumerV2) AddTopic(topic string, handler Handler) error {
	return v2.AddSubscription(newDefaultSubscription(topic, handler, v2.cfg.defaultAckMode))
}

// AddBatchTopic registers a batch handler using the default ack mode.
func (v2 *consumerV2) AddBatchTopic(topic string, handler BatchHandler) error {
	return v2.AddSubscription(newDefaultBatchSubscription(topic, handler, v2.cfg.defaultAckMode))
}

// Run starts the consumer loop.
func (v2 *consumerV2) Run(ctx context.Context) error {
	runCtx, err := v2.runState.Begin()
	if err != nil {
		return err
	}

	pollCtx, stopPolling := mergeRunContexts(ctx, runCtx)
	defer stopPolling()

	if v2.client == nil || v2.client.kgoClient == nil {
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

func (v2 *consumerV2) runDispatch(ctx context.Context) error {
	cl := v2.client.kgoClient
	for {
		fetches := cl.PollRecords(ctx, -1)
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

// onPartitionsRevoked handles partition revocation. STUB — no-op.
//
//nolint:unused
func (v2 *consumerV2) onPartitionsRevoked(
	ctx context.Context,
	cl *kgo.Client,
	partitions map[string][]int32,
) {
}

// onPartitionsLost handles lost partitions. STUB — no-op.
//
//nolint:unused
func (v2 *consumerV2) onPartitionsLost(
	ctx context.Context,
	partitions map[string][]int32,
) {
}
