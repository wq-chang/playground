package auth

import (
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

// tokenKeyfunc abstracts the Keyfunc method used by jwt.Parse
// to retrieve the correct cryptographic key for verifying a JWT.
// This allows mocking in tests and decouples the TokenValidator
// from a specific JWKS implementation.
type tokenKeyfunc interface {
	Keyfunc(token *jwt.Token) (any, error)
}

// TokenValidator validates JWT tokens using a JSON Web Key Set (JWKS).
// It fetches and caches the JWKS from the configured URL.
type TokenValidator struct {
	jwks tokenKeyfunc
}

// NewTokenValidator creates a new TokenValidator
func NewTokenValidator(jwks tokenKeyfunc) *TokenValidator {
	return &TokenValidator{
		jwks: jwks,
	}
}

// Validate parses and validates a JWT string using the configured JWKS.
// It returns true if the token is valid, or false with an error if the
// token cannot be parsed or fails validation.
func (v *TokenValidator) Validate(token string) (bool, error) {
	parsed, err := jwt.Parse(token, v.jwks.Keyfunc)
	if err != nil {
		return false, fmt.Errorf("failed to parse token: %w", err)
	}

	return parsed.Valid, nil
}
