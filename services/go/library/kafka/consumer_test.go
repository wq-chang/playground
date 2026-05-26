package kafka

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"go-services/library/assert"
	"go-services/library/require"
)

func TestNewConsumer_RegistersStartupTopics(t *testing.T) {
	cfg := newConfig([]string{"broker:9092"}, "group")
	startupHandler := func(context.Context, *kgo.Record) error { return nil }
	cfg.subscriptions["topic-a"] = Subscription{
		Topic:         "topic-a",
		Handler:       startupHandler,
		AckMode:       AckModeAtLeastOnce,
		FailurePolicy: FailurePolicy{},
	}

	consumer, err := newConsumer(cfg, nil)
	require.NoError(t, err, "failed to create consumer")

	subscription, ok := consumer.subscriptionForTopic("topic-a")
	if !ok {
		t.Fatal("startup topic should be registered")
	}
	assert.NotNil(t, subscription.Handler, "startup handler should be available")
}

func TestConsumerAddSubscriptionValidation(t *testing.T) {
	cfg := newConfig([]string{"broker:9092"}, "group")
	consumer, err := newConsumer(cfg, nil)
	require.NoError(t, err, "failed to create consumer")

	handler := func(context.Context, *kgo.Record) error { return nil }

	t.Run("reject empty topic", func(t *testing.T) {
		err := consumer.AddSubscription(Subscription{
			Topic:         "",
			Handler:       handler,
			AckMode:       AckModeAtLeastOnce,
			FailurePolicy: FailurePolicy{},
		})
		assert.NotNil(t, err, "should reject empty topic")
		assert.StringContains(t, err.Error(), "topic must not be empty", "error message")
	})

	t.Run("reject nil handler", func(t *testing.T) {
		err := consumer.AddSubscription(Subscription{
			Topic:         "topic-a",
			Handler:       nil,
			AckMode:       AckModeAtLeastOnce,
			FailurePolicy: FailurePolicy{},
		})
		assert.NotNil(t, err, "should reject nil handler")
		assert.StringContains(t, err.Error(), "handler must not be nil", "error message")
	})

	t.Run("reject invalid dlq config", func(t *testing.T) {
		err := consumer.AddSubscription(Subscription{
			Topic:   "topic-a",
			Handler: handler,
			AckMode: AckModeAtLeastOnce,
			FailurePolicy: FailurePolicy{
				MaxAttempts:  0,
				RetryBackoff: 0,
				DLQ:          nil,
				OnExhausted:  ExhaustedActionDLQThenCommit,
			},
		})
		assert.NotNil(t, err, "should reject missing dlq config")
		assert.StringContains(t, err.Error(), "dlq config is required", "error message")
	})

	t.Run("reject duplicate topic", func(t *testing.T) {
		err := consumer.AddSubscription(Subscription{
			Topic:         "topic-a",
			Handler:       handler,
			AckMode:       AckModeAtLeastOnce,
			FailurePolicy: FailurePolicy{},
		})
		require.NoError(t, err, "failed to add topic")

		err = consumer.AddSubscription(Subscription{
			Topic:         "topic-a",
			Handler:       handler,
			AckMode:       AckModeAtLeastOnce,
			FailurePolicy: FailurePolicy{},
		})
		assert.NotNil(t, err, "should reject duplicate topic")
		assert.StringContains(t, err.Error(), `topic handler already registered for "topic-a"`, "error message")
	})
}

func TestConsumerAddTopicRejectsClosedConsumer(t *testing.T) {
	cfg := newConfig([]string{"broker:9092"}, "group")
	client := &Client{
		Consumer:  nil,
		Producer:  nil,
		kgoClient: nil,
		closeOnce: sync.Once{},
		closed:    false,
		mu:        sync.RWMutex{},
	}
	consumer, err := newConsumer(cfg, client)
	require.NoError(t, err, "failed to create consumer")

	client.Close()

	err = consumer.AddSubscription(Subscription{
		Topic:         "topic-a",
		Handler:       func(context.Context, *kgo.Record) error { return nil },
		AckMode:       AckModeAtLeastOnce,
		FailurePolicy: FailurePolicy{},
	})
	assert.NotNil(t, err, "closed consumer should reject new topics")
	assert.StringContains(t, err.Error(), "consumer is closed", "error message")
}

