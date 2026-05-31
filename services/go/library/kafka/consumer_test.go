package kafka

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"go-services/library/assert"
	"go-services/library/gsync"
	"go-services/library/require"
)

func TestNewConsumer_RegistersStartupTopics(t *testing.T) {
	cfg := newConfig([]string{"broker:9092"}, "group")
	startupHandler := func(context.Context, *kgo.Record) error { return nil }
	cfg.subscriptions["topic-a"] = Subscription{
		Topic:         "topic-a",
		Handler:       startupHandler,
		BatchHandler:  nil,
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
			BatchHandler:  nil,
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
			BatchHandler:  nil,
			AckMode:       AckModeAtLeastOnce,
			FailurePolicy: FailurePolicy{},
		})
		assert.NotNil(t, err, "should reject nil handler")
		assert.StringContains(t, err.Error(), "exactly one of handler or batch handler must be set", "error message")
	})

	t.Run("reject invalid dlq config", func(t *testing.T) {
		err := consumer.AddSubscription(Subscription{
			Topic:        "topic-a",
			Handler:      handler,
			BatchHandler: nil,
			AckMode:      AckModeAtLeastOnce,
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
			BatchHandler:  nil,
			AckMode:       AckModeAtLeastOnce,
			FailurePolicy: FailurePolicy{},
		})
		require.NoError(t, err, "failed to add topic")

		err = consumer.AddSubscription(Subscription{
			Topic:         "topic-a",
			Handler:       handler,
			BatchHandler:  nil,
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
		BatchHandler:  nil,
		AckMode:       AckModeAtLeastOnce,
		FailurePolicy: FailurePolicy{},
	})
	assert.NotNil(t, err, "closed consumer should reject new topics")
	assert.StringContains(t, err.Error(), "consumer is closed", "error message")
}

func TestConsumerExecuteRecordRecoversHandlerPanicAndStops(t *testing.T) {
	consumer := newTestConsumer()
	attempts := 0

	subscription, err := Subscription{
		Topic: "topic-a",
		Handler: func(context.Context, *kgo.Record) error {
			attempts++
			panic("boom")
		},
		BatchHandler: nil,
		AckMode:      AckModeAtLeastOnce,
		FailurePolicy: FailurePolicy{
			DLQ:          nil,
			RetryBackoff: 0,
			MaxAttempts:  2,
			OnExhausted:  ExhaustedActionStop,
		},
	}.normalize()
	require.NoError(t, err, "panicking subscription should normalize")

	result, err := consumer.executeRecord(context.Background(), subscription, &kgo.Record{
		Topic:     "topic-a",
		Partition: 1,
		Offset:    9,
	})
	require.NoError(t, err, "recovered handler panic should stay inside failure policy flow")

	assert.Equal(t, attempts, 2, "recovered panic should retry up to max attempts")
	assert.False(t, result.resolved, "stop-on-exhausted should leave the record unresolved")
	assert.True(t, result.pauseTopic, "stop-on-exhausted should pause the topic")
	require.NotNil(t, result.cause, "stop-on-exhausted should return a failure cause")
	assert.ErrorContains(t, result.cause, `handler failed for topic "topic-a" partition 1 offset 9 after 2 attempts`, "result should describe retry exhaustion")
	assert.ErrorContains(t, result.cause, "handler panicked: boom", "panic should be converted into a handler error")
}

func TestConsumerExecuteRecordRecoversHandlerPanicAndCanSucceedOnRetry(t *testing.T) {
	consumer := newTestConsumer()
	attempts := 0

	subscription, err := Subscription{
		Topic: "topic-a",
		Handler: func(context.Context, *kgo.Record) error {
			attempts++
			if attempts == 1 {
				panic("boom")
			}
			return nil
		},
		BatchHandler: nil,
		AckMode:      AckModeAtLeastOnce,
		FailurePolicy: FailurePolicy{
			DLQ:          nil,
			RetryBackoff: 0,
			MaxAttempts:  2,
			OnExhausted:  ExhaustedActionUnspecified,
		},
	}.normalize()
	require.NoError(t, err, "subscription should normalize")

	result, err := consumer.executeRecord(context.Background(), subscription, &kgo.Record{
		Topic:     "topic-a",
		Partition: 1,
		Offset:    9,
	})
	require.NoError(t, err, "successful retry should not return an execution error")

	assert.Equal(t, attempts, 2, "recovered panic should count as the first failed attempt")
	assert.True(t, result.resolved, "successful retry should resolve the record")
	assert.False(t, result.pauseTopic, "successful retry should not pause the topic")
	assert.Nil(t, result.cause, "successful retry should not keep a failure cause")
}

func TestConsumerExecuteRecordRecoversHandlerPanicAndCommitExhausted(t *testing.T) {
	consumer := newTestConsumer()
	attempts := 0

	subscription, err := Subscription{
		Topic: "topic-a",
		Handler: func(context.Context, *kgo.Record) error {
			attempts++
			panic("boom")
		},
		BatchHandler: nil,
		AckMode:      AckModeAtLeastOnce,
		FailurePolicy: FailurePolicy{
			DLQ:          nil,
			RetryBackoff: 0,
			MaxAttempts:  1,
			OnExhausted:  ExhaustedActionCommit,
		},
	}.normalize()
	require.NoError(t, err, "commit-on-exhausted subscription should normalize")

	result, err := consumer.executeRecord(context.Background(), subscription, &kgo.Record{
		Topic:     "topic-a",
		Partition: 1,
		Offset:    9,
	})
	require.NoError(t, err, "commit-on-exhausted should resolve recovered panic without bubbling an error")

	assert.Equal(t, attempts, 1, "single-attempt policy should not retry")
	assert.True(t, result.resolved, "commit-on-exhausted should resolve the record")
	assert.False(t, result.pauseTopic, "commit-on-exhausted should not pause the topic")
	assert.Nil(t, result.cause, "commit-on-exhausted should not return a failure cause")
}

func TestPartitionStateCommitLifecycle(t *testing.T) {
	state := newPartitionState(
		context.Background(),
		recordKey{topic: "topic-a", partition: 1},
		testSubscription("topic-a"),
		4,
	)

	assert.True(t, state.advanceCommitOffset(&kgo.Record{
		Topic:       "topic-a",
		Partition:   1,
		Offset:      2,
		LeaderEpoch: 4,
	}), "first successful record should advance next commit offset")
	assert.True(t, state.advanceCommitOffset(&kgo.Record{
		Topic:       "topic-a",
		Partition:   1,
		Offset:      5,
		LeaderEpoch: 4,
	}), "later successful record should advance next commit offset")

	offset, ok := state.snapshotDirtyOffset()
	assert.True(t, ok, "state should expose dirty commit progress")
	assert.Equal(t, offset.Offset, int64(6), "snapshot should keep latest offset + 1")
	assert.Equal(t, offset.Epoch, int32(4), "snapshot should keep leader epoch")

	state.markCommitted(offset)

	_, ok = state.snapshotDirtyOffset()
	assert.False(t, ok, "marking committed offset should clear dirty state")
}

func TestConsumerSnapshotDirtyOffsets(t *testing.T) {
	consumer := newTestConsumer()

	stateA := newPartitionState(
		context.Background(),
		recordKey{topic: "topic-a", partition: 1},
		testSubscription("topic-a"),
		4,
	)
	stateB := newPartitionState(
		context.Background(),
		recordKey{topic: "topic-b", partition: 0},
		testSubscription("topic-b"),
		4,
	)
	assert.True(t, stateA.advanceCommitOffset(&kgo.Record{
		Topic:       "topic-a",
		Partition:   1,
		Offset:      5,
		LeaderEpoch: 4,
	}), "state should advance topic-a offset")
	assert.True(t, stateB.advanceCommitOffset(&kgo.Record{
		Topic:       "topic-b",
		Partition:   0,
		Offset:      1,
		LeaderEpoch: 7,
	}), "state should advance topic-b offset")

	consumer.partitionStates[recordKey{topic: "topic-a", partition: 1}] = stateA
	consumer.partitionStates[recordKey{topic: "topic-b", partition: 0}] = stateB
	require.True(t, consumer.markDirtyPartitionState(stateA), "state should be tracked as dirty")
	require.True(t, consumer.markDirtyPartitionState(stateB), "state should be tracked as dirty")

	offsets := consumer.snapshotDirtyOffsets()
	assert.Equal(t, len(offsets), 2, "snapshot should contain both topics")
	assert.Equal(t, offsets["topic-a"][1].Offset, int64(6), "snapshot should keep latest offset + 1")
	assert.Equal(t, offsets["topic-b"][0].Offset, int64(2), "snapshot should include requested partition")

	consumer.markCommittedOffsets(offsets)

	remaining := consumer.snapshotDirtyOffsets()
	assert.Nil(t, remaining, "marking committed offsets should clear dirty progress")
}

func TestConsumerBeginClosingPartitionStatesForPartitions(t *testing.T) {
	consumer := newTestConsumer()

	stateA := newPartitionState(
		context.Background(),
		recordKey{topic: "topic-a", partition: 1},
		testSubscription("topic-a"),
		4,
	)
	stateB := newPartitionState(
		context.Background(),
		recordKey{topic: "topic-b", partition: 0},
		testSubscription("topic-b"),
		4,
	)
	assert.True(t, stateA.advanceCommitOffset(&kgo.Record{
		Topic:       "topic-a",
		Partition:   1,
		Offset:      5,
		LeaderEpoch: 4,
	}), "state should advance topic-a offset")
	assert.True(t, stateB.advanceCommitOffset(&kgo.Record{
		Topic:       "topic-b",
		Partition:   0,
		Offset:      1,
		LeaderEpoch: 7,
	}), "state should advance topic-b offset")

	consumer.partitionStates[recordKey{topic: "topic-a", partition: 1}] = stateA
	consumer.partitionStates[recordKey{topic: "topic-b", partition: 0}] = stateB
	require.True(t, consumer.markDirtyPartitionState(stateA), "state should be tracked as dirty")
	require.True(t, consumer.markDirtyPartitionState(stateB), "state should be tracked as dirty")

	states := consumer.beginClosingPartitionStatesForPartitions(map[string][]int32{
		"topic-b": {0},
	})
	assert.Equal(t, len(states), 1, "closing selected partitions should return only matching states")
	assert.Equal(t, states[0].key.topic, "topic-b", "closing should select the requested topic")
	assert.Equal(t, states[0].key.partition, int32(0), "closing should select the requested partition")
	assert.Equal(t, len(consumer.partitionStates), 2, "closing selected partitions should not eagerly remove states")
	assert.False(t, stateB.isRunning(), "selected partition should be marked closing")
	assert.True(t, stateA.isRunning(), "unrelated partition should still be running")
}

func TestPartitionStateBackpressureState(t *testing.T) {
	state := newPartitionState(
		context.Background(),
		recordKey{topic: "topic-a", partition: 1},
		testSubscription("topic-a"),
		4,
	)

	assert.True(t, state.markBackpressurePaused(), "state should enter backpressure pause")
	assert.False(t, state.markBackpressurePaused(), "state should not pause twice")
	assert.True(t, state.clearBackpressurePaused(), "state should clear backpressure pause")
	assert.False(t, state.clearBackpressurePaused(), "state should not clear an inactive pause")
}

func TestPartitionStateTryEnqueueRecordsUsesRemainingRecordCapacity(t *testing.T) {
	state := newPartitionState(
		context.Background(),
		recordKey{topic: "topic-a", partition: 1},
		testSubscription("topic-a"),
		3,
	)

	records := []*kgo.Record{
		{Topic: "topic-a", Partition: 1, Offset: 1},
		{Topic: "topic-a", Partition: 1, Offset: 2},
		{Topic: "topic-a", Partition: 1, Offset: 3},
		{Topic: "topic-a", Partition: 1, Offset: 4},
	}

	enqueued, buffered := state.tryEnqueueRecords(records)
	assert.Equal(t, enqueued, 3, "state should enqueue only up to the remaining record capacity")
	assert.Equal(t, buffered, 3, "state should track the buffered record count")

	queued := <-state.queue
	assert.Equal(t, len(queued), 3, "enqueued batch should contain the admitted records")
	assert.Equal(t, queued[0].Offset, int64(1), "queued batch should preserve order")
	assert.Equal(t, queued[2].Offset, int64(3), "queued batch should keep the third admitted record")

	buffered = state.onDequeueBatch(queued)
	assert.Equal(t, buffered, 0, "dequeueing should release the buffered capacity")

	enqueued, buffered = state.tryEnqueueRecords(records[3:])
	assert.Equal(t, enqueued, 1, "freed capacity should allow the remaining record to enqueue")
	assert.Equal(t, buffered, 1, "buffered count should reflect the new queued record")
}

func TestConsumerPauseTopic(t *testing.T) {
	consumer := newTestConsumer()

	require.NoError(t, consumer.pauseTopic(nil, "topic-a", fmt.Errorf("boom")), "pausing a topic should succeed")
	assert.True(t, consumer.isTopicPaused("topic-a"), "topic should be marked paused")

	require.NoError(t, consumer.pauseTopic(nil, "topic-a", fmt.Errorf("another")), "pausing an already paused topic should be a no-op")
	assert.Equal(t, len(consumer.pausedTopics), 1, "pausing the same topic twice should not duplicate state")
}

func TestConsumerOnPartitionsRevokedCommitsSelectedOffsets(t *testing.T) {
	consumer := newTestConsumer()

	stateA := newPartitionState(
		context.Background(),
		recordKey{topic: "topic-a", partition: 1},
		testSubscription("topic-a"),
		4,
	)
	stateB := newPartitionState(
		context.Background(),
		recordKey{topic: "topic-b", partition: 0},
		testSubscription("topic-b"),
		4,
	)
	assert.True(t, stateA.advanceCommitOffset(&kgo.Record{
		Topic:       "topic-a",
		Partition:   1,
		Offset:      5,
		LeaderEpoch: 4,
	}), "state should advance topic-a offset")
	assert.True(t, stateB.advanceCommitOffset(&kgo.Record{
		Topic:       "topic-b",
		Partition:   0,
		Offset:      1,
		LeaderEpoch: 7,
	}), "state should advance topic-b offset")

	consumer.partitionStates[recordKey{topic: "topic-a", partition: 1}] = stateA
	consumer.partitionStates[recordKey{topic: "topic-b", partition: 0}] = stateB
	markPartitionStateStoppedForTest(stateA)
	markPartitionStateStoppedForTest(stateB)
	require.True(t, consumer.markDirtyPartitionState(stateA), "state should be tracked as dirty")
	require.True(t, consumer.markDirtyPartitionState(stateB), "state should be tracked as dirty")

	originalCommitOffsetsSyncFn := commitOffsetsSyncFn
	t.Cleanup(func() {
		commitOffsetsSyncFn = originalCommitOffsetsSyncFn
	})

	var committed map[string]map[int32]kgo.EpochOffset
	commitOffsetsSyncFn = func(_ context.Context, _ *kgo.Client, offsets map[string]map[int32]kgo.EpochOffset) error {
		committed = offsets
		return nil
	}

	consumer.onPartitionsRevoked(context.Background(), nil, map[string][]int32{
		"topic-b": {0},
	})

	assert.Equal(t, len(committed), 1, "revoked partitions should commit only matching topics")
	assert.Equal(t, committed["topic-b"][0].Offset, int64(2), "revoked partition should commit its next offset")
	assert.Equal(t, committed["topic-b"][0].Epoch, int32(7), "revoked partition should commit its leader epoch")
	assert.Equal(t, len(consumer.partitionStates), 1, "revoked partitions should remove only matching states")
	_, topicARemains := consumer.partitionStates[recordKey{topic: "topic-a", partition: 1}]
	assert.True(t, topicARemains, "unrelated state should remain active")
	assert.Nil(t, consumer.runFailure(), "successful revoke commit should not fail the run")
}

func TestConsumerOnPartitionsRevokedFailsRunOnCommitError(t *testing.T) {
	consumer := newTestConsumer()

	state := newPartitionState(
		context.Background(),
		recordKey{topic: "topic-a", partition: 1},
		testSubscription("topic-a"),
		4,
	)
	assert.True(t, state.advanceCommitOffset(&kgo.Record{
		Topic:       "topic-a",
		Partition:   1,
		Offset:      5,
		LeaderEpoch: 4,
	}), "state should advance topic-a offset")
	consumer.partitionStates[recordKey{topic: "topic-a", partition: 1}] = state
	markPartitionStateStoppedForTest(state)
	require.True(t, consumer.markDirtyPartitionState(state), "state should be tracked as dirty")

	originalCommitOffsetsSyncFn := commitOffsetsSyncFn
	t.Cleanup(func() {
		commitOffsetsSyncFn = originalCommitOffsetsSyncFn
	})

	commitOffsetsSyncFn = func(context.Context, *kgo.Client, map[string]map[int32]kgo.EpochOffset) error {
		return errors.New("boom")
	}

	consumer.onPartitionsRevoked(context.Background(), nil, map[string][]int32{
		"topic-a": {1},
	})

	assert.Equal(t, len(consumer.partitionStates), 0, "revoked states should be removed even if commit fails")
	runErr := consumer.runFailure()
	assert.NotNil(t, runErr, "commit failure on revoke should fail the consumer run")
	assert.StringContains(t, runErr.Error(), "failed to commit processed offsets on revoke", "run failure should explain revoke commit failure")
}

func TestConsumerOnPartitionsRevokedWaitsForDrainBeforeCommit(t *testing.T) {
	consumer := newTestConsumer()
	started := make(chan struct{})
	release := make(chan struct{})

	subscription, err := Subscription{
		Topic: "topic-a",
		Handler: func(context.Context, *kgo.Record) error {
			close(started)
			<-release
			return nil
		},
		BatchHandler:  nil,
		AckMode:       AckModeAtLeastOnce,
		FailurePolicy: FailurePolicy{},
	}.normalize()
	require.NoError(t, err, "subscription should normalize")

	state := newPartitionState(
		context.Background(),
		recordKey{topic: "topic-a", partition: 1},
		subscription,
		4,
	)
	consumer.partitionStates[state.key] = state
	consumer.processSem = make(chan struct{}, 1)

	workerDone := make(chan struct{})
	go func() {
		consumer.runPartitionState(state, nil)
		close(workerDone)
	}()

	_, buffered := state.tryEnqueueRecords([]*kgo.Record{{
		Topic:     "topic-a",
		Partition: 1,
		Offset:    5,
	}})
	assert.Equal(t, buffered, 1, "record should be buffered for the partition worker")

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for handler to start")
	}

	originalCommitOffsetsSyncFn := commitOffsetsSyncFn
	t.Cleanup(func() {
		commitOffsetsSyncFn = originalCommitOffsetsSyncFn
	})

	commitObserved := make(chan struct{}, 1)
	commitOffsetsSyncFn = func(_ context.Context, _ *kgo.Client, offsets map[string]map[int32]kgo.EpochOffset) error {
		select {
		case <-state.done:
		default:
			t.Fatal("revoke committed offsets before the partition worker drained")
		}
		assert.Equal(t, offsets["topic-a"][1].Offset, int64(6), "revoke should commit the processed record offset")
		commitObserved <- struct{}{}
		return nil
	}

	revokedDone := make(chan struct{})
	go func() {
		consumer.onPartitionsRevoked(context.Background(), nil, map[string][]int32{
			"topic-a": {1},
		})
		close(revokedDone)
	}()

	select {
	case <-commitObserved:
		t.Fatal("revoke should not commit before the in-flight handler finishes")
	case <-time.After(100 * time.Millisecond):
	}

	close(release)

	select {
	case <-revokedDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for revoke to finish")
	}

	select {
	case <-workerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for partition worker to stop")
	}

	select {
	case <-commitObserved:
	default:
		t.Fatal("revoke should commit offsets after the partition drains")
	}

	assert.Equal(t, len(consumer.partitionStates), 0, "revoked state should be removed after drain and commit")
}

