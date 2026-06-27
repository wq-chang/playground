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
			"err", err,
		)

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

// ExecuteBatch runs the batch handler with retry and exhaustion resolution.
//
// The batch handler is retried up to MaxAttempts times with RetryBackoff delay
// between attempts. On each attempt the handler receives the remaining
// un-resolved records starting from the first unresolved position. Records
// that succeed before the first failure (FailedAt) are accumulated into
// resolvedCount.
//
// When retries are exhausted, the failure policy determines the outcome:
//   - Stop: returns resolvedCount with pauseTopic=true (does NOT commit past
//     the failed record).
//   - Commit: drops the failed record and continues with remaining records.
//   - DLQThenCommit: publishes to DLQ, then continues. If DLQ write fails,
//     returns the error without pausing (caller should fail).
func (e *RecordExecutor) ExecuteBatch(
	ctx context.Context,
	sub Subscription,
	records []*kgo.Record,
	dlqWriter func(ctx context.Context, enriched *kgo.Record) error,
) (int, error, bool) {
	if len(records) == 0 {
		return 0, nil, false
	}

	resolvedCount := 0
	attempts := 0
	lastFailedAt := -1

	for resolvedCount < len(records) {
		if err := ctx.Err(); err != nil {
			return resolvedCount, err, false
		}

		remaining := records[resolvedCount:]
		result, panicErr := invokeBatchHandler(ctx, sub.BatchHandler, remaining)
		if panicErr != nil {
			// Panic recovered — treat as batch failure at index 0.
			result = BatchResult{Err: panicErr, FailedAt: 0}
		}

		if result.Err == nil {
			return len(records), nil, false
		}

		// Clamp FailedAt into range.
		failedAt := result.FailedAt
		if failedAt < 0 || failedAt >= len(remaining) {
			return resolvedCount,
				fmt.Errorf("batch handler returned failed index %d for remaining batch size %d",
					failedAt, len(remaining)), false
		}

		resolvedCount += failedAt
		failedRecord := records[resolvedCount]

		if resolvedCount != lastFailedAt {
			attempts = 1
			lastFailedAt = resolvedCount
		} else {
			attempts++
		}

		e.logger.ErrorContext(
			ctx,
			"Kafka batch handler error",
			"topic", failedRecord.Topic,
			"partition", failedRecord.Partition,
			"offset", failedRecord.Offset,
			"failedAt", failedAt,
			"attempt", attempts,
			"maxAttempts", sub.FailurePolicy.MaxAttempts,
			"err", result.Err,
		)

		if attempts < sub.FailurePolicy.MaxAttempts {
			if err := waitForRetry(ctx, sub.FailurePolicy.RetryBackoff); err != nil {
				return resolvedCount, err, false
			}
			continue
		}

		exhausted := e.resolveExhausted(ctx, sub, failedRecord, result.Err, attempts, dlqWriter)
		if exhausted.PauseTopic {
			return resolvedCount, exhausted.Cause, true
		}
		if !exhausted.Resolved {
			return resolvedCount, exhausted.Cause, false
		}

		// Record resolved (skipped via Commit or DLQ) — advance past it.
		resolvedCount++
		attempts = 0
		lastFailedAt = -1
	}

	return resolvedCount, nil, false
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
