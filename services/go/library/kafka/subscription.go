package kafka

import "go-services/library/kafka/internal/consumer"

// Subscription configures how a topic is consumed.
type Subscription = consumer.Subscription

// FailurePolicy configures how handler errors are retried and resolved.
type FailurePolicy = consumer.FailurePolicy

// DLQConfig configures where failed records are published after retries are exhausted.
type DLQConfig = consumer.DLQConfig

// ExhaustedAction determines what happens after retries are exhausted.
type ExhaustedAction = consumer.ExhaustedAction

// Handler processes a single Kafka record.
type Handler = consumer.Handler

// BatchHandler processes a topic-partition batch in the polled order.
type BatchHandler = consumer.BatchHandler

// BatchResult reports the outcome of a BatchHandler invocation.
type BatchResult = consumer.BatchResult

const (
	// ExhaustedActionUnspecified applies the package default:
	// pause the topic when no DLQ is configured, or publish to DLQ then commit when a DLQ exists.
	ExhaustedActionUnspecified = consumer.ExhaustedActionUnspecified
	// ExhaustedActionStop pauses the topic until the process restarts and leaves the record uncommitted.
	ExhaustedActionStop = consumer.ExhaustedActionStop
	// ExhaustedActionCommit drops the record and commits past it.
	ExhaustedActionCommit = consumer.ExhaustedActionCommit
	// ExhaustedActionDLQThenCommit publishes the record to DLQ and commits only if that succeeds.
	ExhaustedActionDLQThenCommit = consumer.ExhaustedActionDLQThenCommit
)

func newDefaultSubscription(topic string, handler Handler, ackMode AckMode) Subscription {
	return Subscription{
		Topic:         topic,
		Handler:       handler,
		BatchHandler:  nil,
		FailurePolicy: FailurePolicy{},
		AckMode:       ackMode,
	}
}

func newDefaultBatchSubscription(topic string, handler BatchHandler, ackMode AckMode) Subscription {
	return Subscription{
		Topic:         topic,
		Handler:       nil,
		BatchHandler:  handler,
		FailurePolicy: FailurePolicy{},
		AckMode:       ackMode,
	}
}
