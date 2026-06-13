// services/go/library/kafka/internal/consumer/types_test.go
package consumer_test

import (
	"errors"
	"testing"
	"time"

	"go-services/library/kafka/internal/consumer"
	"go-services/library/assert"
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
	assert.Equal(t, consumer.ExhaustedAction(0), consumer.ExhaustedActionStop, "exhausted action 0 is Stop")
	assert.Equal(t, consumer.ExhaustedAction(1), consumer.ExhaustedActionCommit, "exhausted action 1 is Commit")
	assert.Equal(t, consumer.ExhaustedAction(2), consumer.ExhaustedActionDLQThenCommit, "exhausted action 2 is DLQThenCommit")
}

func TestPauseInfoFields(t *testing.T) {
	sentinel := errors.New("test error")
	now := time.Now()
	info := consumer.PauseInfo{Cause: sentinel, PausedAt: now}
	assert.ErrorIs(t, info.Cause, sentinel, "pause cause should be preserved")
	assert.True(t, info.PausedAt.Equal(now) || info.PausedAt.Before(now.Add(time.Microsecond)), "pause time should be set")
}

func TestSubscriptionFields(t *testing.T) {
	sub := consumer.Subscription{
		Topic:   "test-topic",
		AckMode: consumer.AckModeAtLeastOnce,
		FailurePolicy: consumer.FailurePolicy{
			MaxAttempts:  3,
			RetryBackoff: time.Second,
		},
	}
	assert.Equal(t, sub.Topic, "test-topic", "topic should match")
	assert.Equal(t, sub.AckMode, consumer.AckModeAtLeastOnce, "ack mode should match")
	assert.Equal(t, sub.FailurePolicy.MaxAttempts, 3, "max attempts should match")
}

func TestBatchResult(t *testing.T) {
	r := consumer.BatchResult{}
	assert.Nil(t, r.Err, "default batch result should have nil error")
	assert.Equal(t, r.FailedAt, 0, "default batch result should have FailedAt=0")

	sentinel := errors.New("test error")
	r2 := consumer.BatchResult{Err: sentinel, FailedAt: 2}
	assert.NotNil(t, r2.Err, "error should be set")
	assert.Equal(t, r2.FailedAt, 2, "FailedAt should match")
}
