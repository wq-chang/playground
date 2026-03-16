package testenv

import (
	"context"
	"sync"
	"testing"
)

// TestEnv manages the lifecycle of infrastructure dependencies for a specific test package.
// It leverages lazy initialization to ensure resources are only provisioned when requested.
type TestEnv struct {
	cfg         *config
	postgres    *Postgres
	packageName string
	pgOnce      sync.Once
}

// New creates a new environment manager for the given package.
// The packageName is used to scope resources, such as creating a unique
// PostgreSQL schema name to prevent cross-package data contamination.
func New(packageName string, opts ...Option) *TestEnv {
	cfg := newConfig(opts...)
	return &TestEnv{
		cfg:         cfg,
		packageName: packageName,
		postgres:    nil,
		pgOnce:      sync.Once{},
	}
}

// GetPostgres returns an initialized PostgreSQL environment.
// It lazily initializes the database container and schema on the first call.
//
// If initialization fails, it calls t.Fatalf to stop execution. Subsequent calls
// return the same instance without re-initializing.
func (te *TestEnv) GetPostgres(t *testing.T) *Postgres {
	t.Helper()
	te.pgOnce.Do(func() {
		ctx := context.Background()
		var err error
		// NewPostgres is expected to handle container reuse and schema creation.
		te.postgres, err = NewPostgres(ctx, te.packageName, te.cfg.postgresImage, te.cfg.migrationTableName)
		if err != nil {
			t.Fatalf("failed to start test db: %v", err)
		}
	})
	return te.postgres
}

// Cleanup performs teardown of all initialized services.
// It should typically be called once in TestMain after m.Run() completes.
// If no services were lazily initialized, Cleanup is a no-op.
func (te *TestEnv) Cleanup() {
	if te.postgres != nil && te.postgres.Cleanup != nil {
		te.postgres.Cleanup()
	}
}
