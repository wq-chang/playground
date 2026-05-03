package session

import (
	"context"
)

type sessionQueryRepository interface {
	Get(ctx context.Context, sessionID string) (Session, error)
}

type SessionQueryService struct {
	sessionQueryRepository sessionQueryRepository
}

func NewSessionQueryService(sessionQueryRepository sessionQueryRepository) *SessionQueryService {
	return &SessionQueryService{
		sessionQueryRepository: sessionQueryRepository,
	}
}

func (s *SessionQueryService) GetBySessionID(
	ctx context.Context,
	sessionID string,
) (SessionModel, error) {
	session, err := s.sessionQueryRepository.Get(ctx, sessionID)
	if err != nil {
		return SessionModel{}, err
	}

	return ToSessionDTO(session), nil
}
