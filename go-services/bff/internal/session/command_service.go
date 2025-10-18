package session

import (
	"context"
)

type sessionCommandRepository interface {
	Put(ctx context.Context, sessionID string, session Session) error
	Delete(ctx context.Context, sessionID string) error
}

type SessionCommandService struct {
	sessionCommandRepository sessionCommandRepository
}

func NewSessionCommandService(sessionCommandRepository sessionCommandRepository) *SessionCommandService {
	return &SessionCommandService{
		sessionCommandRepository: sessionCommandRepository,
	}
}

func (s *SessionCommandService) Put(
	ctx context.Context,
	createSessionCommand CreateSessionCommand,
) error {
	session := ToSession(createSessionCommand)

	return s.sessionCommandRepository.Put(ctx, createSessionCommand.SessionID, session)
}

func (s *SessionCommandService) Delete(
	ctx context.Context,
	sessionID string,
) error {
	return s.sessionCommandRepository.Delete(ctx, sessionID)
}
