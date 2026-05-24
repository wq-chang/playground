package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"

	"go-services/backend/internal/config"
)

type App struct {
	Config         *config.Config
	Log            *slog.Logger
	Service        *service
	NatsConnection *nats.Conn
	DBPool         *pgxpool.Pool
}

func New(ctx context.Context) (*App, error) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.New()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize config: %w", err)
	}

	nc, err := nats.Connect(cfg.NatsURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect nats: %w", err)
	}

	dbPool, err := pgxpool.New(ctx, cfg.DB.ConnectionURL)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("failed to connect db: %w", err)
	}

	service, err := newService(log, cfg, dbPool)
	if err != nil {
		nc.Close()
		dbPool.Close()
		return nil, fmt.Errorf("failed to initialize services: %w", err)
	}

	return &App{
		Config:         cfg,
		Log:            log,
		Service:        service,
		NatsConnection: nc,
		DBPool:         dbPool,
	}, nil
}

func (a *App) Close(ctx context.Context) error {
	if a == nil {
		return nil
	}

	var drainErr error
	if a.NatsConnection != nil {
		drainErr = a.NatsConnection.Drain()
		if drainErr != nil {
			if a.Log != nil {
				a.Log.ErrorContext(ctx, "failed to drain nats connection", "err", drainErr)
			}
			a.NatsConnection.Close()
		}
	}
	if a.DBPool != nil {
		a.DBPool.Close()
	}

	return drainErr
}
