package transactor

import (
	"context"
	"log/slog"
	"time"

	"go-services/library/apperror"

	"github.com/gofrs/uuid/v5"
	"github.com/jackc/pgx/v5"
)

// PGTxScope is the concrete implementation of the TX interface for PostgreSQL.
// It wraps a pgx.Tx transaction and a domain-specific repository (T)
// that is scoped to that transaction.
type PGTxScope struct {
	log *slog.Logger
	tx  pgx.Tx
	cfg *config
}

// SavePoint creates a named PostgreSQL SAVEPOINT.
//
// It allows for "partial" rollbacks within a transaction. If the provided function 'fn'
// returns an error, the database state is rolled back only to the start of the
// savepoint, allowing the rest of the parent transaction to proceed.
//
// The savepoint name is generated using a UUID v7 to ensure uniqueness and prevent
// collisions in nested or sequential savepoint calls.
func (s *PGTxScope) SavePoint(ctx context.Context, fn func() error) error {
	id, err := uuid.NewV7()
	if err != nil {
		return apperror.New(apperror.CodeInternalError, "failed to generate savepoint ID")
	}

	// Properly quote the identifier to prevent any weird character issues
	spName := pgx.Identifier{"sp_" + id.String()}.Sanitize()

	if _, err := s.tx.Exec(ctx, "SAVEPOINT "+spName); err != nil {
		return apperror.Wrap(apperror.CodeDBTransaction, err, "failed to create savepoint")
	}
	s.log.DebugContext(ctx, "started savepoint", "savepoint", spName)

	// Use a shielded context for cleanup tasks
	// This ensures the ROLLBACK/RELEASE happens even if ctx is canceled
	cleanupCtx, cancel := context.WithTimeout(
		context.WithoutCancel(ctx),
		time.Duration(s.cfg.timeoutSeconds)*time.Second,
	)
	defer cancel()

	err = fn()
	if err != nil {
		// We use cleanupCtx here. If this fails, the whole TX is likely doomed,
		// but we must attempt to return to the savepoint.
		if _, rbErr := s.tx.Exec(cleanupCtx, "ROLLBACK TO SAVEPOINT "+spName); rbErr != nil {
			s.log.ErrorContext(ctx, "failed to rollback to savepoint", "err", rbErr, "savepoint", spName)
		} else {
			s.log.DebugContext(ctx, "rollbacked to savepoint", "savepoint", spName)
		}
		return err
	}

	if _, err := s.tx.Exec(cleanupCtx, "RELEASE SAVEPOINT "+spName); err != nil {
		return apperror.Wrap(apperror.CodeDBTransaction, err, "failed to release savepoint")
	}
	s.log.DebugContext(ctx, "released savepoint", "savepoint", spName)

	return nil
}

var _ TxScope = (*PGTxScope)(nil)
