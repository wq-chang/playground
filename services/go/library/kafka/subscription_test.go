package kafka

import (
	"context"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"go-services/library/assert"
	"go-services/library/require"
)

func TestNormalizeFailurePolicy_Defaults(t *testing.T) {
	policy, err := normalizeFailurePolicy(FailurePolicy{
		MaxAttempts:  0,
		RetryBackoff: 0,
		DLQ:          nil,
		OnExhausted:  ExhaustedActionUnspecified,
	})
	require.NoError(t, err, "default policy should normalize")

	assert.Equal(t, policy.MaxAttempts, 1, "default attempts")
	assert.Equal(t, policy.OnExhausted, ExhaustedActionStop, "default exhausted action")
	assert.Zero(t, policy.RetryBackoff, "default backoff")
	assert.Nil(t, policy.DLQ, "default dlq")
}

func TestNormalizeFailurePolicy_DLQDefaultsToDLQThenCommit(t *testing.T) {
	policy, err := normalizeFailurePolicy(FailurePolicy{
		MaxAttempts:  0,
		RetryBackoff: 0,
		DLQ:          &DLQConfig{Topic: "dead-letter"},
		OnExhausted:  ExhaustedActionUnspecified,
	})
	require.NoError(t, err, "dlq policy should normalize")

	assert.Equal(t, policy.MaxAttempts, 1, "default attempts")
	assert.Equal(t, policy.OnExhausted, ExhaustedActionDLQThenCommit, "default exhausted action")
}

func TestNormalizeFailurePolicy_RejectsInvalidValues(t *testing.T) {
	_, err := normalizeFailurePolicy(FailurePolicy{
		MaxAttempts:  -1,
		RetryBackoff: 0,
		DLQ:          nil,
		OnExhausted:  ExhaustedActionUnspecified,
	})
	assert.ErrorContains(t, err, "max attempts must not be negative", "negative attempts")

	_, err = normalizeFailurePolicy(FailurePolicy{
		MaxAttempts:  0,
		RetryBackoff: -time.Second,
		DLQ:          nil,
		OnExhausted:  ExhaustedActionUnspecified,
	})
	assert.ErrorContains(t, err, "retry backoff must not be negative", "negative backoff")

	_, err = normalizeFailurePolicy(FailurePolicy{
		MaxAttempts:  0,
		RetryBackoff: 0,
		DLQ:          &DLQConfig{Topic: ""},
		OnExhausted:  ExhaustedActionDLQThenCommit,
	})
	assert.ErrorContains(t, err, "dlq topic must not be empty", "empty dlq topic")
}

func TestSubscriptionNormalize_RejectsInvalidAckMode(t *testing.T) {
	_, err := (Subscription{
		Topic:         "topic-a",
		Handler:       func(context.Context, *kgo.Record) error { return nil },
		AckMode:       AckMode(99),
		FailurePolicy: FailurePolicy{},
	}).normalize()

	assert.ErrorContains(t, err, "unsupported ack mode", "invalid ack mode")
}
