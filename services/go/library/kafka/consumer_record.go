package kafka

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

func (c *Consumer) executeRecord(ctx context.Context, subscription Subscription, record *kgo.Record) (recordResult, error) {
	attempts := subscription.FailurePolicy.MaxAttempts
	var lastErr error

	for attempt := 1; attempt <= attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return recordResult{}, err
		}

		err := invokeHandler(ctx, subscription.Handler, record)
		if err == nil {
			return recordResult{
				cause:      nil,
				resolved:   true,
				pauseTopic: false,
			}, nil
		}

		lastErr = err
		c.log.ErrorContext(
			ctx,
			"Kafka handler error",
			"topic", record.Topic,
			"partition", record.Partition,
			"offset", record.Offset,
			"attempt", attempt,
			"maxAttempts", attempts,
			"err", err)

		if attempt < attempts {
			if err := waitForRetry(ctx, subscription.FailurePolicy.RetryBackoff); err != nil {
				return recordResult{}, err
			}
			continue
		}
	}

	switch subscription.FailurePolicy.OnExhausted {
	case ExhaustedActionStop:
		return recordResult{
			cause: fmt.Errorf(
				"handler failed for topic %q partition %d offset %d after %d attempts: %w",
				record.Topic,
				record.Partition,
				record.Offset,
				attempts,
				lastErr,
			),
			resolved:   false,
			pauseTopic: true,
		}, nil
	case ExhaustedActionCommit:
		c.log.WarnContext(
			ctx,
			"Kafka record dropped after retry exhaustion",
			"topic", record.Topic,
			"partition", record.Partition,
			"offset", record.Offset,
			"attempts", attempts)
		return recordResult{
			cause:      nil,
			resolved:   true,
			pauseTopic: false,
		}, nil
	case ExhaustedActionDLQThenCommit:
		if err := c.publishToDLQ(ctx, subscription, record, lastErr, attempts); err != nil {
			return recordResult{}, fmt.Errorf(
				"failed to publish topic %q partition %d offset %d to dlq after %d attempts: %w",
				record.Topic,
				record.Partition,
				record.Offset,
				attempts,
				errors.Join(lastErr, err),
			)
		}
		c.log.WarnContext(
			ctx,
			"Kafka record sent to DLQ after retry exhaustion",
			"topic", record.Topic,
			"partition", record.Partition,
			"offset", record.Offset,
			"attempts", attempts,
			"dlqTopic", subscription.FailurePolicy.DLQ.Topic)
		return recordResult{
			cause:      nil,
			resolved:   true,
			pauseTopic: false,
		}, nil
	default:
		return recordResult{}, fmt.Errorf("unsupported exhausted action: %d", subscription.FailurePolicy.OnExhausted)
	}
}

func invokeHandler(ctx context.Context, handler Handler, record *kgo.Record) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("handler panicked: %v\n%s", recovered, debug.Stack())
		}
	}()

	return handler(ctx, record)
}

func (c *Consumer) publishToDLQ(
	ctx context.Context,
	subscription Subscription,
	record *kgo.Record,
	handlerErr error,
	attempts int,
) error {
	if c.client == nil || c.client.Producer == nil {
		return fmt.Errorf("dlq publish requires a producer-enabled kafka client")
	}

	dlqRecord := &kgo.Record{
		Topic:   subscription.FailurePolicy.DLQ.Topic,
		Key:     append([]byte(nil), record.Key...),
		Value:   append([]byte(nil), record.Value...),
		Headers: dlqHeaders(record, handlerErr, attempts),
	}

	return c.client.Producer.ProduceSync(ctx, dlqRecord)
}

func dlqHeaders(record *kgo.Record, handlerErr error, attempts int) []kgo.RecordHeader {
	headers := make([]kgo.RecordHeader, 0, len(record.Headers)+5)
	for _, header := range record.Headers {
		headers = append(headers, kgo.RecordHeader{
			Key:   header.Key,
			Value: append([]byte(nil), header.Value...),
		})
	}

	headers = append(headers,
		kgo.RecordHeader{Key: "dlq-original-topic", Value: []byte(record.Topic)},
		kgo.RecordHeader{Key: "dlq-original-partition", Value: []byte(strconv.FormatInt(int64(record.Partition), 10))},
		kgo.RecordHeader{Key: "dlq-original-offset", Value: []byte(strconv.FormatInt(record.Offset, 10))},
		kgo.RecordHeader{Key: "dlq-attempts", Value: []byte(strconv.Itoa(attempts))},
		kgo.RecordHeader{Key: "dlq-error", Value: []byte(handlerErr.Error())},
	)

	return headers
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