func TestConsumerOnPartitionsLostDropsSelectedOffsets(t *testing.T) {
	consumer := newTestConsumer()

	stateA := newPartitionState(
		context.Background(),
		recordKey{topic: "topic-a", partition: 1},
		testSubscription("topic-a"),
		4,
	)
	stateB := newPartitionState(
		context.Background(),
		recordKey{topic: "topic-b", partition: 0},
		testSubscription("topic-b"),
		4,
	)
	assert.True(t, stateA.advanceCommitOffset(&kgo.Record{
		Topic:       "topic-a",
		Partition:   1,
		Offset:      5,
		LeaderEpoch: 4,
	}), "state should advance topic-a offset")
	assert.True(t, stateB.advanceCommitOffset(&kgo.Record{
		Topic:       "topic-b",
		Partition:   0,
		Offset:      1,
		LeaderEpoch: 7,
	}), "state should advance topic-b offset")

	consumer.partitionStates[recordKey{topic: "topic-a", partition: 1}] = stateA
	consumer.partitionStates[recordKey{topic: "topic-b", partition: 0}] = stateB
	require.True(t, consumer.markDirtyPartitionState(stateA), "state should be tracked as dirty")
	require.True(t, consumer.markDirtyPartitionState(stateB), "state should be tracked as dirty")

	consumer.onPartitionsLost(context.Background(), map[string][]int32{
		"topic-b": {0},
	})

	assert.Equal(t, len(consumer.partitionStates), 1, "lost partitions should remove only matching states")
	_, topicARemains := consumer.partitionStates[recordKey{topic: "topic-a", partition: 1}]
	assert.True(t, topicARemains, "unrelated state should remain active")

	offsets := consumer.snapshotDirtyOffsets()
	assert.Equal(t, len(offsets), 1, "lost partition progress should be dropped from dirty offsets")
	assert.Equal(t, offsets["topic-a"][1].Offset, int64(6), "remaining partition should preserve its dirty offset")
	assert.Nil(t, consumer.runFailure(), "lost partitions should not fail the run")
}

