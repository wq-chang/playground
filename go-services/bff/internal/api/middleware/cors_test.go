package middleware

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestCORS(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	trustedOrigin := "https://example.com"

	corsMiddleware, err := CORS(log, trustedOrigin)
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

	expectedHeaders := map[string]string{
		"Access-Control-Allow-Origin":      trustedOrigin,
		"Access-Control-Allow-Methods":     "GET, POST, PUT, PATCH, DELETE, OPTIONS",
		"Access-Control-Allow-Headers":     "Content-Type, Authorization",
		"Access-Control-Allow-Credentials": "true",
	}

	tests := map[string]struct {
		method         string
		origin         string
		expectedStatus int
		expectedBody   string
		shouldCallNext bool
	}{
		"GET request with trusted origin": {
			method:         http.MethodGet,
			origin:         trustedOrigin,
			expectedStatus: http.StatusOK,
			expectedBody:   "OK",
			shouldCallNext: true,
		},
		"OPTIONS preflight request": {
			method:         http.MethodOptions,
			origin:         trustedOrigin,
			expectedStatus: http.StatusNoContent,
			expectedBody:   "",
			shouldCallNext: false,
		},
		"POST request with trusted origin": {
			method:         http.MethodPost,
			origin:         trustedOrigin,
			expectedStatus: http.StatusOK,
			expectedBody:   "OK",
			shouldCallNext: true,
		},
		"GET request without origin header": {
			method:         http.MethodGet,
			origin:         "",
			expectedStatus: http.StatusOK,
			expectedBody:   "OK",
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

			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rr.Code)
			}

			for key, expected := range expectedHeaders {
				if actual := rr.Header().Get(key); actual != expected {
					t.Errorf("header %s: expected %q, got %q", key, expected, actual)
				}
			}

			if body := rr.Body.String(); body != tt.expectedBody {
				t.Errorf("expected body %q, got %q", tt.expectedBody, body)
			}

			if handlerCalled != tt.shouldCallNext {
				t.Errorf("handler called = %v, expected %v", handlerCalled, tt.shouldCallNext)
			}
		})
	}
}
