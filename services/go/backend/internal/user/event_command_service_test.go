package user_test

import (
	"context"
	"testing"

	"github.com/gofrs/uuid/v5"
	"github.com/guregu/null/v6"

	"go-services/backend/internal/user"
	"go-services/backend/internal/user/internal/db"
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
	service := user.NewEventCommandService(log, uuid.NewV4, repo)

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
	service := user.NewEventCommandService(log, uuid.NewV4, repo)

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
