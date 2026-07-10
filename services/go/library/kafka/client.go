package kafka

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/pkg/sasl/scram"
)

// AuthMechanism represents the SASL authentication mechanism.
type AuthMechanism string

const (
	// AuthMechanismPlain uses SASL/PLAIN authentication.
	AuthMechanismPlain AuthMechanism = "PLAIN"
	// AuthMechanismScram256 uses SASL/SCRAM-SHA-256 authentication.
	AuthMechanismScram256 AuthMechanism = "SCRAM-SHA-256"
	// AuthMechanismScram512 uses SASL/SCRAM-SHA-512 authentication.
	AuthMechanismScram512 AuthMechanism = "SCRAM-SHA-512"
)

// Client owns the shared franz-go client and exposes the consumer and producer
// capability wrappers that operate on it.
type Client struct {
	// Consumer exposes the package's topic subscription and consumption APIs.
	Consumer *Consumer
	// Producer exposes the package's record publishing APIs.
	Producer  *Producer
	kgoClient *kgo.Client
	closeOnce sync.Once
}

// Close closes the shared franz-go client used by both Consumer and Producer.
// It is safe to call more than once.
func (c *Client) Close() {
	if c == nil {
		return
	}
	c.closeOnce.Do(func() {
		if c.kgoClient != nil {
			c.kgoClient.Close()
		}
	})
}

// New creates and initializes a Kafka client using a single shared
// kgo.Client. This is the primary entry point for the kafka package.
//
// It configures the shared client from the provided brokers, consumer group ID,
// and functional options. It handles:
//   - SASL Authentication (Plain, SCRAM-256, SCRAM-512)
//   - Topic routing based on registered subscriptions
//   - Package-managed manual offset commits with rebalance coordination
//
// Both Client.Consumer and Client.Producer share the same underlying TCP connections
// to the Kafka brokers, which is more resource-efficient than creating separate clients.
//
// Any topics registered through WithSubscription / WithTopic are subscribed up front
// via kgo.ConsumeTopics during client creation. Additional topics can be registered
// later through Consumer.AddSubscription / Consumer.AddTopic, which update both the
// consumer router and the franz-go runtime subscription.
func New(brokers []string, groupId string, opts ...Option) (*Client, error) {
	cfg := newConfig(brokers, groupId)
	for _, opt := range opts {
		opt(cfg)
	}

	topics := slices.Collect(maps.Keys(cfg.subscriptions))
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

	kgoOpts = append(kgoOpts, cfg.kgoOpts...)

	var consumer *Consumer
	kgoOpts = append(kgoOpts,
		kgo.DisableAutoCommit(),
		kgo.BlockRebalanceOnPoll(),
		kgo.OnPartitionsRevoked(func(ctx context.Context, cl *kgo.Client, partitions map[string][]int32) {
			if consumer == nil {
				return
			}
			consumer.onPartitionsRevoked(ctx, cl, partitions)
		}),
		kgo.OnPartitionsLost(func(ctx context.Context, _ *kgo.Client, partitions map[string][]int32) {
			if consumer == nil {
				return
			}
			consumer.onPartitionsLost(ctx, partitions)
		}),
	)

	kgoClient, err := kgo.NewClient(kgoOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create kgo client: %w", err)
	}

	producer := newProducer(kgoClient)

	consumer, err = newConsumer(cfg, kgoClient, producer)
	if err != nil {
		kgoClient.Close()
		return nil, fmt.Errorf("failed to initialize consumer: %w", err)
	}

	client := &Client{
		Consumer:  consumer,
		Producer:  producer,
		kgoClient: kgoClient,
		closeOnce: sync.Once{},
	}

	return client, nil
}
