// services/go/library/kafka/internal/consumer/record_executor_test.go
package consumer_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"go-services/library/assert"
	"go-services/library/kafka/internal/consumer"
	"go-services/library/testlogger"
)

func testSubRecord(
	fn func(context.Context, *kgo.Record) error,
	ack consumer.AckMode,
	fp consumer.FailurePolicy,
) consumer.Subscription {
	return consumer.Subscription{
		Topic:         "t",
		Handler:       fn,
		BatchHandler:  nil,
		FailurePolicy: fp,
		AckMode:       ack,
	}
}

func testSubBatch(
	fn func(context.Context, []*kgo.Record) consumer.BatchResult,
	ack consumer.AckMode,
	fp consumer.FailurePolicy,
) consumer.Subscription {
	return consumer.Subscription{
		Topic:         "t",
		Handler:       nil,
		BatchHandler:  fn,
		FailurePolicy: fp,
		AckMode:       ack,
	}
}

func fp1(stop consumer.ExhaustedAction) consumer.FailurePolicy {
	return consumer.FailurePolicy{MaxAttempts: 1, RetryBackoff: 0, DLQ: nil, OnExhausted: stop}
}

func fp3() consumer.FailurePolicy {
	return consumer.FailurePolicy{MaxAttempts: 3, RetryBackoff: time.Millisecond, DLQ: nil, OnExhausted: consumer.ExhaustedActionStop}
}

func fpDLQ(topic string) consumer.FailurePolicy {
	return consumer.FailurePolicy{MaxAttempts: 1, RetryBackoff: 0, DLQ: &consumer.DLQConfig{Topic: topic}, OnExhausted: consumer.ExhaustedActionDLQThenCommit}
}

func fpUnsupported() consumer.FailurePolicy {
	return consumer.FailurePolicy{MaxAttempts: 1, RetryBackoff: 0, DLQ: nil, OnExhausted: consumer.ExhaustedAction(99)}
}

func TestRecordExecutor_Success(t *testing.T) {
	exec := consumer.NewRecordExecutor(testlogger.NewLogger())
	var called atomic.Int32

	sub := testSubRecord(
		func(ctx context.Context, record *kgo.Record) error { called.Add(1); return nil },
		consumer.AckModeAtLeastOnce,
		fp1(consumer.ExhaustedActionStop),
	)
	result := exec.ExecuteRecord(context.Background(), sub, &kgo.Record{Topic: "t"}, nil)
	assert.True(t, result.Resolved, "should be resolved")
	assert.Equal(t, int32(1), called.Load(), "handler should be called once")
}

func TestRecordExecutor_RetryThenSuccess(t *testing.T) {
	exec := consumer.NewRecordExecutor(testlogger.NewLogger())
	var attempts atomic.Int32

	sub := testSubRecord(
		func(ctx context.Context, record *kgo.Record) error {
			n := attempts.Add(1)
			if n < 3 {
				return errors.New("not yet")
			}
			return nil
		},
		consumer.AckModeAtLeastOnce,
		fp3(),
	)
	result := exec.ExecuteRecord(context.Background(), sub, &kgo.Record{Topic: "t"}, nil)
	assert.True(t, result.Resolved, "should resolve after retries")
	assert.Equal(t, int32(3), attempts.Load(), "handler should be called 3 times")
}

func TestRecordExecutor_ExhaustedStop_PausesTopic(t *testing.T) {
	exec := consumer.NewRecordExecutor(testlogger.NewLogger())
	sub := testSubRecord(
		func(ctx context.Context, record *kgo.Record) error { return errors.New("always fail") },
		consumer.AckModeAtLeastOnce,
		fp1(consumer.ExhaustedActionStop),
	)
	result := exec.ExecuteRecord(context.Background(), sub, &kgo.Record{Topic: "t", Partition: 0, Offset: 5}, nil)
	assert.False(t, result.Resolved, "should not be resolved")
	assert.True(t, result.PauseTopic, "should pause topic on exhaustion stop")
}