func TestPartitionWorkerCommitLifecycle(t *testing.T) {
	worker := newPartitionWorker(
		context.Background(),
		recordKey{topic: "topic-a", partition: 1},
		testSubscription("topic-a"),
		4,
	)

	assert.True(t, worker.advanceCommitOffset(&kgo.Record{
		Topic:       "topic-a",
		Partition:   1,
		Offset:      2,
		LeaderEpoch: 4,
	}), "first successful record should advance next commit offset")
	assert.True(t, worker.advanceCommitOffset(&kgo.Record{
		Topic:       "topic-a",
		Partition:   1,
		Offset:      5,
		LeaderEpoch: 4,
	}), "later successful record should advance next commit offset")

	offset, ok := worker.snapshotDirtyOffset()
	assert.True(t, ok, "worker should expose dirty commit progress")
	assert.Equal(t, offset.Offset, int64(6), "snapshot should keep latest offset + 1")
	assert.Equal(t, offset.Epoch, int32(4), "snapshot should keep leader epoch")

	worker.markCommitted(offset)

	_, ok = worker.snapshotDirtyOffset()
	assert.False(t, ok, "marking committed offset should clear dirty state")
}

func TestConsumerSnapshotDirtyOffsets(t *testing.T) {
	consumer := newTestConsumer()

	workerA := newPartitionWorker(
		context.Background(),
		recordKey{topic: "topic-a", partition: 1},
		testSubscription("topic-a"),
		4,
	)
	workerB := newPartitionWorker(
		context.Background(),
		recordKey{topic: "topic-b", partition: 0},
		testSubscription("topic-b"),
		4,
	)
	assert.True(t, workerA.advanceCommitOffset(&kgo.Record{
		Topic:       "topic-a",
		Partition:   1,
		Offset:      5,
		LeaderEpoch: 4,
	}), "worker should advance topic-a offset")
	assert.True(t, workerB.advanceCommitOffset(&kgo.Record{
		Topic:       "topic-b",
		Partition:   0,
		Offset:      1,
		LeaderEpoch: 7,
	}), "worker should advance topic-b offset")

	consumer.workers[recordKey{topic: "topic-a", partition: 1}] = workerA
	consumer.workers[recordKey{topic: "topic-b", partition: 0}] = workerB

	offsets := consumer.snapshotDirtyOffsets()
	assert.Equal(t, len(offsets), 2, "snapshot should contain both topics")
	assert.Equal(t, offsets["topic-a"][1].Offset, int64(6), "snapshot should keep latest offset + 1")
	assert.Equal(t, offsets["topic-b"][0].Offset, int64(2), "snapshot should include requested partition")

	consumer.markCommittedOffsets(offsets)

	remaining := consumer.snapshotDirtyOffsets()
	assert.Nil(t, remaining, "marking committed offsets should clear dirty progress")
}

func TestConsumerStopWorkersForPartitions(t *testing.T) {
	consumer := newTestConsumer()

	workerA := newPartitionWorker(
		context.Background(),
		recordKey{topic: "topic-a", partition: 1},
		testSubscription("topic-a"),
		4,
	)
	workerB := newPartitionWorker(
		context.Background(),
		recordKey{topic: "topic-b", partition: 0},
		testSubscription("topic-b"),
		4,
	)
	assert.True(t, workerA.advanceCommitOffset(&kgo.Record{
		Topic:       "topic-a",
		Partition:   1,
		Offset:      5,
		LeaderEpoch: 4,
	}), "worker should advance topic-a offset")
	assert.True(t, workerB.advanceCommitOffset(&kgo.Record{
		Topic:       "topic-b",
		Partition:   0,
		Offset:      1,
		LeaderEpoch: 7,
	}), "worker should advance topic-b offset")

	consumer.workers[recordKey{topic: "topic-a", partition: 1}] = workerA
	consumer.workers[recordKey{topic: "topic-b", partition: 0}] = workerB

	offsets := consumer.stopWorkersForPartitions(map[string][]int32{
		"topic-b": {0},
	})
	assert.Equal(t, len(offsets), 1, "stopping selected partitions should return only matching offsets")
	assert.Equal(t, offsets["topic-b"][0].Offset, int64(2), "stopped partition should expose its next commit offset")
	assert.Equal(t, len(consumer.workers), 1, "stopping selected partitions should keep unrelated workers")
}