func TestConsumerShutdownRunDrainsWorkersBeforeFinalFlush(t *testing.T) {
	consumer := newTestConsumer()
	started := make(chan struct{})
	release := make(chan struct{})

	subscription, err := Subscription{
		Topic: "topic-a",
		Handler: func(context.Context, *kgo.Record) error {
			close(started)
			<-release
			return nil
		},
		BatchHandler:  nil,
		AckMode:       AckModeAtLeastOnce,
		FailurePolicy: FailurePolicy{},
	}.normalize()
	require.NoError(t, err, "subscription should normalize")

	state := newPartitionState(
		context.Background(),
		recordKey{topic: "topic-a", partition: 1},
		subscription,
		4,
	)
	consumer.partitionStates[state.key] = state
	consumer.processSem = make(chan struct{}, 1)

	workerDone := make(chan struct{})
	go func() {
		consumer.runPartitionState(state, nil)
		close(workerDone)
	}()

	_, buffered := state.tryEnqueueRecords([]*kgo.Record{{
		Topic:     "topic-a",
		Partition: 1,
		Offset:    5,
	}})
	assert.Equal(t, buffered, 1, "record should be buffered for the partition worker")

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for handler to start")
	}

	originalCommitOffsetsSyncFn := commitOffsetsSyncFn
	t.Cleanup(func() {
		commitOffsetsSyncFn = originalCommitOffsetsSyncFn
	})

	flushed := make(chan struct{}, 1)
	commitOffsetsSyncFn = func(_ context.Context, _ *kgo.Client, offsets map[string]map[int32]kgo.EpochOffset) error {
		select {
		case <-state.done:
		default:
			t.Fatal("shutdown flushed offsets before the partition worker drained")
		}
		assert.Equal(t, offsets["topic-a"][1].Offset, int64(6), "shutdown should flush the processed record offset")
		flushed <- struct{}{}
		return nil
	}

	shutdownDone := make(chan error, 1)
	go func() {
		shutdownDone <- consumer.shutdownRun(nil)
	}()

	select {
	case <-flushed:
		t.Fatal("shutdown should not flush before the in-flight handler finishes")
	case <-time.After(100 * time.Millisecond):
	}

	close(release)

	select {
	case err := <-shutdownDone:
		require.NoError(t, err, "shutdown should flush successfully")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for shutdown flush")
	}

	select {
	case <-workerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for partition worker to stop")
	}

	select {
	case <-flushed:
	default:
		t.Fatal("shutdown should flush processed offsets after draining workers")
	}

	assert.Equal(t, len(consumer.partitionStates), 0, "shutdown should remove drained partition states")
}

