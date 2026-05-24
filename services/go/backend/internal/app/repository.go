package app

import (
	"context"
	"fmt"

	"go-services/backend/internal/config"
	"go-services/backend/internal/user"
	"go-services/library/transactor"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type repository struct {
	dbPool         *pgxpool.Pool
	userRepository *user.UserRepo
}

func newRepository(ctx context.Context, cfg *config.Config) (*repository, error) {
	dbPool, err := pgxpool.New(ctx, cfg.DB.ConnectionURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect db: %w", err)
	}

	txAccessor := transactor.NewTxAccessor[pgx.Tx]()
	userRepo := user.NewRepo(dbPool, txAccessor)

	return &repository{
		dbPool:         dbPool,
		userRepository: userRepo,
	}, nil
}

func (r *repository) Close() {
	r.dbPool.Close()
}
