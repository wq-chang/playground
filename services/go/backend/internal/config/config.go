package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/nats-io/nats.go"
)

// DBConfig holds PostgreSQL database configuration details.
type DBConfig struct {
	// ConnectionURL is the full PostgreSQL connection string.
	// Example: postgres://user:pass@localhost:5432/dbname
	ConnectionURL string
}

type KeycloakConfig struct {
	BaseURL      string
	Realm        string
	ClientID     string
	ClientSecret string
}

// Config is the top-level application configuration structure.
type Config struct {
	DB       *DBConfig
	Keycloak *KeycloakConfig
	NatsURL  string
}

// New reads environment variables, validates required fields,
// and constructs a Config instance. It returns an error if any
// required environment variables are missing.
//
// Expected environment variables:
//   - DATABASE_URL:                    full PostgreSQL connection string
//   - KEYCLOAK_BASE_URL:               Keycloak base URL
//   - KEYCLOAK_REALM:                  Keycloak realm
//   - KEYCLOAK_BACKEND_CLIENT_ID:      backend Keycloak client ID
//   - KEYCLOAK_BACKEND_CLIENT_SECRET:  backend Keycloak client secret
//   - NATS_URL:                        optional NATS server URL (defaults to nats.DefaultURL)
//
// Example:
//
//	export DATABASE_URL=postgres://backend:secret@localhost:5432/backend
//	export KEYCLOAK_BASE_URL=http://localhost:7777
//	export KEYCLOAK_REALM=playground
//	export KEYCLOAK_BACKEND_CLIENT_ID=backend
//	export KEYCLOAK_BACKEND_CLIENT_SECRET=secret
//
//	cfg, err := config.New()
//	if err != nil {
//	    log.Fatalf("failed to load config: %v", err)
//	}
//	fmt.Println(cfg.DB.ConnectionURL)
//
// Returns a Config pointer and an error (if any).
func New() (*Config, error) {
	const (
		databaseURLKey          = "DATABASE_URL"
		keycloakBaseURLKey      = "KEYCLOAK_BASE_URL"
		keycloakRealmKey        = "KEYCLOAK_REALM"
		keycloakClientIDKey     = "KEYCLOAK_BACKEND_CLIENT_ID"
		keycloakClientSecretKey = "KEYCLOAK_BACKEND_CLIENT_SECRET"
		natsURLKey              = "NATS_URL"
	)
	required := []string{
		databaseURLKey,
		keycloakBaseURLKey,
		keycloakRealmKey,
		keycloakClientIDKey,
		keycloakClientSecretKey,
	}

	missing := []string{}
	// Check all required envs
	for _, key := range required {
		if os.Getenv(key) == "" {
			missing = append(missing, key)
		}
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("missing environment variables: %s", strings.Join(missing, ", "))
	}

	natsURL := strings.TrimSpace(os.Getenv(natsURLKey))
	if natsURL == "" {
		natsURL = nats.DefaultURL
	}

	return &Config{
		DB: &DBConfig{
			ConnectionURL: strings.TrimSpace(os.Getenv(databaseURLKey)),
		},
		Keycloak: &KeycloakConfig{
			BaseURL:      strings.TrimSpace(os.Getenv(keycloakBaseURLKey)),
			Realm:        strings.TrimSpace(os.Getenv(keycloakRealmKey)),
			ClientID:     strings.TrimSpace(os.Getenv(keycloakClientIDKey)),
			ClientSecret: strings.TrimSpace(os.Getenv(keycloakClientSecretKey)),
		},
		NatsURL: natsURL,
	}, nil
}
