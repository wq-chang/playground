package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"go-services/bff/internal/config"
)

type App struct {
	Log     *slog.Logger
	Config  *config.Config
	Handler *Handler
}

func NewApp(ctx context.Context) (*App, error) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.NewConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize config: %w", err)
	}

	service, err := newService(ctx, log, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize services: %w", err)
	}

	handler := newHandler(log, cfg, service)

	return &App{
		Log:     log,
		Config:  cfg,
		Handler: handler,
	}, nil
}
