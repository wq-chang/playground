package session

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

type NatsKVSessionCommandRepository struct {
	sessions jetstream.KeyValue
}

func NewNatsKVSessionCommandRepository(
	ctx context.Context,
	jetstream jetstream.JetStream,
	bucketName string,
) (*NatsKVSessionCommandRepository, error) {
	ctxWithTimeout, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	sessions, err := jetstream.KeyValue(ctxWithTimeout, bucketName)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup sessions bucket: %w", err)
	}

	return &NatsKVSessionCommandRepository{
		sessions: sessions,
	}, nil
}

func (s *NatsKVSessionCommandRepository) Put(
	ctx context.Context,
	sessionID string,
	session Session,
) error {
	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	_, err = s.sessions.Put(ctxWithTimeout, sessionID, data)
	if err != nil {
		return fmt.Errorf("failed to put session to kv bucket: %w", err)
	}
	return nil
}

func (s *NatsKVSessionCommandRepository) Delete(ctx context.Context, sessionID string) error {
	ctxWithTimeout, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	err := s.sessions.Delete(ctxWithTimeout, sessionID)
	if err != nil {
		return fmt.Errorf("failed to dete session from kv bucket: %w", err)
	}

	return nil
}