func TestConsumerFinalizeClosingPartitionStatesDoesNotAllowStaleFlushCommit(t *testing.T) {
	consumer := newTestConsumer()
	state := newPartitionState(
		context.Background(),
		recordKey{topic: "topic-a", partition: 1},
		testSubscription("topic-a"),
		4,
	)
	assert.True(t, state.advanceCommitOffset(&kgo.Record{
		Topic:       "topic-a",
		Partition:   1,
		Offset:      100,
		LeaderEpoch: 4,
	}), "state should advance initial dirty offset")
	consumer.partitionStates[state.key] = state
	require.True(t, consumer.markDirtyPartitionState(state), "state should be tracked as dirty")
	markPartitionStateStoppedForTest(state)

	originalCommitOffsetsSyncFn := commitOffsetsSyncFn
	originalAfterSnapshot := afterDirtyOffsetsSnapshot
	t.Cleanup(func() {
		commitOffsetsSyncFn = originalCommitOffsetsSyncFn
		afterDirtyOffsetsSnapshot = originalAfterSnapshot
	})

	snapshotTaken := make(chan struct{})
	releaseSnapshot := make(chan struct{})
	afterDirtyOffsetsSnapshot = func() {
		close(snapshotTaken)
		<-releaseSnapshot
	}

	committedOffsets := make(chan int64, 2)
	commitOffsetsSyncFn = func(_ context.Context, _ *kgo.Client, offsets map[string]map[int32]kgo.EpochOffset) error {
		committedOffsets <- offsets["topic-a"][1].Offset
		return nil
	}

	flushErrCh := make(chan error, 1)
	go func() {
		flushErrCh <- consumer.flushDirtyOffsets(context.Background(), nil)
	}()

	select {
	case <-snapshotTaken:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the normal flush to snapshot offsets")
	}

	assert.True(t, state.advanceCommitOffset(&kgo.Record{
		Topic:       "topic-a",
		Partition:   1,
		Offset:      101,
		LeaderEpoch: 4,
	}), "state should advance to the final drained offset")
	require.True(t, consumer.markDirtyPartitionState(state), "state should remain tracked as dirty")

	finalizeErrCh := make(chan error, 1)
	go func() {
		finalizeErrCh <- consumer.finalizeClosingPartitionStates(
			context.Background(),
			context.Background(),
			nil,
			[]*partitionState{state},
			"failed to commit processed offsets on shutdown",
		)
	}()

	select {
	case offset := <-committedOffsets:
		t.Fatalf("saw commit %d before releasing the in-flight flush snapshot", offset)
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseSnapshot)

	select {
	case err := <-flushErrCh:
		require.NoError(t, err, "normal flush should succeed")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the normal flush to finish")
	}

	select {
	case err := <-finalizeErrCh:
		require.NoError(t, err, "final closing commit should succeed")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for final closing commit to finish")
	}

	firstCommit := <-committedOffsets
	secondCommit := <-committedOffsets
	assert.Equal(t, firstCommit, int64(101), "normal flush should commit the older snapshot first")
	assert.Equal(t, secondCommit, int64(102), "final closing commit should commit the drained offset last")
	assert.Equal(t, len(consumer.partitionStates), 0, "final closing commit should clean up the partition state")
}

