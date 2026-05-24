package user

import (
	"context"

	"github.com/gofrs/uuid/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"go-services/backend/internal/pgutil"
	"go-services/backend/internal/user/internal/db"
	"go-services/library/transactor"
)

type RepoDBTX interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type UserRepo struct {
	queries  *db.Queries
	accessor transactor.TXAccessor[pgx.Tx]
}

func NewRepo(dbtx RepoDBTX, accessor transactor.TXAccessor[pgx.Tx]) *UserRepo {
	return &UserRepo{
		queries:  db.New(dbtx),
		accessor: accessor,
	}
}

func (r *UserRepo) q(ctx context.Context) *db.Queries {
	return pgutil.Resolve(ctx, r.accessor, r.queries)
}

func (r *UserRepo) CreateUser(ctx context.Context, createUserParams db.CreateUserParams) error {
	q := r.q(ctx)
	return pgutil.MapError(q.CreateUser(ctx, createUserParams), "user")
}

func (r *UserRepo) GetUserByID(ctx context.Context, id uuid.UUID) (db.User, error) {
	q := r.q(ctx)
	user, err := q.GetUserByID(ctx, id)
	return user, pgutil.MapError(err, "user")
}

func (r *UserRepo) UpdateUser(ctx context.Context, updateUserParams db.UpdateUserParams) (int64, error) {
	q := r.q(ctx)
	commandTag, err := q.UpdateUser(ctx, updateUserParams)
	if err != nil {
		return 0, pgutil.MapError(err, "user")
	}

	return commandTag.RowsAffected(), nil
}
