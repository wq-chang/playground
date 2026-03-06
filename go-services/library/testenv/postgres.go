package testenv

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// getPGSharedPool initializes or attaches to a reusable PostgreSQL TestContainer and provides
// a connection pool scoped to a specific schema.
//
// Parameters:
//   - packageName: Used as the schema name (hyphens replaced by underscores).
//
// Behavioral Notes:
//   - Uses 'WithReuseByName' for rapid local test cycles.
//   - The cleanup function drops the schema unless the KEEP_TEST_DB env var is set.
//   - Lifecycle Management: The container is NOT manually terminated by this function.
//     It is managed by Testcontainers' Ryuk sidecar, which will remove the container
//     once the test process (or the parent session) exits. This allows the
//     container to persist across multiple 'go test' runs for speed.
func getPGSharedPool(ctx context.Context, packageName string) (*pgxpool.Pool, func(), error) {
	container, err := postgres.Run(ctx,
		"postgres:18.1-trixie",
		postgres.WithDatabase("shared_db"),
		postgres.WithUsername("user"),
		postgres.WithPassword("pass"),
		postgres.BasicWaitStrategies(),
		testcontainers.WithReuseByName("go_services_db"),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to start/reuse container: %w", err)
	}

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get connection string: %w", err)
	}

	// Root Pool: Minimal admin connection for schema management
	rootCfg, _ := pgxpool.ParseConfig(connStr)
	rootCfg.MaxConns = 1
	rootPool, err := pgxpool.NewWithConfig(ctx, rootCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create root pool: %w", err)
	}

	schemaName := strings.ReplaceAll(packageName, "-", "_")

	// Prepare Schema
	_, err = rootPool.Exec(ctx, fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schemaName))
	if err != nil {
		rootPool.Close()
		return nil, nil, fmt.Errorf("failed to drop old schema: %w", err)
	}
	_, err = rootPool.Exec(ctx, fmt.Sprintf("CREATE SCHEMA %s", schemaName))
	if err != nil {
		rootPool.Close()
		return nil, nil, fmt.Errorf("failed to create schema: %w", err)
	}

	// Scoped Pool: The connection pool for application logic
	scopedConnStr := fmt.Sprintf("%s&search_path=%s", connStr, schemaName)
	scopedCfg, _ := pgxpool.ParseConfig(scopedConnStr)
	scopedCfg.MaxConns = 5

	scopedPool, err := pgxpool.NewWithConfig(ctx, scopedCfg)
	if err != nil {
		rootPool.Close()
		return nil, nil, fmt.Errorf("failed to create scoped pool: %w", err)
	}

	cleanup := func() {
		scopedPool.Close()

		// Skip cleanup if manual inspection is needed
		if strings.ToLower(os.Getenv("KEEP_TEST_DB")) == "true" {
			rootPool.Close()
			return
		}

		_, _ = rootPool.Exec(context.Background(),
			fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schemaName))
		rootPool.Close()
	}

	return scopedPool, cleanup, nil
}
