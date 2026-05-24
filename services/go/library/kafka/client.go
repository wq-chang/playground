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

// Client owns the shared franz-go client and exposes the consumer and producer
// capability wrappers that operate on it.
type Client struct {
	Consumer  *Consumer
	Producer  *Producer
	kgoClient *kgo.Client
	closeOnce sync.Once
	closed    bool
	mu        sync.RWMutex
}

// Close closes the shared franz-go client used by both Consumer and Producer.
func (c *Client) Close() {
	if c == nil {
		return
	}

	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closed = true
		c.mu.Unlock()
		if c.kgoClient != nil {
			c.kgoClient.Close()
		}
	})
}

func (c *Client) isClosed() bool {
	if c == nil {
		return false
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.closed
}

// New creates and initializes a Kafka client using a single shared
// kgo.Client. This is the primary entry point for the kafka package.
//
// It automatically configures the client based on the provided brokers, groupId, and
// functional options. It handles:
//   - SASL Authentication (Plain, SCRAM-256, SCRAM-512)
//   - Topic routing based on registered handlers
//   - Offset management strategy (e.g., disabling auto-commit for AtLeastOnce mode)
//
// Both Client.Consumer and Client.Producer share the same underlying TCP connections
// to the Kafka brokers, which is more resource-efficient than creating separate clients.
//
// Any topics registered through WithTopic are subscribed up front via kgo.ConsumeTopics
// during client creation. Additional topics can be registered later through
// Consumer.AddTopic, which updates both the consumer's handler router and the franz-go
// runtime subscription.
func New(brokers []string, groupId string, opts ...Option) (*Client, error) {
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
			return nil, fmt.Errorf("unsupported auth mechanism: %s", cfg.auth.Mechanism)
		}
		kgoOpts = append(kgoOpts, kgo.SASL(m))
	}

	if cfg.ackMode == AckModeAtLeastOnce {
		kgoOpts = append(kgoOpts, kgo.DisableAutoCommit())
	}

	kgoOpts = append(kgoOpts, cfg.kgoOpts...)

	kgoClient, err := kgo.NewClient(kgoOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create kgo client: %w", err)
	}

	client := &Client{
		Consumer:  nil,
		Producer:  nil,
		kgoClient: kgoClient,
		closeOnce: sync.Once{},
		closed:    false,
		mu:        sync.RWMutex{},
	}

	consumer, err := newConsumer(cfg, client)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to initialize consumer: %w", err)
	}
	producer := newProducer(cfg, client)

	client.Consumer = consumer
	client.Producer = producer

	return client, nil
}
