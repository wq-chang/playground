//go:build integration

package user_test

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"go-services/backend/internal/user"
	"go-services/backend/internal/user/internal/db"
	"go-services/backend/migrations"
	"go-services/library/require"
	"go-services/library/transactor"
)

func TestRepositoryWithPostgres(t *testing.T) {
	err := migrations.Apply(pg.ConnectionString)
	require.NoError(t, err, "failed to apply migration")

	querier := db.New(pg.Pool)
	accesstor := transactor.NewTxAccessor[pgx.Tx]()

	contract := RepositoryContract{
		NewRepository: func() (Repository, func()) {
			repo := user.NewRepo(querier, accesstor)
			return repo, pg.CleanupData
		},
	}

	contract.Test(t)
}
