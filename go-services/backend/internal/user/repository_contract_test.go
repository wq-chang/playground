package user_test

import (
	"context"
	"testing"

	"github.com/gofrs/uuid/v5"

	"go-services/backend/internal/user/internal/db"
	"go-services/library/apperror"
	"go-services/library/assert"
	"go-services/library/require"
)

type Repository interface {
	CreateUser(context.Context, db.CreateUserParams) error
	GetUserByID(uuid.UUID) (db.User, bool)
}

type RepositoryContract struct {
	NewRepository func() (Repository, func())
}

func (r *RepositoryContract) Test(t *testing.T) {
	repo, cleanup := r.NewRepository()
	cleanup()
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
		assert.NoError(t, err, "no error from inserting user")

		createdUser, ok := repo.GetUserByID(id)
		assert.True(t, ok, "user is created")
		assert.Equal(t, createdUser, want, "new created user")
	})

	cleanup()
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
}
