package kafka

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"sync"
	"sync/atomic"
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

// BatchHandler processes a topic-partition batch in the polled order.
//
// Returning a zero-value BatchResult marks the whole batch as successfully
// handled. To report a failure after successfully handling a contiguous prefix,
// set Err and FailedAt to the index of the first failed record in the input
// slice.
type BatchHandler func(ctx context.Context, records []*kgo.Record) BatchResult

// BatchResult reports the outcome of a BatchHandler invocation.
//
// The zero value means the whole input batch succeeded. FailedAt is only used
// when Err is non-nil, and must point at the first failed record in the batch.
type BatchResult struct {
	Err      error
	FailedAt int
}

// Consumer wraps the shared Kafka client with topic routing, per-partition state
// management, and package-managed manual offset commits.
type Consumer struct {
	client            *Client
	cfg               *config
	log               *slog.Logger
	runCtx            context.Context
	runCancel         context.CancelFunc
	runErr            error
	processSem        chan struct{}
	commitSignal      chan struct{}
	dispatchSignal    chan struct{}
	subscriptionState atomic.Value
	pausedTopicsState atomic.Value
	subscriptions     map[string]Subscription
	pausedTopics      map[string]pausedTopic
	partitionStates   map[recordKey]*partitionState
	dirtyStates       map[recordKey]*partitionState
	subscriptionMu    sync.Mutex
	pausedTopicsMu    sync.Mutex
	runMu             sync.RWMutex
	workersMu         sync.RWMutex
	commitMu          sync.Mutex
	runWG             sync.WaitGroup
	runErrOnce        sync.Once
}

type recordKey struct {
	topic     string
	partition int32
}

type pausedTopic struct {
	cause    error
	pausedAt time.Time
}

type partitionRecordBatch struct {
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
// Subscriptions already present in cfg.subscriptions came from
// WithSubscription / WithTopic / WithBatchTopic during startup configuration.
// Those topics are already included in the client's initial kgo.ConsumeTopics
// subscription, so they are copied into the in-memory router with subscribe=false
// to avoid re-adding the same Kafka subscription a second time.
func newConsumer(cfg *config, client *Client) (*Consumer, error) {
	consumer := &Consumer{
		client:            client,
		cfg:               cfg,
		log:               cfg.logger,
		runCtx:            nil,
		runCancel:         nil,
		runErr:            nil,
		processSem:        nil,
		commitSignal:      nil,
		dispatchSignal:    nil,
		subscriptionState: atomic.Value{},
		pausedTopicsState: atomic.Value{},
		subscriptions:     make(map[string]Subscription, len(cfg.subscriptions)),
		pausedTopics:      make(map[string]pausedTopic),
		partitionStates:   make(map[recordKey]*partitionState),
		dirtyStates:       make(map[recordKey]*partitionState),
		commitMu:          sync.Mutex{},
		runMu:             sync.RWMutex{},
		workersMu:         sync.RWMutex{},
		subscriptionMu:    sync.Mutex{},
		pausedTopicsMu:    sync.Mutex{},
		runErrOnce:        sync.Once{},
		runWG:             sync.WaitGroup{},
	}
	consumer.subscriptionState.Store(map[string]Subscription{})
	consumer.pausedTopicsState.Store(map[string]pausedTopic{})

	for _, subscription := range cfg.subscriptions {
		if err := consumer.registerSubscription(subscription, false); err != nil {
			return nil, err
		}
	}

	return consumer, nil
}

func (c *Consumer) partitionBatches(records []*kgo.Record) ([]partitionRecordBatch, error) {
	subscriptions := c.subscriptionSnapshot()
	pausedTopics := c.pausedTopicSnapshot()
	indexByKey := make(map[recordKey]int)
	batches := make([]partitionRecordBatch, 0)

	for _, record := range records {
		if _, paused := pausedTopics[record.Topic]; paused {
			continue
		}

		subscription, ok := subscriptions[record.Topic]
		if !ok {
			return nil, fmt.Errorf("failed to map topic to subscription: %s", record.Topic)
		}

		key := recordKey{
			topic:     record.Topic,
			partition: record.Partition,
		}
		index, ok := indexByKey[key]
		if !ok {
			index = len(batches)
			indexByKey[key] = index
			batches = append(batches, partitionRecordBatch{
				key:          key,
				subscription: subscription,
				records:      make([]*kgo.Record, 0, 1),
			})
		}
		batches[index].records = append(batches[index].records, record)
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

// AddTopic registers a new single-record topic handler using the consumer's
// default acknowledgment mode.
//
// It is a shorthand for AddSubscription with a default Subscription.
func (c *Consumer) AddTopic(topic string, handler Handler) error {
	return c.AddSubscription(newDefaultSubscription(topic, handler, c.cfg.defaultAckMode))
}

// AddBatchTopic registers a new batch topic handler using the consumer's
// default acknowledgment mode.
//
// It is a shorthand for AddSubscription with a default Subscription.
func (c *Consumer) AddBatchTopic(topic string, handler BatchHandler) error {
	return c.AddSubscription(newDefaultBatchSubscription(topic, handler, c.cfg.defaultAckMode))
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

	c.subscriptionMu.Lock()
	defer c.subscriptionMu.Unlock()

	if c.client != nil && c.client.isClosed() {
		return fmt.Errorf("consumer is closed")
	}
	if _, ok := c.subscriptions[normalized.Topic]; ok {
		return fmt.Errorf("topic handler already registered for %q", normalized.Topic)
	}

	c.subscriptions[normalized.Topic] = normalized
	c.subscriptionState.Store(maps.Clone(c.subscriptions))
	if subscribe && c.client != nil && c.client.kgoClient != nil {
		c.client.kgoClient.AddConsumeTopics(normalized.Topic)
	}

	return nil
}

func (c *Consumer) subscriptionForTopic(topic string) (Subscription, bool) {
	subscription, ok := c.subscriptionSnapshot()[topic]
	return subscription, ok
}

func (c *Consumer) subscriptionSnapshot() map[string]Subscription {
	value := c.subscriptionState.Load()
	if value == nil {
		return map[string]Subscription{}
	}
	snapshot, ok := value.(map[string]Subscription)
	if !ok || snapshot == nil {
		return map[string]Subscription{}
	}
	return snapshot
}

func (c *Consumer) pausedTopicSnapshot() map[string]pausedTopic {
	value := c.pausedTopicsState.Load()
	if value == nil {
		return map[string]pausedTopic{}
	}
	snapshot, ok := value.(map[string]pausedTopic)
	if !ok || snapshot == nil {
		return map[string]pausedTopic{}
	}
	return snapshot
}
