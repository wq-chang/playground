package user_test

import (
	"context"
	"errors"
	"testing"

	"github.com/gofrs/uuid/v5"
	"github.com/guregu/null/v6"
	"github.com/twmb/franz-go/pkg/kgo"

	"go-services/backend/internal/user"
	"go-services/library/assert"
	"go-services/library/require"
)

type fakeEventProcessor struct {
	err      error
	received []user.Event
}

func (f *fakeEventProcessor) ProcessEvent(_ context.Context, event user.Event) error {
	f.received = append(f.received, event)
	return f.err
}

func TestEventConsumerHandleRecord(t *testing.T) {
	t.Run("decodes and forwards event", func(t *testing.T) {
		processor := &fakeEventProcessor{
			err:      nil,
			received: nil,
		}
		consumer := user.NewEventConsumer(processor)

		userID, err := uuid.FromString("00000000-0000-0000-0000-000000000123")
		require.NoError(t, err, "failed to parse uuid")

		record := &kgo.Record{
			Value: []byte(`{"eventType":"USER_EVENT","operation":"CREATE","userId":"00000000-0000-0000-0000-000000000123"}`),
		}

		err = consumer.HandleRecord(context.Background(), record)
		require.NoError(t, err, "expected decoded record to be processed")
		require.Equal(t, len(processor.received), 1, "processed events count")
		assert.Equal(t, processor.received[0], user.Event{
			EventType: user.EventTypeUser,
			Operation: user.OperationCreate,
			Updated:   null.Value[user.UpdatedDetails]{},
			UserID:    userID,
		}, "processed event")
	})

	t.Run("returns decode error for invalid payload", func(t *testing.T) {
		consumer := user.NewEventConsumer(&fakeEventProcessor{
			err:      nil,
			received: nil,
		})

		err := consumer.HandleRecord(context.Background(), &kgo.Record{Value: []byte(`{`)})

		require.ErrorContains(t, err, "failed to decode user event", "expected decode error")
	})

	t.Run("returns processor error", func(t *testing.T) {
		wantErr := errors.New("process failed")
		consumer := user.NewEventConsumer(&fakeEventProcessor{
			err:      wantErr,
			received: nil,
		})

		err := consumer.HandleRecord(context.Background(), &kgo.Record{
			Value: []byte(`{"eventType":"USER_EVENT","operation":"CREATE","userId":"00000000-0000-0000-0000-000000000123"}`),
		})

		require.ErrorIs(t, err, wantErr, "expected processor error")
	})
}
