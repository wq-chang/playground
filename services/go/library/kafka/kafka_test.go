//go:build integration

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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	topic := fmt.Sprintf("test-topic-%d", time.Now().UnixNano())
	err := testKafka.CreateTopic(ctx, topic)
	require.NoError(t, err, "failed to create test topic")

	message := "hello kafka"

	tests := map[string]struct {
		brokers []string
		auth    bool
	}{
		"no-auth": {testKafka.PlainBrokers, false},
		"auth":    {testKafka.AuthBrokers, true},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			consumedChan := make(chan string, 1)
			handler := func(ctx context.Context, record *kgo.Record) error {
				consumedChan <- string(record.Value)
				return nil
			}

			opts := []kafka.Option{
				kafka.WithTopic(topic, handler),
				kafka.WithKgoOptions(kgo.ConsumeResetOffset(kgo.NewOffset().AtStart())),
			}
			if tt.auth {
				opts = append(opts, kafka.WithAuth(testKafka.Username, testKafka.Password, kafka.AuthMechanismScram512))
			}

			consumer, producer, err := kafka.New(tt.brokers, "test-group-"+name, opts...)
			require.NoError(t, err, "failed to create producer and consumer")
			defer consumer.Close()
			defer producer.Close()

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
		})
	}
}

func TestKafkaConsumerAddTopic(t *testing.T) {
	t.Run("after construction before run", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		topic := fmt.Sprintf("test-add-topic-before-run-%d", time.Now().UnixNano())
		err := testKafka.CreateTopic(ctx, topic)
		require.NoError(t, err, "failed to create test topic")

		consumedChan := make(chan string, 1)
		handler := func(ctx context.Context, record *kgo.Record) error {
			consumedChan <- string(record.Value)
			return nil
		}

		consumer, producer, err := kafka.New(
			testKafka.PlainBrokers,
			fmt.Sprintf("test-group-before-run-%d", time.Now().UnixNano()),
			kafka.WithKgoOptions(kgo.ConsumeResetOffset(kgo.NewOffset().AtStart())),
		)
		require.NoError(t, err, "failed to create producer and consumer")
		defer consumer.Close()
		defer producer.Close()

		err = consumer.AddTopic(topic, handler)
		require.NoError(t, err, "failed to add topic before run")

		go func() {
			if runErr := consumer.Run(ctx); runErr != nil && ctx.Err() == nil {
				t.Errorf("consumer run failed: %v", runErr)
			}
		}()

		err = producer.ProduceSync(ctx, &kgo.Record{
			Topic: topic,
			Value: []byte("before-run"),
		})
		require.NoError(t, err, "failed to produce message")

		select {
		case val := <-consumedChan:
			assert.Equal(t, val, "before-run", "kafka message")
		case <-ctx.Done():
			t.Fatal("timed out waiting for message")
		}
	})

	t.Run("while run is active", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		topic := fmt.Sprintf("test-add-topic-during-run-%d", time.Now().UnixNano())
		err := testKafka.CreateTopic(ctx, topic)
		require.NoError(t, err, "failed to create test topic")

		consumedChan := make(chan string, 1)
		handler := func(ctx context.Context, record *kgo.Record) error {
			consumedChan <- string(record.Value)
			return nil
		}

		consumer, producer, err := kafka.New(
			testKafka.PlainBrokers,
			fmt.Sprintf("test-group-during-run-%d", time.Now().UnixNano()),
			kafka.WithKgoOptions(kgo.ConsumeResetOffset(kgo.NewOffset().AtStart())),
		)
		require.NoError(t, err, "failed to create producer and consumer")
		defer consumer.Close()
		defer producer.Close()

		go func() {
			if runErr := consumer.Run(ctx); runErr != nil && ctx.Err() == nil {
				t.Errorf("consumer run failed: %v", runErr)
			}
		}()

		time.Sleep(500 * time.Millisecond)

		err = consumer.AddTopic(topic, handler)
		require.NoError(t, err, "failed to add topic during run")

		err = producer.ProduceSync(ctx, &kgo.Record{
			Topic: topic,
			Value: []byte("during-run"),
		})
		require.NoError(t, err, "failed to produce message")

		select {
		case val := <-consumedChan:
			assert.Equal(t, val, "during-run", "kafka message")
		case <-ctx.Done():
			t.Fatal("timed out waiting for message")
		}
	})
}
