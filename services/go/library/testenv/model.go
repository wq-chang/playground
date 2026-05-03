package testenv

import "github.com/jackc/pgx/v5/pgxpool"

// Postgres represents an initialized PostgreSQL environment for testing.
// It provides a connection pool, cleanup functions, and connection details.
type Postgres struct {
	Pool             *pgxpool.Pool
	Cleanup          func()
	CleanupData      func()
	ConnectionString string
}
