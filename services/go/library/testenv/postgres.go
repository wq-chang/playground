package testenv

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// NewPostgres initializes or attaches to a reusable PostgreSQL TestContainer and provides
// a connection pool scoped to a specific PostgreSQL schema.
//
// Parameters:
//   - packageName: Used as the schema name (hyphens replaced by underscores). This ensures
//     isolation when multiple packages share the same physical container.
//   - imageName: The Docker image to use (e.g., "postgres:16-alpine").
//   - migrationTableName: The name of the migration version table (e.g., "goose_db_version").
//     This table will be excluded from the data cleanup (truncation) process.
//
// Behavioral Notes:
//   - Reuse Logic: Uses 'WithReuseByName' with a name derived from the imageName.
//     This ensures that if you upgrade the requested image version, a fresh container
//     is started instead of reusing an outdated one.
//   - Isolation: Every call creates a fresh schema named after the package.
//     Existing schemas with the same name are dropped to ensure a clean state.
//   - Search Path: The returned pool is configured with 'search_path', so all
//     queries/migrations automatically target the package-specific schema.
//   - Lifecycle Management: The container is managed by Testcontainers' Ryuk sidecar.
//     It persists across 'go test' runs for speed but is automatically reaped when the Docker session or parent process terminates.
//   - The cleanup function drops the schema unless the KEEP_TEST_DB env var is set.
func NewPostgres(ctx context.Context, packageName, imageName, migrationTableName string) (*Postgres, error) {
	reuseName := SanitizeContainerName("pg", imageName)
	container, err := postgres.Run(ctx,
		imageName,
		postgres.WithDatabase("shared_db"),
		postgres.WithUsername("user"),
		postgres.WithPassword("pass"),
		postgres.BasicWaitStrategies(),
		testcontainers.WithReuseByName(reuseName),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start/reuse container: %w", err)
	}

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return nil, fmt.Errorf("failed to get connection string: %w", err)
	}

	// Root Pool: Minimal admin connection for schema management
	rootCfg, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}
	rootCfg.MaxConns = 1
	rootPool, err := pgxpool.NewWithConfig(ctx, rootCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create root pool: %w", err)
	}

	schemaName := strings.ReplaceAll(packageName, "-", "_")

	// Prepare Schema
	_, err = rootPool.Exec(ctx, fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schemaName))
	if err != nil {
		rootPool.Close()
		return nil, fmt.Errorf("failed to drop old schema: %w", err)
	}
	_, err = rootPool.Exec(ctx, fmt.Sprintf("CREATE SCHEMA %s", schemaName))
	if err != nil {
		rootPool.Close()
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	// Scoped Pool: The connection pool for application logic
	scopedConnStr := fmt.Sprintf("%s&search_path=%s", connStr, schemaName)
	scopedCfg, err := pgxpool.ParseConfig(scopedConnStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse scoped connection string: %w", err)
	}
	scopedCfg.MaxConns = 5

	scopedPool, err := pgxpool.NewWithConfig(ctx, scopedCfg)
	if err != nil {
		rootPool.Close()
		return nil, fmt.Errorf("failed to create scoped pool: %w", err)
	}

	cleanup := func() {
		scopedPool.Close()

		// Skip cleanup if manual inspection is needed
		if strings.ToLower(os.Getenv("KEEP_TEST_DB")) == "true" {
			rootPool.Close()
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err := rootPool.Exec(ctx, fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schemaName))
		if err != nil {
			fmt.Fprintf(os.Stderr, "testenv: SCHEMA DROP failed: %v\n", err)
		}
		rootPool.Close()
	}

	cleanupData := func() {
		query := "SELECT tablename FROM pg_tables WHERE schemaname = $1"
		args := []any{schemaName}
		if migrationTableName != "" {
			query += " AND tablename != $2"
			args = append(args, migrationTableName)
		}

		rows, err := scopedPool.Query(context.Background(), query, args...)
		if err != nil {
			return
		}
		defer rows.Close()

		var tables []string
		for rows.Next() {
			var t string
			if err := rows.Scan(&t); err == nil {
				tables = append(tables, fmt.Sprintf("\"%s\"", t))
			}
		}

		if len(tables) > 0 {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			sql := fmt.Sprintf("TRUNCATE TABLE %s CASCADE", strings.Join(tables, ", "))
			if _, err := scopedPool.Exec(ctx, sql); err != nil {
				fmt.Fprintf(os.Stderr, "testenv: TRUNCATE failed: %v\n", err)
			}
		}
	}

	return &Postgres{
		Pool:             scopedPool,
		Cleanup:          cleanup,
		CleanupData:      cleanupData,
		ConnectionString: scopedConnStr,
	}, nil
}
