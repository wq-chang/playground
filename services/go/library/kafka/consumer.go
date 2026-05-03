package kafka

import (
	"context"
	"log/slog"
	"sync"

	"github.com/twmb/franz-go/pkg/kgo"
)

// Handler is the function that processes a single Kafka record.
type Handler func(ctx context.Context, record *kgo.Record) error

// Consumer is a wrapper around franz-go kgo.Client.
// It manages the consumption loop and parallel processing of records.
type Consumer struct {
	cfg    *config
	log    *slog.Logger
	client *kgo.Client
}

// newConsumer creates a new Kafka consumer
func newConsumer(cfg *config, client *kgo.Client) *Consumer {
	return &Consumer{
		cfg:    cfg,
		client: client,
		log:    cfg.logger,
	}
}

// Run starts the consumer loop. It blocks until the context is cancelled or a fatal error occurs.
// It uses a pool of workers to process records in parallel if configured.
func (c *Consumer) Run(ctx context.Context) error {
	c.log.InfoContext(ctx, "Starting Kafka consumer loop",
		"groupId", c.cfg.groupId,
		"workers", c.cfg.workers)

	if err := c.runClient(ctx, c.client); err != nil {
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

func (c *Consumer) handleRecord(ctx context.Context, record *kgo.Record) {
	handler, ok := c.cfg.topicRouter[record.Topic]
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

// Close closes the underlying kafka client and stops any active consumption.
func (c *Consumer) Close() {
	if c.client != nil {
		c.client.Close()
	}
}
