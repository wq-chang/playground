package transactor

import (
	"context"
)

type (
	// txKey is used to store and retrieve the transaction status (active/inactive)
	// within the context to manage propagation.
	txKey struct{}

	// txScopeKey is used to store the actual database transaction object
	// (e.g., *sql.Tx or pgx.Tx) in the context.
	txScopeKey struct{}
)

// Transactor defines the interface for managing atomic database operations.
// It handles transaction lifecycle, including Begin, Commit, and Rollback.
type Transactor interface {
	// Atomic executes the provided function within a transaction.
	// If a transaction already exists in the context, it joins the existing one.
	Atomic(context.Context, func(context.Context) error) error

	// AtomicWithScope is similar to Atomic but provides a TxScope,
	// allowing for advanced control like manual Savepoints.
	AtomicWithScope(context.Context, func(context.Context, TxScope) error) error
}

// TxScope provides fine-grained control over a transaction's behavior
// while it is in progress, such as creating sub-transactions or savepoints.
type TxScope interface {
	// SavePoint wraps a function in a database savepoint.
	// If the function returns an error, the database rolls back only
	// to this specific point rather than the entire transaction.
	SavePoint(context.Context, func() error) error
}

// TXAccessor defines the interface for retrieving the underlying
// transaction handle from a context. This is used by repositories
// to access the database connection.
//
// T represents the driver-specific transaction type (e.g., pgx.Tx).
type TXAccessor[T any] interface {
	// GetTx attempts to retrieve the transaction handle from the context.
	// Returns the handle and true if found, otherwise zero value and false.
	GetTx(context.Context) (T, bool)
}
