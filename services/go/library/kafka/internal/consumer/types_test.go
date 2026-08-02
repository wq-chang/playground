package consumer_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go-services/library/assert"
	"go-services/library/kafka/internal/consumer"

	"github.com/twmb/franz-go/pkg/kgo"
)

func TestKeyEquality(t *testing.T) {
	a := consumer.Key{Topic: "t", Partition: 1}
	b := consumer.Key{Topic: "t", Partition: 1}
	c := consumer.Key{Topic: "t", Partition: 2}

	assert.Equal(t, a, b, "same topic+partition should be equal")
	assert.True(t, a != c, "different partition should differ")
}

func TestAckModeConstants(t *testing.T) {
	assert.Equal(t, consumer.AckMode(0), consumer.AckModeAtLeastOnce, "ack mode 0 is AtLeastOnce")
	assert.Equal(t, consumer.AckMode(1), consumer.AckModeAtMostOnce, "ack mode 1 is AtMostOnce")
}

func TestExhaustedActionConstants(t *testing.T) {
	assert.Equal(t, consumer.ExhaustedAction(0), consumer.ExhaustedActionUnspecified, "0 is Unspecified")
	assert.Equal(t, consumer.ExhaustedAction(1), consumer.ExhaustedActionStop, "1 is Stop")
	assert.Equal(t, consumer.ExhaustedAction(2), consumer.ExhaustedActionCommit, "2 is Commit")
	assert.Equal(t, consumer.ExhaustedAction(3), consumer.ExhaustedActionDLQThenCommit, "3 is DLQThenCommit")
}

func TestNormalizeFailurePolicy_Defaults(t *testing.T) {
	policy, err := consumer.NormalizeFailurePolicy(consumer.FailurePolicy{
		MaxAttempts:  0,
		RetryBackoff: 0,
		DLQ:          nil,
		OnExhausted:  consumer.ExhaustedActionUnspecified,
	})
	assert.NoError(t, err, "should normalize without error")

	assert.Equal(t, policy.MaxAttempts, 1, "default attempts should be 1")
	assert.Equal(t, policy.OnExhausted, consumer.ExhaustedActionStop, "default without DLQ should be Stop")
	assert.Zero(t, policy.RetryBackoff, "default backoff should be 0")
	assert.Nil(t, policy.DLQ, "default DLQ should be nil")
}

func TestNormalizeFailurePolicy_DLQDefaultsToDLQThenCommit(t *testing.T) {
	policy, err := consumer.NormalizeFailurePolicy(consumer.FailurePolicy{
		MaxAttempts:  0,
		RetryBackoff: 0,
		DLQ:          &consumer.DLQConfig{Topic: "dead-letter"},
		OnExhausted:  consumer.ExhaustedActionUnspecified,
	})
	assert.NoError(t, err, "should normalize without error")

	assert.Equal(t, policy.MaxAttempts, 1, "default attempts should be 1")
	assert.Equal(
		t,
		policy.OnExhausted,
		consumer.ExhaustedActionDLQThenCommit,
		"with DLQ should default to DLQThenCommit",
	)
}

func TestNormalizeFailurePolicy_RejectsInvalidValues(t *testing.T) {
	_, err := consumer.NormalizeFailurePolicy(consumer.FailurePolicy{
		MaxAttempts:  -1,
		RetryBackoff: 0,
		DLQ:          nil,
		OnExhausted:  consumer.ExhaustedActionUnspecified,
	})
	assert.ErrorContains(t, err, "max attempts must not be negative", "negative attempts rejected")

	_, err = consumer.NormalizeFailurePolicy(consumer.FailurePolicy{
		MaxAttempts:  0,
		RetryBackoff: -time.Second,
		DLQ:          nil,
		OnExhausted:  consumer.ExhaustedActionUnspecified,
	})
	assert.ErrorContains(t, err, "retry backoff must not be negative", "negative backoff rejected")

	_, err = consumer.NormalizeFailurePolicy(consumer.FailurePolicy{
		MaxAttempts:  0,
		RetryBackoff: 0,
		DLQ:          &consumer.DLQConfig{Topic: ""},
		OnExhausted:  consumer.ExhaustedActionDLQThenCommit,
	})
	assert.ErrorContains(t, err, "dlq topic must not be empty", "empty dlq topic rejected")
}

