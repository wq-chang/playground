package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type OIDCProviderConfig struct {
	URL          string
	ClientID     string
	ClientSecret string
	CallbackURL  string
	LogoutURL    string
}

type Config struct {
	Keycloak                *OIDCProviderConfig
	ServerPort              string
	SessionSecret           []byte
	UseHTTPS                bool
	FrontendBaseURL         string
	NatsURL                 string
	NatsKVSessionBucketName string
}

func New() (*Config, error) {
	const (
		keycloakBaseURLKey         = "KEYCLOAK_BASE_URL"
		keycloakRealmKey           = "KEYCLOAK_REALM"
		keycloakClientIDKey        = "KEYCLOAK_CLIENT_ID"
		keycloakClientSecretKey    = "KEYCLOAK_CLIENT_SECRET"
		keycloakCallbackURLKey     = "KEYCLOAK_CALLBACK_URL"
		serverPortKey              = "SERVER_PORT"
		sessionSecretKey           = "SESSION_SECRET"
		useHTTPSKey                = "USE_HTTPS"
		frontendBaseURLKey         = "FRONTEND_BASE_URL"
		natsURLKey                 = "NATS_URL"
		natsKVSessionBucketNameKey = "NATS_KV_SESSION_BUCKET_NAME"
	)
	required := []string{
		keycloakBaseURLKey,
		keycloakRealmKey,
		keycloakClientIDKey,
		keycloakClientSecretKey,
		keycloakCallbackURLKey,
		serverPortKey,
		sessionSecretKey,
		useHTTPSKey,
		frontendBaseURLKey,
		natsURLKey,
		natsKVSessionBucketNameKey,
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

	// Convert SERVER_PORT to int
	_, err := strconv.Atoi(os.Getenv(serverPortKey))
	if err != nil {
		return nil, fmt.Errorf("invalid SERVER_PORT: %w", err)
	}

	frontendBaseURL := os.Getenv(frontendBaseURLKey)

	keycloakBaseURL := os.Getenv(keycloakBaseURLKey)
	keycloakRealm := os.Getenv(keycloakRealmKey)
	keycloakURL := fmt.Sprintf("%s/realms/%s", keycloakBaseURL, keycloakRealm)
	keycloakLogoutURL := fmt.Sprintf(
		"%s/protocol/openid-connect/logout?post_logout_redirect_uri=%s",
		keycloakBaseURL,
		url.QueryEscape(frontendBaseURL),
	)
	keycloak := &OIDCProviderConfig{
		URL:          keycloakURL,
		ClientID:     os.Getenv(keycloakClientIDKey),
		ClientSecret: os.Getenv(keycloakClientSecretKey),
		CallbackURL:  os.Getenv(keycloakCallbackURLKey),
		LogoutURL:    keycloakLogoutURL,
	}

	useHTTPSKeyEnv := os.Getenv(useHTTPSKey)
	useHTTPS, err := strconv.ParseBool(useHTTPSKeyEnv)
	if err != nil {
		return nil, fmt.Errorf("invalid boolean for %s: %v", useHTTPSKey, useHTTPSKeyEnv)
	}

	return &Config{
		Keycloak:                keycloak,
		ServerPort:              os.Getenv(serverPortKey),
		SessionSecret:           []byte(os.Getenv(sessionSecretKey)),
		UseHTTPS:                useHTTPS,
		FrontendBaseURL:         frontendBaseURL,
		NatsURL:                 os.Getenv(natsURLKey),
		NatsKVSessionBucketName: os.Getenv(natsKVSessionBucketNameKey),
	}, nil
}
