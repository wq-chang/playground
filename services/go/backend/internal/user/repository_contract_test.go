package user_test

import (
	"context"
	"testing"

	"github.com/gofrs/uuid/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"go-services/backend/internal/user/internal/db"
	"go-services/library/apperror"
	"go-services/library/assert"
	"go-services/library/require"
)

type Repository interface {
	CreateUser(context.Context, db.CreateUserParams) error
	GetUserByID(context.Context, uuid.UUID) (db.User, error)
	UpdateUser(context.Context, db.UpdateUserParams) (int64, error)
}

type RepositoryContract struct {
	NewRepository func() (Repository, func())
}

func (r *RepositoryContract) Test(t *testing.T) {
	repo, cleanupData := r.NewRepository()
	cleanupData()
	t.Run("CreateUser can create user", func(t *testing.T) {
		ctx := context.Background()
		id, err := uuid.NewV4()
		require.NoError(t, err, "failed to create uuid")
		want := db.User{
			ID:        id,
			Username:  "username",
			Email:     "email@email.com",
			FirstName: "first",
			LastName:  "last",
		}

		input := db.CreateUserParams{
			ID:        id,
			Username:  "username",
			Email:     "email@email.com",
			FirstName: "first",
			LastName:  "last",
		}

		err = repo.CreateUser(ctx, input)
		assert.NoError(t, err, "failed to create user")

		createdUser, err := repo.GetUserByID(ctx, id)
		require.NoError(t, err, "failed to find new created user")

		assert.Equal(t, createdUser, want, "new created user")
	})

	cleanupData()
	t.Run("CreateUser return error when user exists", func(t *testing.T) {
		ctx := context.Background()
		id, err := uuid.NewV4()
		require.NoError(t, err, "failed to create uuid")
		input := db.CreateUserParams{
			ID:        id,
			Username:  "username",
			Email:     "email@email.com",
			FirstName: "first",
			LastName:  "last",
		}

		err = repo.CreateUser(ctx, input)
		require.NoError(t, err, "no error from inserting user")

		err = repo.CreateUser(ctx, input)
		var appErr *apperror.AppError
		assert.ErrorAs(t, err, &appErr, "duplicate record should hit error")
		assert.Equal(t, appErr.Code, apperror.CodeDuplicateRecord, "duplicate err code")
	})

	cleanupData()
	t.Run("GetUserByID returns error when user not found", func(t *testing.T) {
		ctx := context.Background()
		randomID, err := uuid.NewV4()
		require.NoError(t, err, "failed to create uuid")

		_, err = repo.GetUserByID(ctx, randomID)

		var appErr *apperror.AppError
		if assert.ErrorAs(t, err, &appErr, "should return an AppError for missing records") {
			assert.Equal(t, apperror.CodeNotFound, appErr.Code, "expected 404/NotFound code")
		}
	})

	cleanupData()
	t.Run("UpdateUser updates only the provided fields", func(t *testing.T) {
		ctx := context.Background()
		id, err := uuid.NewV4()
		require.NoError(t, err, "failed to create uuid")

		err = repo.CreateUser(ctx, db.CreateUserParams{
			ID:        id,
			Username:  "username",
			Email:     "email@email.com",
			FirstName: "first",
			LastName:  "last",
		})
		require.NoError(t, err, "failed to create user")

		updatedRows, err := repo.UpdateUser(ctx, db.UpdateUserParams{
			ID:        id,
			Username:  pgtype.Text{},
			FirstName: pgtype.Text{String: "updated-first", Valid: true},
			LastName:  pgtype.Text{},
			Email:     pgtype.Text{String: "updated@email.com", Valid: true},
		})
		require.NoError(t, err, "failed to update user")
		assert.Equal(t, updatedRows, int64(1), "updated rows")

		updatedUser, err := repo.GetUserByID(ctx, id)
		require.NoError(t, err, "failed to get updated user")

		assert.Equal(t, updatedUser, db.User{
			ID:        id,
			Username:  "username",
			Email:     "updated@email.com",
			FirstName: "updated-first",
			LastName:  "last",
		}, "updated user")
	})
}
