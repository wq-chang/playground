package user_test

import (
	"testing"

	"github.com/gofrs/uuid/v5"
	"github.com/jackc/pgx/v5"

	"go-services/backend/internal/user"
	"go-services/backend/internal/user/internal/db"
	"go-services/backend/migrations"
	"go-services/library/require"
	"go-services/library/transactor"
)

func TestRepository(t *testing.T) {
	t.Run("fake repository", func(t *testing.T) {
		contract := RepositoryContract{
			NewRepository: func() (Repository, func()) {
				fakeRepo := &FakeRepository{Users: make(map[uuid.UUID]db.User)}
				cleanup := fakeRepo.Clear
				return fakeRepo, cleanup
			},
		}

		contract.Test(t)
	})

	t.Run("real repository", func(t *testing.T) {
		pg := te.GetPostgres(t)
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
	})
}
