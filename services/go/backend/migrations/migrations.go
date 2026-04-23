package migrations

import (
	"database/sql"
	"embed"
	"fmt"

	// Import the pgx stdlib driver to register it with database/sql
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

const MigrationTableName = "goose_db_version"

//go:embed *.sql
var embedMigrations embed.FS

// Apply opens a temporary connection using the pgx driver,
// runs all embedded migrations, and closes the connection.
func Apply(connString string) (err error) {
	// 1. Open the database connection using the 'pgx' driver
	db, err := sql.Open("pgx", connString)
	if err != nil {
		return fmt.Errorf("failed to open db connection: %w", err)
	}

	// Ensure the connection is closed after migrations are finished
	defer func() {
		err = db.Close()
	}()

	// 2. Configure Goose to use the embedded filesystem
	goose.SetBaseFS(embedMigrations)

	// 3. Set the dialect specifically for PostgreSQL
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("failed to set dialect: %w", err)
	}

	// 4. Run migrations from the current directory (root of the embedded FS)
	if err := goose.Up(db, "."); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	return nil
}
