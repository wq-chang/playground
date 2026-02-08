package transactor

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"go-services/library/apperror"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PGTransactor implements the Transactor interface specifically for PostgreSQL
// using the pgx/v5 driver. It manages transaction propagation by storing
// pgx.Tx handles within the request context.
type PGTransactor struct {
	log  *slog.Logger
	pool *pgxpool.Pool
	cfg  *config
}

// NewPGTransactor creates a new Postgres-backed transactor.
// It accepts optional configuration for timeouts and other behavioral settings.
func NewPGTransactor(log *slog.Logger, pool *pgxpool.Pool, opts ...Option) *PGTransactor {
	return &PGTransactor{
		log:  log,
		pool: pool,
		cfg:  NewConfig(opts...),
	}
}

// Atomic executes a function within a Postgres transaction.
// If an existing transaction is found in the context, this method reuses it
// (Propagation: Required). If no transaction exists, it begins a new one.
func (t *PGTransactor) Atomic(ctx context.Context, fn func(context.Context) error) error {
	_, _, active := t.getExistingTx(ctx)
	if active {
		return fn(ctx)
	}

	return t.runInTx(ctx, func(txCtx context.Context, _ pgx.Tx) error {
		return fn(txCtx)
	})
}

// AtomicWithScope executes a function providing a TxScope for advanced operations
// like savepoints. If a raw transaction exists but no scope is present, it
// "upgrades" the context by injecting a new PGTxScope.
func (t *PGTransactor) AtomicWithScope(ctx context.Context, fn func(context.Context, TxScope) error) error {
	tx, scope, active := t.getExistingTx(ctx)

	if active {
		// If a scope already exists, use it as-is.
		if scope != nil {
			return fn(ctx, scope)
		}

		// Upgrade a raw transaction to a scoped transaction.
		newScope := &PGTxScope{log: t.log, tx: tx, cfg: t.cfg}
		fullCtx := context.WithValue(ctx, txScopeKey{}, newScope)
		return fn(fullCtx, newScope)
	}

	// Start a completely new scoped transaction.
	return t.runInTx(ctx, func(txCtx context.Context, newTx pgx.Tx) error {
		newScope := &PGTxScope{log: t.log, tx: newTx, cfg: t.cfg}
		fullCtx := context.WithValue(txCtx, txScopeKey{}, newScope)
		return fn(fullCtx, newScope)
	})
}

// getExistingTx is a helper to extract the transaction or scope from the context.
// It prioritizes finding a TxScope over a raw pgx.Tx.
func (t *PGTransactor) getExistingTx(ctx context.Context) (pgx.Tx, TxScope, bool) {
	if scope, ok := ctx.Value(txScopeKey{}).(TxScope); ok {
		if pgScope, ok := scope.(*PGTxScope); ok {
			return pgScope.tx, scope, true
		}
	}

	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx, nil, true
	}

	return nil, nil, false
}

// runInTx handles the boilerplate of transaction lifecycle management.
// It includes:
//  1. Starting the transaction.
//  2. Panic recovery with automatic rollback.
//  3. Error-based rollback.
//  4. Detached context for cleanup (Commit/Rollback) to ensure completion
//     even if the parent request context is cancelled.
func (t *PGTransactor) runInTx(ctx context.Context, action func(context.Context, pgx.Tx) error) (err error) {
	tx, beginErr := t.pool.Begin(ctx)
	if beginErr != nil {
		return apperror.Wrap(apperror.CodeDBTransaction, beginErr, "failed to start tx")
	}

	// Use context.WithoutCancel to ensure Commit/Rollback succeeds even if
	// the client disconnects or the request times out mid-operation.
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx),
		time.Duration(t.cfg.timeoutSeconds)*time.Second)
	defer cancel()

	defer func() {
		if r := recover(); r != nil {
			_ = tx.Rollback(cleanupCtx)
			panic(r) // Re-panic after rolling back to allow standard recovery middleware to catch it.
		}
		if err != nil {
			if rbErr := tx.Rollback(cleanupCtx); rbErr != nil && !isTxClosed(rbErr) {
				t.log.ErrorContext(cleanupCtx, "failed to rollback", "err", rbErr)
			}
		}
	}()

	// Inject the transaction into the context for propagation.
	txCtx := context.WithValue(ctx, txKey{}, tx)

	if err = action(txCtx, tx); err != nil {
		return err
	}

	if err = tx.Commit(cleanupCtx); err != nil {
		return apperror.Wrap(apperror.CodeDBTransaction, err, "commit failed")
	}

	return nil
}

// isTxClosed checks if the error indicates the transaction was already closed.
func isTxClosed(err error) bool {
	return errors.Is(err, pgx.ErrTxClosed)
}

// Verify that PGTransactor implements the Transactor interface.
var _ Transactor = (*PGTransactor)(nil)
