package user

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/twmb/franz-go/pkg/kgo"
)

type eventProcessor interface {
	ProcessEvent(context.Context, Event) error
}

type EventConsumer struct {
	processor eventProcessor
}

func NewEventConsumer(processor eventProcessor) *EventConsumer {
	return &EventConsumer{processor: processor}
}

func (c *EventConsumer) HandleRecord(ctx context.Context, record *kgo.Record) error {
	var event Event
	if err := json.Unmarshal(record.Value, &event); err != nil {
		return fmt.Errorf("failed to decode user event: %w", err)
	}

	return c.processor.ProcessEvent(ctx, event)
}