func TestSubscriptionNormalize_RejectsInvalidAckMode(t *testing.T) {
	_, err := consumer.Subscription{
		Topic:         "topic-a",
		Handler:       func(_ context.Context, _ *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: consumer.FailurePolicy{},
		AckMode:       consumer.AckMode(99),
	}.Normalize()

	assert.ErrorContains(t, err, "unsupported ack mode", "invalid ack mode rejected")
}

func TestSubscriptionNormalize_EmptyTopic(t *testing.T) {
	_, err := consumer.Subscription{
		Topic:         "",
		Handler:       func(_ context.Context, _ *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: consumer.FailurePolicy{},
		AckMode:       consumer.AckModeAtLeastOnce,
	}.Normalize()
	assert.ErrorContains(t, err, "topic must not be empty", "empty topic rejected")
}

func TestSubscriptionNormalize_NoHandler(t *testing.T) {
	_, err := consumer.Subscription{
		Topic:         "topic",
		Handler:       nil,
		BatchHandler:  nil,
		FailurePolicy: consumer.FailurePolicy{},
		AckMode:       consumer.AckModeAtLeastOnce,
	}.Normalize()
	assert.ErrorContains(t, err, "exactly one of handler or batch handler", "no handler rejected")
}

func TestSubscriptionNormalize_BothHandlers(t *testing.T) {
	_, err := consumer.Subscription{
		Topic:         "topic",
		Handler:       func(_ context.Context, _ *kgo.Record) error { return nil },
		BatchHandler:  func(_ context.Context, _ []*kgo.Record) consumer.BatchResult { return consumer.BatchResult{} },
		FailurePolicy: consumer.FailurePolicy{},
		AckMode:       consumer.AckModeAtLeastOnce,
	}.Normalize()
	assert.ErrorContains(t, err, "exactly one of handler or batch handler", "both handlers rejected")
}

func TestSubscriptionFields(t *testing.T) {
	sub := consumer.Subscription{
		Topic:        "test-topic",
		Handler:      func(_ context.Context, _ *kgo.Record) error { return nil },
		BatchHandler: nil,
		FailurePolicy: consumer.FailurePolicy{
			MaxAttempts:  3,
			RetryBackoff: time.Second,
			DLQ:          nil,
			OnExhausted:  consumer.ExhaustedActionStop,
		},
		AckMode: consumer.AckModeAtLeastOnce,
	}
	assert.Equal(t, sub.Topic, "test-topic", "topic should match")
	assert.NotNil(t, sub.Handler, "handler should be set")
	assert.Nil(t, sub.BatchHandler, "batch handler should be nil")
	assert.Equal(t, sub.AckMode, consumer.AckModeAtLeastOnce, "ack mode should match")
	assert.Equal(t, sub.FailurePolicy.MaxAttempts, 3, "max attempts should match")
}

func TestBatchResult(t *testing.T) {
	r := consumer.BatchResult{}
	assert.Nil(t, r.Err, "default batch result should have nil error")
	assert.Equal(t, r.FailedAt, 0, "default FailedAt should be 0")

	sentinel := errors.New("test error")
	r2 := consumer.BatchResult{Err: sentinel, FailedAt: 2}
	assert.NotNil(t, r2.Err, "error should be set")
	assert.Equal(t, r2.FailedAt, 2, "FailedAt should match")
}

func TestNormalizeFailurePolicy_DLQThenCommitWithoutDLQ(t *testing.T) {
	_, err := consumer.NormalizeFailurePolicy(consumer.FailurePolicy{
		MaxAttempts:  0,
		RetryBackoff: 0,
		DLQ:          nil,
		OnExhausted:  consumer.ExhaustedActionDLQThenCommit,
	})
	assert.ErrorContains(t, err, "dlq config is required for dlq exhausted action", "dlq action without dlq config rejected")
}

func TestNormalizeFailurePolicy_DLQThenCommitWithDLQ(t *testing.T) {
	policy, err := consumer.NormalizeFailurePolicy(consumer.FailurePolicy{
		MaxAttempts:  0,
		RetryBackoff: time.Second,
		DLQ:          &consumer.DLQConfig{Topic: "dead-letter"},
		OnExhausted:  consumer.ExhaustedActionDLQThenCommit,
	})
	assert.NoError(t, err, "dlq action with dlq config should succeed")
	assert.Equal(t, policy.MaxAttempts, 1, "default attempts should be 1")
	assert.Equal(t, policy.OnExhausted, consumer.ExhaustedActionDLQThenCommit, "action should be preserved")
	assert.Equal(t, policy.RetryBackoff, time.Second, "backoff should be preserved")
	assert.NotNil(t, policy.DLQ, "dlq should be preserved")
	assert.Equal(t, policy.DLQ.Topic, "dead-letter", "dlq topic should be preserved")
}

func TestNormalizeFailurePolicy_RejectsUnsupportedAction(t *testing.T) {
	_, err := consumer.NormalizeFailurePolicy(consumer.FailurePolicy{
		MaxAttempts:  0,
		RetryBackoff: 0,
		DLQ:          nil,
		OnExhausted:  consumer.ExhaustedAction(99),
	})
	assert.ErrorContains(t, err, "unsupported exhausted action", "unsupported action rejected")
}

func TestSubscriptionNormalize_ValidHandler(t *testing.T) {
	handler := func(_ context.Context, _ *kgo.Record) error { return nil }
	sub, err := consumer.Subscription{
		Topic:         "topic-a",
		Handler:       handler,
		BatchHandler:  nil,
		FailurePolicy: consumer.FailurePolicy{},
		AckMode:       consumer.AckModeAtLeastOnce,
	}.Normalize()

	assert.NoError(t, err, "valid subscription should normalize")
	assert.Equal(t, sub.Topic, "topic-a", "topic should be preserved")
	assert.NotNil(t, sub.Handler, "handler should be preserved")
	assert.Nil(t, sub.BatchHandler, "batch handler should stay nil")
	assert.Equal(t, sub.AckMode, consumer.AckModeAtLeastOnce, "ack mode should be preserved")
	assert.Equal(t, sub.FailurePolicy.MaxAttempts, 1, "failure policy defaults should be filled")
	assert.Equal(
		t,
		sub.FailurePolicy.OnExhausted,
		consumer.ExhaustedActionStop,
		"failure policy default action should be Stop",
	)
}

func TestSubscriptionNormalize_ValidBatchHandler(t *testing.T) {
	batchHandler := func(_ context.Context, _ []*kgo.Record) consumer.BatchResult { return consumer.BatchResult{} }
	sub, err := consumer.Subscription{
		Topic:         "topic-b",
		Handler:       nil,
		BatchHandler:  batchHandler,
		FailurePolicy: consumer.FailurePolicy{},
		AckMode:       consumer.AckModeAtLeastOnce,
	}.Normalize()

	assert.NoError(t, err, "valid batch subscription should normalize")
	assert.Equal(t, sub.Topic, "topic-b", "topic should be preserved")
	assert.Nil(t, sub.Handler, "handler should stay nil")
	assert.NotNil(t, sub.BatchHandler, "batch handler should be preserved")
	assert.Equal(t, sub.FailurePolicy.MaxAttempts, 1, "failure policy defaults should be filled")
}

func TestSubscriptionNormalize_AtMostOnceAckMode(t *testing.T) {
	sub, err := consumer.Subscription{
		Topic:         "topic-c",
		Handler:       func(_ context.Context, _ *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: consumer.FailurePolicy{},
		AckMode:       consumer.AckModeAtMostOnce,
	}.Normalize()

	assert.NoError(t, err, "at-most-once ack mode should be accepted")
	assert.Equal(t, sub.AckMode, consumer.AckModeAtMostOnce, "ack mode should be preserved")
}

func TestSubscriptionNormalize_PropagatesPolicyError(t *testing.T) {
	_, err := consumer.Subscription{
		Topic:        "topic-d",
		Handler:      func(_ context.Context, _ *kgo.Record) error { return nil },
		BatchHandler: nil,
		FailurePolicy: consumer.FailurePolicy{
			MaxAttempts:  -1,
			RetryBackoff: 0,
			DLQ:          nil,
			OnExhausted:  consumer.ExhaustedActionUnspecified,
		},
		AckMode: consumer.AckModeAtLeastOnce,
	}.Normalize()

	assert.ErrorContains(t, err, "max attempts must not be negative", "policy error should propagate")
}
