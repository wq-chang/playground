package user_test

import (
	"context"
	"testing"

	"github.com/gofrs/uuid/v5"
	"github.com/guregu/null/v6"

	"go-services/backend/internal/idp"
	"go-services/backend/internal/user"
	"go-services/backend/internal/user/internal/db"
	"go-services/library/apperror"
	"go-services/library/assert"
	"go-services/library/require"
	"go-services/library/testlogger"
)

func TestProcessEvent_UpdateUser(t *testing.T) {
	tests := map[string]user.UpdatedDetails{
		"update the user": {
			FirstName: null.StringFrom("new first name"),
			LastName:  null.StringFrom("new last name"),
			Username:  null.StringFrom("new username"),
			Email:     null.StringFrom("new email"),
		},
		"update the user and ignore null value": {
			FirstName: null.StringFrom("new first name"),
			LastName:  null.String{},
			Username:  null.StringFrom("new username"),
			Email:     null.String{},
		},
		"should not update the user with null value": {
			FirstName: null.String{},
			LastName:  null.String{},
			Username:  null.String{},
			Email:     null.String{},
		},
	}

	ctx := context.Background()
	log, logCapture := testlogger.New()
	repo := NewFakeRepository()
	service := user.NewEventCommandService(log, uuid.NewV4, repo, nil)

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			existingUser := buildUser(t)
			userID := existingUser.ID
			event := user.Event{
				EventType: user.EventTypeUser,
				Operation: user.OperationUpdate,
				UserID:    userID,
				Updated:   null.ValueFrom(tt),
			}
			expectedUser := db.User{
				ID:        userID,
				FirstName: tt.FirstName.ValueOr(existingUser.FirstName),
				LastName:  tt.LastName.ValueOr(existingUser.LastName),
				Username:  tt.Username.ValueOr(existingUser.Username),
				Email:     tt.Email.ValueOr(existingUser.Email),
			}
			repo.SaveUser(existingUser)

			err := service.ProcessEvent(ctx, event)

			updatedUser, getUserErr := repo.GetUserByID(ctx, userID)
			require.NoError(t, getUserErr, "the user is not created")

			assert.NoError(t, err, "should not have error when processing keycloak update event")
			assert.Equal(t, updatedUser, expectedUser, "updated user")
		})

		repo.Clear()
		logCapture.Reset()
	}
}

func TestProcess_UpdateUser_EmptyDetails(t *testing.T) {
	ctx := context.Background()
	log, _ := testlogger.New()
	repo := NewFakeRepository()
	service := user.NewEventCommandService(log, uuid.NewV4, repo, nil)

	t.Run("return missing details error", func(t *testing.T) {
		userID, err := uuid.NewV4()
		if err != nil {
			t.Fatalf("failed to generate uuid: %v", err)
		}

		event := user.Event{
			EventType: user.EventTypeUser,
			Operation: user.OperationUpdate,
			UserID:    userID,
			Updated:   null.Value[user.UpdatedDetails]{},
		}

		err = service.ProcessEvent(ctx, event)

		assert.NotNil(t, err, "error returned by ProcessEvent")
		assert.StringContains(t, err.Error(), "missing updated details", "error message")
	})
}

func TestProcessEvent_SyncUserFromIDP(t *testing.T) {
	tests := map[string]struct {
		existingUser *db.User
		event        user.Event
	}{
		"user create syncs fetched user": {
			existingUser: nil,
			event: user.Event{
				EventType: user.EventTypeUser,
				Operation: user.OperationCreate,
				Updated:   null.Value[user.UpdatedDetails]{},
				UserID:    uuid.Nil,
			},
		},
		"admin create syncs fetched user": {
			existingUser: nil,
			event: user.Event{
				EventType: user.EventTypeAdmin,
				Operation: user.OperationCreate,
				Updated:   null.Value[user.UpdatedDetails]{},
				UserID:    uuid.Nil,
			},
		},
		"admin update syncs missing user": {
			existingUser: nil,
			event: user.Event{
				EventType: user.EventTypeAdmin,
				Operation: user.OperationUpdate,
				Updated:   null.Value[user.UpdatedDetails]{},
				UserID:    uuid.Nil,
			},
		},
		"admin update refreshes existing user": {
			existingUser: &db.User{
				ID:        uuid.Nil,
				FirstName: "old first",
				LastName:  "old last",
				Username:  "old-username",
				Email:     "old@example.com",
			},
			event: user.Event{
				EventType: user.EventTypeAdmin,
				Operation: user.OperationUpdate,
				Updated:   null.Value[user.UpdatedDetails]{},
				UserID:    uuid.Nil,
			},
		},
	}

	ctx := context.Background()

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			userID, err := uuid.NewV4()
			require.NoError(t, err, "failed to create uuid")

			log, _ := testlogger.New()
			repo := NewFakeRepository()
			provider := NewFakeUserProvider()

			if tt.existingUser != nil {
				existingUser := *tt.existingUser
				existingUser.ID = userID
				repo.SaveUser(existingUser)
			}

			fetchedUser := idp.User{
				ID:        userID,
				FirstName: "new first",
				LastName:  "new last",
				Username:  "new-username",
				Email:     "new@example.com",
			}
			provider.SaveUser(fetchedUser)

			service := user.NewEventCommandService(log, uuid.NewV4, repo, provider)
			event := tt.event
			event.UserID = userID

			err = service.ProcessEvent(ctx, event)

			require.NoError(t, err, "should sync fetched user details")

			syncedUser, getUserErr := repo.GetUserByID(ctx, userID)
			require.NoError(t, getUserErr, "failed to get synced user")
			assert.Equal(t, syncedUser, db.User{
				ID:        userID,
				FirstName: "new first",
				LastName:  "new last",
				Username:  "new-username",
				Email:     "new@example.com",
			}, "synced user")
		})
	}
}

func TestProcessEvent_CreateUser_RequiresIDPProvider(t *testing.T) {
	ctx := context.Background()
	log, _ := testlogger.New()
	repo := NewFakeRepository()
	service := user.NewEventCommandService(log, uuid.NewV4, repo, nil)

	userID, err := uuid.NewV4()
	require.NoError(t, err, "failed to create uuid")

	err = service.ProcessEvent(ctx, user.Event{
		EventType: user.EventTypeUser,
		Operation: user.OperationCreate,
		Updated:   null.Value[user.UpdatedDetails]{},
		UserID:    userID,
	})

	var appErr *apperror.AppError
	if assert.ErrorAs(t, err, &appErr, "expected app error") {
		assert.Equal(t, appErr.Code, apperror.CodeDependencyFailed, "app error code")
	}
}

func buildUser(t *testing.T) db.User {
	userID, err := uuid.NewV4()
	if err != nil {
		t.Fatalf("failed to generate uuid: %v", err)
	}

	return db.User{
		ID:        userID,
		FirstName: "first name",
		LastName:  "last name",
		Username:  "username",
		Email:     "email",
	}
}

type FakeUserProvider struct {
	Users map[uuid.UUID]idp.User
}

func NewFakeUserProvider() *FakeUserProvider {
	return &FakeUserProvider{
		Users: make(map[uuid.UUID]idp.User),
	}
}

func (r *FakeUserProvider) SaveUser(u idp.User) {
	r.Users[u.ID] = u
}

func (r *FakeUserProvider) GetUserByID(_ context.Context, id uuid.UUID) (idp.User, error) {
	u, ok := r.Users[id]
	if !ok {
		return u, apperror.New(apperror.CodeNotFound, "cannot find idp user by id %v", id)
	}

	return u, nil
}
