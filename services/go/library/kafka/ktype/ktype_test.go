// services/go/library/kafka/ktype/ktype_test.go
package ktype_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go-services/library/assert"
	"go-services/library/kafka/ktype"

	"github.com/twmb/franz-go/pkg/kgo"
)

func TestKeyEquality(t *testing.T) {
	a := ktype.Key{Topic: "t", Partition: 1}
	b := ktype.Key{Topic: "t", Partition: 1}
	c := ktype.Key{Topic: "t", Partition: 2}

	assert.Equal(t, a, b, "same topic+partition should be equal")
	assert.True(t, a != c, "different partition should differ")
}

func TestAckModeConstants(t *testing.T) {
	assert.Equal(t, ktype.AckMode(0), ktype.AckModeAtLeastOnce, "ack mode 0 is AtLeastOnce")
	assert.Equal(t, ktype.AckMode(1), ktype.AckModeAtMostOnce, "ack mode 1 is AtMostOnce")
}

func TestExhaustedActionConstants(t *testing.T) {
	assert.Equal(t, ktype.ExhaustedAction(0), ktype.ExhaustedActionUnspecified, "0 is Unspecified")
	assert.Equal(t, ktype.ExhaustedAction(1), ktype.ExhaustedActionStop, "1 is Stop")
	assert.Equal(t, ktype.ExhaustedAction(2), ktype.ExhaustedActionCommit, "2 is Commit")
	assert.Equal(t, ktype.ExhaustedAction(3), ktype.ExhaustedActionDLQThenCommit, "3 is DLQThenCommit")
}

func TestPauseInfoFields(t *testing.T) {
	sentinel := errors.New("test error")
	now := time.Now()
	info := ktype.PauseInfo{Cause: sentinel, PausedAt: now}
	assert.ErrorIs(t, info.Cause, sentinel, "pause cause should be preserved")
	assert.True(t, info.PausedAt.Equal(now), "pause time should be set")
}

func TestNormalizeFailurePolicy_Defaults(t *testing.T) {
	policy, err := ktype.NormalizeFailurePolicy(ktype.FailurePolicy{
		MaxAttempts:  0,
		RetryBackoff: 0,
		DLQ:          nil,
		OnExhausted:  ktype.ExhaustedActionUnspecified,
	})
	assert.NoError(t, err, "should normalize without error")

	assert.Equal(t, policy.MaxAttempts, 1, "default attempts should be 1")
	assert.Equal(t, policy.OnExhausted, ktype.ExhaustedActionStop, "default without DLQ should be Stop")
	assert.Zero(t, policy.RetryBackoff, "default backoff should be 0")
	assert.Nil(t, policy.DLQ, "default DLQ should be nil")
}

func TestNormalizeFailurePolicy_DLQDefaultsToDLQThenCommit(t *testing.T) {
	policy, err := ktype.NormalizeFailurePolicy(ktype.FailurePolicy{
		MaxAttempts:  0,
		RetryBackoff: 0,
		DLQ:          &ktype.DLQConfig{Topic: "dead-letter"},
		OnExhausted:  ktype.ExhaustedActionUnspecified,
	})
	assert.NoError(t, err, "should normalize without error")

	assert.Equal(t, policy.MaxAttempts, 1, "default attempts should be 1")
	assert.Equal(t, policy.OnExhausted, ktype.ExhaustedActionDLQThenCommit, "with DLQ should default to DLQThenCommit")
}

