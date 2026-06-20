// services/go/library/kafka/internal/consumer/record_executor.go
package consumer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

// RecordResult reports the outcome of a single-record handler execution.
type RecordResult struct {
	Cause      error
	Resolved   bool
	PauseTopic bool
}

// RecordExecutor handles single-record and batch handler execution with retry
// logic, backoff, and exhaustion resolution (DLQ, Stop, Commit).
type RecordExecutor struct {
	logger *slog.Logger
}

// NewRecordExecutor creates a record executor with the given logger.
func NewRecordExecutor(logger *slog.Logger) *RecordExecutor {
	if logger == nil {
		logger = slog.Default()
	}
	return &RecordExecutor{logger: logger}
}

// ExecuteRecord runs the handler with retry and exhaustion resolution.
func (e *RecordExecutor) ExecuteRecord(
	ctx context.Context,
	sub Subscription,
	record *kgo.Record,
	dlqWriter func(ctx context.Context, enriched *kgo.Record) error,
) RecordResult {
	attempts := sub.FailurePolicy.MaxAttempts
	var lastErr error

	for attempt := 1; attempt <= attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return RecordResult{Cause: err, Resolved: false, PauseTopic: false}
		}

		err := invokeHandler(ctx, sub.Handler, record)
		if err == nil {
			return RecordResult{Cause: nil, Resolved: true, PauseTopic: false}
		}

		lastErr = err
		e.logger.ErrorContext(
			ctx,
			"Kafka handler error",
			"topic", record.Topic,
			"partition", record.Partition,
			"offset", record.Offset,
			"attempt", attempt,
			"maxAttempts", attempts,
			"err", err)

		if attempt < attempts {
			if err := waitForRetry(ctx, sub.FailurePolicy.RetryBackoff); err != nil {
				return RecordResult{Cause: err, Resolved: false, PauseTopic: false}
			}
		}
	}

	return e.resolveExhausted(ctx, sub, record, lastErr, attempts, dlqWriter)
}

func (e *RecordExecutor) resolveExhausted(
	ctx context.Context,
	sub Subscription,
	record *kgo.Record,
	lastErr error,
	attempts int,
	dlqWriter func(ctx context.Context, enriched *kgo.Record) error,
) RecordResult {
	switch sub.FailurePolicy.OnExhausted {
	case ExhaustedActionStop:
		return RecordResult{
			Cause: fmt.Errorf(
				"handler failed for topic %q partition %d offset %d after %d attempts: %w",
				record.Topic,
				record.Partition,
				record.Offset,
				attempts,
				lastErr,
			),
			Resolved:   false,
			PauseTopic: true,
		}
	case ExhaustedActionCommit:
		e.logger.WarnContext(
			ctx,
			"Kafka record dropped after retry exhaustion",
			"topic",
			record.Topic,
			"partition",
			record.Partition,
			"offset",
			record.Offset,
			"attempts",
			attempts,
		)
		return RecordResult{Cause: nil, Resolved: true, PauseTopic: false}
	case ExhaustedActionDLQThenCommit:
		enriched := enrichDLQRecord(record, lastErr, attempts, sub.FailurePolicy.DLQ.Topic)
		if err := dlqWriter(ctx, enriched); err != nil {
			return RecordResult{
				Cause:      fmt.Errorf("failed to publish topic %q to dlq: %w", record.Topic, errors.Join(lastErr, err)),
				Resolved:   false,
				PauseTopic: false,
			}
		}
		e.logger.WarnContext(ctx, "Kafka record sent to DLQ after retry exhaustion",
			"topic", record.Topic, "partition", record.Partition, "offset", record.Offset,
			"attempts", attempts, "dlqTopic", sub.FailurePolicy.DLQ.Topic)
		return RecordResult{Cause: nil, Resolved: true, PauseTopic: false}
	default:
		return RecordResult{
			Cause:      fmt.Errorf("unsupported exhausted action: %d", sub.FailurePolicy.OnExhausted),
			Resolved:   false,
			PauseTopic: false,
		}
	}
}

