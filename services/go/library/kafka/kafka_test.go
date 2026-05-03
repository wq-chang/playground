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

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	k := te.GetKafka(t)
	topic := fmt.Sprintf("test-topic-%d", time.Now().UnixNano())
	err := k.CreateTopic(ctx, topic)
	require.NoError(t, err, "failed to create test topic")

	message := "hello kafka"

	tests := map[string]struct {
		brokers []string
		auth    bool
	}{
		"no-auth": {k.PlainBrokers, false},
		"auth":    {k.AuthBrokers, true},
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
				opts = append(opts, kafka.WithAuth(k.Username, k.Password, kafka.AuthMechanismScram512))
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
