package kafka

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/twmb/franz-go/pkg/kgo"
)

// Handler is the function that processes a single Kafka record.
type Handler func(ctx context.Context, record *kgo.Record) error

// Consumer is a wrapper around franz-go kgo.Client.
// It manages the consumption loop and parallel processing of records.
type Consumer struct {
	topicRouter map[string]Handler
	shared      *sharedClient
	cfg         *config
	log         *slog.Logger
	mu          sync.RWMutex
	closed      bool
}

// newConsumer creates a new Kafka consumer.
//
// Topics already present in cfg.topicRouter came from WithTopic during startup
// configuration. Those topics are already included in the client's initial
// kgo.ConsumeTopics subscription, so they are copied into the in-memory router with
// subscribe=false to avoid re-adding the same Kafka subscription a second time.
func newConsumer(cfg *config, shared *sharedClient) (*Consumer, error) {
	consumer := &Consumer{
		topicRouter: make(map[string]Handler, len(cfg.topicRouter)),
		shared:      shared,
		mu:          sync.RWMutex{},
		closed:      false,
		cfg:         cfg,
		log:         cfg.logger,
	}

	for topic, handler := range cfg.topicRouter {
		if err := consumer.registerTopic(topic, handler, false); err != nil {
			return nil, err
		}
	}

	return consumer, nil
}

// Run starts the consumer loop. It blocks until the context is cancelled or a fatal error occurs.
// It uses a pool of workers to process records in parallel if configured.
func (c *Consumer) Run(ctx context.Context) error {
	c.log.InfoContext(ctx, "Starting Kafka consumer loop",
		"groupId", c.cfg.groupId,
		"workers", c.cfg.workers)

	if c.shared == nil || c.shared.client == nil {
		return fmt.Errorf("consumer client is not initialized")
	}

	if err := c.runClient(ctx, c.shared.client); err != nil {
		if ctx.Err() != nil {
			c.log.InfoContext(ctx, "Kafka consumer context cancelled, shutting down...")
			return ctx.Err()
		}
		c.log.Error("Kafka consumer loop error", "err", err)
		return err
	}
	return nil
}

func (c *Consumer) runClient(ctx context.Context, cl *kgo.Client) error {
	sem := make(chan struct{}, c.cfg.workers)
	for {
		fetches := cl.PollRecords(ctx, -1)
		if fetches.IsClientClosed() {
			return nil
		}
		if err := fetches.Err(); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			c.log.Warn("Kafka poll error", "err", err)
			continue
		}

		records := fetches.Records()
		if len(records) == 0 {
			continue
		}

		if c.cfg.ackMode == AckModeAtMostOnce {
			if err := cl.CommitRecords(ctx, records...); err != nil {
				c.log.ErrorContext(ctx, "failed to commit records (at most once)", "err", err)
				continue
			}
		}

		var recordWg sync.WaitGroup
		aborted := false

		for _, record := range records {
			select {
			case <-ctx.Done():
				aborted = true
			case sem <- struct{}{}:
				recordWg.Add(1)
				go func(r *kgo.Record) {
					defer recordWg.Done()
					defer func() { <-sem }()
					c.handleRecord(ctx, r)
				}(record)
			}

			if aborted {
				break
			}
		}

		recordWg.Wait()

		if c.cfg.ackMode == AckModeAtLeastOnce {
			if err := cl.CommitRecords(ctx, records...); err != nil {
				c.log.ErrorContext(ctx, "failed to commit records (at least once)", "err", err)
			}
		}

		if aborted {
			return ctx.Err()
		}
	}
}

// AddTopic registers a new topic handler and updates the underlying client to
// start consuming the topic immediately.
//
// Unlike startup topics registered through WithTopic, topics added here were not part
// of the client's initial kgo.ConsumeTopics configuration, so AddTopic also calls the
// franz-go runtime subscription API to begin consuming the new topic.
func (c *Consumer) AddTopic(topic string, handler Handler) error {
	return c.registerTopic(topic, handler, true)
}

// registerTopic stores a topic handler in the consumer router.
//
// When subscribe is true, the topic is also added to the underlying franz-go client at
// runtime. When subscribe is false, only the handler router is updated because the
// client is already subscribed from initial construction.
func (c *Consumer) registerTopic(topic string, handler Handler, subscribe bool) error {
	if topic == "" {
		return fmt.Errorf("topic must not be empty")
	}
	if handler == nil {
		return fmt.Errorf("handler must not be nil")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return fmt.Errorf("consumer is closed")
	}
	if _, ok := c.topicRouter[topic]; ok {
		return fmt.Errorf("topic handler already registered for %q", topic)
	}

	c.topicRouter[topic] = handler
	if subscribe && c.shared != nil && c.shared.client != nil {
		c.shared.client.AddConsumeTopics(topic)
	}

	return nil
}

func (c *Consumer) handleRecord(ctx context.Context, record *kgo.Record) {
	handler, ok := c.handlerForTopic(record.Topic)
	if !ok {
		c.log.ErrorContext(ctx, "failed to map topic to handler", "topic", record.Topic)
		return
	}

	if err := handler(ctx, record); err != nil {
		c.log.ErrorContext(ctx, "Handler error",
			"topic", record.Topic,
			"partition", record.Partition,
			"offset", record.Offset,
			"err", err)
	}
}

func (c *Consumer) handlerForTopic(topic string) (Handler, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	handler, ok := c.topicRouter[topic]
	return handler, ok
}

// Close closes the underlying kafka client and stops any active consumption.
func (c *Consumer) Close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	shared := c.shared
	c.mu.Unlock()

	if shared != nil {
		shared.Close()
	}
}
