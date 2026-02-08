package transactor_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"go-services/library/testenv"

	"github.com/jackc/pgx/v5/pgxpool"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	ctx := context.Background()

	pool, cleanup, err := testenv.GetSharedPool(ctx, "library_transactor")
	if err != nil {
		panic(fmt.Sprintf("failed to start test db: %v", err))
	}

	testPool = pool

	code := m.Run()

	cleanup()

	os.Exit(code)
}
