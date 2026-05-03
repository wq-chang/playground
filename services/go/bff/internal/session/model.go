package session

import "time"

type CreateSessionCommand struct {
	ExpiresAt    time.Time
	SessionID    string
	AccessToken  string
	RefreshToken string
	IDToken      string
}

type SessionModel struct {
	ExpiresAt    time.Time
	AccessToken  string
	RefreshToken string
	IDToken      string
}
