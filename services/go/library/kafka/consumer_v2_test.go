// services/go/library/kafka/consumer_v2_test.go
// TEMPORARY: Same-package tests for the v2 façade. Will be deleted in Step 10
// when v2 becomes the canonical Consumer and tests move to package kafka_test.
package kafka

import (
	"context"
	"testing"

	"go-services/library/assert"
	"go-services/library/require"

	"github.com/twmb/franz-go/pkg/kgo"
)

// newTestClientV2 creates a minimal Client for v2 unit testing.
// It creates a real kgo.Client so AddConsumeTopics doesn't panic.
func newTestClientV2(t *testing.T) *Client {
	t.Helper()

	kgoClient, err := kgo.NewClient(
		kgo.SeedBrokers("localhost:9092"),
		kgo.ConsumerGroup("test-group"),
	)
	require.NoError(t, err, "failed to create test kgo client")
	t.Cleanup(kgoClient.Close)

	return &Client{
		kgoClient: kgoClient,
		Consumer:  nil,
	}
}

func TestConsumerV2_New_ValidConfig(t *testing.T) {
	client := newTestClientV2(t)
	cfg := newConfig([]string{"localhost:9092"}, "test-group")

	v2, err := newConsumerV2(cfg, client)
	require.NoError(t, err, "newConsumerV2 should succeed")
	require.NotNil(t, v2, "consumerV2 should not be nil")
}

func TestConsumerV2_New_WithStartupSubscriptions(t *testing.T) {
	client := newTestClientV2(t)
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

	v2, err := newConsumerV2(cfg, client)
	require.NoError(t, err, "newConsumerV2 should succeed with startup subscriptions")
	assert.Equal(t, len(v2.subscriptions), 2, "should have 2 subscriptions")
}

func TestConsumerV2_AddSubscription(t *testing.T) {
	client := newTestClientV2(t)
	cfg := newConfig([]string{"localhost:9092"}, "test-group")
	v2, err := newConsumerV2(cfg, client)
	require.NoError(t, err, "newConsumerV2 should succeed")

	sub := Subscription{
		Topic:         "my-topic",
		Handler:       func(context.Context, *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: FailurePolicy{},
		AckMode:       AckModeAtLeastOnce,
	}

	err = v2.AddSubscription(sub)
	require.NoError(t, err, "AddSubscription should succeed")

	assert.Equal(t, len(v2.subscriptions), 1, "should have 1 subscription")
	assert.Equal(t, v2.subscriptions["my-topic"].Topic, "my-topic", "topic should match")
}

func TestConsumerV2_AddSubscription_Duplicate(t *testing.T) {
	client := newTestClientV2(t)
	cfg := newConfig([]string{"localhost:9092"}, "test-group")
	v2, err := newConsumerV2(cfg, client)
	require.NoError(t, err, "newConsumerV2 should succeed")

	sub := Subscription{
		Topic:         "my-topic",
		Handler:       func(context.Context, *kgo.Record) error { return nil },
		FailurePolicy: FailurePolicy{},
		AckMode:       AckModeAtLeastOnce,
	}

	err = v2.AddSubscription(sub)
	require.NoError(t, err, "first AddSubscription should succeed")

	err = v2.AddSubscription(sub)
	assert.ErrorContains(t, err, "topic handler already registered", "duplicate should error")
}

func TestConsumerV2_AddSubscription_Invalid(t *testing.T) {
	client := newTestClientV2(t)
	cfg := newConfig([]string{"localhost:9092"}, "test-group")
	v2, err := newConsumerV2(cfg, client)
	require.NoError(t, err, "newConsumerV2 should succeed")

	sub := Subscription{
		Topic:         "",
		Handler:       func(context.Context, *kgo.Record) error { return nil },
		FailurePolicy: FailurePolicy{},
		AckMode:       AckModeAtLeastOnce,
	}
	err = v2.AddSubscription(sub)
	assert.ErrorContains(t, err, "topic must not be empty", "empty topic should error")
}

func TestConsumerV2_AddTopic(t *testing.T) {
	client := newTestClientV2(t)
	cfg := newConfig([]string{"localhost:9092"}, "test-group")
	v2, err := newConsumerV2(cfg, client)
	require.NoError(t, err, "newConsumerV2 should succeed")

	handler := func(context.Context, *kgo.Record) error { return nil }
	err = v2.AddTopic("topic-a", handler)
	require.NoError(t, err, "AddTopic should succeed")

	assert.Equal(t, len(v2.subscriptions), 1, "should have 1 subscription")
	assert.Equal(t, v2.subscriptions["topic-a"].Topic, "topic-a", "topic should match")
	assert.NotNil(t, v2.subscriptions["topic-a"].Handler, "handler should be set")
}

func TestConsumerV2_AddBatchTopic(t *testing.T) {
	client := newTestClientV2(t)
	cfg := newConfig([]string{"localhost:9092"}, "test-group")
	v2, err := newConsumerV2(cfg, client)
	require.NoError(t, err, "newConsumerV2 should succeed")

	handler := func(context.Context, []*kgo.Record) BatchResult { return BatchResult{} }
	err = v2.AddBatchTopic("topic-b", handler)
	require.NoError(t, err, "AddBatchTopic should succeed")

	assert.Equal(t, len(v2.subscriptions), 1, "should have 1 subscription")
	assert.Equal(t, v2.subscriptions["topic-b"].Topic, "topic-b", "topic should match")
	assert.NotNil(t, v2.subscriptions["topic-b"].BatchHandler, "batch handler should be set")
}

func TestConsumerV2_Run_Stub(t *testing.T) {
	client := newTestClientV2(t)
	cfg := newConfig([]string{"localhost:9092"}, "test-group")
	v2, err := newConsumerV2(cfg, client)
	require.NoError(t, err, "newConsumerV2 should succeed")

	err = v2.Run(context.Background())
	assert.ErrorIs(t, err, errV2NotImplemented, "Run should return errV2NotImplemented")
}

func TestConsumerV2_SubscriptionSnapshot_IsImmutable(t *testing.T) {
	client := newTestClientV2(t)
	cfg := newConfig([]string{"localhost:9092"}, "test-group")
	v2, err := newConsumerV2(cfg, client)
	require.NoError(t, err, "newConsumerV2 should succeed")

	err = v2.AddSubscription(Subscription{
		Topic:         "t",
		Handler:       func(context.Context, *kgo.Record) error { return nil },
		FailurePolicy: FailurePolicy{},
		AckMode:       AckModeAtLeastOnce,
	})
	require.NoError(t, err, "AddSubscription should succeed")

	assert.Equal(t, v2.subscriptions["t"].FailurePolicy.MaxAttempts, 1, "should normalize max attempts")
}
