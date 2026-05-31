package kafka

import (
	"fmt"
	"time"
)

// Subscription configures how a topic is consumed.
type Subscription struct {
	// Topic is the Kafka topic name consumed by this subscription.
	Topic string
	// Handler processes each record delivered for Topic in single-record mode.
	Handler Handler
	// BatchHandler processes each dequeued topic-partition batch for Topic.
	BatchHandler BatchHandler
	// FailurePolicy controls retry and exhaustion behavior for handler errors.
	FailurePolicy FailurePolicy
	// AckMode controls whether commits happen before or after successful
	// handler execution.
	AckMode AckMode
}

// FailurePolicy configures how handler errors are retried and resolved.
type FailurePolicy struct {
	// DLQ configures dead-letter publishing after retries are exhausted.
	DLQ *DLQConfig
	// RetryBackoff waits between retry attempts. Zero retries immediately.
	RetryBackoff time.Duration
	// MaxAttempts is the total number of handler attempts, including the first
	// execution. Zero defaults to 1.
	MaxAttempts int
	// OnExhausted determines what happens after MaxAttempts is reached.
	OnExhausted ExhaustedAction
}

// DLQConfig configures where failed records are published after retries are
// exhausted.
type DLQConfig struct {
	// Topic is the dead-letter topic that receives exhausted records.
	Topic string
}

// ExhaustedAction determines what happens after retries are exhausted.
type ExhaustedAction int

const (
	// ExhaustedActionUnspecified applies the package default:
	// pause the topic when no DLQ is configured, or publish to DLQ then commit when a DLQ exists.
	ExhaustedActionUnspecified ExhaustedAction = iota
	// ExhaustedActionStop pauses the topic until the process restarts and leaves the record uncommitted.
	ExhaustedActionStop
	// ExhaustedActionCommit drops the record and commits past it.
	ExhaustedActionCommit
	// ExhaustedActionDLQThenCommit publishes the record to DLQ and commits only if that succeeds.
	ExhaustedActionDLQThenCommit
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

func (s Subscription) normalize() (Subscription, error) {
	if s.Topic == "" {
		return Subscription{}, fmt.Errorf("topic must not be empty")
	}
	switch {
	case s.Handler == nil && s.BatchHandler == nil:
		return Subscription{}, fmt.Errorf("exactly one of handler or batch handler must be set")
	case s.Handler != nil && s.BatchHandler != nil:
		return Subscription{}, fmt.Errorf("exactly one of handler or batch handler must be set")
	}
	if s.AckMode != AckModeAtLeastOnce && s.AckMode != AckModeAtMostOnce {
		return Subscription{}, fmt.Errorf("unsupported ack mode: %d", s.AckMode)
	}

	policy, err := normalizeFailurePolicy(s.FailurePolicy)
	if err != nil {
		return Subscription{}, err
	}
	s.FailurePolicy = policy
	return s, nil
}

func normalizeFailurePolicy(policy FailurePolicy) (FailurePolicy, error) {
	if policy.MaxAttempts < 0 {
		return FailurePolicy{}, fmt.Errorf("max attempts must not be negative")
	}
	if policy.MaxAttempts == 0 {
		policy.MaxAttempts = 1
	}
	if policy.RetryBackoff < 0 {
		return FailurePolicy{}, fmt.Errorf("retry backoff must not be negative")
	}
	if policy.DLQ != nil && policy.DLQ.Topic == "" {
		return FailurePolicy{}, fmt.Errorf("dlq topic must not be empty")
	}
	if policy.OnExhausted == ExhaustedActionUnspecified {
		if policy.DLQ != nil {
			policy.OnExhausted = ExhaustedActionDLQThenCommit
		} else {
			policy.OnExhausted = ExhaustedActionStop
		}
	}

	switch policy.OnExhausted {
	case ExhaustedActionStop, ExhaustedActionCommit:
		return policy, nil
	case ExhaustedActionDLQThenCommit:
		if policy.DLQ == nil {
			return FailurePolicy{}, fmt.Errorf("dlq config is required for dlq exhausted action")
		}
		return policy, nil
	default:
		return FailurePolicy{}, fmt.Errorf("unsupported exhausted action: %d", policy.OnExhausted)
	}
}
