package config

import (
	"fmt"
	"os"
	"strings"
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

type KafkaConfig struct {
	Username        string
	Password        string
	UserEventTopic  string
	ConsumerGroupID string
	BrokerURLs      []string
}

// Config is the top-level application configuration structure.
type Config struct {
	DB       *DBConfig
	Keycloak *KeycloakConfig
	Kafka    *KafkaConfig
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
//   - KAFKA_BROKER_URLS:               comma-separated Kafka broker URLs
//   - KAFKA_USERNAME:                  Kafka SASL username
//   - KAFKA_PASSWORD:                  Kafka SASL password
//   - KAFKA_TOPIC_USER_EVENT:          Kafka topic for user events
//   - KAFKA_CONSUMER_GROUP_ID:         Kafka consumer group ID
//
// Example:
//
//	export DATABASE_URL=postgres://backend:secret@localhost:5432/backend
//	export KEYCLOAK_BASE_URL=http://localhost:7777
//	export KEYCLOAK_REALM=playground
//	export KEYCLOAK_BACKEND_CLIENT_ID=backend
//	export KEYCLOAK_BACKEND_CLIENT_SECRET=secret
//	export KAFKA_BROKER_URLS=localhost:9092,localhost:9093
//	export KAFKA_USERNAME=backend
//	export KAFKA_PASSWORD=secret
//	export KAFKA_TOPIC_USER_EVENT=iam.user.event.v1
//	export KAFKA_CONSUMER_GROUP_ID=backend-user-events
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
		kafkaBrokerURLsKey      = "KAFKA_BROKER_URLS"
		kafkaUsernameKey        = "KAFKA_USERNAME"
		kafkaPasswordKey        = "KAFKA_PASSWORD"
		kafkaTopicUserEventKey  = "KAFKA_TOPIC_USER_EVENT"
		kafkaConsumerGroupIDKey = "KAFKA_CONSUMER_GROUP_ID"
	)
	required := []string{
		databaseURLKey,
		keycloakBaseURLKey,
		keycloakRealmKey,
		keycloakClientIDKey,
		keycloakClientSecretKey,
		kafkaBrokerURLsKey,
		kafkaUsernameKey,
		kafkaPasswordKey,
		kafkaTopicUserEventKey,
		kafkaConsumerGroupIDKey,
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

	brokerURLs, err := splitCSVEnv(strings.TrimSpace(os.Getenv(kafkaBrokerURLsKey)))
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", kafkaBrokerURLsKey, err)
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
		Kafka: &KafkaConfig{
			BrokerURLs:      brokerURLs,
			Username:        strings.TrimSpace(os.Getenv(kafkaUsernameKey)),
			Password:        strings.TrimSpace(os.Getenv(kafkaPasswordKey)),
			UserEventTopic:  strings.TrimSpace(os.Getenv(kafkaTopicUserEventKey)),
			ConsumerGroupID: strings.TrimSpace(os.Getenv(kafkaConsumerGroupIDKey)),
		},
	}, nil
}

func splitCSVEnv(value string) ([]string, error) {
	parts := strings.Split(value, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			return nil, fmt.Errorf("must contain only non-empty values")
		}
		values = append(values, trimmed)
	}

	return values, nil
}
