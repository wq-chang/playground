package kafka

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"go-services/library/assert"
	"go-services/library/require"
)

func TestNewConsumer_RegistersStartupBatchTopics(t *testing.T) {
	cfg := newConfig([]string{"broker:9092"}, "group")
	startupHandler := func(context.Context, []*kgo.Record) BatchResult { return BatchResult{} }
	cfg.subscriptions["topic-a"] = Subscription{
		Topic:         "topic-a",
		Handler:       nil,
		BatchHandler:  startupHandler,
		AckMode:       AckModeAtLeastOnce,
		FailurePolicy: FailurePolicy{},
	}

	consumer, err := newConsumer(cfg, nil)
	require.NoError(t, err, "failed to create consumer")

	subscription, ok := consumer.subscriptionForTopic("topic-a")
	if !ok {
		t.Fatal("startup batch topic should be registered")
	}
	assert.True(t, subscription.Handler == nil, "startup batch topic should not register a single-record handler")
	assert.NotNil(t, subscription.BatchHandler, "startup batch handler should be available")
}

func TestConsumerAddSubscriptionValidationForBatchModes(t *testing.T) {
	cfg := newConfig([]string{"broker:9092"}, "group")
	consumer, err := newConsumer(cfg, nil)
	require.NoError(t, err, "failed to create consumer")

	handler := func(context.Context, *kgo.Record) error { return nil }
	batchHandler := func(context.Context, []*kgo.Record) BatchResult { return BatchResult{} }

	t.Run("reject missing handlers", func(t *testing.T) {
		err := consumer.AddSubscription(Subscription{
			Topic:         "topic-a",
			Handler:       nil,
			BatchHandler:  nil,
			AckMode:       AckModeAtLeastOnce,
			FailurePolicy: FailurePolicy{},
		})
		assert.NotNil(t, err, "should reject subscriptions without a handler")
		assert.StringContains(t, err.Error(), "exactly one of handler or batch handler must be set", "error message")
	})

	t.Run("reject both handler modes", func(t *testing.T) {
		err := consumer.AddSubscription(Subscription{
			Topic:         "topic-a",
			Handler:       handler,
			BatchHandler:  batchHandler,
			AckMode:       AckModeAtLeastOnce,
			FailurePolicy: FailurePolicy{},
		})
		assert.NotNil(t, err, "should reject subscriptions that mix handler modes")
		assert.StringContains(t, err.Error(), "exactly one of handler or batch handler must be set", "error message")
	})
}

func TestConsumerAddBatchTopicUsesDefaultAckMode(t *testing.T) {
	cfg := newConfig([]string{"broker:9092"}, "group")
	cfg.defaultAckMode = AckModeAtMostOnce

	consumer, err := newConsumer(cfg, nil)
	require.NoError(t, err, "failed to create consumer")

	err = consumer.AddBatchTopic("topic-a", func(context.Context, []*kgo.Record) BatchResult {
		return BatchResult{}
	})
	require.NoError(t, err, "batch topic should register")

	subscription, ok := consumer.subscriptionForTopic("topic-a")
	require.True(t, ok, "batch topic should be registered")
	assert.Equal(t, subscription.AckMode, AckModeAtMostOnce, "batch topic should inherit the consumer default ack mode")
}

func TestConsumerExecuteBatchRetriesRemainingSuffix(t *testing.T) {
	consumer := newTestConsumer()
	calls := make([][]int64, 0, 2)
	attempts := 0

	subscription, err := Subscription{
		Topic:   "topic-a",
		Handler: nil,
		BatchHandler: func(_ context.Context, records []*kgo.Record) BatchResult {
			calls = append(calls, recordOffsets(records))
			attempts++
			if attempts == 1 {
				return BatchResult{FailedAt: 1, Err: errors.New("boom")}
			}
			return BatchResult{}
		},
		AckMode: AckModeAtLeastOnce,
		FailurePolicy: FailurePolicy{
			DLQ:          nil,
			RetryBackoff: 0,
			MaxAttempts:  2,
			OnExhausted:  ExhaustedActionUnspecified,
		},
	}.normalize()
	require.NoError(t, err, "batch subscription should normalize")

	result, err := consumer.executeBatch(context.Background(), subscription, testRecords("topic-a", 0, 10, 11, 12))
	require.NoError(t, err, "batch retry should succeed")

	assert.Equal(t, attempts, 2, "batch retry should invoke the handler twice")
	assert.Equal(t, len(calls), 2, "batch retry should keep one call per attempt")
	assert.Equal(t, calls[0][0], int64(10), "first attempt should receive the full batch")
	assert.Equal(t, len(calls[0]), 3, "first attempt should receive all records")
	assert.Equal(t, calls[1][0], int64(11), "retry should restart from the first failed record")
	assert.Equal(t, len(calls[1]), 2, "retry should exclude the successful prefix")
	assert.Equal(t, result.resolvedCount, 3, "successful retry should resolve the full batch")
	assert.False(t, result.pauseTopic, "successful retry should not pause the topic")
}

