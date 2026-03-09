package transactor_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/jackc/pgx/v5"

	"go-services/library/assert"
	"go-services/library/require"
	"go-services/library/transactor"
)

func TestPGTransactor_Atomic(t *testing.T) {
	testPool := te.GetPGPool(t)
	ctx := context.Background()

	_, insertErr := testPool.Exec(ctx, "DROP TABLE IF EXISTS users;")
	require.NoError(t, insertErr, "failed to run drop user table if exits")
	_, insertErr = testPool.Exec(ctx, "CREATE TABLE users (id SERIAL PRIMARY KEY, name TEXT)")
	require.NoError(t, insertErr, "failed to create user table")

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	tr := transactor.NewPGTransactor(logger, testPool)
	txAccessor := transactor.NewTxAccessor[pgx.Tx]()

	t.Run("commit success", func(t *testing.T) {
		err := tr.Atomic(ctx, func(txCtx context.Context) error {
			tx, _ := txAccessor.GetTx(txCtx)
			_, err := tx.Exec(txCtx, "INSERT INTO users (name) VALUES ($1)", "Alice")
			return err
		})

		assert.NoError(t, err, "failed to insert record to user table")

		var count int
		err = testPool.QueryRow(ctx, "SELECT COUNT(*) FROM users WHERE name = 'Alice'").Scan(&count)
		require.NoError(t, err, "failed to query user table")
		assert.Equal(t, 1, count, "inserted records count")
	})

	t.Run("rollback on error", func(t *testing.T) {
		err := tr.Atomic(ctx, func(txCtx context.Context) error {
			tx, _ := txAccessor.GetTx(txCtx)
			_, insertErr = tx.Exec(txCtx, "INSERT INTO users (name) VALUES ($1)", "Bob")
			require.NoError(t, insertErr, "failed to insert record to user table")
			return errors.New("force error")
		})

		assert.Error(t, err, "atomic should return the error returned by the func")

		var count int
		err = testPool.QueryRow(ctx, "SELECT COUNT(*) FROM users WHERE name = 'Bob'").Scan(&count)
		require.NoError(t, err, "failed to count user table")
		assert.Equal(t, 0, count, "Bob should not exist in DB")
	})

	t.Run("rollback on panic", func(t *testing.T) {
		assert.Panics(t, func() {
			err := tr.Atomic(ctx, func(txCtx context.Context) error {
				tx, _ := txAccessor.GetTx(txCtx)
				_, err := tx.Exec(txCtx, "INSERT INTO users (name) VALUES ($1)", "Charlie")
				require.NoError(t, err, "failed to insert record to user table")
				panic("something went wrong")
			})

			require.NoError(t, err, "should not throw error since the function panicked")
		}, "atomic should recovered from panic")

		var count int
		err := testPool.QueryRow(ctx, "SELECT COUNT(*) FROM users WHERE name = 'Charlie'").Scan(&count)
		require.NoError(t, err, "failed to count user table")
		assert.Equal(t, 0, count, "Charlie should not exist in DB after panic")
	})
}