func TestConsumerFinalizeClosingPartitionStatesRespectsCommitContextWhileWaitingForCommitSerialization(t *testing.T) {
	consumer := newTestConsumer()
	state := newPartitionState(
		context.Background(),
		recordKey{topic: "topic-a", partition: 1},
		testSubscription("topic-a"),
		4,
	)
	assert.True(t, state.advanceCommitOffset(&kgo.Record{
		Topic:       "topic-a",
		Partition:   1,
		Offset:      5,
		LeaderEpoch: 4,
	}), "state should advance topic-a offset")
	consumer.partitionStates[state.key] = state
	markPartitionStateStoppedForTest(state)
	require.True(t, consumer.markDirtyPartitionState(state), "state should be tracked as dirty")

	originalCommitOffsetsSyncFn := commitOffsetsSyncFn
	t.Cleanup(func() {
		commitOffsetsSyncFn = originalCommitOffsetsSyncFn
	})

	commitOffsetsSyncFn = func(context.Context, *kgo.Client, map[string]map[int32]kgo.EpochOffset) error {
		t.Fatal("finalization should not reach the commit RPC when waiting for serialization times out")
		return nil
	}

	held := <-consumer.commitMu
	defer func() {
		consumer.commitMu <- held
	}()

	commitCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := consumer.finalizeClosingPartitionStates(
		context.Background(),
		commitCtx,
		nil,
		[]*partitionState{state},
		"failed to commit processed offsets on shutdown",
	)
	elapsed := time.Since(start)

	assert.NotNil(t, err, "finalization should fail when commit serialization cannot be acquired in time")
	assert.ErrorContains(t, err, context.DeadlineExceeded.Error(), "timeout should surface through finalization")
	assert.True(t, elapsed < time.Second, "finalization should stop waiting once the commit context expires")
}

