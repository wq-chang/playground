package user_test

import (
	"context"

	"go-services/backend/internal/postgres"

	"github.com/gofrs/uuid/v5"
)

type FakeRepository struct {
	Users map[uuid.UUID]postgres.User
}

func NewFakeRepository() *FakeRepository {
	return &FakeRepository{
		Users: make(map[uuid.UUID]postgres.User),
	}
}

func (r *FakeRepository) SaveUser(u postgres.User) {
	r.Users[u.ID] = u
}

func (r *FakeRepository) GetUserByID(id uuid.UUID) (postgres.User, bool) {
	u, ok := r.Users[id]
	return u, ok
}

func (r *FakeRepository) CreateUser(_ context.Context, createUserParams postgres.CreateUserParams) {
	newUser := postgres.User{
		ID:        createUserParams.ID,
		FirstName: createUserParams.FirstName,
		LastName:  createUserParams.LastName,
		Email:     createUserParams.Email,
		Username:  createUserParams.Username,
	}

	r.SaveUser(newUser)
}

func (r *FakeRepository) UpdateUser(_ context.Context, updateUserParams postgres.UpdateUserParams) error {
	existingUser := r.Users[updateUserParams.ID]
	if updateUserParams.FirstName.Valid {
		existingUser.FirstName = updateUserParams.FirstName.String
	}
	if updateUserParams.LastName.Valid {
		existingUser.LastName = updateUserParams.LastName.String
	}
	if updateUserParams.Email.Valid {
		existingUser.Email = updateUserParams.Email.String
	}
	if updateUserParams.Username.Valid {
		existingUser.Username = updateUserParams.Username.String
	}

	r.SaveUser(existingUser)

	return nil
}

func (r *FakeRepository) Clear() {
	clear(r.Users)
}