func TestNormalizeFailurePolicy_RejectsInvalidValues(t *testing.T) {
	_, err := ktype.NormalizeFailurePolicy(ktype.FailurePolicy{
		MaxAttempts:  -1,
		RetryBackoff: 0,
		DLQ:          nil,
		OnExhausted:  ktype.ExhaustedActionUnspecified,
	})
	assert.ErrorContains(t, err, "max attempts must not be negative", "negative attempts rejected")

	_, err = ktype.NormalizeFailurePolicy(ktype.FailurePolicy{
		MaxAttempts:  0,
		RetryBackoff: -time.Second,
		DLQ:          nil,
		OnExhausted:  ktype.ExhaustedActionUnspecified,
	})
	assert.ErrorContains(t, err, "retry backoff must not be negative", "negative backoff rejected")

	_, err = ktype.NormalizeFailurePolicy(ktype.FailurePolicy{
		MaxAttempts:  0,
		RetryBackoff: 0,
		DLQ:          &ktype.DLQConfig{Topic: ""},
		OnExhausted:  ktype.ExhaustedActionDLQThenCommit,
	})
	assert.ErrorContains(t, err, "dlq topic must not be empty", "empty dlq topic rejected")
}

func TestSubscriptionNormalize_RejectsInvalidAckMode(t *testing.T) {
	_, err := ktype.Subscription{
		Topic:         "topic-a",
		Handler:       func(_ context.Context, _ *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: ktype.FailurePolicy{},
		AckMode:       ktype.AckMode(99),
	}.Normalize()

	assert.ErrorContains(t, err, "unsupported ack mode", "invalid ack mode rejected")
}

func TestSubscriptionNormalize_EmptyTopic(t *testing.T) {
	_, err := ktype.Subscription{
		Topic:         "",
		Handler:       func(_ context.Context, _ *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: ktype.FailurePolicy{},
		AckMode:       ktype.AckModeAtLeastOnce,
	}.Normalize()
	assert.ErrorContains(t, err, "topic must not be empty", "empty topic rejected")
}

func TestSubscriptionNormalize_NoHandler(t *testing.T) {
	_, err := ktype.Subscription{
		Topic:         "topic",
		Handler:       nil,
		BatchHandler:  nil,
		FailurePolicy: ktype.FailurePolicy{},
		AckMode:       ktype.AckModeAtLeastOnce,
	}.Normalize()
	assert.ErrorContains(t, err, "exactly one of handler or batch handler", "no handler rejected")
}

func TestSubscriptionNormalize_BothHandlers(t *testing.T) {
	_, err := ktype.Subscription{
		Topic:         "topic",
		Handler:       func(_ context.Context, _ *kgo.Record) error { return nil },
		BatchHandler:  func(_ context.Context, _ []*kgo.Record) ktype.BatchResult { return ktype.BatchResult{} },
		FailurePolicy: ktype.FailurePolicy{},
		AckMode:       ktype.AckModeAtLeastOnce,
	}.Normalize()
	assert.ErrorContains(t, err, "exactly one of handler or batch handler", "both handlers rejected")
}

func TestSubscriptionFields(t *testing.T) {
	sub := ktype.Subscription{
		Topic:        "test-topic",
		Handler:      func(_ context.Context, _ *kgo.Record) error { return nil },
		BatchHandler: nil,
		FailurePolicy: ktype.FailurePolicy{
			MaxAttempts:  3,
			RetryBackoff: time.Second,
			DLQ:          nil,
			OnExhausted:  ktype.ExhaustedActionStop,
		},
		AckMode: ktype.AckModeAtLeastOnce,
	}
	assert.Equal(t, sub.Topic, "test-topic", "topic should match")
	assert.NotNil(t, sub.Handler, "handler should be set")
	assert.Nil(t, sub.BatchHandler, "batch handler should be nil")
	assert.Equal(t, sub.AckMode, ktype.AckModeAtLeastOnce, "ack mode should match")
	assert.Equal(t, sub.FailurePolicy.MaxAttempts, 3, "max attempts should match")
}

func TestBatchResult(t *testing.T) {
	r := ktype.BatchResult{}
	assert.Nil(t, r.Err, "default batch result should have nil error")
	assert.Equal(t, r.FailedAt, 0, "default FailedAt should be 0")

	sentinel := errors.New("test error")
	r2 := ktype.BatchResult{Err: sentinel, FailedAt: 2}
	assert.NotNil(t, r2.Err, "error should be set")
	assert.Equal(t, r2.FailedAt, 2, "FailedAt should match")
}