// ExecuteBatch runs the batch handler with exhaustion resolution.
func (e *RecordExecutor) ExecuteBatch(
	ctx context.Context,
	sub Subscription,
	records []*kgo.Record,
	dlqWriter func(ctx context.Context, enriched *kgo.Record) error,
) (int, error, bool) {
	if len(records) == 0 {
		return 0, nil, false
	}

	result, err := invokeBatchHandler(ctx, sub.BatchHandler, records)
	if err != nil {
		// Panic recovered — treat as batch failure at index 0 so
		// the configured OnExhausted action (Stop/Commit/DLQ) is applied.
		result = BatchResult{Err: err, FailedAt: 0}
	}

	if result.Err == nil {
		return len(records), nil, false
	}

	failedAt := result.FailedAt
	if failedAt < 0 || failedAt >= len(records) {
		failedAt = 0
	}

	failedRecord := records[failedAt]

	switch sub.FailurePolicy.OnExhausted {
	case ExhaustedActionStop:
		return failedAt, fmt.Errorf("batch handler failed for topic %q at index %d: %w",
			failedRecord.Topic, failedAt, result.Err), true
	case ExhaustedActionCommit:
		e.logger.WarnContext(ctx, "Kafka batch partially dropped after handler error",
			"topic", failedRecord.Topic, "partition", failedRecord.Partition,
			"failedAt", failedAt, "err", result.Err)
		return failedAt, nil, false
	case ExhaustedActionDLQThenCommit:
		enriched := enrichDLQRecord(failedRecord, result.Err, 1, sub.FailurePolicy.DLQ.Topic)
		if err := dlqWriter(ctx, enriched); err != nil {
			return failedAt, fmt.Errorf("failed to publish batch record to dlq: %w", err), false
		}
		e.logger.WarnContext(ctx, "Kafka batch record sent to DLQ after handler error",
			"topic", failedRecord.Topic, "partition", failedRecord.Partition,
			"failedAt", failedAt, "dlqTopic", sub.FailurePolicy.DLQ.Topic)
		return failedAt, nil, false
	default:
		return 0, fmt.Errorf("unsupported exhausted action: %d", sub.FailurePolicy.OnExhausted), false
	}
}

func invokeHandler(
	ctx context.Context,
	handler func(context.Context, *kgo.Record) error,
	record *kgo.Record,
) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("handler panicked: %v\n%s", recovered, debug.Stack())
		}
	}()
	return handler(ctx, record)
}

func invokeBatchHandler(
	ctx context.Context,
	handler func(context.Context, []*kgo.Record) BatchResult,
	records []*kgo.Record,
) (result BatchResult, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("batch handler panicked: %v\n%s", recovered, debug.Stack())
		}
	}()
	return handler(ctx, records), nil
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func enrichDLQRecord(original *kgo.Record, handlerErr error, attempts int, dlqTopic string) *kgo.Record {
	headers := make([]kgo.RecordHeader, 0, len(original.Headers)+5)
	for _, h := range original.Headers {
		headers = append(headers, kgo.RecordHeader{
			Key:   h.Key,
			Value: append([]byte(nil), h.Value...),
		})
	}
	headers = append(
		headers,
		kgo.RecordHeader{Key: "dlq-original-topic", Value: []byte(original.Topic)},
		kgo.RecordHeader{Key: "dlq-original-partition", Value: []byte(strconv.FormatInt(int64(original.Partition), 10))},
		kgo.RecordHeader{Key: "dlq-original-offset", Value: []byte(strconv.FormatInt(original.Offset, 10))},
		kgo.RecordHeader{Key: "dlq-attempts", Value: []byte(strconv.Itoa(attempts))},
		kgo.RecordHeader{Key: "dlq-error", Value: []byte(handlerErr.Error())},
	)
	return &kgo.Record{
		Topic:   dlqTopic,
		Key:     append([]byte(nil), original.Key...),
		Value:   append([]byte(nil), original.Value...),
		Headers: headers,
	}
}
