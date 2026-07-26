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
	return consumer.FailurePolicy{
		MaxAttempts:  1,
		RetryBackoff: 0,
		DLQ:          nil,
		OnExhausted:  stop,
	}
}

func fp3() consumer.FailurePolicy {
	return consumer.FailurePolicy{
		MaxAttempts:  3,
		RetryBackoff: time.Millisecond,
		DLQ:          nil,
		OnExhausted:  consumer.ExhaustedActionStop,
	}
}

func fpDLQ(topic string) consumer.FailurePolicy {
	return consumer.FailurePolicy{
		MaxAttempts:  1,
		RetryBackoff: 0,
		DLQ:          &consumer.DLQConfig{Topic: topic},
		OnExhausted:  consumer.ExhaustedActionDLQThenCommit,
	}
}

func fpUnsupported() consumer.FailurePolicy {
	return consumer.FailurePolicy{
		MaxAttempts:  1,
		RetryBackoff: 0,
		DLQ:          nil,
		OnExhausted:  consumer.ExhaustedAction(99),
	}
}

func TestRecordExecutor_ExecuteRecord_Success(t *testing.T) {
	exec := consumer.NewRecordExecutor(testlogger.NewLogger())
	var called atomic.Int32

	sub := testSubRecord(
		func(ctx context.Context, record *kgo.Record) error { called.Add(1); return nil },
		consumer.AckModeAtLeastOnce,
		fp1(consumer.ExhaustedActionStop),
	)
	result := exec.ExecuteRecord(context.Background(), sub, &kgo.Record{Topic: "t"}, nil)
	assert.True(t, result.Resolved, "should be resolved")
	assert.Equal(t, called.Load(), 1, "handler should be called once")
}

func TestRecordExecutor_ExecuteRecord_RetryThenSuccess(t *testing.T) {
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
	assert.Equal(t, attempts.Load(), 3, "handler should be called 3 times")
}

func TestRecordExecutor_ExecuteRecord_RetryWithNoBackoff(t *testing.T) {
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
		consumer.FailurePolicy{MaxAttempts: 3, RetryBackoff: 0, DLQ: nil, OnExhausted: consumer.ExhaustedActionStop},
	)
	result := exec.ExecuteRecord(context.Background(), sub, &kgo.Record{Topic: "t"}, nil)
	assert.True(t, result.Resolved, "should resolve after retries with zero backoff")
	assert.Equal(t, attempts.Load(), 3, "handler should be called 3 times")
}

