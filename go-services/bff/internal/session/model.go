package session

import "time"

type CreateSessionCommand struct {
	SessionID    string
	AccessToken  string
	RefreshToken string
	IDToken      string
	ExpiresAt    time.Time
}

type SessionModel struct {
	AccessToken  string
	RefreshToken string
	IDToken      string
	ExpiresAt    time.Time
}
