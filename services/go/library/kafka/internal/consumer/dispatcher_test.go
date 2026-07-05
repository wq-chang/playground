// services/go/library/kafka/internal/consumer/dispatcher_test.go
package consumer_test

import (
	"context"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"go-services/library/assert"
	"go-services/library/kafka/internal/consumer"
	"go-services/library/testlogger"
)

func TestDispatcher_New(t *testing.T) {
	router := consumer.NewRouter(&stubRegisterClient{})
	pauses := consumer.NewPauseRegistry(time.Now)
	registry := consumer.NewPartitionRegistry(testlogger.NewLogger())
	d := consumer.NewDispatcher(router, pauses, registry, nil, nil, make(chan struct{}, 1))
	assert.NotNil(t, d, "NewDispatcher should not return nil")
}

func TestDispatcher_SkipsPausedTopics(t *testing.T) {
	router := consumer.NewRouter(&stubRegisterClient{})
	pauses := consumer.NewPauseRegistry(time.Now)
	registry := consumer.NewPartitionRegistry(testlogger.NewLogger())

	sub := consumer.Subscription{
		Topic:         "t",
		Handler:       func(ctx context.Context, record *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: consumer.FailurePolicy{MaxAttempts: 1, RetryBackoff: 0, DLQ: nil, OnExhausted: consumer.ExhaustedActionStop},
		AckMode:       consumer.AckModeAtLeastOnce,
	}
	if err := router.Register(sub); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Pause the topic.
	pauses.Pause("t", nil)

	// Create dispatcher and verify no state is created for paused topic.
	consumer.NewDispatcher(router, pauses, registry, nil, nil, make(chan struct{}, 1))

	_, ok := registry.Get(consumer.Key{Topic: "t", Partition: 0})
	assert.False(t, ok, "no partition should be created for paused topic")
}
