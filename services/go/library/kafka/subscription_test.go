package kafka

import (
	"context"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"go-services/library/assert"
	"go-services/library/kafka/ktype"
	"go-services/library/require"
)

func TestNormalizeFailurePolicy_Defaults(t *testing.T) {
	policy, err := ktype.NormalizeFailurePolicy(ktype.FailurePolicy{
		MaxAttempts:  0,
		RetryBackoff: 0,
		DLQ:          nil,
		OnExhausted:  ktype.ExhaustedActionUnspecified,
	})
	require.NoError(t, err, "default policy should normalize")

	assert.Equal(t, policy.MaxAttempts, 1, "default attempts")
	assert.Equal(t, policy.OnExhausted, ktype.ExhaustedActionStop, "default exhausted action")
	assert.Zero(t, policy.RetryBackoff, "default backoff")
	assert.Nil(t, policy.DLQ, "default dlq")
}

func TestNormalizeFailurePolicy_DLQDefaultsToDLQThenCommit(t *testing.T) {
	policy, err := ktype.NormalizeFailurePolicy(ktype.FailurePolicy{
		MaxAttempts:  0,
		RetryBackoff: 0,
		DLQ:          &ktype.DLQConfig{Topic: "dead-letter"},
		OnExhausted:  ktype.ExhaustedActionUnspecified,
	})
	require.NoError(t, err, "dlq policy should normalize")

	assert.Equal(t, policy.MaxAttempts, 1, "default attempts")
	assert.Equal(t, policy.OnExhausted, ktype.ExhaustedActionDLQThenCommit, "default exhausted action")
}

func TestNormalizeFailurePolicy_RejectsInvalidValues(t *testing.T) {
	_, err := ktype.NormalizeFailurePolicy(ktype.FailurePolicy{
		MaxAttempts:  -1,
		RetryBackoff: 0,
		DLQ:          nil,
		OnExhausted:  ktype.ExhaustedActionUnspecified,
	})
	assert.ErrorContains(t, err, "max attempts must not be negative", "negative attempts")

	_, err = ktype.NormalizeFailurePolicy(ktype.FailurePolicy{
		MaxAttempts:  0,
		RetryBackoff: -time.Second,
		DLQ:          nil,
		OnExhausted:  ktype.ExhaustedActionUnspecified,
	})
	assert.ErrorContains(t, err, "retry backoff must not be negative", "negative backoff")

	_, err = ktype.NormalizeFailurePolicy(ktype.FailurePolicy{
		MaxAttempts:  0,
		RetryBackoff: 0,
		DLQ:          &ktype.DLQConfig{Topic: ""},
		OnExhausted:  ktype.ExhaustedActionDLQThenCommit,
	})
	assert.ErrorContains(t, err, "dlq topic must not be empty", "empty dlq topic")
}

func TestSubscriptionNormalize_RejectsInvalidAckMode(t *testing.T) {
	_, err := (Subscription{
		Topic:         "topic-a",
		Handler:       func(context.Context, *kgo.Record) error { return nil },
		BatchHandler:  nil,
		AckMode:       AckMode(99),
		FailurePolicy: FailurePolicy{},
	}).Normalize()

	assert.ErrorContains(t, err, "unsupported ack mode", "invalid ack mode")
}
