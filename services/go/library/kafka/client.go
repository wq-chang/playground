package kafka

import (
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/pkg/sasl/scram"
)

type sharedClient struct {
	client    *kgo.Client
	closeOnce sync.Once
}

func newSharedClient(client *kgo.Client) *sharedClient {
	return &sharedClient{
		client:    client,
		closeOnce: sync.Once{},
	}
}

func (c *sharedClient) Close() {
	if c == nil {
		return
	}

	c.closeOnce.Do(func() {
		if c.client != nil {
			c.client.Close()
		}
	})
}

// New creates and initializes a Kafka Consumer and Producer pair using a single shared
// kgo.Client. This is the primary entry point for the kafka package.
//
// It automatically configures the client based on the provided brokers, groupId, and
// functional options. It handles:
//   - SASL Authentication (Plain, SCRAM-256, SCRAM-512)
//   - Topic routing based on registered handlers
//   - Offset management strategy (e.g., disabling auto-commit for AtLeastOnce mode)
//
// Both the returned Consumer and Producer share the same underlying TCP connections
// to the Kafka brokers, which is more resource-efficient than creating separate clients.
//
// Any topics registered through WithTopic are subscribed up front via kgo.ConsumeTopics
// during client creation. Additional topics can be registered later through
// Consumer.AddTopic, which updates both the consumer's handler router and the franz-go
// runtime subscription.
func New(brokers []string, groupId string, opts ...Option) (*Consumer, *Producer, error) {
	cfg := newConfig(brokers, groupId)
	for _, opt := range opts {
		opt(cfg)
	}

	topics := slices.Collect(maps.Keys(cfg.topicRouter))
	kgoOpts := []kgo.Opt{
		kgo.SeedBrokers(cfg.brokers...),
		kgo.ConsumerGroup(cfg.groupId),
	}
	if len(topics) > 0 {
		kgoOpts = append(kgoOpts, kgo.ConsumeTopics(topics...))
	}

	if cfg.auth != nil {
		var m sasl.Mechanism
		switch cfg.auth.Mechanism {
		case AuthMechanismPlain:
			m = plain.Auth{
				User: cfg.auth.Username,
				Pass: cfg.auth.Password,
			}.AsMechanism()
		case AuthMechanismScram256:
			m = scram.Auth{
				User: cfg.auth.Username,
				Pass: cfg.auth.Password,
			}.AsSha256Mechanism()
		case AuthMechanismScram512:
			m = scram.Auth{
				User: cfg.auth.Username,
				Pass: cfg.auth.Password,
			}.AsSha512Mechanism()
		default:
			return nil, nil, fmt.Errorf("unsupported auth mechanism: %s", cfg.auth.Mechanism)
		}
		kgoOpts = append(kgoOpts, kgo.SASL(m))
	}

	if cfg.ackMode == AckModeAtLeastOnce {
		kgoOpts = append(kgoOpts, kgo.DisableAutoCommit())
	}

	kgoOpts = append(kgoOpts, cfg.kgoOpts...)

	client, err := kgo.NewClient(kgoOpts...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create kgo client: %w", err)
	}

	shared := newSharedClient(client)

	consumer, err := newConsumer(cfg, shared)
	if err != nil {
		shared.Close()
		return nil, nil, fmt.Errorf("failed to initialize consumer: %w", err)
	}
	producer := newProducer(cfg, shared)

	return consumer, producer, err
}
