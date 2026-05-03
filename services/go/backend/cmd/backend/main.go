package main

import (
	"context"
	"log/slog"
	"os"

	"go-services/backend/internal/app"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	appl, err := app.New(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to initialize app", "err", err)
		os.Exit(1)
	}
	defer func() {
		err := appl.NatsConnection.Drain()
		if err != nil {
			appl.Log.ErrorContext(ctx, "failed to drain nats connection", "err", err)
			appl.NatsConnection.Close()
		}
		appl.DBPool.Close()
	}()
}
