package user

import (
	"context"

	"github.com/gofrs/uuid/v5"
	"github.com/jackc/pgx/v5"

	"go-services/backend/internal/pgutil"
	"go-services/backend/internal/user/internal/db"
	"go-services/library/transactor"
)

type UserRepo struct {
	queries  *db.Queries
	accessor transactor.TXAccessor[pgx.Tx]
}

func NewRepo(queries *db.Queries, accessor transactor.TXAccessor[pgx.Tx]) *UserRepo {
	return &UserRepo{
		queries:  queries,
		accessor: accessor,
	}
}

func (r *UserRepo) q(ctx context.Context) *db.Queries {
	return pgutil.Resolve(ctx, r.accessor, r.queries)
}

func (r *UserRepo) CreateUser(ctx context.Context, createUserParams db.CreateUserParams) error {
	q := r.q(ctx)
	// TODO:change resource string
	return pgutil.MapError(q.CreateUser(ctx, createUserParams), "test")
}

func (r *UserRepo) GetUserByID(ctx context.Context, id uuid.UUID) (db.User, error) {
	q := r.q(ctx)
	user, err := q.GetUserByID(ctx, id)
	// TODO:change resource string
	return user, pgutil.MapError(err, "test")
}
