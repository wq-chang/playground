package config

import (
	"fmt"
	"os"
)

type Config struct {
	KeycloakUrl   string
	KeycloakRealm string
	JwksUrl       string
}

func NewConfig() (*Config, error) {
	keycloakURL := os.Getenv("KEYCLOAK_URL")
	keycloakRealm := os.Getenv("KEYCLOAK_REALM")

	if keycloakURL == "" || keycloakRealm == "" {
		return nil, fmt.Errorf("KEYCLOAK_URL or KEYCLOAK_REALM env variable is/are empty")
	}

	jwksUrl := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/certs", keycloakURL, keycloakRealm)

	return &Config{
		KeycloakUrl:   keycloakURL,
		KeycloakRealm: keycloakRealm,
		JwksUrl:       jwksUrl,
	}, nil
}
