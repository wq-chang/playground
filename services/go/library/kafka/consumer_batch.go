package kafka

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"

	"github.com/twmb/franz-go/pkg/kgo"
)

type batchExecutionResult struct {
	cause         error
	resolvedCount int
	pauseTopic    bool
}

func (c *Consumer) executeBatch(
	ctx context.Context,
	subscription Subscription,
	records []*kgo.Record,
) (batchExecutionResult, error) {
	return newBatchExecutor(c.log, c.publishToDLQ).execute(ctx, subscription, records)
}

type batchExecutor struct {
	recordExecutor recordExecutor
}

func newBatchExecutor(
	log *slog.Logger,
	publishToDLQ func(context.Context, Subscription, *kgo.Record, error, int) error,
) batchExecutor {
	return batchExecutor{
		recordExecutor: newRecordExecutor(log, publishToDLQ),
	}
}

func (e batchExecutor) execute(
	ctx context.Context,
	subscription Subscription,
	records []*kgo.Record,
) (batchExecutionResult, error) {
	if len(records) == 0 {
		return batchExecutionResult{}, nil
	}
	if subscription.BatchHandler == nil {
		return batchExecutionResult{}, fmt.Errorf("batch handler must not be nil")
	}

	resolvedCount := 0
	attempts := 0
	lastFailedAt := -1

	for resolvedCount < len(records) {
		if err := ctx.Err(); err != nil {
			return batchExecutionResult{}, err
		}

		remaining := records[resolvedCount:]
		result := invokeBatchHandler(ctx, subscription.BatchHandler, remaining)
		successCount, handlerErr, err := normalizeBatchResult(result, len(remaining))
		if err != nil {
			return batchExecutionResult{}, err
		}
		if handlerErr == nil {
			return batchExecutionResult{
				cause:         nil,
				resolvedCount: len(records),
				pauseTopic:    false,
			}, nil
		}

		resolvedCount += successCount
		failedRecord := records[resolvedCount]
		if resolvedCount != lastFailedAt {
			attempts = 1
			lastFailedAt = resolvedCount
		} else {
			attempts++
		}

		e.recordExecutor.log.ErrorContext(
			ctx,
			"Kafka batch handler error",
			"topic", failedRecord.Topic,
			"partition", failedRecord.Partition,
			"offset", failedRecord.Offset,
			"failedAt", result.FailedAt,
			"attempt", attempts,
			"maxAttempts", subscription.FailurePolicy.MaxAttempts,
			"err", handlerErr,
		)

		if attempts < subscription.FailurePolicy.MaxAttempts {
			if retryErr := waitForRetry(ctx, subscription.FailurePolicy.RetryBackoff); retryErr != nil {
				return batchExecutionResult{}, retryErr
			}
			continue
		}

		exhaustedResult, err := e.recordExecutor.resolveExhausted(
			ctx,
			subscription,
			failedRecord,
			handlerErr,
			attempts,
		)
		if err != nil {
			return batchExecutionResult{}, err
		}
		if exhaustedResult.pauseTopic {
			return batchExecutionResult{
				cause:         exhaustedResult.cause,
				resolvedCount: resolvedCount,
				pauseTopic:    true,
			}, nil
		}
		if !exhaustedResult.resolved {
			panic("unreachable: resolveExhausted returned resolved=false without pauseTopic=true")
		}

		resolvedCount++
		attempts = 0
		lastFailedAt = -1
	}

	return batchExecutionResult{
		cause:         nil,
		resolvedCount: len(records),
		pauseTopic:    false,
	}, nil
}

func normalizeBatchResult(result BatchResult, batchSize int) (int, error, error) {
	if batchSize <= 0 {
		return 0, nil, nil
	}
	if result.Err == nil {
		if result.FailedAt != 0 {
			return 0, nil, fmt.Errorf("batch handler returned FailedAt=%d without an error", result.FailedAt)
		}
		return batchSize, nil, nil
	}
	if result.FailedAt < 0 || result.FailedAt >= batchSize {
		return 0, nil, fmt.Errorf(
			"batch handler returned failed index %d for batch size %d",
			result.FailedAt,
			batchSize,
		)
	}
	return result.FailedAt, result.Err, nil
}

func invokeBatchHandler(
	ctx context.Context,
	handler BatchHandler,
	records []*kgo.Record,
) (result BatchResult) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = BatchResult{
				FailedAt: 0,
				Err:      fmt.Errorf("batch handler panicked: %v\n%s", recovered, debug.Stack()),
			}
		}
	}()

	return handler(ctx, records)
}
