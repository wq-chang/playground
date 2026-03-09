package session

import "time"

type Session struct {
	ExpiresAt    time.Time
	AccessToken  string
	RefreshToken string
	IDToken      string
}
