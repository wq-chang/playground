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
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"go-services/library/kafka/internal/consumer"
)

// consumerV2 is the temporary v2 consumer façade.
type consumerV2 struct {
	cfg      *config
	client   *Client
	log      *slog.Logger
	router   *consumer.Router
	pauses   *consumer.PauseRegistry
	runState *consumer.RunState
}

// v2RegisterClient adapts consumerV2's Client to consumer.RegisterClient.
type v2RegisterClient struct {
	v2 *consumerV2
}

func (a v2RegisterClient) IsClosed() bool {
	return a.v2.client != nil && a.v2.client.isClosed()
}

func (a v2RegisterClient) AddConsumeTopics(topics ...string) {
	if a.v2.client != nil && a.v2.client.kgoClient != nil {
		a.v2.client.kgoClient.AddConsumeTopics(topics...)
	}
}

// newConsumerV2 creates a v2 consumer from the shared config and client.
func newConsumerV2(cfg *config, client *Client) (*consumerV2, error) {
	v2 := &consumerV2{
		cfg:      cfg,
		client:   client,
		log:      cfg.logger,
		router:   nil,
		pauses:   nil,
		runState: nil,
	}
	regClient := v2RegisterClient{v2: v2}
	v2.router = consumer.NewRouter(regClient)
	v2.pauses = consumer.NewPauseRegistry(time.Now)
	v2.runState = consumer.NewRunState()

	// Register startup subscriptions from config.
	// Subscriptions are already normalized by WithSubscription/WithTopic.
	// The Kafka client is already subscribed via kgo.ConsumeTopics during
	// New(), so use RegisterQuietBatch to avoid redundant AddConsumeTopics
	// and clone the map once instead of per-topic.
	subs := make([]Subscription, 0, len(cfg.subscriptions))
	for _, sub := range cfg.subscriptions {
		subs = append(subs, sub)
	}
	if len(subs) > 0 {
		if err := v2.router.RegisterQuietBatch(subs); err != nil {
			return nil, fmt.Errorf("v2 init: %w", err)
		}
	}

	return v2, nil
}

// AddSubscription registers a new topic subscription.
func (v2 *consumerV2) AddSubscription(subscription Subscription) error {
	normalized, err := subscription.Normalize()
	if err != nil {
		return err
	}

	return v2.router.Register(normalized)
}

// AddTopic registers a single-record handler using the default ack mode.
func (v2 *consumerV2) AddTopic(topic string, handler Handler) error {
	return v2.AddSubscription(newDefaultSubscription(topic, handler, v2.cfg.defaultAckMode))
}

// AddBatchTopic registers a batch handler using the default ack mode.
func (v2 *consumerV2) AddBatchTopic(topic string, handler BatchHandler) error {
	return v2.AddSubscription(newDefaultBatchSubscription(topic, handler, v2.cfg.defaultAckMode))
}

// Run starts the consumer loop.
func (v2 *consumerV2) Run(ctx context.Context) error {
	runCtx, err := v2.runState.Begin()
	if err != nil {
		return err
	}

	pollCtx, stopPolling := mergeRunContexts(ctx, runCtx)
	defer stopPolling()

	// TODO: dispatch loop goes here in Steps 7+.
	// For now, block until either context cancels.
	<-pollCtx.Done()

	// Graceful drain sequence.
	v2.runState.Stop()
	v2.runState.Wait()

	err = v2.runState.Err()
	v2.runState.Reset()

	if err != nil {
		return err
	}
	return ctx.Err()
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
