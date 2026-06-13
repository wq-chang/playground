// Package consumer holds internal runtime types and collaborators for the
// kafka package's consumer implementation. It must not import its parent
// go-services/library/kafka package to avoid an import cycle.
package consumer

import (
	"context"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

// Key identifies a Kafka topic-partition in internal operations.
type Key struct {
	Topic     string
	Partition int32
}

// AckMode controls when offsets are committed relative to handler execution.
type AckMode int

const (
	AckModeAtLeastOnce AckMode = iota
	AckModeAtMostOnce
)

// ExhaustedAction determines what happens after handler retries are exhausted.
type ExhaustedAction int

const (
	ExhaustedActionStop ExhaustedAction = iota
	ExhaustedActionCommit
	ExhaustedActionDLQThenCommit
)

// DLQConfig configures where failed records are published after retry exhaustion.
type DLQConfig struct {
	Topic string
}

// FailurePolicy controls retry and exhaustion behavior for handler errors.
type FailurePolicy struct {
	DLQ          *DLQConfig
	RetryBackoff time.Duration
	MaxAttempts  int
	OnExhausted  ExhaustedAction
}

// PauseInfo records why and when a topic was paused.
type PauseInfo struct {
	Cause    error
	PausedAt time.Time
}

// BatchResult reports the outcome of a batch handler invocation.
type BatchResult struct {
	Err      error
	FailedAt int
}

// Subscription is the normalized runtime form of a topic subscription.
// It uses the same handler signatures as the public kafka package, but
// BatchResult is our own type (not kafka.BatchResult) to avoid import cycle.
type Subscription struct {
	Topic         string
	Handler       func(ctx context.Context, record *kgo.Record) error
	BatchHandler  func(ctx context.Context, records []*kgo.Record) BatchResult
	FailurePolicy FailurePolicy
	AckMode       AckMode
}
