package kafka

import (
	"context"
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"

	"go-services/library/assert"
	"go-services/library/require"
)

func TestNewConsumer_RegistersStartupTopics(t *testing.T) {
	cfg := newConfig([]string{"broker:9092"}, "group")
	startupHandler := func(context.Context, *kgo.Record) error { return nil }
	cfg.topicRouter["topic-a"] = startupHandler

	consumer, err := newConsumer(cfg, nil)
	require.NoError(t, err, "failed to create consumer")

	handler, ok := consumer.handlerForTopic("topic-a")
	if !ok {
		t.Fatal("startup topic should be registered")
	}
	assert.NotNil(t, handler, "startup handler should be available")
}

func TestConsumerAddTopicValidation(t *testing.T) {
	cfg := newConfig([]string{"broker:9092"}, "group")
	consumer, err := newConsumer(cfg, nil)
	require.NoError(t, err, "failed to create consumer")

	handler := func(context.Context, *kgo.Record) error { return nil }

	t.Run("reject empty topic", func(t *testing.T) {
		err := consumer.AddTopic("", handler)
		assert.NotNil(t, err, "should reject empty topic")
		assert.StringContains(t, err.Error(), "topic must not be empty", "error message")
	})

	t.Run("reject nil handler", func(t *testing.T) {
		err := consumer.AddTopic("topic-a", nil)
		assert.NotNil(t, err, "should reject nil handler")
		assert.StringContains(t, err.Error(), "handler must not be nil", "error message")
	})

	t.Run("reject duplicate topic", func(t *testing.T) {
		err := consumer.AddTopic("topic-a", handler)
		require.NoError(t, err, "failed to add topic")

		err = consumer.AddTopic("topic-a", handler)
		assert.NotNil(t, err, "should reject duplicate topic")
		assert.StringContains(t, err.Error(), `topic handler already registered for "topic-a"`, "error message")
	})
}

func TestConsumerAddTopicRejectsClosedConsumer(t *testing.T) {
	cfg := newConfig([]string{"broker:9092"}, "group")
	client := &Client{}
	consumer, err := newConsumer(cfg, client)
	require.NoError(t, err, "failed to create consumer")

	client.Close()

	err = consumer.AddTopic("topic-a", func(context.Context, *kgo.Record) error { return nil })
	assert.NotNil(t, err, "closed consumer should reject new topics")
	assert.StringContains(t, err.Error(), "consumer is closed", "error message")
}