func TestPGTransactor_AtomicScenarios(t *testing.T) {
	ctx := context.Background()

	testPool := te.GetPGPool(t)

	_, err := testPool.Exec(ctx, "DROP TABLE IF EXISTS users;")
	require.NoError(t, err, "failed to drop user table if exists")
	_, err = testPool.Exec(ctx, "CREATE TABLE users (id SERIAL PRIMARY KEY, name TEXT)")
	require.NoError(t, err, "failed to create user table")

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	tr := transactor.NewPGTransactor(logger, testPool)
	txAccessor := transactor.NewTxAccessor[pgx.Tx]()

	cleanup := func() {
		_, deleteErr := testPool.Exec(ctx, "DELETE FROM users")
		require.NoError(t, deleteErr, "failed to delete records from user table")
	}

	t.Run("partial rollback with savepoint", func(t *testing.T) {
		defer cleanup()
		atomicErr := tr.AtomicWithScope(ctx, func(txCtx context.Context, txScope transactor.TxScope) error {
			tx, _ := txAccessor.GetTx(txCtx)

			_, insertErr := tx.Exec(txCtx, "INSERT INTO users (name) VALUES ($1)", "Dave-Main")
			require.NoError(t, insertErr, "failed to insert user record")

			// We explicitly ignore the error from the savepoint to allow the main tx to continue
			insertErr = txScope.SavePoint(txCtx, func() error {
				_, savePointInsertErr := tx.Exec(txCtx, "INSERT INTO users (name) VALUES ($1)", "Dave-Risky")
				require.NoError(t, savePointInsertErr, "failed to insert %s into user table", "Dave-Risky")
				if savePointInsertErr != nil {
					return savePointInsertErr
				}
				return errors.New("something went wrong in savepoint")
			})
			require.ErrorContains(
				t,
				insertErr,
				"something went wrong in savepoint",
				"SavePoint should return error from the operation function",
			)

			return nil
		})

		assert.NoError(t, atomicErr, "no error from AtomicWithScope")
		verifyUser(t, ctx, "Dave-Main", 1)
		verifyUser(t, ctx, "Dave-Risky", 0)
	})

	// Atomic -> Atomic (Propagation + Full Rollback)
	t.Run("nested atomic rollback kills entire transaction", func(t *testing.T) {
		defer cleanup()
		atomicErr := tr.Atomic(ctx, func(ctx1 context.Context) error {
			tx, _ := txAccessor.GetTx(ctx1)
			_, err = tx.Exec(ctx1, "INSERT INTO users (name) VALUES ($1)", "Outer-Data")
			require.NoError(t, err, "failed to insert user record")

			// Nested call returns an error
			return tr.Atomic(ctx1, func(ctx2 context.Context) error {
				return errors.New("inner failure")
			})
		})

		assert.Error(t, atomicErr, "error from nested atomic")
		verifyUser(t, ctx, "Outer-Data", 0) // The whole thing should be gone
	})

	// Atomic -> AtomicWithScope
	t.Run("upgrade raw atomic to scoped atomic", func(t *testing.T) {
		defer cleanup()
		err := tr.Atomic(ctx, func(ctx1 context.Context) error {
			return tr.AtomicWithScope(ctx1, func(ctx2 context.Context, scope transactor.TxScope) error {
				assert.NotNil(t, scope, "the nested AtomicWithScope should create the scope")
				tx, _ := txAccessor.GetTx(ctx2)
				_, err := tx.Exec(ctx2, "INSERT INTO users (name) VALUES ($1)", "Upgraded-Data")
				return err
			})
		})

		assert.NoError(t, err, "error from nested AtomicWithScope")
		verifyUser(t, ctx, "Upgraded-Data", 1)
	})

	// AtomicWithScope -> AtomicWithScope (Reuse scope)
	t.Run("reuse existing scope in nested call", func(t *testing.T) {
		defer cleanup()
		err := tr.AtomicWithScope(ctx, func(ctx1 context.Context, scope1 transactor.TxScope) error {
			return tr.AtomicWithScope(ctx1, func(ctx2 context.Context, scope2 transactor.TxScope) error {
				assert.EqualOpt(
					t,
					scope1,
					scope2,
					[]cmp.Option{cmpopts.IgnoreUnexported(transactor.PGTxScope{})},
					"the scope of outer AtomicWithScope and nested AtomicWithScope",
				)
				return nil
			})
		})
		assert.NoError(t, err, "error from nested AtomicWithScope")
	})
}

// verifyUser is a helper to check counts in the DB
func verifyUser(t *testing.T, ctx context.Context, name string, expected int) {
	testPool := te.GetPGPool(t)
	var count int
	err := testPool.QueryRow(ctx, "SELECT COUNT(*) FROM users WHERE name = $1", name).Scan(&count)
	require.NoError(t, err, "failed to count user table")
	assert.Equal(t, expected, count, "count mismatch for user: "+name)
}
