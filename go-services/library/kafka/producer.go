package kafka

import (
	"context"

	"github.com/twmb/franz-go/pkg/kgo"
)

// Producer is a wrapper around franz-go kgo.Client for producing records.
type Producer struct {
	cfg    *config
	client *kgo.Client
}

// newProducer creates a new Kafka producer.
func newProducer(cfg *config, client *kgo.Client) *Producer {
	return &Producer{
		cfg:    cfg,
		client: client,
	}
}

// Produce sends a record to Kafka.
func (p *Producer) Produce(ctx context.Context, record *kgo.Record, promise func(*kgo.Record, error)) {
	p.client.Produce(ctx, record, promise)
}

// ProduceSync sends a record to Kafka and waits for it to be acknowledged.
func (p *Producer) ProduceSync(ctx context.Context, record *kgo.Record) error {
	results := p.client.ProduceSync(ctx, record)
	return results.FirstErr()
}

// Close closes the underlying kafka client.
func (p *Producer) Close() {
	if p.client != nil {
		p.client.Close()
	}
}

// Flush waits for all buffered records to be sent.
func (p *Producer) Flush(ctx context.Context) error {
	return p.client.Flush(ctx)
}