func TestConsumerOnPartitionsRevokedUsesCallbackContextForFinalCommit(t *testing.T) {
	consumer := newTestConsumer()
	state := newPartitionState(
		context.Background(),
		recordKey{topic: "topic-a", partition: 1},
		testSubscription("topic-a"),
		4,
	)
	assert.True(t, state.advanceCommitOffset(&kgo.Record{
		Topic:       "topic-a",
		Partition:   1,
		Offset:      5,
		LeaderEpoch: 4,
	}), "state should advance topic-a offset")
	consumer.partitionStates[state.key] = state
	markPartitionStateStoppedForTest(state)
	require.True(t, consumer.markDirtyPartitionState(state), "state should be tracked as dirty")

	originalCommitOffsetsSyncFn := commitOffsetsSyncFn
	t.Cleanup(func() {
		commitOffsetsSyncFn = originalCommitOffsetsSyncFn
	})

	revokeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	commitOffsetsSyncFn = func(ctx context.Context, _ *kgo.Client, offsets map[string]map[int32]kgo.EpochOffset) error {
		assert.True(t, ctx == revokeCtx, "revoke final commit should use the rebalance callback context")
		assert.Equal(t, offsets["topic-a"][1].Offset, int64(6), "revoke should commit the processed record offset")
		return nil
	}

	consumer.onPartitionsRevoked(revokeCtx, nil, map[string][]int32{
		"topic-a": {1},
	})

	assert.Nil(t, consumer.runFailure(), "successful revoke commit should not fail the run")
}

func TestConsumerOnPartitionsRevokedStopsWaitingWhenCallbackContextIsCanceled(t *testing.T) {
	consumer := newTestConsumer()
	started := make(chan struct{})
	release := make(chan struct{})

	subscription, err := Subscription{
		Topic: "topic-a",
		Handler: func(context.Context, *kgo.Record) error {
			close(started)
			<-release
			return nil
		},
		BatchHandler:  nil,
		AckMode:       AckModeAtLeastOnce,
		FailurePolicy: FailurePolicy{},
	}.normalize()
	require.NoError(t, err, "subscription should normalize")

	state := newPartitionState(
		context.Background(),
		recordKey{topic: "topic-a", partition: 1},
		subscription,
		4,
	)
	consumer.partitionStates[state.key] = state
	consumer.processSem = make(chan struct{}, 1)

	workerDone := make(chan struct{})
	go func() {
		consumer.runPartitionState(state, nil)
		close(workerDone)
	}()

	_, buffered := state.tryEnqueueRecords([]*kgo.Record{{
		Topic:     "topic-a",
		Partition: 1,
		Offset:    5,
	}})
	assert.Equal(t, buffered, 1, "record should be buffered for the partition worker")

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for handler to start")
	}

	originalCommitOffsetsSyncFn := commitOffsetsSyncFn
	t.Cleanup(func() {
		commitOffsetsSyncFn = originalCommitOffsetsSyncFn
	})

	commitOffsetsSyncFn = func(context.Context, *kgo.Client, map[string]map[int32]kgo.EpochOffset) error {
		t.Fatal("revoke should not reach final commit after callback cancellation")
		return nil
	}

	revokeCtx, cancel := context.WithCancel(context.Background())
	revokedDone := make(chan struct{})
	go func() {
		consumer.onPartitionsRevoked(revokeCtx, nil, map[string][]int32{
			"topic-a": {1},
		})
		close(revokedDone)
	}()

	cancel()

	select {
	case <-revokedDone:
	case <-time.After(2 * time.Second):
		t.Fatal("revoke should stop waiting once the callback context is canceled")
	}

	runErr := consumer.runFailure()
	assert.NotNil(t, runErr, "canceled revoke drain should fail the consumer run")
	assert.ErrorContains(t, runErr, "context canceled", "revoke failure should surface callback cancellation")

	close(release)
	select {
	case <-workerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for partition worker to stop")
	}
}

