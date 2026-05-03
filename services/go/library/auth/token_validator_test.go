package auth_test

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"go-services/library/auth"

	"github.com/golang-jwt/jwt/v5"
)

type mockKeyfunc struct {
	keyfunc func(token *jwt.Token) (any, error)
}

func (m mockKeyfunc) Keyfunc(token *jwt.Token) (any, error) {
	return m.keyfunc(token)
}

func generateFakeKeycloakToken(t *testing.T) (string, *rsa.PublicKey) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to create private key: %v", err)
	}
	publicKey := &privateKey.PublicKey

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub":                "user-123",
		"iss":                "http://localhost:8080/realms/myrealm",
		"aud":                "account",
		"exp":                time.Now().Add(time.Hour).Unix(),
		"iat":                time.Now().Unix(),
		"preferred_username": "testuser",
		"email":              "test@example.com",
	})

	signedToken, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	return signedToken, publicKey
}

func TestValidate_KeycloakToken(t *testing.T) {
	token, pubKey := generateFakeKeycloakToken(t)
	mockedJWKS := mockKeyfunc{
		keyfunc: func(token *jwt.Token) (any, error) {
			return pubKey, nil
		},
	}

	validator := auth.NewTokenValidator(mockedJWKS)

	valid, err := validator.Validate(token)
	if err != nil {
		t.Fatalf("failed to validate token: %q", err)
	}
	if got, want := valid, true; got != want {
		t.Errorf("got: %v, want: %v", got, want)
	}
}
