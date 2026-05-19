package user

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/gofrs/uuid/v5"

	"go-services/backend/internal/idp"
	"go-services/backend/internal/user/internal/db"
	"go-services/library/apperror"
)

type userSyncCommandRepository interface {
	CreateUser(context context.Context, createUserParams db.CreateUserParams) error
	UpdateUser(context context.Context, updateUserParams db.UpdateUserParams) (int64, error)
}

type EventCommandService struct {
	log          *slog.Logger
	genUUID      func() (uuid.UUID, error)
	repo         userSyncCommandRepository
	userProvider idp.UserProvider
}

func NewEventCommandService(
	log *slog.Logger,
	genUUID func() (uuid.UUID, error),
	repo userSyncCommandRepository,
	userProvider idp.UserProvider,
) *EventCommandService {
	return &EventCommandService{
		log:          log,
		genUUID:      genUUID,
		repo:         repo,
		userProvider: userProvider,
	}
}

func (s *EventCommandService) ProcessEvent(ctx context.Context, event Event) error {
	switch event.EventType {
	case EventTypeUser:
		return s.handleUserEvent(ctx, event)
	case EventTypeAdmin:
		return s.handleAdminEvent(ctx, event)
	default:
		return apperror.New(apperror.CodeNotImplemented, "unsupported event type: %s", event.EventType)
	}
}

func (s *EventCommandService) handleUserEvent(ctx context.Context, event Event) error {
	switch event.Operation {
	case OperationCreate:
		return s.syncUserByID(ctx, event.UserID)
	case OperationUpdate:
		if !event.Updated.Valid {
			return apperror.New(apperror.CodeInvalidInput, "missing updated details")
		}
		return s.updateUser(ctx, event.UserID, event.Updated.V)
	case OperationDelete:
		// return s.deleteUser(event)
	default:
		return apperror.New(apperror.CodeNotImplemented, "unsupported user operation: %s", event.Operation)
	}
	return nil
}

func (s *EventCommandService) handleAdminEvent(ctx context.Context, event Event) error {
	switch event.Operation {
	case OperationCreate:
		return s.syncUserByID(ctx, event.UserID)
	case OperationUpdate:
		return s.syncUserByID(ctx, event.UserID)
	case OperationDelete:
		// return s.deleteAdmin(event)
	default:
		return apperror.New(apperror.CodeNotImplemented, "unsupported admin operation: %s", event.Operation)
	}
	return nil
}

func (s *EventCommandService) updateUser(ctx context.Context, userID uuid.UUID, details UpdatedDetails) error {
	updateUserParams := toUpdateUserParams(userID, details)

	_, err := s.repo.UpdateUser(ctx, updateUserParams)
	if err != nil {
		return fmt.Errorf("failed to update user %s: %w", userID, err)
	}

	return nil
}

func (s *EventCommandService) syncUserByID(ctx context.Context, userID uuid.UUID) error {
	if s.userProvider == nil {
		return apperror.New(apperror.CodeDependencyFailed, "idp user provider is not configured")
	}

	fetchedUser, err := s.userProvider.GetUserByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to fetch user %s from idp: %w", userID, err)
	}

	return s.syncFetchedUser(ctx, fetchedUser)
}

func (s *EventCommandService) syncFetchedUser(ctx context.Context, identityUser idp.User) error {
	updateUserParams := toUpdateUserParamsFromIDP(identityUser)

	updatedCount, err := s.repo.UpdateUser(ctx, updateUserParams)
	if err != nil {
		return fmt.Errorf("failed to update user %s: %w", identityUser.ID, err)
	}
	if updatedCount > 0 {
		return nil
	}

	if err := s.repo.CreateUser(ctx, toCreateUserParams(identityUser)); err != nil {
		appErr, ok := apperror.As(err)
		if ok && appErr.Code == apperror.CodeDuplicateRecord {
			updatedCount, updateErr := s.repo.UpdateUser(ctx, updateUserParams)
			if updateErr != nil {
				return fmt.Errorf("failed to update user %s after duplicate create: %w", identityUser.ID, updateErr)
			}
			if updatedCount > 0 {
				return nil
			}

			return apperror.New(
				apperror.CodeConflict,
				"user %s was created concurrently but could not be synchronized",
				identityUser.ID,
			)
		}

		return fmt.Errorf("failed to create user %s: %w", identityUser.ID, err)
	}

	return nil
}
