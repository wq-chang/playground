package kafka

import (
	"fmt"
	"maps"
	"slices"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/pkg/sasl/scram"
)

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
func New(brokers []string, groupId string, opts ...Option) (*Consumer, *Producer, error) {
	cfg := newConfig(brokers, groupId)
	for _, opt := range opts {
		opt(cfg)
	}

	topics := slices.Collect(maps.Keys(cfg.topicRouter))
	kgoOpts := []kgo.Opt{
		kgo.SeedBrokers(cfg.brokers...),
		kgo.ConsumerGroup(cfg.groupId),
		kgo.ConsumeTopics(topics...),
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

	consumer := newConsumer(cfg, client)
	producer := newProducer(cfg, client)

	return consumer, producer, err
}
