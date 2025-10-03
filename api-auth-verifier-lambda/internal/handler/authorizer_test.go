package handler

import (
	"context"
	"errors"
	testutil "go-library/testutils"
	"strings"
	"testing"

	"api-auth-verifier/internal/config"

	"github.com/aws/aws-lambda-go/events"
)

type mockValidator struct {
	valid bool
	err   error
}

func (m *mockValidator) Validate(token string) (bool, error) {
	return m.valid, m.err
}

func TestHandlerRequest(t *testing.T) {
	tests := []struct {
		name      string
		headers   map[string]string
		validator tokenValidator
		wantCode  int
		wantBody  string
		wantLog   string
	}{
		{
			name:      "missing Authorization header",
			headers:   map[string]string{},
			validator: &mockValidator{},
			wantCode:  401,
			wantBody:  "missing Authorization header",
			wantLog:   "",
		},
		{
			name:      "invalid Authorization header format",
			headers:   map[string]string{"Authorization": "Foo token"},
			validator: &mockValidator{},
			wantCode:  401,
			wantBody:  "invalid Authorization header format",
			wantLog:   "",
		},
		{
			name:      "validation error",
			headers:   map[string]string{"Authorization": "Bearer sometoken"},
			validator: &mockValidator{err: errors.New("jwks fetch failed")},
			wantCode:  500,
			wantBody:  "failed to validate token",
			wantLog:   "token validation failed",
		},
		{
			name:      "invalid token",
			headers:   map[string]string{"Authorization": "Bearer sometoken"},
			validator: &mockValidator{valid: false, err: nil},
			wantCode:  401,
			wantBody:  "invalid token",
			wantLog:   "",
		},
		{
			name:      "valid token",
			headers:   map[string]string{"Authorization": "Bearer sometoken"},
			validator: &mockValidator{valid: true, err: nil},
			wantCode:  200,
			wantBody:  "success",
			wantLog:   "",
		},
		{
			name:      "valid token with lowercase header",
			headers:   map[string]string{"authorization": "Bearer sometoken"},
			validator: &mockValidator{valid: true, err: nil},
			wantCode:  200,
			wantBody:  "success",
			wantLog:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log, buf := testutil.NewFakeLogger()
			resp, _ := HandlerRequest(
				context.Background(),
				log,
				&config.Config{},
				events.APIGatewayProxyRequest{Headers: tt.headers},
				tt.validator,
			)

			if resp.StatusCode != tt.wantCode {
				t.Errorf("got status %d, want %d", resp.StatusCode, tt.wantCode)
			}
			if resp.Body != tt.wantBody {
				t.Errorf("got body %q, want %q", resp.Body, tt.wantBody)
			}
			if tt.wantLog != "" && !strings.Contains(buf.String(), "token validation failed") {
				t.Errorf("got log %q, want %q", buf.String(), tt.wantLog)
			}
			if tt.wantLog == "" && strings.TrimSpace(buf.String()) != "" {
				t.Errorf("expected no logs, got %q", buf.String())
			}
		})
	}
}
