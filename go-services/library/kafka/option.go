package kafka

import (
	"fmt"
	"log/slog"

	"github.com/twmb/franz-go/pkg/kgo"
)

// AuthConfig contains credentials and mechanism for SASL authentication.
type AuthConfig struct {
	// Username for authentication.
	Username string
	// Password for authentication.
	Password string
	// Mechanism for authentication (e.g., PLAIN, SCRAM-SHA-256).
	Mechanism AuthMechanism
}

// config holds the configuration for a Kafka consumer.
type config struct {
	// auth is the SASL authentication configuration.
	auth *AuthConfig
	// logger is the logger used by the consumer.
	logger *slog.Logger
	// groupId is the Kafka consumer group ID.
	groupId string
	// brokers is the list of seed brokers.
	brokers []string
	// TopicRouter maps Kafka topic names to their respective processing logic.
	// It acts as a central switchboard to ensure each message is handled by
	// the correct domain function.
	topicRouter map[string]Handler
	// kgoOpts are additional franz-go client options.
	kgoOpts []kgo.Opt
	// workers is the number of parallel workers for processing records.
	workers int
	// ackMode determines when records are committed.
	ackMode AckMode
}

// newConfig creates a new kafka onfig with default values.
func newConfig(brokers []string, groupId string) *config {
	return &config{
		groupId:     groupId,
		topicRouter: make(map[string]Handler),
		workers:     1,
		ackMode:     AckModeAtLeastOnce,
		logger:      slog.Default(),
		auth:        nil,
		brokers:     brokers,
		kgoOpts:     []kgo.Opt{},
	}
}

// Option is a configuration function that can be applied to both Consumer and Producer.
type Option func(*config)

// WithAuth sets the SASL authentication configuration.
func WithAuth(username, password string, mechanism AuthMechanism) Option {
	return func(c *config) {
		auth := &AuthConfig{
			Username:  username,
			Password:  password,
			Mechanism: mechanism,
		}
		c.auth = auth
	}
}

// WithLogger sets the logger.
func WithLogger(logger *slog.Logger) Option {
	return func(c *config) {
		c.logger = logger
	}
}

// WithKgoOptions allows passing additional franz-go client options for both.
func WithKgoOptions(opts ...kgo.Opt) Option {
	return func(c *config) {
		c.kgoOpts = append(c.kgoOpts, opts...)
	}
}

// --- Consumer Specific Options ---

// WithWorkers sets the number of worker goroutines for processing records (Consumer only).
func WithWorkers(workers int) Option {
	return func(c *config) {
		if workers > 0 {
			c.workers = workers
		}
	}
}

// WithAckMode sets the acknowledgment mode (Consumer only).
func WithAckMode(mode AckMode) Option {
	return func(c *config) {
		c.ackMode = mode
	}
}

// WithTopic registers a processing handler for a specific Kafka topic.
// If a handler is already registered for the given topic, this function
// will panic.
func WithTopic(topic string, handler Handler) Option {
	return func(c *config) {
		if _, ok := c.topicRouter[topic]; ok {
			panic(fmt.Sprintf("topic handler already registered for %q", topic))
		}
		c.topicRouter[topic] = handler
	}
}

// --- Producer Specific Options ---

// WithProducerAcks sets the required acknowledgments for the producer.
func WithProducerAcks(acks kgo.Acks) Option {
	return func(c *config) {
		c.kgoOpts = append(c.kgoOpts, kgo.RequiredAcks(acks))
	}
}
