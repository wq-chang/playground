// services/go/library/kafka/consumer_v2.go
//
// consumerV2 is a temporary v2 façade that will be built alongside the existing
// v1 Consumer during the refactor. It starts as a subscription-storage shell
// with stub runtime methods. As Steps 2–8 extract collaborators into
// internal/consumer, those collaborators are wired here.
//
// When v2 reaches behavioral parity with v1 (Step 9), newConsumer switches to
// newConsumerV2. In Step 10, consumerV2 is renamed to Consumer and v1 files
// are deleted.
package kafka

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"sync"

	"github.com/twmb/franz-go/pkg/kgo"
)

// errV2NotImplemented is returned by stub runtime methods on consumerV2.
var errV2NotImplemented = fmt.Errorf("consumerV2: not yet implemented")

// consumerV2 is the temporary v2 consumer façade.
type consumerV2 struct {
	cfg           *config
	client        *Client
	log           *slog.Logger
	subscriptions map[string]Subscription
	subMu         sync.Mutex
}

// newConsumerV2 creates a v2 consumer from the shared config and client.
func newConsumerV2(cfg *config, client *Client) (*consumerV2, error) {
	v2 := &consumerV2{
		cfg:           cfg,
		client:        client,
		log:           cfg.logger,
		subscriptions: make(map[string]Subscription, len(cfg.subscriptions)),
		subMu:         sync.Mutex{},
	}

	// Convert startup subscriptions from config.
	for _, sub := range cfg.subscriptions {
		normalized, err := sub.Normalize()
		if err != nil {
			return nil, fmt.Errorf("v2 init: %w", err)
		}
		v2.subscriptions[normalized.Topic] = normalized
	}

	return v2, nil
}

// AddSubscription registers a new topic subscription.
func (v2 *consumerV2) AddSubscription(subscription Subscription) error {
	normalized, err := subscription.Normalize()
	if err != nil {
		return err
	}

	v2.subMu.Lock()
	defer v2.subMu.Unlock()

	if v2.client != nil && v2.client.isClosed() {
		return fmt.Errorf("consumer is closed")
	}
	if _, ok := v2.subscriptions[normalized.Topic]; ok {
		return fmt.Errorf("topic handler already registered for %q", normalized.Topic)
	}

	v2.subscriptions[normalized.Topic] = normalized
	if v2.client != nil && v2.client.kgoClient != nil {
		v2.client.kgoClient.AddConsumeTopics(normalized.Topic)
	}

	return nil
}

// AddTopic registers a single-record handler using the default ack mode.
func (v2 *consumerV2) AddTopic(topic string, handler Handler) error {
	return v2.AddSubscription(newDefaultSubscription(topic, handler, v2.cfg.defaultAckMode))
}

// AddBatchTopic registers a batch handler using the default ack mode.
func (v2 *consumerV2) AddBatchTopic(topic string, handler BatchHandler) error {
	return v2.AddSubscription(newDefaultBatchSubscription(topic, handler, v2.cfg.defaultAckMode))
}

// Run starts the consumer loop. STUB — not yet implemented.
func (v2 *consumerV2) Run(ctx context.Context) error {
	v2.log.InfoContext(ctx, "consumerV2.Run called (stub)")
	return errV2NotImplemented
}

// onPartitionsRevoked handles partition revocation. STUB — no-op.
//
//nolint:unused
func (v2 *consumerV2) onPartitionsRevoked(
	ctx context.Context,
	cl *kgo.Client,
	partitions map[string][]int32,
) {
}

// onPartitionsLost handles lost partitions. STUB — no-op.
//
//nolint:unused
func (v2 *consumerV2) onPartitionsLost(
	ctx context.Context,
	partitions map[string][]int32,
) {
}

// subscriptionSnapshot returns an immutable snapshot of the current subscriptions.
//
//nolint:unused
func (v2 *consumerV2) subscriptionSnapshot() map[string]Subscription {
	v2.subMu.Lock()
	defer v2.subMu.Unlock()
	return maps.Clone(v2.subscriptions)
}