func TestRecordExecutor_ExhaustedCommit_Resolves(t *testing.T) {
	exec := consumer.NewRecordExecutor(testlogger.NewLogger())
	sub := testSubRecord(
		func(ctx context.Context, record *kgo.Record) error { return errors.New("always fail") },
		consumer.AckModeAtLeastOnce,
		fp1(consumer.ExhaustedActionCommit),
	)
	result := exec.ExecuteRecord(context.Background(), sub, &kgo.Record{Topic: "t"}, nil)
	assert.True(t, result.Resolved, "should resolve on commit exhaustion")
	assert.False(t, result.PauseTopic, "should not pause topic on commit exhaustion")
}

func TestRecordExecutor_ExhaustedDLQ_CallsWriter(t *testing.T) {
	exec := consumer.NewRecordExecutor(testlogger.NewLogger())
	var dlqCalled atomic.Int32

	dlqWriter := func(ctx context.Context, enriched *kgo.Record) error {
		dlqCalled.Add(1)
		assert.Equal(t, "dlq", enriched.Topic, "should set DLQ topic")
		found := false
		for _, h := range enriched.Headers {
			if h.Key == "dlq-original-topic" {
				found = true
				assert.Equal(t, "t", string(h.Value), "original topic should match")
			}
		}
		assert.True(t, found, "should have dlq-original-topic header")
		return nil
	}

	sub := testSubRecord(
		func(ctx context.Context, record *kgo.Record) error { return errors.New("fail") },
		consumer.AckModeAtLeastOnce,
		fpDLQ("dlq"),
	)
	result := exec.ExecuteRecord(context.Background(), sub, &kgo.Record{Topic: "t", Offset: 5}, dlqWriter)
	assert.True(t, result.Resolved, "should resolve on DLQ commit")
	assert.Equal(t, int32(1), dlqCalled.Load(), "DLQ writer should be called once")
}

func TestRecordExecutor_HandlerPanic(t *testing.T) {
	exec := consumer.NewRecordExecutor(testlogger.NewLogger())
	sub := testSubRecord(
		func(ctx context.Context, record *kgo.Record) error { panic("handler panic") },
		consumer.AckModeAtLeastOnce,
		fp1(consumer.ExhaustedActionStop),
	)
	result := exec.ExecuteRecord(context.Background(), sub, &kgo.Record{Topic: "t"}, nil)
	assert.False(t, result.Resolved, "should not resolve on panic")
	assert.True(t, result.PauseTopic, "should pause on panic exhaustion")
}

func TestRecordExecutor_Batch_Success(t *testing.T) {
	exec := consumer.NewRecordExecutor(testlogger.NewLogger())
	sub := testSubBatch(
		func(ctx context.Context, records []*kgo.Record) consumer.BatchResult { return consumer.BatchResult{} },
		consumer.AckModeAtLeastOnce,
		fp1(consumer.ExhaustedActionStop),
	)

	records := []*kgo.Record{{Topic: "t", Offset: 0}, {Topic: "t", Offset: 1}}
	resolvedCount, cause, pauseTopic := exec.ExecuteBatch(context.Background(), sub, records, nil)
	assert.Equal(t, 2, resolvedCount, "all records should be resolved")
	assert.Nil(t, cause, "no error on success")
	assert.False(t, pauseTopic, "should not pause on success")
}

func TestRecordExecutor_Batch_PartialFailure(t *testing.T) {
	exec := consumer.NewRecordExecutor(testlogger.NewLogger())
	sub := testSubBatch(
		func(ctx context.Context, records []*kgo.Record) consumer.BatchResult {
			return consumer.BatchResult{Err: errors.New("batch failed"), FailedAt: 1}
		},
		consumer.AckModeAtLeastOnce,
		fp1(consumer.ExhaustedActionStop),
	)

	records := []*kgo.Record{{Topic: "t", Offset: 0}, {Topic: "t", Offset: 1}}
	resolvedCount, cause, pauseTopic := exec.ExecuteBatch(context.Background(), sub, records, nil)
	assert.Equal(t, 1, resolvedCount, "first record should be resolved")
	assert.NotNil(t, cause, "should return error on batch failure")
	assert.True(t, pauseTopic, "should pause on stop exhaustion")
}

