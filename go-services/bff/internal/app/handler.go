package app

import (
	"log/slog"

	"go-services/bff/internal/auth"
	"go-services/bff/internal/config"
)

type Handler struct {
	AuthCommandHandler *auth.AuthCommandHandler
}

func newHandler(log *slog.Logger, cfg *config.Config, service *service) *Handler {
	authCommandHandler := auth.NewAuthCommandHandler(
		log,
		cfg.Keycloak,
		cfg.UseHTTPS,
		cfg.FrontendBaseURL,
		service.AuthCommandService,
	)

	return &Handler{
		AuthCommandHandler: authCommandHandler,
	}
}
