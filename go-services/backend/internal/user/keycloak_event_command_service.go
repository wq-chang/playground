package user

import (
	"context"
	"fmt"
	"log/slog"

	"go-services/backend/internal/user/internal/db"
	"go-services/library/apperror"

	"github.com/gofrs/uuid/v5"
)

type userSyncCommandRepository interface {
	// CreateUser(createUserParams postgres.CreateUserParams)
	UpdateUser(context context.Context, updateUserParams db.UpdateUserParams) (int64, error)
}

type KeycloakEventCommandService struct {
	log     *slog.Logger
	genUUID func() (uuid.UUID, error)
	repo    userSyncCommandRepository
}

func NewKeycloakEventService(
	log *slog.Logger,
	genUUID func() (uuid.UUID, error),
	repo userSyncCommandRepository,
) *KeycloakEventCommandService {
	return &KeycloakEventCommandService{
		log:     log,
		genUUID: genUUID,
		repo:    repo,
	}
}

func (s *KeycloakEventCommandService) ProcessEvent(ctx context.Context, event KeycloakEvent) error {
	switch event.EventType {
	case EventTypeUser:
		return s.handleUserEvent(ctx, event)
	case EventTypeAdmin:
		return s.handleAdminEvent(ctx, event)
	default:
		return apperror.New(apperror.CodeNotImplemented, "unsupported event type: %s", event.EventType)
	}
}

func (s *KeycloakEventCommandService) handleUserEvent(ctx context.Context, event KeycloakEvent) error {
	switch event.Operation {
	case OperationCreate:
		// return s.createUser(event)
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

func (s *KeycloakEventCommandService) handleAdminEvent(_ context.Context, event KeycloakEvent) error {
	switch event.Operation {
	case OperationCreate:
		// return s.createAdmin(event)
	case OperationUpdate:
		// return s.updateAdmin(event)
	case OperationDelete:
		// return s.deleteAdmin(event)
	default:
		return apperror.New(apperror.CodeNotImplemented, "unsupported admin operation: %s", event.Operation)
	}
	return nil
}

func (s *KeycloakEventCommandService) updateUser(ctx context.Context, userID uuid.UUID, details UpdatedDetails) error {
	updateUserParams := toUpdateUserParams(userID, details)

	_, err := s.repo.UpdateUser(ctx, updateUserParams)
	if err != nil {
		return fmt.Errorf("failed to update user %s: %w", userID, err)
	}

	return nil
}
