package user_test

import (
	"testing"

	"github.com/gofrs/uuid/v5"

	"go-services/backend/internal/user/internal/db"
)

func TestRepository(t *testing.T) {
	t.Run("fake repository", func(t *testing.T) {
		contract := RepositoryContract{
			NewRepository: func() (Repository, func()) {
				fakeRepo := &FakeRepository{Users: make(map[uuid.UUID]db.User)}
				cleanup := fakeRepo.Clear
				return fakeRepo, cleanup
			},
		}
		contract.Test(t)
	})

	// t.Run("real repository", func(t *testing.T) {
	// 	contract := RepositoryContract{
	// 		NewRepository: func() Repository {
	// 			return user.UserRepo{}
	// 		},
	// 	}
	// })
}
