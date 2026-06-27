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
	router := consumer.NewRouter(nil)
	pauses := consumer.NewPauseRegistry(time.Now)
	registry := consumer.NewPartitionRegistry(testlogger.NewLogger())
	d := consumer.NewDispatcher(router, pauses, registry, nil)
	assert.NotNil(t, d, "NewDispatcher should not return nil")
}

func TestDispatcher_NotifyCapacity_WaitForCapacity(t *testing.T) {
	router := consumer.NewRouter(nil)
	pauses := consumer.NewPauseRegistry(nil)
	registry := consumer.NewPartitionRegistry(testlogger.NewLogger())
	d := consumer.NewDispatcher(router, pauses, registry, nil)

	done := make(chan struct{})
	go func() {
		err := d.WaitForCapacity(context.Background())
		assert.NoError(t, err, "WaitForCapacity should return nil when signaled")
		close(done)
	}()

	d.NotifyCapacity()
	<-done
}

func TestDispatcher_SkipsPausedTopics(t *testing.T) {
	router := consumer.NewRouter(nil)
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
	consumer.NewDispatcher(router, pauses, registry, nil)

	_, ok := registry.Get(consumer.Key{Topic: "t", Partition: 0})
	assert.False(t, ok, "no partition should be created for paused topic")
}