func TestPartitionWorkerBackpressureState(t *testing.T) {
	worker := newPartitionWorker(
		context.Background(),
		recordKey{topic: "topic-a", partition: 1},
		testSubscription("topic-a"),
		4,
	)

	assert.True(t, worker.markBackpressurePaused(), "worker should enter backpressure pause")
	assert.False(t, worker.markBackpressurePaused(), "worker should not pause twice")
	assert.True(t, worker.clearBackpressurePaused(), "worker should clear backpressure pause")
	assert.False(t, worker.clearBackpressurePaused(), "worker should not clear an inactive pause")
}

func TestConsumerPauseTopic(t *testing.T) {
	consumer := newTestConsumer()

	require.NoError(t, consumer.pauseTopic(nil, "topic-a", fmt.Errorf("boom")), "pausing a topic should succeed")
	assert.True(t, consumer.isTopicPaused("topic-a"), "topic should be marked paused")

	require.NoError(t, consumer.pauseTopic(nil, "topic-a", fmt.Errorf("another")), "pausing an already paused topic should be a no-op")
	assert.Equal(t, len(consumer.pausedTopics), 1, "pausing the same topic twice should not duplicate state")
}

func TestConsumerDispatchRecordsDoesNotBlockOtherPartitions(t *testing.T) {
	consumer := newTestConsumer()
	consumer.cfg.workers = 4

	slowTopic := "topic-a"
	otherTopic := "topic-b"
	slowStarted := make(chan struct{}, 1)
	releaseSlow := make(chan struct{})
	fastValues := make(chan string, 2)

	slowSubscription, err := Subscription{
		Topic: slowTopic,
		Handler: func(ctx context.Context, record *kgo.Record) error {
			if record.Partition == 0 {
				select {
				case slowStarted <- struct{}{}:
				default:
				}

				select {
				case <-releaseSlow:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			}

			fastValues <- fmt.Sprintf("%s/%d:%s", record.Topic, record.Partition, string(record.Value))
			return nil
		},
		AckMode:       AckModeAtLeastOnce,
		FailurePolicy: FailurePolicy{},
	}.normalize()
	require.NoError(t, err, "slow topic subscription should normalize")
	consumer.subscriptions[slowTopic] = slowSubscription

	otherSubscription, err := Subscription{
		Topic: otherTopic,
		Handler: func(context.Context, *kgo.Record) error {
			fastValues <- fmt.Sprintf("%s/%d:%s", otherTopic, 0, "other-fast")
			return nil
		},
		AckMode:       AckModeAtLeastOnce,
		FailurePolicy: FailurePolicy{},
	}.normalize()
	require.NoError(t, err, "other topic subscription should normalize")
	consumer.subscriptions[otherTopic] = otherSubscription

	runCtx, err := consumer.beginRun(context.Background())
	require.NoError(t, err, "consumer run state should initialize")
	defer func() {
		close(releaseSlow)
		consumer.stopRun()
		consumer.runWG.Wait()
		consumer.resetRunState()
	}()

	require.NoError(t, consumer.dispatchRecords(runCtx, nil, []*kgo.Record{
		{Topic: slowTopic, Partition: 0, Offset: 0, Value: []byte("slow")},
	}), "first dispatch should enqueue slow partition")

	select {
	case <-slowStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for slow partition handler to start")
	}

	require.NoError(t, consumer.dispatchRecords(runCtx, nil, []*kgo.Record{
		{Topic: slowTopic, Partition: 1, Offset: 0, Value: []byte("same-topic-fast")},
		{Topic: otherTopic, Partition: 0, Offset: 0, Value: []byte("other-fast")},
	}), "second dispatch should enqueue unrelated partitions while slow partition is busy")

	seen := make(map[string]struct{})
	for len(seen) < 2 {
		select {
		case value := <-fastValues:
			seen[value] = struct{}{}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for fast partitions to finish while slow partition is blocked")
		}
	}

	_, sameTopicSeen := seen[fmt.Sprintf("%s/%d:%s", slowTopic, 1, "same-topic-fast")]
	assert.True(t, sameTopicSeen, "same-topic other partition should finish while slow partition is blocked")
	_, otherTopicSeen := seen[fmt.Sprintf("%s/%d:%s", otherTopic, 0, "other-fast")]
	assert.True(t, otherTopicSeen, "other topic should finish while slow partition is blocked")
}

func TestConsumerPartitionBatchesSortRecordsByOffsetWithinPartition(t *testing.T) {
	consumer := newTestConsumer()
	consumer.subscriptions["topic-a"] = testSubscription("topic-a")

	batches, err := consumer.partitionBatches([]*kgo.Record{
		{Topic: "topic-a", Partition: 1, Offset: 5},
		{Topic: "topic-a", Partition: 1, Offset: 2},
		{Topic: "topic-a", Partition: 1, Offset: 3},
	})
	require.NoError(t, err, "partition batching should succeed")
	assert.Equal(t, len(batches), 1, "records from one partition should form one batch")
	assert.Equal(t, batches[0].records[0].Offset, int64(2), "lowest offset should be processed first")
	assert.Equal(t, batches[0].records[1].Offset, int64(3), "records should remain sorted by offset")
	assert.Equal(t, batches[0].records[2].Offset, int64(5), "highest offset should be processed last")
}

func TestConsumerPartitionBatchesSortKeysByTopicPartition(t *testing.T) {
	consumer := newTestConsumer()
	consumer.subscriptions["topic-a"] = testSubscription("topic-a")
	consumer.subscriptions["topic-b"] = testSubscription("topic-b")

	batches, err := consumer.partitionBatches([]*kgo.Record{
		{Topic: "topic-b", Partition: 2, Offset: 1},
		{Topic: "topic-a", Partition: 3, Offset: 1},
		{Topic: "topic-a", Partition: 1, Offset: 1},
	})
	require.NoError(t, err, "partition batching should succeed")
	assert.Equal(t, len(batches), 3, "each topic-partition should produce one batch")
	assert.Equal(t, batches[0].records[0].Topic, "topic-a", "batches should sort by topic first")
	assert.Equal(t, batches[0].records[0].Partition, int32(1), "lowest partition should come first within topic")
	assert.Equal(t, batches[1].records[0].Topic, "topic-a", "same topic batches should remain grouped")
	assert.Equal(t, batches[1].records[0].Partition, int32(3), "higher partition should come after lower partition")
	assert.Equal(t, batches[2].records[0].Topic, "topic-b", "later topics should come after earlier topics")
	assert.Equal(t, batches[2].records[0].Partition, int32(2), "topic-b batch should preserve its partition")
}

func newTestConsumer() *Consumer {
	cfg := newConfig([]string{"broker:9092"}, "group")
	return &Consumer{
		subscriptions:  make(map[string]Subscription),
		client:         nil,
		cfg:            cfg,
		log:            slog.Default(),
		pausedTopics:   make(map[string]pausedTopic),
		workers:        make(map[recordKey]*partitionWorker),
		commitMu:       sync.Mutex{},
		mu:             sync.RWMutex{},
		runCtx:         nil,
		runCancel:      nil,
		runErr:         nil,
		runErrOnce:     sync.Once{},
		runWG:          sync.WaitGroup{},
		commitSignal:   nil,
		dispatchSignal: nil,
		processSem:     nil,
	}
}

func testSubscription(topic string) Subscription {
	subscription, err := Subscription{
		Topic:         topic,
		Handler:       func(context.Context, *kgo.Record) error { return nil },
		AckMode:       AckModeAtLeastOnce,
		FailurePolicy: FailurePolicy{},
	}.normalize()
	if err != nil {
		panic(err)
	}
	return subscription
}