func TestConsumerShutdownRunUsesFinalCommitTimeoutAfterDrain(t *testing.T) {
	consumer := newTestConsumer()
	started := make(chan struct{})
	release := make(chan struct{})

	subscription, err := Subscription{
		Topic: "topic-a",
		Handler: func(context.Context, *kgo.Record) error {
			close(started)
			<-release
			return nil
		},
		BatchHandler:  nil,
		AckMode:       AckModeAtLeastOnce,
		FailurePolicy: FailurePolicy{},
	}.normalize()
	require.NoError(t, err, "subscription should normalize")

	state := newPartitionState(
		context.Background(),
		recordKey{topic: "topic-a", partition: 1},
		subscription,
		4,
	)
	consumer.partitionStates[state.key] = state
	consumer.processSem = make(chan struct{}, 1)

	workerDone := make(chan struct{})
	go func() {
		consumer.runPartitionState(state, nil)
		close(workerDone)
	}()

	_, buffered := state.tryEnqueueRecords([]*kgo.Record{{
		Topic:     "topic-a",
		Partition: 1,
		Offset:    5,
	}})
	assert.Equal(t, buffered, 1, "record should be buffered for the partition worker")

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for handler to start")
	}

	originalCommitOffsetsSyncFn := commitOffsetsSyncFn
	t.Cleanup(func() {
		commitOffsetsSyncFn = originalCommitOffsetsSyncFn
	})

	releaseAtCh := make(chan time.Time, 1)
	commitOffsetsSyncFn = func(ctx context.Context, _ *kgo.Client, offsets map[string]map[int32]kgo.EpochOffset) error {
		deadline, ok := ctx.Deadline()
		assert.True(t, ok, "shutdown final commit should use a bounded timeout")
		releaseAt := <-releaseAtCh
		assert.True(
			t,
			deadline.After(releaseAt.Add(finalCommitTimeout-(250*time.Millisecond))),
			"shutdown final commit timeout should start after worker drain completes",
		)
		assert.Equal(t, offsets["topic-a"][1].Offset, int64(6), "shutdown should commit the processed record offset")
		return nil
	}

	shutdownDone := make(chan error, 1)
	go func() {
		shutdownDone <- consumer.shutdownRun(nil)
	}()

	time.Sleep(750 * time.Millisecond)
	releaseAt := time.Now()
	releaseAtCh <- releaseAt
	close(release)

	select {
	case err := <-shutdownDone:
		require.NoError(t, err, "shutdown should commit with the final timeout budget")
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for shutdown to finish")
	}

	select {
	case <-workerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for partition worker to stop")
	}

	assert.Equal(t, len(consumer.partitionStates), 0, "shutdown should clean up drained partition state")
}

func TestShouldGracefullyShutdown(t *testing.T) {
	parentCtx, cancel := context.WithCancel(context.Background())
	cancel()

	t.Run("clean run exits through shutdown", func(t *testing.T) {
		assert.Equal(
			t,
			shouldGracefullyShutdown(context.Background(), nil, nil),
			true,
			"clean runs should proceed through graceful shutdown",
		)
	})

	t.Run("caller cancellation still shuts down gracefully", func(t *testing.T) {
		assert.Equal(
			t,
			shouldGracefullyShutdown(parentCtx, context.Canceled, nil),
			true,
			"caller cancellation without a run failure should still drain and flush",
		)
	})

	t.Run("internal run failure skips graceful shutdown", func(t *testing.T) {
		assert.Equal(
			t,
			shouldGracefullyShutdown(parentCtx, context.Canceled, errors.New("boom")),
			false,
			"internal failures should bypass graceful shutdown",
		)
	})

	t.Run("other dispatch errors skip graceful shutdown", func(t *testing.T) {
		assert.Equal(
			t,
			shouldGracefullyShutdown(context.Background(), errors.New("boom"), nil),
			false,
			"non-cancellation dispatch errors should bypass graceful shutdown",
		)
	})
}

