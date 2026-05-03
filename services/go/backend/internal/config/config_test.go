package config_test

import (
	"os"
	"testing"

	"go-services/backend/internal/config"
	"go-services/library/assert"
)

// Helper to set environment variable
func setEnv(t *testing.T, key, value string) {
	err := os.Setenv(key, value)
	if err != nil {
		t.Fatalf("failed to set env %s", key)
	}
}

// Helper to reset environment variables after each test
func unsetEnv(t *testing.T, keys ...string) {
	for _, k := range keys {
		err := os.Unsetenv(k)
		if err != nil {
			t.Fatalf("failed to unset env %s", k)
		}
	}
}

func TestNew_MissingEnvs(t *testing.T) {
	unsetEnv(t, "DB_URL", "DB_USERNAME", "DB_PASSWORD", "DB_NAME")
	expected := "missing environment variables"

	_, err := config.New()

	assert.ErrorContains(t, err, expected, "expected error when environment variables are missing")
}

func TestNew_Success(t *testing.T) {
	setEnv(t, "DB_URL", "localhost:5432")
	setEnv(t, "DB_USERNAME", "testuser")
	setEnv(t, "DB_PASSWORD", "testpass")
	setEnv(t, "DB_NAME", "testdb")

	defer unsetEnv(t, "DB_URL", "DB_USERNAME", "DB_PASSWORD", "DB_NAME")

	cfg, err := config.New()
	if err != nil {
		t.Fatalf("expected no error when creating config, got %v", err)
	}

	// Create expected struct
	expected := &config.Config{
		DB: &config.DBConfig{
			URL:           "localhost:5432",
			Username:      "testuser",
			Password:      "testpass",
			Name:          "testdb",
			ConnectionURL: "postgres://testuser:testpass@localhost:5432/testdb",
		},
	}

	// Compare the whole struct
	assert.Equal(t, expected, cfg, "failed to create the correct config")
}

// func TestNew_Successa(t *testing.T) {
// 	type a struct {
// 		ID any
// 	}
//
// 	b := a{
// 		ID: "a",
// 	}
// 	c := a{ID: int(64)}
//
// 	var d any
// 	var e any
// 	d = int(2)
// 	e = "abc"
// 	l := []any{"1", int64(2)}
// 	e1 := errors.New("new arror")
//
// 	// Compare the whole struct
// 	assert.Equal(t, b, c, "failed to create the correct config")
// 	tt.Equal(t, b, c, "tt")
//
// 	assert.Equal(t, d, e, "failed to create the correct config")
// 	tt.Equal(t, d, e, "tt")
//
// 	tt.Equal(t, map[string]string{}, nil, "tt")
// 	assert.Contains(t, l, 2)
//
// 	assert.ErrorContains(t, e1, "b")
// }
