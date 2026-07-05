// services/go/library/kafka/consumer_v2_test.go
// TEMPORARY: Same-package tests for the v2 façade. Will be deleted in Step 10
// when v2 becomes the canonical Consumer and tests move to package kafka_test.
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

	v2, err := newConsumer(cfg, kgoClient, nil)
	require.NoError(t, err, "newConsumer should succeed")
	require.NotNil(t, v2, "Consumer should not be nil")
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

	v2, err := newConsumer(cfg, kgoClient, nil)
	require.NoError(t, err, "newConsumer should succeed with startup subscriptions")

	snap := v2.router.Snapshot()
	assert.Equal(t, len(snap), 2, "should have 2 subscriptions")
}

func TestConsumer_AddSubscription(t *testing.T) {
	kgoClient := newTestKgoClient(t)
	cfg := newConfig([]string{"localhost:9092"}, "test-group")
	v2, err := newConsumer(cfg, kgoClient, nil)
	require.NoError(t, err, "newConsumer should succeed")

	sub := Subscription{
		Topic:         "my-topic",
		Handler:       func(context.Context, *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: FailurePolicy{},
		AckMode:       AckModeAtLeastOnce,
	}

	err = v2.AddSubscription(sub)
	require.NoError(t, err, "AddSubscription should succeed")

	snap := v2.router.Snapshot()
	assert.Equal(t, len(snap), 1, "should have 1 subscription")

	got, ok := v2.router.Lookup("my-topic")
	assert.True(t, ok, "topic should be found")
	assert.Equal(t, got.Topic, "my-topic", "topic should match")
}

func TestConsumer_AddSubscription_Duplicate(t *testing.T) {
	kgoClient := newTestKgoClient(t)
	cfg := newConfig([]string{"localhost:9092"}, "test-group")
	v2, err := newConsumer(cfg, kgoClient, nil)
	require.NoError(t, err, "newConsumer should succeed")

	sub := Subscription{
		Topic:         "my-topic",
		Handler:       func(context.Context, *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: FailurePolicy{},
		AckMode:       AckModeAtLeastOnce,
	}

	err = v2.AddSubscription(sub)
	require.NoError(t, err, "first AddSubscription should succeed")

	err = v2.AddSubscription(sub)
	assert.ErrorContains(t, err, "topic handler already registered", "duplicate should error")
}

func TestConsumer_AddSubscription_Invalid(t *testing.T) {
	kgoClient := newTestKgoClient(t)
	cfg := newConfig([]string{"localhost:9092"}, "test-group")
	v2, err := newConsumer(cfg, kgoClient, nil)
	require.NoError(t, err, "newConsumer should succeed")

	sub := Subscription{
		Topic:         "",
		Handler:       func(context.Context, *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: FailurePolicy{},
		AckMode:       AckModeAtLeastOnce,
	}
	err = v2.AddSubscription(sub)
	assert.ErrorContains(t, err, "topic must not be empty", "empty topic should error")
}

func TestConsumer_AddTopic(t *testing.T) {
	kgoClient := newTestKgoClient(t)
	cfg := newConfig([]string{"localhost:9092"}, "test-group")
	v2, err := newConsumer(cfg, kgoClient, nil)
	require.NoError(t, err, "newConsumer should succeed")

	handler := func(context.Context, *kgo.Record) error { return nil }
	err = v2.AddTopic("topic-a", handler)
	require.NoError(t, err, "AddTopic should succeed")

	got, ok := v2.router.Lookup("topic-a")
	assert.True(t, ok, "topic should be found")
	assert.Equal(t, got.Topic, "topic-a", "topic should match")
	assert.NotNil(t, got.Handler, "handler should be set")
}

func TestConsumer_AddBatchTopic(t *testing.T) {
	kgoClient := newTestKgoClient(t)
	cfg := newConfig([]string{"localhost:9092"}, "test-group")
	v2, err := newConsumer(cfg, kgoClient, nil)
	require.NoError(t, err, "newConsumer should succeed")

	handler := func(context.Context, []*kgo.Record) BatchResult { return BatchResult{} }
	err = v2.AddBatchTopic("topic-b", handler)
	require.NoError(t, err, "AddBatchTopic should succeed")

	got, ok := v2.router.Lookup("topic-b")
	assert.True(t, ok, "topic should be found")
	assert.Equal(t, got.Topic, "topic-b", "topic should match")
	assert.NotNil(t, got.BatchHandler, "batch handler should be set")
}

func TestConsumer_Run_CancelsOnContext(t *testing.T) {
	kgoClient := newTestKgoClient(t)
	cfg := newConfig([]string{"localhost:9092"}, "test-group")
	v2, err := newConsumer(cfg, kgoClient, nil)
	require.NoError(t, err, "newConsumer should succeed")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	err = v2.Run(ctx)
	assert.ErrorIs(t, err, context.Canceled, "Run should return context.Canceled")
}

func TestConsumer_Run_RejectsConcurrentRun(t *testing.T) {
	kgoClient := newTestKgoClient(t)
	cfg := newConfig([]string{"localhost:9092"}, "test-group")
	v2, err := newConsumer(cfg, kgoClient, nil)
	require.NoError(t, err, "newConsumer should succeed")

	// Start the first run's lifecycle so runState is active, without
	// calling Run() itself (which would block on dispatch).
	_, beginErr := v2.runState.Begin()
	require.NoError(t, beginErr, "first Begin should succeed")

	ch := make(chan struct{})
	go func() {
		<-v2.runState.Context().Done()
		close(ch)
	}()

	// Second Run should be rejected immediately.
	ctx2 := context.Background()
	err = v2.Run(ctx2)
	assert.ErrorContains(t, err, "run is already active", "concurrent Run should error")

	v2.runState.Stop()
	<-ch
}

func TestConsumer_NormalizesOnRegister(t *testing.T) {
	kgoClient := newTestKgoClient(t)
	cfg := newConfig([]string{"localhost:9092"}, "test-group")
	v2, err := newConsumer(cfg, kgoClient, nil)
	require.NoError(t, err, "newConsumer should succeed")

	err = v2.AddSubscription(Subscription{
		Topic:         "t",
		Handler:       func(context.Context, *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: FailurePolicy{},
		AckMode:       AckModeAtLeastOnce,
	})
	require.NoError(t, err, "AddSubscription should succeed")

	got, ok := v2.router.Lookup("t")
	require.True(t, ok, "topic should be found")
	assert.Equal(t, got.FailurePolicy.MaxAttempts, 1, "should normalize max attempts")
}
