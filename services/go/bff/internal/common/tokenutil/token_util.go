package tokenutil

import (
	"encoding/base64"
)

type secureRand interface {
	Read(b []byte) (n int, err error)
}

type TokenUtil struct {
	rand secureRand
}

func NewTokenUtil(secureRand secureRand) *TokenUtil {
	return &TokenUtil{rand: secureRand}
}

// Generate secure random state token
func (s *TokenUtil) GenerateStateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := s.rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
