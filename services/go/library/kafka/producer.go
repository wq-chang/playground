package kafka

import (
	"context"

	"github.com/twmb/franz-go/pkg/kgo"
)

// Producer wraps the shared Kafka client with record publishing helpers.
type Producer struct {
	cfg    *config
	client *Client
}

// newProducer creates a new Kafka producer.
func newProducer(cfg *config, client *Client) *Producer {
	return &Producer{
		cfg:    cfg,
		client: client,
	}
}

// Produce sends a record to Kafka asynchronously and invokes promise when the
// broker acknowledges the record or the send fails.
func (p *Producer) Produce(ctx context.Context, record *kgo.Record, promise func(*kgo.Record, error)) {
	p.client.kgoClient.Produce(ctx, record, promise)
}

// ProduceSync sends a record to Kafka and waits for the send result.
func (p *Producer) ProduceSync(ctx context.Context, record *kgo.Record) error {
	results := p.client.kgoClient.ProduceSync(ctx, record)
	return results.FirstErr()
}

// Flush waits for all buffered records to be sent before returning.
func (p *Producer) Flush(ctx context.Context) error {
	return p.client.kgoClient.Flush(ctx)
}