func TestConsumerExecuteBatchCommitExhaustedContinuesRemainingRecords(t *testing.T) {
	consumer := newTestConsumer()
	calls := make([][]int64, 0, 2)

	subscription, err := Subscription{
		Topic:   "topic-a",
		Handler: nil,
		BatchHandler: func(_ context.Context, records []*kgo.Record) BatchResult {
			calls = append(calls, recordOffsets(records))
			if len(records) > 1 {
				return BatchResult{FailedAt: 1, Err: errors.New("boom")}
			}
			return BatchResult{}
		},
		AckMode: AckModeAtLeastOnce,
		FailurePolicy: FailurePolicy{
			DLQ:          nil,
			RetryBackoff: 0,
			MaxAttempts:  1,
			OnExhausted:  ExhaustedActionCommit,
		},
	}.normalize()
	require.NoError(t, err, "batch subscription should normalize")

	result, err := consumer.executeBatch(context.Background(), subscription, testRecords("topic-a", 0, 10, 11, 12))
	require.NoError(t, err, "commit-on-exhausted should keep processing later records")

	assert.Equal(t, len(calls), 2, "commit exhaustion should continue with the remaining suffix")
	assert.Equal(t, calls[0], []int64{10, 11, 12}, "first attempt should receive the full batch")
	assert.Equal(t, calls[1], []int64{12}, "failed record should be skipped before continuing")
	assert.Equal(t, result.resolvedCount, 3, "commit exhaustion should resolve the full batch")
	assert.False(t, result.pauseTopic, "commit exhaustion should not pause the topic")
}

func TestConsumerExecuteBatchStopOnExhaustedReturnsResolvedPrefix(t *testing.T) {
	consumer := newTestConsumer()

	subscription, err := Subscription{
		Topic:   "topic-a",
		Handler: nil,
		BatchHandler: func(context.Context, []*kgo.Record) BatchResult {
			return BatchResult{FailedAt: 2, Err: errors.New("boom")}
		},
		AckMode: AckModeAtLeastOnce,
		FailurePolicy: FailurePolicy{
			DLQ:          nil,
			RetryBackoff: 0,
			MaxAttempts:  1,
			OnExhausted:  ExhaustedActionStop,
		},
	}.normalize()
	require.NoError(t, err, "batch subscription should normalize")

	result, err := consumer.executeBatch(context.Background(), subscription, testRecords("topic-a", 0, 10, 11, 12))
	require.NoError(t, err, "stop-on-exhausted should stay inside the failure policy flow")

	assert.Equal(t, result.resolvedCount, 2, "stop-on-exhausted should keep the successful prefix")
	assert.True(t, result.pauseTopic, "stop-on-exhausted should pause the topic")
	require.NotNil(t, result.cause, "stop-on-exhausted should surface the failure cause")
	assert.ErrorContains(t, result.cause, `handler failed for topic "topic-a" partition 0 offset 12 after 1 attempts`, "failure cause should describe the failed record")
}

func TestConsumerExecuteBatchRejectsInvalidFailedIndex(t *testing.T) {
	consumer := newTestConsumer()

	subscription, err := Subscription{
		Topic:   "topic-a",
		Handler: nil,
		BatchHandler: func(context.Context, []*kgo.Record) BatchResult {
			return BatchResult{FailedAt: 3, Err: errors.New("boom")}
		},
		AckMode: AckModeAtLeastOnce,
		FailurePolicy: FailurePolicy{
			DLQ:          nil,
			RetryBackoff: 0,
			MaxAttempts:  1,
			OnExhausted:  ExhaustedActionUnspecified,
		},
	}.normalize()
	require.NoError(t, err, "batch subscription should normalize")

	_, err = consumer.executeBatch(context.Background(), subscription, testRecords("topic-a", 0, 10, 11, 12))
	assert.NotNil(t, err, "invalid failed index should fail execution")
	assert.ErrorContains(t, err, "batch handler returned failed index 3 for batch size 3", "error should explain the invalid batch result")
}

func TestConsumerDispatchRecordsUsesBatchHandler(t *testing.T) {
	consumer := newTestConsumer()
	handled := make(chan []int64, 1)

	subscription, err := Subscription{
		Topic:   "topic-a",
		Handler: nil,
		BatchHandler: func(_ context.Context, records []*kgo.Record) BatchResult {
			handled <- recordOffsets(records)
			return BatchResult{}
		},
		AckMode:       AckModeAtLeastOnce,
		FailurePolicy: FailurePolicy{},
	}.normalize()
	require.NoError(t, err, "batch subscription should normalize")
	require.NoError(t, consumer.registerSubscription(subscription, false), "batch topic should register")

	runCtx, err := consumer.beginRun()
	require.NoError(t, err, "consumer run state should initialize")
	defer func() {
		consumer.stopRun()
		consumer.runWG.Wait()
		consumer.resetRunState()
	}()

	require.NoError(t, consumer.dispatchRecords(runCtx, nil, testRecords("topic-a", 0, 10, 11)), "dispatch should enqueue the batch")

	select {
	case offsets := <-handled:
		assert.Equal(t, offsets, []int64{10, 11}, "worker should invoke the batch handler once with the whole partition batch")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for batch handler invocation")
	}

	state, ok := consumer.partitionStates[recordKey{topic: "topic-a", partition: 0}]
	require.True(t, ok, "dispatch should create partition state")
	offset, dirty := state.snapshotDirtyOffset()
	require.True(t, dirty, "successful batch handling should advance commit state")
	assert.Equal(t, offset.Offset, int64(12), "commit progress should advance past the last resolved record")
}

func recordOffsets(records []*kgo.Record) []int64 {
	offsets := make([]int64, 0, len(records))
	for _, record := range records {
		offsets = append(offsets, record.Offset)
	}
	return offsets
}

func testRecords(topic string, partition int32, offsets ...int64) []*kgo.Record {
	records := make([]*kgo.Record, 0, len(offsets))
	for _, offset := range offsets {
		records = append(records, &kgo.Record{
			Topic:     topic,
			Partition: partition,
			Offset:    offset,
		})
	}
	return records
}
