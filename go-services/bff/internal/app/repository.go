package app

import (
	"context"
	"fmt"

	"go-services/bff/internal/config"
	"go-services/bff/internal/session"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type repository struct {
	NatsKVSessionQueryRepository   *session.NatsKVSessionQueryRepository
	NatsKVSessionCommandRepository *session.NatsKVSessionCommandRepository
}

func newRepository(ctx context.Context, cfg *config.Config) (*repository, error) {
	nc, err := nats.Connect(cfg.NatsURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to nats server: %w", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("failed to get JetStream context: %w", err)
	}

	natsKVSessionQueryRepository, err :=
		session.NewNatsKVSessionQueryRepository(ctx, js, cfg.NatsKVSessionBucketName)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize natsKVSessionQueryRepository: %w", err)
	}
	natsKVSessionCommandRepository, err :=
		session.NewNatsKVSessionCommandRepository(ctx, js, cfg.NatsKVSessionBucketName)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize natsKVSessionCommandRepository: %w", err)
	}

	return &repository{
		NatsKVSessionQueryRepository:   natsKVSessionQueryRepository,
		NatsKVSessionCommandRepository: natsKVSessionCommandRepository,
	}, nil
}
