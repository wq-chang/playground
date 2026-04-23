package transactor

import "context"

// ContextTxAccessor is a generic implementation of the TXAccessor interface.
// It provides a type-safe way to extract a database transaction handle from
// a context, regardless of the underlying database driver.
type ContextTxAccessor[T any] struct{}

// NewTxAccessor creates a new instance of ContextTxAccessor for a specific
// transaction type (e.g., pgx.Tx or *sql.Tx).
func NewTxAccessor[T any]() *ContextTxAccessor[T] {
	return &ContextTxAccessor[T]{}
}

// GetTx retrieves the transaction handle from the provided context.
// It looks for the private txKey used by the Transactor.
//
// Returns:
//   - The transaction handle of type T and 'true' if a valid transaction exists.
//   - The zero value of T and 'false' if no transaction is found or if the
//     type does not match T.
func (t *ContextTxAccessor[T]) GetTx(ctx context.Context) (T, bool) {
	val := ctx.Value(txKey{})
	if val == nil {
		var zero T
		return zero, false
	}

	// Type assertion ensures we return the specific driver type requested.
	tx, ok := val.(T)
	if ok {
		return tx, true
	}

	var zero T
	return zero, false
}

// Ensure ContextTxAccessor satisfies the TXAccessor interface.
var _ TXAccessor[any] = (*ContextTxAccessor[any])(nil)
