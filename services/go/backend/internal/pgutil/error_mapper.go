package pgutil

import (
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"go-services/library/apperror"
)

// MapError returns a domain-friendly AppError from a pgx error.
func MapError(err error, resource string) error {
	if err == nil {
		return nil
	}

	// 1. Handle pgx "No Rows" (404)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperror.Wrap(apperror.CodeNotFound, err, "%s not found", resource)
	}

	// 2. Handle PostgreSQL specific errors (SQLSTATE)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			return apperror.Wrap(apperror.CodeDuplicateRecord, err, "%s already exists", resource)
		case "23503": // foreign_key_violation
			return apperror.Wrap(apperror.CodeConflict, err, "%s relates to a non-existent record", resource)
		case "23502": // not_null_violation
			return apperror.Wrap(apperror.CodeInvalidInput, err, "%s has missing required fields", resource)
		case "40001": // serialization_failure (concurrency)
			return apperror.Wrap(apperror.CodeConflict, err, "concurrent update for %s, please try again", resource)
		case "57014": // query_canceled (usually timeout)
			return apperror.Wrap(apperror.CodeDBTimeout, err, "database request timed out for %s", resource)
		}
	}

	// 3. Fallback for unknown DB errors (500)
	return apperror.Wrap(apperror.CodeInternalError, err, "unexpected database error for %s", resource)
}
