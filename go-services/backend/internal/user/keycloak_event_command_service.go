package user

import (
	"context"
	"fmt"
	"log/slog"

	"go-services/backend/internal/postgres"
	"go-services/library/apperror"

	"github.com/gofrs/uuid/v5"
)

type commandRepository interface {
	// CreateUser(createUserParams postgres.CreateUserParams)
	UpdateUser(context context.Context, updateUserParams postgres.UpdateUserParams) error
}

type KeycloakEventCommandService struct {
	log         *slog.Logger
	commandRepo commandRepository
	genUUID     func() (uuid.UUID, error)
}

func NewKeycloakEventService(
	log *slog.Logger,
	commandRepo commandRepository,
	genUUID func() (uuid.UUID, error),
) *KeycloakEventCommandService {
	return &KeycloakEventCommandService{
		log:         log,
		commandRepo: commandRepo,
		genUUID:     genUUID,
	}
}

func (s *KeycloakEventCommandService) ProcessEvent(ctx context.Context, event KeycloakEvent) error {
	switch event.EventType {
	case EventTypeUser:
		return s.handleUserEvent(ctx, event)
	case EventTypeAdmin:
		return s.handleAdminEvent(ctx, event)
	default:
		// TODO: change to apperror
		// Providing context here helps debug unexpected Keycloak configurations
		return fmt.Errorf("unsupported event type: %s", event.EventType)
	}
}

func (s *KeycloakEventCommandService) handleUserEvent(ctx context.Context, event KeycloakEvent) error {
	switch event.Operation {
	case OperationCreate:
		// return s.createUser(event)
	case OperationUpdate:
		if !event.Updated.Valid {
			return apperror.New("missing updated details", apperror.CodeInvalidInput, nil)
		}
		return s.updateUser(ctx, event.UserID, event.Updated.V)
	case OperationDelete:
		// return s.deleteUser(event)
	default:
		// TODO: change to apperror
		// Providing context here helps debug unexpected Keycloak configurations
		return fmt.Errorf("unsupported user operation: %s", event.Operation)
	}
	return nil
}

func (s *KeycloakEventCommandService) handleAdminEvent(ctx context.Context, event KeycloakEvent) error {
	// switch event.Operation {
	// case OperationCreate:
	// 	return s.createAdmin(event)
	// case OperationUpdate:
	// 	return s.updateAdmin(event)
	// case OperationDelete:
	// 	return s.deleteAdmin(event)
	// default:
	// 	return fmt.Errorf("unsupported admin operation: %s", event.Operation)
	// }
	return nil
}

func (s *KeycloakEventCommandService) updateUser(ctx context.Context, userID uuid.UUID, details UpdatedDetails) error {
	updateUserParams := toUpdateUserParams(userID, details)

	err := s.commandRepo.UpdateUser(ctx, updateUserParams)
	if err != nil {
		return fmt.Errorf("failed to update user %s: %w", userID, err)
	}

	return nil
}
