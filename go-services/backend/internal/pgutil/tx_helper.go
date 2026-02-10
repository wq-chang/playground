package pgutil

import (
	"context"

	"go-services/library/transactor"

	"github.com/jackc/pgx/v5"
)

// Executable is a generic that represents sqlc Queries struct
type Executable[T any] interface {
	WithTx(tx pgx.Tx) T
}

func Resolve[T Executable[T]](ctx context.Context, accessor transactor.TXAccessor[pgx.Tx], defaultQ T) T {
	if tx, ok := accessor.GetTx(ctx); ok {
		return defaultQ.WithTx(tx)
	}

	return defaultQ
}
