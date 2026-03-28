package kafka_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go-services/library/assert"
	"go-services/library/kafka"
	"go-services/library/require"

	"github.com/twmb/franz-go/pkg/kgo"
)

func TestKafkaProducerConsumer(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	k := te.GetKafka(t)
	topic := fmt.Sprintf("test-topic-%d", time.Now().UnixNano())
	err := k.CreateTopic(ctx, topic)
	require.NoError(t, err, "failed to create test topic")

	consumedChan := make(chan string, 1)
	handler := func(ctx context.Context, record *kgo.Record) error {
		consumedChan <- string(record.Value)
		return nil
	}

	consumer, producer, err := kafka.New(
		k.AuthBrokers,
		"test-group",
		kafka.WithTopic(topic, handler),
		kafka.WithKgoOptions(kgo.ConsumeResetOffset(kgo.NewOffset().AtStart())),
		kafka.WithAuth(k.Username, k.Password, kafka.AuthMechanismScram512),
	)
	require.NoError(t, err, "failed to create producer and consumer")
	defer consumer.Close()
	defer producer.Close()

	message := "hello kafka"
	err = producer.ProduceSync(ctx, &kgo.Record{
		Topic: topic,
		Value: []byte(message),
	})
	require.NoError(t, err, "failed to produce message")

	go func() {
		if err := consumer.Run(ctx); err != nil {
			if ctx.Err() == nil {
				t.Errorf("consumer run failed: %v", err)
			}
		}
	}()

	select {
	case val := <-consumedChan:
		assert.Equal(t, val, message, "kafka message")
	case <-ctx.Done():
		t.Fatal("timed out waiting for message")
	}
}
