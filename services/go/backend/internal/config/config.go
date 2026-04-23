package config

import (
	"fmt"
	"os"
	"strings"
)

// DBConfig holds PostgreSQL database configuration details.
type DBConfig struct {
	// URL is the base database host and port, e.g. "localhost:5432".
	URL string
	// Username is the PostgreSQL user to connect as.
	Username string
	// Password is the password for the PostgreSQL user.
	Password string
	// Name is the database name to connect to.
	Name string
	// ConnectionURL is the full PostgreSQL connection string,
	// assembled from the other fields.
	// Example: postgres://user:pass@localhost:5432/dbname
	ConnectionURL string
}

// Config is the top-level application configuration structure.
type Config struct {
	DB *DBConfig
}

// New reads environment variables, validates required fields,
// and constructs a Config instance. It returns an error if any
// required environment variables are missing.
//
// Expected environment variables:
//   - DB_URL:       database host and port (e.g., "localhost:5432")
//   - DB_USERNAME:  database username
//   - DB_PASSWORD:  database password
//   - DB_NAME:      database name
//
// Example:
//
//	export DB_URL=localhost:5432
//	export DB_USERNAME=postgres
//	export DB_PASSWORD=secret
//	export DB_NAME=mydb
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
		dbURL      = "DB_URL"
		dbUsername = "DB_USERNAME"
		dbPassword = "DB_PASSWORD"
		dbName     = "DB_NAME"
	)
	required := []string{
		dbURL,
		dbUsername,
		dbPassword,
		dbName,
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

	dbURLEnv := os.Getenv(dbURL)
	dbUsernameEnv := os.Getenv(dbUsername)
	dbPasswordEnv := os.Getenv(dbPassword)
	dbNameEnv := os.Getenv(dbName)
	connectionURL := fmt.Sprintf("postgres://%s:%s@%s/%s", dbUsernameEnv, dbPasswordEnv, dbURLEnv, dbNameEnv)
	db := &DBConfig{
		URL:           dbURLEnv,
		Username:      dbUsernameEnv,
		Password:      dbPasswordEnv,
		Name:          dbNameEnv,
		ConnectionURL: connectionURL,
	}

	return &Config{
		DB: db,
	}, nil
}
