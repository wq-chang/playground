package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"go-services/backend/internal/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	appl, err := app.New(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to initialize app", "err", err)
		os.Exit(1)
	}
	defer func() {
		err := appl.Close(ctx)
		if err != nil {
			appl.Log.ErrorContext(ctx, "failed to close app", "err", err)
		}
	}()

	if err := appl.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		appl.Log.ErrorContext(ctx, "backend app run failed", "err", err)
		os.Exit(1)
	}
}
