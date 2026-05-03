package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
)

type App struct {
	Log            *slog.Logger
	NatsConnection *nats.Conn
	DBPool         *pgxpool.Pool
}

func New(ctx context.Context) (*App, error) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect nats: %w", err)
	}

	// js, err := jetstream.New(nc)
	// if err != nil {
	// 	// TODO:
	// }
	// defer cc.Drain()

	dbPool, err := pgxpool.New(context.Background(), os.Getenv("DATABASE_URL"))
	if err != nil {
		return nil, fmt.Errorf("failed to connect db: %w", err)
	}

	return &App{
		Log:            log,
		NatsConnection: nc,
		DBPool:         dbPool,
	}, nil
}