func TestRecordExecutor_Batch_DLQ(t *testing.T) {
	exec := consumer.NewRecordExecutor(testlogger.NewLogger())
	var dlqCalled atomic.Int32

	dlqWriter := func(ctx context.Context, enriched *kgo.Record) error {
		dlqCalled.Add(1)
		return nil
	}

	sub := testSubBatch(
		func(ctx context.Context, records []*kgo.Record) consumer.BatchResult {
			return consumer.BatchResult{Err: errors.New("fail"), FailedAt: 0}
		},
		consumer.AckModeAtLeastOnce,
		fpDLQ("dlq"),
	)

	records := []*kgo.Record{{Topic: "t", Offset: 0}}
	resolvedCount, cause, pauseTopic := exec.ExecuteBatch(context.Background(), sub, records, dlqWriter)
	assert.Equal(t, 1, resolvedCount, "record resolved via DLQ should count as resolved")
	assert.Nil(t, cause, "no error on DLQ commit")
	assert.False(t, pauseTopic, "should not pause on DLQ commit")
	assert.Equal(t, int32(1), dlqCalled.Load(), "DLQ writer should be called")
}

func TestRecordExecutor_Batch_HandlerPanic(t *testing.T) {
	exec := consumer.NewRecordExecutor(testlogger.NewLogger())
	sub := testSubBatch(
		func(ctx context.Context, records []*kgo.Record) consumer.BatchResult { panic("batch panic") },
		consumer.AckModeAtLeastOnce,
		fp1(consumer.ExhaustedActionStop),
	)

	records := []*kgo.Record{{Topic: "t", Offset: 0}}
	resolvedCount, cause, pauseTopic := exec.ExecuteBatch(context.Background(), sub, records, nil)
	assert.Equal(t, 0, resolvedCount, "should not resolve on panic")
	assert.NotNil(t, cause, "should return error on panic")
	assert.True(t, pauseTopic, "should pause on stop exhaustion after panic")
}

func TestRecordExecutor_ContextCancelled(t *testing.T) {
	exec := consumer.NewRecordExecutor(testlogger.NewLogger())
	sub := testSubRecord(
		func(ctx context.Context, record *kgo.Record) error { return errors.New("fail") },
		consumer.AckModeAtLeastOnce,
		fp3(),
	)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := exec.ExecuteRecord(ctx, sub, &kgo.Record{Topic: "t"}, nil)
	assert.False(t, result.Resolved, "should not resolve on cancelled context")
}

func TestRecordExecutor_ExhaustedDLQ_WriterFails(t *testing.T) {
	exec := consumer.NewRecordExecutor(testlogger.NewLogger())
	dlqWriter := func(ctx context.Context, enriched *kgo.Record) error {
		return errors.New("dlq write failed")
	}
	sub := testSubRecord(
		func(ctx context.Context, record *kgo.Record) error { return errors.New("fail") },
		consumer.AckModeAtLeastOnce,
		fpDLQ("dlq"),
	)
	result := exec.ExecuteRecord(context.Background(), sub, &kgo.Record{Topic: "t"}, dlqWriter)
	assert.False(t, result.Resolved, "should not resolve when DLQ write fails")
	assert.True(t, result.PauseTopic, "should pause topic when DLQ write fails after retries")
	assert.NotNil(t, result.Cause, "should return error from DLQ failure")
}

func TestRecordExecutor_ExhaustedDLQ_RetriesThenSucceeds(t *testing.T) {
	exec := consumer.NewRecordExecutor(testlogger.NewLogger())
	var dlqCalls atomic.Int32

	dlqWriter := func(ctx context.Context, enriched *kgo.Record) error {
		n := dlqCalls.Add(1)
		if n < 3 {
			return errors.New("dlq write failed")
		}
		return nil
	}

	sub := testSubRecord(
		func(ctx context.Context, record *kgo.Record) error { return errors.New("fail") },
		consumer.AckModeAtLeastOnce,
		consumer.FailurePolicy{MaxAttempts: 3, RetryBackoff: time.Millisecond, DLQ: &consumer.DLQConfig{Topic: "dlq"}, OnExhausted: consumer.ExhaustedActionDLQThenCommit},
	)
	result := exec.ExecuteRecord(context.Background(), sub, &kgo.Record{Topic: "t"}, dlqWriter)
	assert.True(t, result.Resolved, "should resolve after DLQ retry succeeds")
	assert.False(t, result.PauseTopic, "should not pause topic on DLQ success")
	assert.Equal(t, int32(3), dlqCalls.Load(), "DLQ writer should be called 3 times")
}

