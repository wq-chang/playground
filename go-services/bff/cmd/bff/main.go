package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"go-services/bff/internal/api/middleware"
	"go-services/bff/internal/app"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	appl, err := app.NewApp(ctx)
	if err != nil {
		slog.Error("failed to initialize app", "err", err)
		os.Exit(1)
	}

	cors, err := middleware.CORS(appl.Log, appl.Config.FrontendBaseURL)
	if err != nil {
		slog.Error("failed to initialize cors middleware", "err", err)
		os.Exit(1)
	}
	errMw := middleware.Error(appl.Log)

	chain := middleware.NewChain()
	chain.Add(
		middleware.Logging(appl.Log),
		cors,
		middleware.RequireJSON,
		middleware.Recover(appl.Log),
	)

	mux := http.NewServeMux()
	// Auth routes
	mux.HandleFunc("GET /auth/login", errMw(appl.Handler.AuthCommandHandler.LoginHandler))
	mux.HandleFunc("GET /auth/callback", errMw(appl.Handler.AuthCommandHandler.CallbackHandler))
	mux.HandleFunc("GET /auth/logout", appl.Handler.AuthCommandHandler.LogutoutHandler)
	// mux.HandleFunc("POST /auth/logout", corsMiddleware(sessionHandler.LogoutHandler))
	// mux.HandleFunc("/auth/refresh", corsMiddleware(sessionHandler.RefreshHandler))

	// Protected routes
	// http.HandleFunc("/api/protected", sessionHandler.ProtectedHandler)

	addr := fmt.Sprintf(":%v", appl.Config.ServerPort)
	appl.Log.Info("BFF server starting", "port", appl.Config.ServerPort)

	if err := http.ListenAndServe(addr, chain.Apply(mux)); err != nil {
		appl.Log.Error("server failed:", "err", err)
		os.Exit(1)
	}
}
