package user_test

import (
	"context"

	"github.com/gofrs/uuid/v5"

	"go-services/backend/internal/user/internal/db"
	"go-services/library/apperror"
)

type FakeRepository struct {
	Users map[uuid.UUID]db.User
}

func NewFakeRepository() *FakeRepository {
	return &FakeRepository{
		Users: make(map[uuid.UUID]db.User),
	}
}

func (r *FakeRepository) SaveUser(u db.User) {
	r.Users[u.ID] = u
}

func (r *FakeRepository) GetUserByID(_ context.Context, id uuid.UUID) (db.User, error) {
	u, ok := r.Users[id]
	if !ok {
		return u, apperror.New(apperror.CodeNotFound, "cannot find the user by id %v", id)
	}

	return u, nil
}

func (r *FakeRepository) CreateUser(_ context.Context, createUserParams db.CreateUserParams) error {
	if _, ok := r.Users[createUserParams.ID]; ok {
		return apperror.New(apperror.CodeDuplicateRecord, "user already exists")
	}

	newUser := db.User{
		ID:        createUserParams.ID,
		FirstName: createUserParams.FirstName,
		LastName:  createUserParams.LastName,
		Email:     createUserParams.Email,
		Username:  createUserParams.Username,
	}

	r.SaveUser(newUser)

	return nil
}

func (r *FakeRepository) UpdateUser(_ context.Context, updateUserParams db.UpdateUserParams) (int64, error) {
	existingUser, ok := r.Users[updateUserParams.ID]

	if !ok {
		return 0, nil
	}

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

	return 1, nil
}

func (r *FakeRepository) Clear() {
	clear(r.Users)
}