func TestConsumerRunCallerCancellationDuringBlockedDispatchStillDrainsAndFlushes(t *testing.T) {
	consumer := newTestConsumer()
	consumer.client = &Client{
		Consumer:  nil,
		Producer:  nil,
		kgoClient: &kgo.Client{},
		closeOnce: sync.Once{},
		closed:    false,
		mu:        sync.RWMutex{},
	}

	started := make(chan struct{})
	releaseHandler := make(chan struct{})

	subscription, err := Subscription{
		Topic: "topic-a",
		Handler: func(context.Context, *kgo.Record) error {
			close(started)
			<-releaseHandler
			return nil
		},
		BatchHandler:  nil,
		AckMode:       AckModeAtLeastOnce,
		FailurePolicy: FailurePolicy{},
	}.normalize()
	require.NoError(t, err, "subscription should normalize")
	require.NoError(t, consumer.registerSubscription(subscription, false), "topic should register")

	originalRunClientFn := runClientFn
	originalCommitOffsetsSyncFn := commitOffsetsSyncFn
	originalBeforeWaitHook := beforeWaitForDispatchHook
	t.Cleanup(func() {
		runClientFn = originalRunClientFn
		commitOffsetsSyncFn = originalCommitOffsetsSyncFn
		beforeWaitForDispatchHook = originalBeforeWaitHook
	})

	stateCh := make(chan *partitionState, 1)
	waitingForCapacity := make(chan struct{}, 1)
	beforeWaitForDispatchHook = func() {
		select {
		case waitingForCapacity <- struct{}{}:
		default:
		}
	}

	runClientFn = func(c *Consumer, ctx context.Context, _ *kgo.Client) error {
		c.runMu.RLock()
		runCtx := c.runCtx
		c.runMu.RUnlock()

		state := newPartitionState(
			runCtx,
			recordKey{topic: "topic-a", partition: 1},
			subscription,
			1,
		)
		c.workersMu.Lock()
		c.partitionStates[state.key] = state
		c.workersMu.Unlock()

		enqueued, buffered := state.tryEnqueueRecords([]*kgo.Record{{
			Topic:     "topic-a",
			Partition: 1,
			Offset:    5,
		}})
		if enqueued != 1 || buffered != 1 {
			return fmt.Errorf("failed to seed full partition queue: enqueued=%d buffered=%d", enqueued, buffered)
		}

		stateCh <- state

		return c.dispatchRecords(ctx, nil, []*kgo.Record{{
			Topic:     "topic-a",
			Partition: 1,
			Offset:    6,
		}})
	}

	commitStarted := make(chan struct{}, 1)
	allowCommitReturn := make(chan struct{})
	commitOffsetsSyncFn = func(_ context.Context, _ *kgo.Client, offsets map[string]map[int32]kgo.EpochOffset) error {
		assert.Equal(t, offsets["topic-a"][1].Offset, int64(6), "shutdown should flush the processed queued record")
		select {
		case commitStarted <- struct{}{}:
		default:
		}
		<-allowCommitReturn
		return nil
	}

	runCtx, cancel := context.WithCancel(context.Background())
	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- consumer.Run(runCtx)
	}()

	var state *partitionState
	select {
	case state = <-stateCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for seeded partition state")
	}

	select {
	case <-waitingForCapacity:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for dispatch to block on capacity")
	}

	cancel()

	workerDone := make(chan struct{})
	go func() {
		consumer.runPartitionState(state, nil)
		close(workerDone)
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for drained handler to start")
	}

	select {
	case err := <-runErrCh:
		t.Fatalf("Run returned before draining and flushing queued work: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseHandler)

	select {
	case <-commitStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for final offset flush")
	}

	select {
	case err := <-runErrCh:
		t.Fatalf("Run returned before the final commit completed: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(allowCommitReturn)

	select {
	case err := <-runErrCh:
		assert.ErrorContains(t, err, context.Canceled.Error(), "Run should still report caller cancellation")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Run to finish")
	}

	select {
	case <-workerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for partition worker to stop")
	}
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
		BatchHandler:  nil,
		AckMode:       AckModeAtLeastOnce,
		FailurePolicy: FailurePolicy{},
	}.normalize()
	require.NoError(t, err, "slow topic subscription should normalize")
	require.NoError(t, consumer.registerSubscription(slowSubscription, false), "slow topic should register")

	otherSubscription, err := Subscription{
		Topic: otherTopic,
		Handler: func(context.Context, *kgo.Record) error {
			fastValues <- fmt.Sprintf("%s/%d:%s", otherTopic, 0, "other-fast")
			return nil
		},
		BatchHandler:  nil,
		AckMode:       AckModeAtLeastOnce,
		FailurePolicy: FailurePolicy{},
	}.normalize()
	require.NoError(t, err, "other topic subscription should normalize")
	require.NoError(t, consumer.registerSubscription(otherSubscription, false), "other topic should register")

	runCtx, err := consumer.beginRun()
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

func TestConsumerPartitionBatchesPreservePolledOrderWithinPartition(t *testing.T) {
	consumer := newTestConsumer()
	require.NoError(t, consumer.registerSubscription(testSubscription("topic-a"), false), "topic should register")

	batches, err := consumer.partitionBatches([]*kgo.Record{
		{Topic: "topic-a", Partition: 1, Offset: 5},
		{Topic: "topic-a", Partition: 1, Offset: 2},
		{Topic: "topic-a", Partition: 1, Offset: 3},
	})
	require.NoError(t, err, "partition batching should succeed")
	assert.Equal(t, len(batches), 1, "records from one partition should form one batch")
	assert.Equal(t, batches[0].records[0].Offset, int64(5), "batching should preserve the polled record order")
	assert.Equal(t, batches[0].records[1].Offset, int64(2), "batching should not reorder records within a partition")
	assert.Equal(t, batches[0].records[2].Offset, int64(3), "later records should remain in polled order")
}

func TestConsumerPartitionBatchesPreserveFirstSeenPartitionOrder(t *testing.T) {
	consumer := newTestConsumer()
	require.NoError(t, consumer.registerSubscription(testSubscription("topic-a"), false), "topic-a should register")
	require.NoError(t, consumer.registerSubscription(testSubscription("topic-b"), false), "topic-b should register")

	batches, err := consumer.partitionBatches([]*kgo.Record{
		{Topic: "topic-b", Partition: 2, Offset: 1},
		{Topic: "topic-a", Partition: 3, Offset: 1},
		{Topic: "topic-a", Partition: 1, Offset: 1},
	})
	require.NoError(t, err, "partition batching should succeed")
	assert.Equal(t, len(batches), 3, "each topic-partition should produce one batch")
	assert.Equal(t, batches[0].records[0].Topic, "topic-b", "batch order should follow the first seen partition in the poll")
	assert.Equal(t, batches[0].records[0].Partition, int32(2), "first seen partition should remain first")
	assert.Equal(t, batches[1].records[0].Topic, "topic-a", "later first-seen partitions should follow in encounter order")
	assert.Equal(t, batches[1].records[0].Partition, int32(3), "second batch should preserve its partition")
	assert.Equal(t, batches[2].records[0].Topic, "topic-a", "subsequent partitions from the same topic should keep encounter order")
	assert.Equal(t, batches[2].records[0].Partition, int32(1), "third batch should preserve its partition")
}

func newTestConsumer() *Consumer {
	cfg := newConfig([]string{"broker:9092"}, "group")
	consumer := &Consumer{
		client:            nil,
		cfg:               cfg,
		log:               slog.Default(),
		runCtx:            nil,
		runCancel:         nil,
		runErr:            nil,
		processSem:        nil,
		commitSignal:      nil,
		dispatchSignal:    nil,
		subscriptionState: gsync.Value[map[string]Subscription]{},
		pausedTopicsState: gsync.Value[map[string]pausedTopic]{},
		subscriptions:     make(map[string]Subscription),
		pausedTopics:      make(map[string]pausedTopic),
		partitionStates:   make(map[recordKey]*partitionState),
		dirtyStates:       make(map[recordKey]*partitionState),
		commitMu:          newCommitCoordinator(),
		runMu:             sync.RWMutex{},
		workersMu:         sync.RWMutex{},
		subscriptionMu:    sync.Mutex{},
		pausedTopicsMu:    sync.Mutex{},
		runErrOnce:        sync.Once{},
		runWG:             sync.WaitGroup{},
	}
	consumer.subscriptionState.Store(map[string]Subscription{})
	consumer.pausedTopicsState.Store(map[string]pausedTopic{})
	return consumer
}

func testSubscription(topic string) Subscription {
	subscription, err := Subscription{
		Topic:         topic,
		Handler:       func(context.Context, *kgo.Record) error { return nil },
		BatchHandler:  nil,
		AckMode:       AckModeAtLeastOnce,
		FailurePolicy: FailurePolicy{},
	}.normalize()
	if err != nil {
		panic(err)
	}
	return subscription
}

func markPartitionStateStoppedForTest(state *partitionState) {
	state.markStopped()
	close(state.done)
}
