package kafka

import (
	"go-services/library/kafka/ktype"
)

// Subscription configures how a topic is consumed.
type Subscription = ktype.Subscription

// FailurePolicy configures how handler errors are retried and resolved.
type FailurePolicy = ktype.FailurePolicy

// DLQConfig configures where failed records are published after retries are exhausted.
type DLQConfig = ktype.DLQConfig

// ExhaustedAction determines what happens after retries are exhausted.
type ExhaustedAction = ktype.ExhaustedAction

const (
	// ExhaustedActionUnspecified applies the package default:
	// pause the topic when no DLQ is configured, or publish to DLQ then commit when a DLQ exists.
	ExhaustedActionUnspecified = ktype.ExhaustedActionUnspecified
	// ExhaustedActionStop pauses the topic until the process restarts and leaves the record uncommitted.
	ExhaustedActionStop = ktype.ExhaustedActionStop
	// ExhaustedActionCommit drops the record and commits past it.
	ExhaustedActionCommit = ktype.ExhaustedActionCommit
	// ExhaustedActionDLQThenCommit publishes the record to DLQ and commits only if that succeeds.
	ExhaustedActionDLQThenCommit = ktype.ExhaustedActionDLQThenCommit
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
