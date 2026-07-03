// Package consumer holds internal runtime types and collaborators for the
// kafka package's consumer implementation. It must not import its parent
// go-services/library/kafka package.
package consumer

import (
	"context"
	"fmt"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

// Key identifies a Kafka topic-partition in internal operations.
type Key struct {
	Topic     string
	Partition int32
}

// AckMode determines how the consumer acknowledges records.
type AckMode int

const (
	// AckModeAtLeastOnce ensures records are processed at least once.
	// Records are committed after the handler finishes successfully.
	// If the handler fails, the record might be processed again.
	AckModeAtLeastOnce AckMode = iota

	// AckModeAtMostOnce ensures records are processed at most once.
	// Records are committed before the handler is called.
	// If the handler fails, the record might be lost.
	AckModeAtMostOnce
)

// Handler processes a single Kafka record.
//
// Returning nil marks the record as successfully handled. Returning an error
// activates the subscription's failure policy.
type Handler func(ctx context.Context, record *kgo.Record) error

// BatchHandler processes a topic-partition batch in the polled order.
//
// Returning a zero-value BatchResult marks the whole batch as successfully
// handled. To report a failure after successfully handling a contiguous prefix,
// set Err and FailedAt to the index of the first failed record in the input
// slice.
type BatchHandler func(ctx context.Context, records []*kgo.Record) BatchResult

// BatchResult reports the outcome of a BatchHandler invocation.
//
// The zero value means the whole input batch succeeded. FailedAt is only used
// when Err is non-nil, and must point at the first failed record in the batch.
type BatchResult struct {
	Err      error
	FailedAt int
}

// ExhaustedAction determines what happens after handler retries are exhausted.
type ExhaustedAction int

const (
	// ExhaustedActionUnspecified applies the package default:
	// pause the topic when no DLQ is configured, or publish to DLQ then
	// commit when a DLQ exists.
	ExhaustedActionUnspecified ExhaustedAction = iota

	// ExhaustedActionStop pauses the topic until the process restarts and
	// leaves the record uncommitted.
	ExhaustedActionStop

	// ExhaustedActionCommit drops the record and commits past it.
	ExhaustedActionCommit

	// ExhaustedActionDLQThenCommit publishes the record to DLQ and commits
	// only if that succeeds.
	ExhaustedActionDLQThenCommit
)

// DLQConfig configures where failed records are published after retries are
// exhausted.
type DLQConfig struct {
	// Topic is the dead-letter topic that receives exhausted records.
	Topic string
}

// FailurePolicy configures how handler errors are retried and resolved.
type FailurePolicy struct {
	// DLQ configures dead-letter publishing after retries are exhausted.
	DLQ *DLQConfig

	// RetryBackoff waits between retry attempts. Zero retries immediately.
	RetryBackoff time.Duration

	// MaxAttempts is the total number of handler attempts, including the
	// first execution. Zero defaults to 1.
	MaxAttempts int

	// OnExhausted determines what happens after MaxAttempts is reached.
	OnExhausted ExhaustedAction
}

// PauseInfo records why and when a topic was paused.
type PauseInfo struct {
	Cause    error
	PausedAt time.Time
}

// Subscription configures how a topic is consumed.
type Subscription struct {
	// Topic is the Kafka topic name consumed by this subscription.
	Topic string

	// Handler processes each record delivered for Topic in single-record
	// mode.
	Handler Handler

	// BatchHandler processes each dequeued topic-partition batch for Topic.
	BatchHandler BatchHandler

	// FailurePolicy controls retry and exhaustion behavior for handler
	// errors.
	FailurePolicy FailurePolicy

	// AckMode controls whether commits happen before or after successful
	// handler execution.
	AckMode AckMode
}

// Normalize validates and fills defaults for the subscription.
// It returns an error if the subscription is invalid.
func (s Subscription) Normalize() (Subscription, error) {
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

	policy, err := NormalizeFailurePolicy(s.FailurePolicy)
	if err != nil {
		return Subscription{}, err
	}
	s.FailurePolicy = policy
	return s, nil
}

// NormalizeFailurePolicy validates a FailurePolicy and fills defaults.
func NormalizeFailurePolicy(policy FailurePolicy) (FailurePolicy, error) {
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
