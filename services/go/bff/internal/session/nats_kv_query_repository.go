package session

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

type NatsKVSessionQueryRepository struct {
	sessions jetstream.KeyValue
}

func NewNatsKVSessionQueryRepository(
	ctx context.Context,
	jetstream jetstream.JetStream,
	bucketName string,
) (*NatsKVSessionQueryRepository, error) {
	ctxWithTimeout, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	sessions, err := jetstream.KeyValue(ctxWithTimeout, bucketName)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup sessions bucket: %w", err)
	}

	return &NatsKVSessionQueryRepository{
		sessions: sessions,
	}, nil
}

func (s *NatsKVSessionQueryRepository) Get(ctx context.Context, sessionID string) (Session, error) {
	ctxWithTimeout, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	entry, err := s.sessions.Get(ctxWithTimeout, sessionID)
	if err != nil {
		return Session{}, fmt.Errorf("failed to get session: %w", err)
	}

	var session Session
	if err := json.Unmarshal(entry.Value(), &session); err != nil {
		return Session{}, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	return session, nil
}
