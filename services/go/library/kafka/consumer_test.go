// Same-package tests for the Consumer.
package kafka

import (
	"context"
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"

	"go-services/library/assert"
	"go-services/library/require"
)

// newTestKgoClient creates a standalone kgo.Client for unit testing.
func newTestKgoClient(t *testing.T) *kgo.Client {
	t.Helper()

	kgoClient, err := kgo.NewClient(
		kgo.SeedBrokers("localhost:9092"),
		kgo.ConsumerGroup("test-group"),
	)
	require.NoError(t, err, "failed to create test kgo client")
	t.Cleanup(kgoClient.Close)

	return kgoClient
}

func TestConsumer_New_ValidConfig(t *testing.T) {
	kgoClient := newTestKgoClient(t)
	cfg := newConfig([]string{"localhost:9092"}, "test-group")

	c, err := newConsumer(cfg, kgoClient, nil)
	require.NoError(t, err, "newConsumer should succeed")
	require.NotNil(t, c, "Consumer should not be nil")
}

func TestConsumer_New_WithStartupSubscriptions(t *testing.T) {
	kgoClient := newTestKgoClient(t)
	cfg := newConfig([]string{"localhost:9092"}, "test-group")
	cfg.subscriptions["topic-a"] = newDefaultSubscription(
		"topic-a",
		func(context.Context, *kgo.Record) error { return nil },
		AckModeAtLeastOnce,
	)
	cfg.subscriptions["topic-b"] = newDefaultBatchSubscription(
		"topic-b",
		func(context.Context, []*kgo.Record) BatchResult { return BatchResult{} },
		AckModeAtMostOnce,
	)

	c, err := newConsumer(cfg, kgoClient, nil)
	require.NoError(t, err, "newConsumer should succeed with startup subscriptions")

	snap := c.router.Snapshot()
	assert.Equal(t, len(snap), 2, "should have 2 subscriptions")
}

func TestConsumer_AddSubscription(t *testing.T) {
	kgoClient := newTestKgoClient(t)
	cfg := newConfig([]string{"localhost:9092"}, "test-group")
	c, err := newConsumer(cfg, kgoClient, nil)
	require.NoError(t, err, "newConsumer should succeed")

	sub := Subscription{
		Topic:         "my-topic",
		Handler:       func(context.Context, *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: FailurePolicy{},
		AckMode:       AckModeAtLeastOnce,
	}

	err = c.AddSubscription(sub)
	require.NoError(t, err, "AddSubscription should succeed")

	snap := c.router.Snapshot()
	assert.Equal(t, len(snap), 1, "should have 1 subscription")

	got, ok := c.router.Lookup("my-topic")
	assert.True(t, ok, "topic should be found")
	assert.Equal(t, got.Topic, "my-topic", "topic should match")
}

func TestConsumer_AddSubscription_Duplicate(t *testing.T) {
	kgoClient := newTestKgoClient(t)
	cfg := newConfig([]string{"localhost:9092"}, "test-group")
	c, err := newConsumer(cfg, kgoClient, nil)
	require.NoError(t, err, "newConsumer should succeed")

	sub := Subscription{
		Topic:         "my-topic",
		Handler:       func(context.Context, *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: FailurePolicy{},
		AckMode:       AckModeAtLeastOnce,
	}

	err = c.AddSubscription(sub)
	require.NoError(t, err, "first AddSubscription should succeed")

	err = c.AddSubscription(sub)
	assert.ErrorContains(t, err, "topic handler already registered", "duplicate should error")
}

func TestConsumer_AddSubscription_Invalid(t *testing.T) {
	kgoClient := newTestKgoClient(t)
	cfg := newConfig([]string{"localhost:9092"}, "test-group")
	c, err := newConsumer(cfg, kgoClient, nil)
	require.NoError(t, err, "newConsumer should succeed")

	sub := Subscription{
		Topic:         "",
		Handler:       func(context.Context, *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: FailurePolicy{},
		AckMode:       AckModeAtLeastOnce,
	}
	err = c.AddSubscription(sub)
	assert.ErrorContains(t, err, "topic must not be empty", "empty topic should error")
}

func TestConsumer_AddTopic(t *testing.T) {
	kgoClient := newTestKgoClient(t)
	cfg := newConfig([]string{"localhost:9092"}, "test-group")
	c, err := newConsumer(cfg, kgoClient, nil)
	require.NoError(t, err, "newConsumer should succeed")

	handler := func(context.Context, *kgo.Record) error { return nil }
	err = c.AddTopic("topic-a", handler)
	require.NoError(t, err, "AddTopic should succeed")

	got, ok := c.router.Lookup("topic-a")
	assert.True(t, ok, "topic should be found")
	assert.Equal(t, got.Topic, "topic-a", "topic should match")
	assert.NotNil(t, got.Handler, "handler should be set")
}

func TestConsumer_AddBatchTopic(t *testing.T) {
	kgoClient := newTestKgoClient(t)
	cfg := newConfig([]string{"localhost:9092"}, "test-group")
	c, err := newConsumer(cfg, kgoClient, nil)
	require.NoError(t, err, "newConsumer should succeed")

	handler := func(context.Context, []*kgo.Record) BatchResult { return BatchResult{} }
	err = c.AddBatchTopic("topic-b", handler)
	require.NoError(t, err, "AddBatchTopic should succeed")

	got, ok := c.router.Lookup("topic-b")
	assert.True(t, ok, "topic should be found")
	assert.Equal(t, got.Topic, "topic-b", "topic should match")
	assert.NotNil(t, got.BatchHandler, "batch handler should be set")
}

func TestConsumer_Run_CancelsOnContext(t *testing.T) {
	kgoClient := newTestKgoClient(t)
	cfg := newConfig([]string{"localhost:9092"}, "test-group")
	c, err := newConsumer(cfg, kgoClient, nil)
	require.NoError(t, err, "newConsumer should succeed")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	err = c.Run(ctx)
	assert.ErrorIs(t, err, context.Canceled, "Run should return context.Canceled")
}

func TestConsumer_Run_RejectsConcurrentRun(t *testing.T) {
	kgoClient := newTestKgoClient(t)
	cfg := newConfig([]string{"localhost:9092"}, "test-group")
	c, err := newConsumer(cfg, kgoClient, nil)
	require.NoError(t, err, "newConsumer should succeed")

	// Start the first run's lifecycle so runState is active, without
	// calling Run() itself (which would block on dispatch).
	_, beginErr := c.runState.Begin()
	require.NoError(t, beginErr, "first Begin should succeed")

	ch := make(chan struct{})
	go func() {
		<-c.runState.Context().Done()
		close(ch)
	}()

	// Second Run should be rejected immediately.
	ctx2 := context.Background()
	err = c.Run(ctx2)
	assert.ErrorContains(t, err, "run is already active", "concurrent Run should error")

	c.runState.Stop()
	<-ch
}

func TestConsumer_NormalizesOnRegister(t *testing.T) {
	kgoClient := newTestKgoClient(t)
	cfg := newConfig([]string{"localhost:9092"}, "test-group")
	c, err := newConsumer(cfg, kgoClient, nil)
	require.NoError(t, err, "newConsumer should succeed")

	err = c.AddSubscription(Subscription{
		Topic:         "t",
		Handler:       func(context.Context, *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: FailurePolicy{},
		AckMode:       AckModeAtLeastOnce,
	})
	require.NoError(t, err, "AddSubscription should succeed")

	got, ok := c.router.Lookup("t")
	require.True(t, ok, "topic should be found")
	assert.Equal(t, got.FailurePolicy.MaxAttempts, 1, "should normalize max attempts")
}
