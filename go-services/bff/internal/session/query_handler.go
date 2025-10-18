package session

import (
	"context"
	"net/http"
)

type sessionQueryService interface {
	GetBySessionID(ctx context.Context, sessionID string) (SessionModel, error)
}

type SessionQueryHandler struct {
	sessionQueryService sessionQueryService
}

func NewSessionQueryHandler(sessionQueryService sessionQueryService) *SessionQueryHandler {
	return &SessionQueryHandler{
		sessionQueryService: sessionQueryService,
	}
}

func (s *SessionQueryHandler) GetSessionStatus(w http.ResponseWriter, r *http.Request) {
	// sessionID, err := r.Cookie(shared.SessionTokenCookieName)
	// if err != nil {
	// 	w.w
	// }
	// session, err := s.sessionQueryService.GetBySessionID(r.context())
}
