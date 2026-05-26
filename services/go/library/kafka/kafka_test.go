//go:build integration

package kafka_test

import (
	"context"
	"fmt"
	"sync/atomic"
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
				kafka.WithSubscription(kafka.Subscription{
					Topic:         topic,
					Handler:       handler,
					AckMode:       kafka.AckModeAtLeastOnce,
					FailurePolicy: kafka.FailurePolicy{},
				}),
				kafka.WithKgoOptions(kgo.ConsumeResetOffset(kgo.NewOffset().AtStart())),
			}
			if tt.auth {
				opts = append(opts, kafka.WithAuth(testKafka.Username, testKafka.Password, kafka.AuthMechanismScram512))
			}

			client, err := kafka.New(tt.brokers, "test-group-"+name, opts...)
			require.NoError(t, err, "failed to create kafka client")
			defer client.Close()

			err = client.Producer.ProduceSync(ctx, &kgo.Record{
				Topic: topic,
				Value: []byte(message),
			})
			require.NoError(t, err, "failed to produce message")

			go func() {
				if err := client.Consumer.Run(ctx); err != nil {
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

func TestKafkaConsumerAddSubscription(t *testing.T) {
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

		client, err := kafka.New(
			testKafka.PlainBrokers,
			fmt.Sprintf("test-group-before-run-%d", time.Now().UnixNano()),
			kafka.WithKgoOptions(kgo.ConsumeResetOffset(kgo.NewOffset().AtStart())),
		)
		require.NoError(t, err, "failed to create kafka client")
		defer client.Close()

		err = client.Consumer.AddSubscription(kafka.Subscription{
			Topic:         topic,
			Handler:       handler,
			AckMode:       kafka.AckModeAtLeastOnce,
			FailurePolicy: kafka.FailurePolicy{},
		})
		require.NoError(t, err, "failed to add topic before run")

		go func() {
			if runErr := client.Consumer.Run(ctx); runErr != nil && ctx.Err() == nil {
				t.Errorf("consumer run failed: %v", runErr)
			}
		}()

		err = client.Producer.ProduceSync(ctx, &kgo.Record{
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

		client, err := kafka.New(
			testKafka.PlainBrokers,
			fmt.Sprintf("test-group-during-run-%d", time.Now().UnixNano()),
			kafka.WithKgoOptions(kgo.ConsumeResetOffset(kgo.NewOffset().AtStart())),
		)
		require.NoError(t, err, "failed to create kafka client")
		defer client.Close()

		go func() {
			if runErr := client.Consumer.Run(ctx); runErr != nil && ctx.Err() == nil {
				t.Errorf("consumer run failed: %v", runErr)
			}
		}()

		time.Sleep(500 * time.Millisecond)

		err = client.Consumer.AddSubscription(kafka.Subscription{
			Topic:         topic,
			Handler:       handler,
			AckMode:       kafka.AckModeAtLeastOnce,
			FailurePolicy: kafka.FailurePolicy{},
		})
		require.NoError(t, err, "failed to add topic during run")

		err = client.Producer.ProduceSync(ctx, &kgo.Record{
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

func TestKafkaConsumerDLQOnFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mainTopic := fmt.Sprintf("test-dlq-main-%d", time.Now().UnixNano())
	dlqTopic := fmt.Sprintf("test-dlq-topic-%d", time.Now().UnixNano())

	require.NoError(t, testKafka.CreateTopic(ctx, mainTopic), "failed to create main topic")
	require.NoError(t, testKafka.CreateTopic(ctx, dlqTopic), "failed to create dlq topic")

	dlqRecords := make(chan *kgo.Record, 1)
	client, err := kafka.New(
		testKafka.PlainBrokers,
		fmt.Sprintf("test-group-dlq-%d", time.Now().UnixNano()),
		kafka.WithSubscription(kafka.Subscription{
			Topic: mainTopic,
			Handler: func(context.Context, *kgo.Record) error {
				return fmt.Errorf("boom")
			},
			AckMode: kafka.AckModeAtLeastOnce,
			FailurePolicy: kafka.FailurePolicy{
				MaxAttempts:  0,
				RetryBackoff: 0,
				DLQ:          &kafka.DLQConfig{Topic: dlqTopic},
				OnExhausted:  kafka.ExhaustedActionUnspecified,
			},
		}),
		kafka.WithSubscription(kafka.Subscription{
			Topic: dlqTopic,
			Handler: func(ctx context.Context, record *kgo.Record) error {
				dlqRecords <- record
				return nil
			},
			AckMode:       kafka.AckModeAtLeastOnce,
			FailurePolicy: kafka.FailurePolicy{},
		}),
		kafka.WithKgoOptions(kgo.ConsumeResetOffset(kgo.NewOffset().AtStart())),
	)
	require.NoError(t, err, "failed to create kafka client")
	defer client.Close()

	runErrs := make(chan error, 1)
	go func() {
		runErrs <- client.Consumer.Run(ctx)
	}()

	require.NoError(t, client.Producer.ProduceSync(ctx, &kgo.Record{
		Topic: mainTopic,
		Value: []byte("failed-message"),
	}), "failed to produce main topic message")

	select {
	case record := <-dlqRecords:
		assert.Equal(t, string(record.Value), "failed-message", "dlq payload")
		headerValue, ok := headerValue(record.Headers, "dlq-original-topic")
		assert.True(t, ok, "dlq metadata header should exist")
		assert.Equal(t, headerValue, mainTopic, "dlq original topic")
	case err := <-runErrs:
		if err != nil && ctx.Err() == nil {
			t.Fatalf("consumer should continue after dlq publish: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for dlq message")
	}
}

func headerValue(headers []kgo.RecordHeader, key string) (string, bool) {
	for _, header := range headers {
		if header.Key == key {
			return string(header.Value), true
		}
	}
	return "", false
}

func TestKafkaConsumerStopPausesTopicOnly(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	failingTopic := fmt.Sprintf("test-stop-topic-%d", time.Now().UnixNano())
	healthyTopic := fmt.Sprintf("test-healthy-topic-%d", time.Now().UnixNano())
	require.NoError(t, testKafka.CreateTopic(ctx, failingTopic), "failed to create failing topic")
	require.NoError(t, testKafka.CreateTopic(ctx, healthyTopic), "failed to create healthy topic")

	var failingCalls atomic.Int32
	failingSeen := make(chan struct{}, 1)
	healthyValues := make(chan string, 2)

	client, err := kafka.New(
		testKafka.PlainBrokers,
		fmt.Sprintf("test-group-stop-%d", time.Now().UnixNano()),
		kafka.WithSubscription(kafka.Subscription{
			Topic: failingTopic,
			Handler: func(context.Context, *kgo.Record) error {
				if failingCalls.Add(1) == 1 {
					failingSeen <- struct{}{}
				}
				return fmt.Errorf("boom")
			},
			AckMode:       kafka.AckModeAtLeastOnce,
			FailurePolicy: kafka.FailurePolicy{},
		}),
		kafka.WithSubscription(kafka.Subscription{
			Topic: healthyTopic,
			Handler: func(ctx context.Context, record *kgo.Record) error {
				healthyValues <- string(record.Value)
				return nil
			},
			AckMode:       kafka.AckModeAtLeastOnce,
			FailurePolicy: kafka.FailurePolicy{},
		}),
		kafka.WithKgoOptions(kgo.ConsumeResetOffset(kgo.NewOffset().AtStart())),
	)
	require.NoError(t, err, "failed to create kafka client")
	defer client.Close()

	runErrs := make(chan error, 1)
	go func() {
		runErrs <- client.Consumer.Run(ctx)
	}()

	require.NoError(t, client.Producer.ProduceSync(ctx, &kgo.Record{
		Topic: failingTopic,
		Value: []byte("failed-message-1"),
	}), "failed to produce main topic message")

	select {
	case <-failingSeen:
	case <-ctx.Done():
		t.Fatal("timed out waiting for failing topic to be handled")
	}

	require.NoError(t, client.Producer.ProduceSync(ctx, &kgo.Record{
		Topic: healthyTopic,
		Value: []byte("healthy-message"),
	}), "failed to produce healthy topic message")
	require.NoError(t, client.Producer.ProduceSync(ctx, &kgo.Record{
		Topic: failingTopic,
		Value: []byte("failed-message-2"),
	}), "failed to produce second failing topic message")

	select {
	case value := <-healthyValues:
		assert.Equal(t, value, "healthy-message", "healthy topic should continue consuming")
	case err := <-runErrs:
		if err != nil && ctx.Err() == nil {
			t.Fatalf("consumer should keep running after pausing one topic: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for healthy topic message")
	}

	time.Sleep(500 * time.Millisecond)
	assert.Equal(t, failingCalls.Load(), int32(1), "failing topic should be paused after the first exhausted failure")

	select {
	case err := <-runErrs:
		if err != nil && ctx.Err() == nil {
			t.Fatalf("consumer should keep running after pausing one topic: %v", err)
		}
	default:
	}
}
