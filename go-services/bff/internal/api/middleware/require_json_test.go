package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go-services/bff/internal/api"
	"go-services/bff/internal/api/middleware"
	"go-services/library/testutil"
)

func TestRequireJSONMiddleware(t *testing.T) {
	logger := testutil.NewTestLogger(t)

	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("ok"))
		if err != nil {
			t.Fatal("failed to write ok")
		}
	})

	mw := middleware.RequireJSON(logger.Logger)
	handler := mw(okHandler)

	tests := map[string]struct {
		method           string
		contentType      string
		accept           string
		wantStatus       int
		wantBodyContains string
	}{
		"OPTIONS request allowed": {
			method:           http.MethodOptions,
			contentType:      "",
			accept:           "",
			wantStatus:       http.StatusOK,
			wantBodyContains: "",
		},
		"GET request allowed": {
			method:           http.MethodGet,
			contentType:      "",
			accept:           "",
			wantStatus:       http.StatusOK,
			wantBodyContains: "",
		},
		"HEAD request allowed": {
			method:           http.MethodHead,
			contentType:      "",
			accept:           "",
			wantStatus:       http.StatusOK,
			wantBodyContains: "",
		},
		"POST with valid JSON": {
			method:           http.MethodPost,
			contentType:      api.ContentTypeJSON,
			accept:           api.ContentTypeJSON,
			wantStatus:       http.StatusOK,
			wantBodyContains: "",
		},
		"POST with invalid Content-Type": {
			method:           http.MethodPost,
			contentType:      "text/plain",
			accept:           api.ContentTypeJSON,
			wantStatus:       http.StatusUnsupportedMediaType,
			wantBodyContains: "Content-Type must be application/json string",
		},
		"PUT with invalid Content-Type": {
			method:           http.MethodPut,
			contentType:      "text/plain",
			accept:           api.ContentTypeJSON,
			wantStatus:       http.StatusUnsupportedMediaType,
			wantBodyContains: "Content-Type must be application/json string",
		},
		"PATCH with invalid Content-Type": {
			method:           http.MethodPatch,
			contentType:      "text/plain",
			accept:           api.ContentTypeJSON,
			wantStatus:       http.StatusUnsupportedMediaType,
			wantBodyContains: "Content-Type must be application/json string",
		},
		"POST with unacceptable Accept header": {
			method:           http.MethodPost,
			contentType:      api.ContentTypeJSON,
			accept:           "text/html",
			wantStatus:       http.StatusNotAcceptable,
			wantBodyContains: "Accept header must include application/json",
		},
		"POST with Accept */* header allowed": {
			method:           http.MethodPost,
			contentType:      api.ContentTypeJSON,
			accept:           "*/*",
			wantStatus:       http.StatusOK,
			wantBodyContains: "",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/test", nil)
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			if tt.accept != "" {
				req.Header.Set("Accept", tt.accept)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			resBody := rec.Body.String()

			if got, want := rec.Code, tt.wantStatus; got != want {
				t.Errorf("got status %d, want %d", got, want)
			}

			if !strings.Contains(resBody, tt.wantBodyContains) {
				t.Errorf("got response: %q, want it contains %q", resBody, tt.wantBodyContains)
			}
		})
	}
}
