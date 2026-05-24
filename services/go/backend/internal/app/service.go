package app

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gofrs/uuid/v5"

	"go-services/backend/internal/config"
	"go-services/backend/internal/idp/keycloak"
	"go-services/backend/internal/user"
)

type service struct {
	UserEventCommandService *user.EventCommandService
}

func newService(log *slog.Logger, cfg *config.Config, repo *repository) (*service, error) {
	userProvider, err := keycloak.NewClient(
		keycloak.Config{
			BaseURL:      cfg.Keycloak.BaseURL,
			Realm:        cfg.Keycloak.Realm,
			ClientID:     cfg.Keycloak.ClientID,
			ClientSecret: cfg.Keycloak.ClientSecret,
		},
		http.DefaultClient,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize keycloak client: %w", err)
	}

	return &service{
		UserEventCommandService: user.NewEventCommandService(log, uuid.NewV4, repo.userRepository, userProvider),
	}, nil
}