func TestRecordExecutor_ExhaustedDLQ_RetriesExhausted_PausesTopic(t *testing.T) {
	exec := consumer.NewRecordExecutor(testlogger.NewLogger())
	var dlqCalls atomic.Int32

	dlqWriter := func(ctx context.Context, enriched *kgo.Record) error {
		dlqCalls.Add(1)
		return errors.New("dlq write failed")
	}

	sub := testSubRecord(
		func(ctx context.Context, record *kgo.Record) error { return errors.New("fail") },
		consumer.AckModeAtLeastOnce,
		consumer.FailurePolicy{MaxAttempts: 3, RetryBackoff: time.Millisecond, DLQ: &consumer.DLQConfig{Topic: "dlq"}, OnExhausted: consumer.ExhaustedActionDLQThenCommit},
	)
	result := exec.ExecuteRecord(context.Background(), sub, &kgo.Record{Topic: "t"}, dlqWriter)
	assert.False(t, result.Resolved, "should not resolve when all DLQ retries fail")
	assert.True(t, result.PauseTopic, "should pause topic after all DLQ retries fail")
	assert.NotNil(t, result.Cause, "should return error after DLQ retries exhausted")
	assert.Equal(t, int32(3), dlqCalls.Load(), "DLQ writer should be called MaxAttempts times")
}

func TestRecordExecutor_UnsupportedExhaustedAction(t *testing.T) {
	exec := consumer.NewRecordExecutor(testlogger.NewLogger())
	sub := testSubRecord(
		func(ctx context.Context, record *kgo.Record) error { return errors.New("fail") },
		consumer.AckModeAtLeastOnce,
		fpUnsupported(),
	)
	result := exec.ExecuteRecord(context.Background(), sub, &kgo.Record{Topic: "t"}, nil)
	assert.False(t, result.Resolved, "should not resolve with unknown exhausted action")
	assert.NotNil(t, result.Cause, "should return unsupported exhausted action error")
}

func TestRecordExecutor_Batch_RejectsInvalidFailedIndex(t *testing.T) {
	exec := consumer.NewRecordExecutor(nil)
	sub := testSubBatch(
		func(_ context.Context, _ []*kgo.Record) consumer.BatchResult {
			return consumer.BatchResult{Err: errors.New("fail"), FailedAt: -1}
		},
		consumer.AckModeAtLeastOnce,
		fp1(consumer.ExhaustedActionStop),
	)
	records := []*kgo.Record{{Topic: "t", Offset: 0}, {Topic: "t", Offset: 1}}
	resolved, cause, _ := exec.ExecuteBatch(context.Background(), sub, records, nil)
	assert.Equal(t, 0, resolved, "should resolve 0 with invalid FailedAt")
	assert.NotNil(t, cause, "should return error for negative FailedAt")
}

func TestRecordExecutor_Batch_RejectsFailedIndexOutOfBounds(t *testing.T) {
	exec := consumer.NewRecordExecutor(nil)
	sub := testSubBatch(
		func(_ context.Context, _ []*kgo.Record) consumer.BatchResult {
			return consumer.BatchResult{Err: errors.New("fail"), FailedAt: 99}
		},
		consumer.AckModeAtLeastOnce,
		fp1(consumer.ExhaustedActionStop),
	)
	records := []*kgo.Record{{Topic: "t", Offset: 0}, {Topic: "t", Offset: 1}}
	resolved, cause, _ := exec.ExecuteBatch(context.Background(), sub, records, nil)
	assert.Equal(t, 0, resolved, "should resolve 0 with out-of-bounds FailedAt")
	assert.NotNil(t, cause, "should return error for out-of-bounds FailedAt")
}
