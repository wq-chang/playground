package middleware_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"go-services/bff/internal/api/middleware"
	"go-services/library/assert"
)

func TestCORS(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	trustedOrigin := "https://example.com"

	corsMiddleware, err := middleware.CORS(log, trustedOrigin)
	if err != nil {
		t.Fatalf("failed to create CORS middleware: %v", err)
	}

	handlerCalled := false
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("OK")); err != nil {
			t.Fatalf("failed to write body")
		}
	})

	handler := corsMiddleware(testHandler)

	wantHeaders := map[string]string{
		"Access-Control-Allow-Origin":      trustedOrigin,
		"Access-Control-Allow-Methods":     "GET, POST, PUT, PATCH, DELETE, OPTIONS",
		"Access-Control-Allow-Headers":     "Content-Type, Authorization",
		"Access-Control-Allow-Credentials": "true",
	}

	tests := map[string]struct {
		method         string
		origin         string
		wantStatus     int
		wantBody       string
		shouldCallNext bool
	}{
		"GET request with trusted origin": {
			method:         http.MethodGet,
			origin:         trustedOrigin,
			wantStatus:     http.StatusOK,
			wantBody:       "OK",
			shouldCallNext: true,
		},
		"OPTIONS preflight request": {
			method:         http.MethodOptions,
			origin:         trustedOrigin,
			wantStatus:     http.StatusNoContent,
			wantBody:       "",
			shouldCallNext: false,
		},
		"POST request with trusted origin": {
			method:         http.MethodPost,
			origin:         trustedOrigin,
			wantStatus:     http.StatusOK,
			wantBody:       "OK",
			shouldCallNext: true,
		},
		"GET request without origin header": {
			method:         http.MethodGet,
			origin:         "",
			wantStatus:     http.StatusOK,
			wantBody:       "OK",
			shouldCallNext: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			handlerCalled = false
			req := httptest.NewRequest(tt.method, "/test", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			assert.Equal(t, rr.Code, tt.wantStatus, "wrong status code")

			for key, want := range wantHeaders {
				got := rr.Header().Get(key)
				assert.Equal(t, got, want, "wrong header value for header %s", key)
			}

			assert.Equal(t, rr.Body.String(), tt.wantBody, "wrong response body")
			assert.Equal(t, handlerCalled, tt.shouldCallNext, "called next handler")
		})
	}
}
