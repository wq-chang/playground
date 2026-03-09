package testenv

import (
	"context"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestEnv manages the lifecycle of infrastructure dependencies for a specific test package.
// It leverages lazy initialization to ensure resources are only provisioned when requested.
type TestEnv struct {
	pgPool      *pgxpool.Pool
	pgCleanup   func()
	packageName string
	pgOnce      sync.Once
}

// NewTestEnv creates a new environment manager for the given package.
// The packageName is used to scope resources, such as creating a unique
// PostgreSQL schema name to prevent cross-package data contamination.
func NewTestEnv(packageName string) *TestEnv {
	return &TestEnv{
		packageName: packageName,
		pgPool:      nil,
		pgOnce:      sync.Once{},
		pgCleanup:   nil,
	}
}

// GetPGPool returns a connection pool to a PostgreSQL instance.
// It lazily initializes the database container and schema on the first call.
//
// If initialization fails, it calls t.Fatalf to stop execution. Subsequent calls
// return the same pool instance without re-initializing.
func (te *TestEnv) GetPGPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	te.pgOnce.Do(func() {
		ctx := context.Background()
		var err error
		// getPGSharedPool is expected to handle container reuse and schema creation.
		te.pgPool, te.pgCleanup, err = getPGSharedPool(ctx, te.packageName)
		if err != nil {
			t.Fatalf("failed to start test db: %v", err)
		}
	})
	return te.pgPool
}

// Cleanup performs teardown of all initialized services.
// It should typically be called once in TestMain after m.Run() completes.
// If no services were lazily initialized, Cleanup is a no-op.
func (te *TestEnv) Cleanup() {
	if te.pgCleanup != nil {
		te.pgCleanup()
	}
}
