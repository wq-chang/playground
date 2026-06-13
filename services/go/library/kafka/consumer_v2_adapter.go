// services/go/library/kafka/consumer_v2_adapter.go
//
// Temporary adapter functions that convert public kafka types to internal
// consumer types. These are tested through the consumerV2 façade methods.
package kafka

import (
	"context"
	"fmt"

	"go-services/library/kafka/internal/consumer"

	"github.com/twmb/franz-go/pkg/kgo"
)

// errV2NotImplemented is returned by stub runtime methods on consumerV2.
var errV2NotImplemented = fmt.Errorf("consumerV2: not yet implemented")

// toInternalSubscription converts a public Subscription to the internal runtime form.
func toInternalSubscription(s Subscription) (consumer.Subscription, error) {
	normalized, err := s.normalize()
	if err != nil {
		return consumer.Subscription{}, err
	}

	intSub := consumer.Subscription{
		Topic:         normalized.Topic,
		Handler:       nil,
		BatchHandler:  nil,
		FailurePolicy: toInternalFailurePolicy(normalized.FailurePolicy),
		AckMode:       toInternalAckMode(normalized.AckMode),
	}

	if normalized.Handler != nil {
		// Same signature: func(ctx, *kgo.Record) error — assignable directly.
		intSub.Handler = normalized.Handler
	}
	if normalized.BatchHandler != nil {
		// Different return type (kafka.BatchResult vs consumer.BatchResult).
		intSub.BatchHandler = func(ctx context.Context, records []*kgo.Record) consumer.BatchResult {
			result := normalized.BatchHandler(ctx, records)
			return toInternalBatchResult(result)
		}
	}

	return intSub, nil
}

// toInternalAckMode converts a public AckMode to the internal representation.
func toInternalAckMode(mode AckMode) consumer.AckMode {
	switch mode {
	case AckModeAtLeastOnce:
		return consumer.AckModeAtLeastOnce
	case AckModeAtMostOnce:
		return consumer.AckModeAtMostOnce
	default:
		return consumer.AckModeAtLeastOnce
	}
}

// toInternalFailurePolicy converts a public FailurePolicy to the internal form.
func toInternalFailurePolicy(p FailurePolicy) consumer.FailurePolicy {
	intPolicy := consumer.FailurePolicy{
		MaxAttempts:  p.MaxAttempts,
		RetryBackoff: p.RetryBackoff,
		OnExhausted:  toInternalExhaustedAction(p.OnExhausted),
	}
	if p.DLQ != nil {
		intPolicy.DLQ = &consumer.DLQConfig{Topic: p.DLQ.Topic}
	}
	return intPolicy
}

// toInternalExhaustedAction converts a public ExhaustedAction to the internal form.
func toInternalExhaustedAction(a ExhaustedAction) consumer.ExhaustedAction {
	switch a {
	case ExhaustedActionStop:
		return consumer.ExhaustedActionStop
	case ExhaustedActionCommit:
		return consumer.ExhaustedActionCommit
	case ExhaustedActionDLQThenCommit:
		return consumer.ExhaustedActionDLQThenCommit
	default:
		return consumer.ExhaustedActionStop
	}
}

// toInternalBatchResult converts a public BatchResult to the internal form.
func toInternalBatchResult(r BatchResult) consumer.BatchResult {
	return consumer.BatchResult{
		Err:      r.Err,
		FailedAt: r.FailedAt,
	}
}
