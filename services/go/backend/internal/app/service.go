package app

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gofrs/uuid/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"go-services/backend/internal/config"
	"go-services/backend/internal/idp/keycloak"
	"go-services/backend/internal/user"
	"go-services/library/transactor"
)

type service struct {
	UserEventCommandService *user.EventCommandService
}

func newService(log *slog.Logger, cfg *config.Config, dbPool *pgxpool.Pool) (*service, error) {
	txAccessor := transactor.NewTxAccessor[pgx.Tx]()
	userRepo := user.NewRepoFromDB(dbPool, txAccessor)

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
		UserEventCommandService: user.NewEventCommandService(log, uuid.NewV4, userRepo, userProvider),
	}, nil
}