func TestRecordExecutor_ExecuteRecord_HandlerPanic(t *testing.T) {
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

func TestRecordExecutor_ExecuteRecord_ContextCancelled(t *testing.T) {
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

func TestRecordExecutor_ExecuteRecord_ContextCancelledDuringRetry(t *testing.T) {
	exec := consumer.NewRecordExecutor(testlogger.NewLogger())
	var called atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())

	sub := testSubRecord(
		func(ctx context.Context, record *kgo.Record) error {
			if called.Add(1) == 1 {
				cancel()
			}
			return errors.New("fail")
		},
		consumer.AckModeAtLeastOnce,
		fp3(),
	)
	result := exec.ExecuteRecord(ctx, sub, &kgo.Record{Topic: "t"}, nil)
	assert.False(t, result.Resolved, "should not resolve on cancelled context")
	assert.False(t, result.PauseTopic, "should not pause topic on context cancellation")
	assert.NotNil(t, result.Cause, "should return context error")
}

func TestRecordExecutor_ExecuteRecord_ExhaustedStop(t *testing.T) {
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

func TestRecordExecutor_ExecuteRecord_ExhaustedCommit(t *testing.T) {
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

func TestRecordExecutor_ExecuteRecord_ExhaustedDLQ(t *testing.T) {
	exec := consumer.NewRecordExecutor(testlogger.NewLogger())
	var dlqCalled atomic.Int32

	dlqWriter := func(ctx context.Context, enriched *kgo.Record) error {
		dlqCalled.Add(1)
		assert.Equal(t, enriched.Topic, "dlq", "should set DLQ topic")
		found := false
		for _, h := range enriched.Headers {
			if h.Key == "dlq-original-topic" {
				found = true
				assert.Equal(t, string(h.Value), "t", "original topic should match")
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
	assert.Equal(t, dlqCalled.Load(), 1, "DLQ writer should be called once")
}

func TestRecordExecutor_ExecuteRecord_ExhaustedDLQPreservesHeaders(t *testing.T) {
	exec := consumer.NewRecordExecutor(testlogger.NewLogger())

	dlqWriter := func(ctx context.Context, enriched *kgo.Record) error {
		hasOriginalHeader := false
		hasDLQHeader := false
		for _, h := range enriched.Headers {
			if h.Key == "x-custom" && string(h.Value) == "custom-value" {
				hasOriginalHeader = true
			}
			if h.Key == "dlq-original-topic" {
				hasDLQHeader = true
			}
		}
		assert.True(t, hasOriginalHeader, "should preserve original headers")
		assert.True(t, hasDLQHeader, "should add DLQ headers")
		return nil
	}

	sub := testSubRecord(
		func(ctx context.Context, record *kgo.Record) error { return errors.New("fail") },
		consumer.AckModeAtLeastOnce,
		fpDLQ("dlq"),
	)
	record := &kgo.Record{
		Topic: "t",
		Headers: []kgo.RecordHeader{
			{Key: "x-custom", Value: []byte("custom-value")},
		},
	}
	result := exec.ExecuteRecord(context.Background(), sub, record, dlqWriter)
	assert.True(t, result.Resolved, "should resolve when DLQ succeeds")
}

func TestRecordExecutor_ExecuteRecord_ExhaustedDLQWriterFails(t *testing.T) {
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

func TestRecordExecutor_ExecuteRecord_ExhaustedDLQRetriesThenSucceeds(t *testing.T) {
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
	assert.Equal(t, dlqCalls.Load(), 3, "DLQ writer should be called 3 times")
}

func TestRecordExecutor_ExecuteRecord_ExhaustedDLQRetriesExhausted(t *testing.T) {
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
	assert.Equal(t, dlqCalls.Load(), 3, "DLQ writer should be called MaxAttempts times")
}

func TestRecordExecutor_ExecuteRecord_ExhaustedDLQContextCancelledDuringPublish(t *testing.T) {
	exec := consumer.NewRecordExecutor(testlogger.NewLogger())
	ctx, cancel := context.WithCancel(context.Background())

	dlqWriter := func(ctx context.Context, enriched *kgo.Record) error {
		cancel()
		return errors.New("dlq write failed")
	}

	sub := testSubRecord(
		func(ctx context.Context, record *kgo.Record) error { return errors.New("fail") },
		consumer.AckModeAtLeastOnce,
		consumer.FailurePolicy{MaxAttempts: 3, RetryBackoff: time.Millisecond, DLQ: &consumer.DLQConfig{Topic: "dlq"}, OnExhausted: consumer.ExhaustedActionDLQThenCommit},
	)
	result := exec.ExecuteRecord(ctx, sub, &kgo.Record{Topic: "t"}, dlqWriter)
	assert.False(t, result.Resolved, "should not resolve when context cancelled during DLQ publish")
	assert.False(t, result.PauseTopic, "should not pause topic on context cancellation")
	assert.NotNil(t, result.Cause, "should return context error")
}

func TestRecordExecutor_ExecuteRecord_ExhaustedDLQContextCancelledAtLoopEntry(t *testing.T) {
	exec := consumer.NewRecordExecutor(testlogger.NewLogger())
	var handlerCalls atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())

	dlqWriter := func(ctx context.Context, enriched *kgo.Record) error {
		cancel()
		return errors.New("dlq write failed")
	}

	sub := testSubRecord(
		func(ctx context.Context, record *kgo.Record) error {
			handlerCalls.Add(1)
			return errors.New("fail")
		},
		consumer.AckModeAtLeastOnce,
		consumer.FailurePolicy{MaxAttempts: 3, RetryBackoff: 0, DLQ: &consumer.DLQConfig{Topic: "dlq"}, OnExhausted: consumer.ExhaustedActionDLQThenCommit},
	)
	result := exec.ExecuteRecord(ctx, sub, &kgo.Record{Topic: "t"}, dlqWriter)
	assert.False(t, result.Resolved, "should not resolve when context cancelled at DLQ loop entry")
	assert.False(t, result.PauseTopic, "should not pause on context cancellation")
	assert.NotNil(t, result.Cause, "should return context error")
	assert.Equal(t, handlerCalls.Load(), 3, "handler should be called MaxAttempts times")
}

func TestRecordExecutor_ExecuteRecord_UnsupportedExhaustedAction(t *testing.T) {
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

func TestRecordExecutor_ExecuteBatch_Success(t *testing.T) {
	exec := consumer.NewRecordExecutor(testlogger.NewLogger())
	sub := testSubBatch(
		func(ctx context.Context, records []*kgo.Record) consumer.BatchResult { return consumer.BatchResult{} },
		consumer.AckModeAtLeastOnce,
		fp1(consumer.ExhaustedActionStop),
	)

	records := []*kgo.Record{{Topic: "t", Offset: 0}, {Topic: "t", Offset: 1}}
	resolvedCount, cause, pauseTopic := exec.ExecuteBatch(context.Background(), sub, records, nil)
	assert.Equal(t, resolvedCount, 2, "all records should be resolved")
	assert.Nil(t, cause, "no error on success")
	assert.False(t, pauseTopic, "should not pause on success")
}

func TestRecordExecutor_ExecuteBatch_EmptyRecords(t *testing.T) {
	exec := consumer.NewRecordExecutor(nil)
	sub := testSubBatch(
		func(_ context.Context, _ []*kgo.Record) consumer.BatchResult { return consumer.BatchResult{} },
		consumer.AckModeAtLeastOnce,
		fp1(consumer.ExhaustedActionStop),
	)
	resolved, cause, pauseTopic := exec.ExecuteBatch(context.Background(), sub, nil, nil)
	assert.Equal(t, resolved, 0, "should resolve 0 records")
	assert.Nil(t, cause, "no error for empty batch")
	assert.False(t, pauseTopic, "should not pause for empty batch")
}

func TestRecordExecutor_ExecuteBatch_HandlerPanic(t *testing.T) {
	exec := consumer.NewRecordExecutor(testlogger.NewLogger())
	sub := testSubBatch(
		func(ctx context.Context, records []*kgo.Record) consumer.BatchResult { panic("batch panic") },
		consumer.AckModeAtLeastOnce,
		fp1(consumer.ExhaustedActionStop),
	)

	records := []*kgo.Record{{Topic: "t", Offset: 0}}
	resolvedCount, cause, pauseTopic := exec.ExecuteBatch(context.Background(), sub, records, nil)
	assert.Equal(t, resolvedCount, 0, "should not resolve on panic")
	assert.NotNil(t, cause, "should return error on panic")
	assert.True(t, pauseTopic, "should pause on stop exhaustion after panic")
}

func TestRecordExecutor_ExecuteBatch_ContextCancelled(t *testing.T) {
	exec := consumer.NewRecordExecutor(testlogger.NewLogger())
	sub := testSubBatch(
		func(_ context.Context, _ []*kgo.Record) consumer.BatchResult {
			return consumer.BatchResult{Err: errors.New("fail"), FailedAt: 0}
		},
		consumer.AckModeAtLeastOnce,
		fp3(),
	)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	records := []*kgo.Record{{Topic: "t", Offset: 0}}
	resolved, cause, pauseTopic := exec.ExecuteBatch(ctx, sub, records, nil)
	assert.Equal(t, resolved, 0, "should not resolve on cancelled context")
	assert.NotNil(t, cause, "should return error")
	assert.False(t, pauseTopic, "should not pause on context cancellation")
}

func TestRecordExecutor_ExecuteBatch_ContextCancelledDuringRetry(t *testing.T) {
	exec := consumer.NewRecordExecutor(testlogger.NewLogger())
	var called atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())

	sub := testSubBatch(
		func(ctx context.Context, records []*kgo.Record) consumer.BatchResult {
			if called.Add(1) == 1 {
				cancel()
			}
			return consumer.BatchResult{Err: errors.New("fail"), FailedAt: 0}
		},
		consumer.AckModeAtLeastOnce,
		fp3(),
	)
	records := []*kgo.Record{{Topic: "t", Offset: 0}}
	resolved, cause, pauseTopic := exec.ExecuteBatch(ctx, sub, records, nil)
	assert.Equal(t, resolved, 0, "should not resolve on cancelled context")
	assert.NotNil(t, cause, "should return error")
	assert.False(t, pauseTopic, "should not pause on context cancellation")
}

func TestRecordExecutor_ExecuteBatch_RetryThenSuccess(t *testing.T) {
	exec := consumer.NewRecordExecutor(testlogger.NewLogger())
	var calls atomic.Int32

	sub := testSubBatch(
		func(ctx context.Context, records []*kgo.Record) consumer.BatchResult {
			n := calls.Add(1)
			if n < 3 {
				return consumer.BatchResult{Err: errors.New("not yet"), FailedAt: 0}
			}
			return consumer.BatchResult{}
		},
		consumer.AckModeAtLeastOnce,
		fp3(),
	)
	records := []*kgo.Record{{Topic: "t", Offset: 0}}
	resolved, cause, pauseTopic := exec.ExecuteBatch(context.Background(), sub, records, nil)
	assert.Equal(t, resolved, 1, "should resolve after batch retries")
	assert.Nil(t, cause, "no error on success")
	assert.False(t, pauseTopic, "should not pause on success")
	assert.Equal(t, calls.Load(), 3, "handler should be called 3 times")
}

func TestRecordExecutor_ExecuteBatch_RetrySameRecord(t *testing.T) {
	exec := consumer.NewRecordExecutor(testlogger.NewLogger())
	var calls atomic.Int32

	sub := testSubBatch(
		func(ctx context.Context, records []*kgo.Record) consumer.BatchResult {
			calls.Add(1)
			return consumer.BatchResult{Err: errors.New("fail"), FailedAt: 0}
		},
		consumer.AckModeAtLeastOnce,
		fp3(),
	)
	records := []*kgo.Record{{Topic: "t", Offset: 0}}
	resolved, cause, pauseTopic := exec.ExecuteBatch(context.Background(), sub, records, nil)
	assert.Equal(t, resolved, 0, "should not resolve on exhausted retries")
	assert.NotNil(t, cause, "should return error")
	assert.True(t, pauseTopic, "should pause on stop exhaustion")
	assert.Equal(t, calls.Load(), 3, "handler should be called MaxAttempts times")
}

func TestRecordExecutor_ExecuteBatch_RejectsInvalidFailedIndex(t *testing.T) {
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
	assert.Equal(t, resolved, 0, "should resolve 0 with invalid FailedAt")
	assert.NotNil(t, cause, "should return error for negative FailedAt")
}

func TestRecordExecutor_ExecuteBatch_RejectsFailedIndexOutOfBounds(t *testing.T) {
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
	assert.Equal(t, resolved, 0, "should resolve 0 with out-of-bounds FailedAt")
	assert.NotNil(t, cause, "should return error for out-of-bounds FailedAt")
}

func TestRecordExecutor_ExecuteBatch_PartialFailure(t *testing.T) {
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
	assert.Equal(t, resolvedCount, 1, "first record should be resolved")
	assert.NotNil(t, cause, "should return error on batch failure")
	assert.True(t, pauseTopic, "should pause on stop exhaustion")
}

func TestRecordExecutor_ExecuteBatch_ExhaustedCommit(t *testing.T) {
	exec := consumer.NewRecordExecutor(testlogger.NewLogger())
	var handlerCalls atomic.Int32

	sub := testSubBatch(
		func(ctx context.Context, records []*kgo.Record) consumer.BatchResult {
			n := handlerCalls.Add(1)
			if n == 1 {
				return consumer.BatchResult{Err: errors.New("fail"), FailedAt: 0}
			}
			return consumer.BatchResult{}
		},
		consumer.AckModeAtLeastOnce,
		fp1(consumer.ExhaustedActionCommit),
	)
	records := []*kgo.Record{{Topic: "t", Offset: 0}, {Topic: "t", Offset: 1}}
	resolved, cause, pauseTopic := exec.ExecuteBatch(context.Background(), sub, records, nil)
	assert.Equal(t, resolved, 2, "both records should be resolved (first dropped via commit)")
	assert.Nil(t, cause, "no error after commit exhaustion drops record")
	assert.False(t, pauseTopic, "should not pause on commit exhaustion")
	assert.Equal(t, handlerCalls.Load(), 2, "handler should be called twice")
}

func TestRecordExecutor_ExecuteBatch_ExhaustedDLQ(t *testing.T) {
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
	assert.Equal(t, resolvedCount, 1, "record resolved via DLQ should count as resolved")
	assert.Nil(t, cause, "no error on DLQ commit")
	assert.False(t, pauseTopic, "should not pause on DLQ commit")
	assert.Equal(t, dlqCalled.Load(), 1, "DLQ writer should be called")
}

func TestRecordExecutor_ExecuteBatch_ExhaustedDLQThenContinue(t *testing.T) {
	exec := consumer.NewRecordExecutor(testlogger.NewLogger())
	var handlerCalls atomic.Int32
	var dlqCalled atomic.Int32

	dlqWriter := func(ctx context.Context, enriched *kgo.Record) error {
		dlqCalled.Add(1)
		return nil
	}

	sub := testSubBatch(
		func(ctx context.Context, records []*kgo.Record) consumer.BatchResult {
			n := handlerCalls.Add(1)
			if n == 1 {
				return consumer.BatchResult{Err: errors.New("fail"), FailedAt: 0}
			}
			return consumer.BatchResult{}
		},
		consumer.AckModeAtLeastOnce,
		fpDLQ("dlq"),
	)
	records := []*kgo.Record{{Topic: "t", Offset: 0}, {Topic: "t", Offset: 1}, {Topic: "t", Offset: 2}}
	resolved, cause, pauseTopic := exec.ExecuteBatch(context.Background(), sub, records, dlqWriter)
	assert.Equal(t, resolved, 3, "all records should be resolved (first via DLQ)")
	assert.Nil(t, cause, "no error after DLQ resolve")
	assert.False(t, pauseTopic, "should not pause on DLQ commit")
	assert.Equal(t, dlqCalled.Load(), 1, "DLQ writer should be called once")
	assert.Equal(t, handlerCalls.Load(), 2, "handler should be called twice (first fail, second success)")
}

func TestRecordExecutor_ExecuteBatch_UnsupportedExhaustedAction(t *testing.T) {
	exec := consumer.NewRecordExecutor(testlogger.NewLogger())
	sub := testSubBatch(
		func(_ context.Context, _ []*kgo.Record) consumer.BatchResult {
			return consumer.BatchResult{Err: errors.New("fail"), FailedAt: 0}
		},
		consumer.AckModeAtLeastOnce,
		fpUnsupported(),
	)
	records := []*kgo.Record{{Topic: "t", Offset: 0}}
	resolved, cause, pauseTopic := exec.ExecuteBatch(context.Background(), sub, records, nil)
	assert.Equal(t, resolved, 0, "should not resolve with unsupported action")
	assert.NotNil(t, cause, "should return error for unsupported exhausted action")
	assert.False(t, pauseTopic, "should not pause on unsupported action")
}
