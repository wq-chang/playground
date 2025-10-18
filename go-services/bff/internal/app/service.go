package app

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"

	"go-services/bff/internal/auth"
	"go-services/bff/internal/common/tokenutil"
	"go-services/bff/internal/config"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type service struct {
	AuthCommandService *auth.AuthCommandService
	// SessinQueryService    *session.SessionQueryService
	// SessionCommandService *session.SessionCommandService
}

func newService(
	ctx context.Context,
	log *slog.Logger,
	cfg *config.Config,
	// repository *repository,
) (*service, error) {
	// sessionQueryService := session.NewSessionQueryService(
	// 	repository.NatsKVSessionQueryRepository)
	// sessionCommandService := session.NewSessionCommandService(
	// 	repository.NatsKVSessionCommandRepository)

	provider, err := oidc.NewProvider(ctx, cfg.Keycloak.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to create OIDC provider: %w", err)
	}
	verifier := provider.VerifierContext(ctx, &oidc.Config{ClientID: cfg.Keycloak.ClientID})
	oauth2Config := &oauth2.Config{
		ClientID:     cfg.Keycloak.ClientID,
		ClientSecret: cfg.Keycloak.ClientSecret,
		RedirectURL:  cfg.Keycloak.CallbackURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}
	tokenGenerator := tokenutil.NewTokenUtil(rand.Reader)

	authCommandService := auth.NewAuthCommandService(
		log,
		cfg.Keycloak,
		oauth2Config,
		verifier,
		tokenGenerator,
	)

	return &service{
		AuthCommandService: authCommandService,
		// SessinQueryService:    sessionQueryService,
		// SessionCommandService: sessionCommandService,
	}, nil
}
