package config

import (
	"cmp"
	"os"
	"testing"
)

const (
	failedSetEnvErrMsg   = "failed to set environment variable: %v"
	failedUnsetEnvErrMsg = "failed to unset environment variable: %v"
)

func TestNewConfig(t *testing.T) {
	t.Run("success - valid env vars", func(t *testing.T) {
		// Arrange
		err1 := os.Setenv("KEYCLOAK_URL", "http://localhost:8080")
		err2 := os.Setenv("KEYCLOAK_REALM", "myrealm")
		if err := cmp.Or(err1, err2); err != nil {
			t.Fatalf(failedSetEnvErrMsg, err)
		}
		defer func() {
			err1 := os.Unsetenv("KEYCLOAK_URL")
			err2 := os.Unsetenv("KEYCLOAK_REALM")
			if err := cmp.Or(err1, err2); err != nil {
				t.Fatalf(failedUnsetEnvErrMsg, err)
			}
		}()

		// Act
		cfg, err := NewConfig()
		// Assert
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		expectedJwks := "http://localhost:8080/realms/myrealm/protocol/openid-connect/certs"
		if cfg.JwksURL != expectedJwks {
			t.Errorf("expected jwks url %s, got %s", expectedJwks, cfg.JwksURL)
		}
		if cfg.KeycloakURL != "http://localhost:8080" {
			t.Errorf("expected KeycloakUrl %s, got %s", "http://localhost:8080", cfg.KeycloakURL)
		}
		if cfg.KeycloakRealm != "myrealm" {
			t.Errorf("expected KeycloakRealm %s, got %s", "myrealm", cfg.KeycloakRealm)
		}
	})

	t.Run("failure - missing KEYCLOAK_URL", func(t *testing.T) {
		// Arrange
		err := os.Setenv("KEYCLOAK_REALM", "myrealm")
		if err != nil {
			t.Fatalf(failedSetEnvErrMsg, err)
		}
		err = os.Unsetenv("KEYCLOAK_URL")
		if err != nil {
			t.Fatalf(failedUnsetEnvErrMsg, err)
		}
		defer func() {
			err := os.Unsetenv("KEYCLOAK_REALM")
			if err != nil {
				t.Fatalf(failedUnsetEnvErrMsg, err)
			}
		}()

		// Act
		cfg, err := NewConfig()

		// Assert
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if cfg != nil {
			t.Errorf("expected nil config, got %+v", cfg)
		}
	})

	t.Run("failure - missing KEYCLOAK_REALM", func(t *testing.T) {
		// Arrange
		err := os.Setenv("KEYCLOAK_URL", "http://localhost:8080")
		if err != nil {
			t.Fatalf(failedSetEnvErrMsg, err)
		}
		err = os.Unsetenv("KEYCLOAK_REALM")
		if err != nil {
			t.Fatalf(failedUnsetEnvErrMsg, err)
		}
		defer func() {
			err := os.Unsetenv("KEYCLOAK_URL")
			if err != nil {
				t.Fatalf(failedUnsetEnvErrMsg, err)
			}
		}()

		// Act
		cfg, err := NewConfig()

		// Assert
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if cfg != nil {
			t.Errorf("expected nil config, got %+v", cfg)
		}
	})
}
