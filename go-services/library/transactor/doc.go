// Package transactor provides a generic, context-aware abstraction for database transactions.
//
// The package is designed to solve the problem of "Transaction Propagation"—ensuring that
// multiple repository calls can participate in the same database transaction without
// leaking implementation details (like *sql.Tx or pgx.Tx) into the business logic layer.
//
// # Key Components
//
//   - Transactor: The primary interface used by services to define atomic boundaries.
//   - TxAccessor[T]: A generic utility to retrieve the active transaction handle from a context.
//
// # Usage Example
//
// Define your service with the Transactor interface:
//
//	type UserService struct {
//		tr   transactor.Transactor
//		repo *UserRepo
//	}
//
//	func (s *UserService) UpdateProfile(ctx context.Context, userID int, bio string) error {
//		return s.tr.Atomic(ctx, func(ctx context.Context) error {
//			// All calls inside this function share the same transaction
//			return s.repo.UpdateBio(ctx, userID, bio)
//		})
//	}
//
// # Propagation and Nesting
//
// This package supports nested calls to Atomic. If a transaction already exists in the
// context, the transactor will "join" it rather than starting a new one. The outermost
// call maintains control over the final Commit or Rollback.
//
// # Thread Safety
//
// Implementations provided by this package are safe for concurrent use by multiple goroutines;
// however, the individual transaction handles stored within the context are intended
// to be used sequentially within a single request flow.
package transactor
