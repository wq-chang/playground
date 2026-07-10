package kafka

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

// AuthConfig contains credentials and mechanism for SASL authentication.
type AuthConfig struct {
	// Username is the SASL username.
	Username string
	// Password is the SASL password.
	Password string
	// Mechanism selects the SASL mechanism, such as PLAIN or SCRAM-SHA-256.
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
	// subscriptions stores startup topic registrations that are applied when the
	// consumer is constructed. Runtime additions live on Consumer itself.
	subscriptions map[string]Subscription
	// kgoOpts are additional franz-go client options.
	kgoOpts []kgo.Opt
	// workers is the max number of handler invocations processed concurrently
	// across all per-partition runners.
	workers int
	// defaultAckMode determines which acknowledgment mode is applied by the
	// compatibility topic APIs.
	defaultAckMode AckMode
	// fetchMaxRecords is the max number of records returned by a single PollRecords call.
	// Defaults to 500, matching Apache Kafka's max.poll.records default.
	fetchMaxRecords int
	// drainTimeout is the maximum time to wait for partition workers to drain
	// during graceful shutdown. Defaults to 30 seconds.
	drainTimeout time.Duration
	// queueCapacity is the per-partition record queue capacity. Defaults to 64.
	queueCapacity int
}

// newConfig creates a new kafka onfig with default values.
func newConfig(brokers []string, groupId string) *config {
	return &config{
		groupId:         groupId,
		subscriptions:   make(map[string]Subscription),
		workers:         16,
		defaultAckMode:  AckModeAtLeastOnce,
		fetchMaxRecords: 500,
		drainTimeout:    30 * time.Second,
		queueCapacity:   64,
		logger:          slog.Default(),
		auth:            nil,
		brokers:         brokers,
		kgoOpts:         []kgo.Opt{},
	}
}

// Option configures the shared Client created by New.
type Option func(*config)

// WithAuth configures SASL authentication for the shared client.
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

// WithLogger sets the logger used by the consumer runtime.
func WithLogger(logger *slog.Logger) Option {
	return func(c *config) {
		c.logger = logger
	}
}

// WithKgoOptions appends raw franz-go options to the shared client
// configuration.
func WithKgoOptions(opts ...kgo.Opt) Option {
	return func(c *config) {
		c.kgoOpts = append(c.kgoOpts, opts...)
	}
}

// --- Consumer Specific Options ---

// WithWorkers sets the maximum number of handler invocations processed
// concurrently across all per-partition runners.
func WithWorkers(workers int) Option {
	return func(c *config) {
		if workers > 0 {
			c.workers = workers
		}
	}
}

// WithAckMode sets the default acknowledgment mode used by WithTopic,
// WithBatchTopic, Consumer.AddTopic, and Consumer.AddBatchTopic.
func WithAckMode(mode AckMode) Option {
	return func(c *config) {
		c.defaultAckMode = mode
	}
}

// WithSubscription registers a processing subscription during client
// construction.
//
// For runtime registration after New, use Consumer.AddSubscription. If a
// subscription is already registered for the given topic, this option panics.
func WithSubscription(subscription Subscription) Option {
	return func(c *config) {
		normalized, err := subscription.Normalize()
		if err != nil {
			panic(err)
		}
		if _, ok := c.subscriptions[normalized.Topic]; ok {
			panic(fmt.Sprintf("topic handler already registered for %q", normalized.Topic))
		}
		c.subscriptions[normalized.Topic] = normalized
	}
}

// WithTopic registers a single-record processing handler for a specific Kafka
// topic during client construction.
//
// The created subscription uses the default acknowledgment mode configured by
// WithAckMode. For runtime registration after New, use Consumer.AddTopic. If a
// handler is already registered for the given topic, this option panics.
func WithTopic(topic string, handler Handler) Option {
	return func(c *config) {
		WithSubscription(newDefaultSubscription(topic, handler, c.defaultAckMode))(c)
	}
}

// WithBatchTopic registers a batch processing handler for a specific Kafka
// topic during client construction.
//
// The created subscription uses the default acknowledgment mode configured by
// WithAckMode. For runtime registration after New, use Consumer.AddBatchTopic.
// If a handler is already registered for the given topic, this option panics.
func WithBatchTopic(topic string, handler BatchHandler) Option {
	return func(c *config) {
		WithSubscription(newDefaultBatchSubscription(topic, handler, c.defaultAckMode))(c)
	}
}

// --- Consumer Specific Options (continued) ---

// WithFetchMaxRecords sets the maximum number of records returned by a single
// PollRecords call. Defaults to 500, matching Apache Kafka's max.poll.records.
// Pass -1 for unlimited (poll all available records).
func WithFetchMaxRecords(n int) Option {
	return func(c *config) {
		if n > 0 || n == -1 {
			c.fetchMaxRecords = n
		}
	}
}

// WithDrainTimeout sets the maximum time to wait for partition workers to
// drain during graceful shutdown. Defaults to 30 seconds.
func WithDrainTimeout(d time.Duration) Option {
	return func(c *config) {
		if d > 0 {
			c.drainTimeout = d
		}
	}
}

// WithQueueCapacity sets the per-partition record queue capacity.
// Larger values increase memory usage but reduce backpressure pauses.
// Defaults to 64.
func WithQueueCapacity(n int) Option {
	return func(c *config) {
		if n > 0 {
			c.queueCapacity = n
		}
	}
}

// --- Producer Specific Options ---

// WithProducerAcks sets the broker acknowledgment requirement for produced
// records.
func WithProducerAcks(acks kgo.Acks) Option {
	return func(c *config) {
		c.kgoOpts = append(c.kgoOpts, kgo.RequiredAcks(acks))
	}
}
