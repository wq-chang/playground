package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"go-services/backend/internal/config"
	"go-services/library/kafka"
)

type App struct {
	Log         *slog.Logger
	repository  *repository
	kafkaClient *kafka.Client
}

func New(ctx context.Context) (*App, error) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.New()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize config: %w", err)
	}

	repository, err := newRepository(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize repositories: %w", err)
	}

	service, err := newService(log, cfg, repository)
	if err != nil {
		repository.Close()
		return nil, fmt.Errorf("failed to initialize services: %w", err)
	}

	kafkaClient, err := newKafkaConsumer(cfg, service)
	if err != nil {
		repository.Close()
		return nil, fmt.Errorf("failed to initialize kafka consumer: %w", err)
	}

	return &App{
		Log:         log,
		repository:  repository,
		kafkaClient: kafkaClient,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	if a == nil || a.kafkaClient == nil || a.kafkaClient.Consumer == nil {
		return fmt.Errorf("app kafka consumer is not initialized")
	}

	return a.kafkaClient.Consumer.Run(ctx)
}

func (a *App) Close(ctx context.Context) error {
	if a == nil {
		return nil
	}
	_ = ctx

	if a.kafkaClient != nil {
		a.kafkaClient.Close()
	}
	if a.repository != nil {
		a.repository.Close()
	}

	return nil
}
