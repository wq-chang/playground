package kafka

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

const (
	defaultPartitionQueueCapacity = 64
	commitDebounceInterval        = 100 * time.Millisecond
	commitFlushInterval           = 500 * time.Millisecond
)

// Handler processes a single Kafka record.
//
// Returning nil marks the record as successfully handled. Returning an error
// activates the subscription's failure policy.
type Handler func(ctx context.Context, record *kgo.Record) error

// Consumer wraps the shared Kafka client with topic routing, partition worker
// management, and package-managed manual offset commits.
type Consumer struct {
	runCtx         context.Context
	runErr         error
	runCancel      context.CancelFunc
	processSem     chan struct{}
	workers        map[recordKey]*partitionWorker
	client         *Client
	cfg            *config
	log            *slog.Logger
	subscriptions  map[string]Subscription
	pausedTopics   map[string]pausedTopic
	commitSignal   chan struct{}
	dispatchSignal chan struct{}
	runWG          sync.WaitGroup
	mu             sync.RWMutex
	runErrOnce     sync.Once
	commitMu       sync.Mutex
}

type recordKey struct {
	topic     string
	partition int32
}

type pausedTopic struct {
	cause    error
	pausedAt time.Time
}

type partitionBatch struct {
	key          recordKey
	records      []*kgo.Record
	subscription Subscription
}

type recordResult struct {
	cause      error
	resolved   bool
	pauseTopic bool
}

// newConsumer creates a new Kafka consumer.
//
// Subscriptions already present in cfg.subscriptions came from WithSubscription /
// WithTopic during startup configuration. Those topics are already included in the
// client's initial kgo.ConsumeTopics subscription, so they are copied into the
// in-memory router with subscribe=false to avoid re-adding the same Kafka
// subscription a second time.
func newConsumer(cfg *config, client *Client) (*Consumer, error) {
	consumer := &Consumer{
		runCtx:         nil,
		runErr:         nil,
		subscriptions:  make(map[string]Subscription, len(cfg.subscriptions)),
		pausedTopics:   make(map[string]pausedTopic),
		workers:        make(map[recordKey]*partitionWorker),
		client:         client,
		cfg:            cfg,
		log:            cfg.logger,
		runCancel:      nil,
		commitSignal:   nil,
		dispatchSignal: nil,
		processSem:     nil,
		commitMu:       sync.Mutex{},
		mu:             sync.RWMutex{},
		runErrOnce:     sync.Once{},
		runWG:          sync.WaitGroup{},
	}

	for _, subscription := range cfg.subscriptions {
		if err := consumer.registerSubscription(subscription, false); err != nil {
			return nil, err
		}
	}

	return consumer, nil
}

func (c *Consumer) partitionBatches(records []*kgo.Record) ([]partitionBatch, error) {
	grouped := make(map[recordKey]partitionBatch)

	for _, record := range records {
		if c.isTopicPaused(record.Topic) {
			continue
		}

		subscription, ok := c.subscriptionForTopic(record.Topic)
		if !ok {
			return nil, fmt.Errorf("failed to map topic to subscription: %s", record.Topic)
		}

		key := recordKey{
			topic:     record.Topic,
			partition: record.Partition,
		}
		batch, ok := grouped[key]
		if !ok {
			batch = partitionBatch{
				key:          key,
				subscription: subscription,
				records:      make([]*kgo.Record, 0, 1),
			}
		}
		batch.records = append(batch.records, record)
		grouped[key] = batch
	}

	keys := make([]recordKey, 0, len(grouped))
	for key := range grouped {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(left, right recordKey) int {
		switch {
		case left.topic < right.topic:
			return -1
		case left.topic > right.topic:
			return 1
		case left.partition < right.partition:
			return -1
		case left.partition > right.partition:
			return 1
		default:
			return 0
		}
	})

	batches := make([]partitionBatch, 0, len(keys))
	for _, key := range keys {
		batch := grouped[key]
		slices.SortFunc(batch.records, func(left, right *kgo.Record) int {
			switch {
			case left.Offset < right.Offset:
				return -1
			case left.Offset > right.Offset:
				return 1
			default:
				return 0
			}
		})
		batches = append(batches, batch)
	}
	return batches, nil
}

// AddSubscription registers a new topic subscription and updates the underlying
// client to start consuming the topic immediately.
//
// It returns an error if the subscription is invalid, the topic is already
// registered, or the client has been closed.
func (c *Consumer) AddSubscription(subscription Subscription) error {
	return c.registerSubscription(subscription, true)
}

// AddTopic registers a new topic handler using the consumer's default
// acknowledgment mode.
//
// It is a shorthand for AddSubscription with a default Subscription.
func (c *Consumer) AddTopic(topic string, handler Handler) error {
	return c.AddSubscription(newDefaultSubscription(topic, handler, c.cfg.defaultAckMode))
}

// registerSubscription stores a topic subscription in the consumer router.
//
// When subscribe is true, the topic is also added to the underlying franz-go client at
// runtime. When subscribe is false, only the subscription router is updated because the
// client is already subscribed from initial construction.
func (c *Consumer) registerSubscription(subscription Subscription, subscribe bool) error {
	normalized, err := subscription.normalize()
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client != nil && c.client.isClosed() {
		return fmt.Errorf("consumer is closed")
	}
	if _, ok := c.subscriptions[normalized.Topic]; ok {
		return fmt.Errorf("topic handler already registered for %q", normalized.Topic)
	}

	c.subscriptions[normalized.Topic] = normalized
	if subscribe && c.client != nil && c.client.kgoClient != nil {
		c.client.kgoClient.AddConsumeTopics(normalized.Topic)
	}

	return nil
}

func (c *Consumer) subscriptionForTopic(topic string) (Subscription, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	subscription, ok := c.subscriptions[topic]
	return subscription, ok
}
