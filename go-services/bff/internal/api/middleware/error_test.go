package middleware_test

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"go-services/bff/internal/api/middleware"
	"go-services/library/apperror"
	"go-services/library/assert"
	"go-services/library/testlogger"
)

func TestError(t *testing.T) {
	genericErr := errors.New("unexpected error")

	tests := map[string]struct {
		handler        func(w http.ResponseWriter, r *http.Request) error
		wantStatusCode int
		wantLogLevel   slog.Level
		wantLogged     bool
		wantMessage    string
		wantFields     map[string]any
	}{
		"no error - handler succeeds": {
			handler: func(w http.ResponseWriter, r *http.Request) error {
				w.WriteHeader(http.StatusOK)
				return nil
			},
			wantStatusCode: http.StatusOK,
			wantLogLevel:   0,
			wantLogged:     false,
			wantMessage:    "",
			wantFields:     nil,
		},
		"app error - 4xx client error": {
			handler: func(w http.ResponseWriter, r *http.Request) error {
				return apperror.New(apperror.CodeInvalidInput, "invalid input")
			},
			wantStatusCode: http.StatusBadRequest,
			wantLogLevel:   slog.LevelWarn,
			wantLogged:     true,
			wantMessage:    "client error",
			wantFields: map[string]any{
				"method": "GET",
				"path":   "/test",
				"status": int64(400),
				"code":   apperror.CodeInvalidInput,
				"msg":    "invalid input",
			},
		},
		"app error - 5xx server error": {
			handler: func(w http.ResponseWriter, r *http.Request) error {
				return apperror.New(apperror.CodeDBConnection, "database connection failed")
			},
			wantStatusCode: http.StatusInternalServerError,
			wantLogLevel:   slog.LevelError,
			wantLogged:     true,
			wantMessage:    "backend error",
			wantFields: map[string]any{
				"method": "GET",
				"path":   "/test",
			},
		},
		"app error - 404 not found": {
			handler: func(w http.ResponseWriter, r *http.Request) error {
				return apperror.New(apperror.CodeNotFound, "resource not found")
			},
			wantStatusCode: http.StatusNotFound,
			wantLogLevel:   slog.LevelWarn,
			wantLogged:     true,
			wantMessage:    "client error",
			wantFields: map[string]any{
				"status": int64(404),
				"code":   apperror.CodeNotFound,
			},
		},
		"unhandled error - generic error": {
			handler: func(w http.ResponseWriter, r *http.Request) error {
				return genericErr
			},
			wantStatusCode: http.StatusInternalServerError,
			wantLogLevel:   slog.LevelError,
			wantLogged:     true,
			wantMessage:    "unhandled error",
			wantFields: map[string]any{
				"method": "GET",
				"path":   "/test",
				"error":  genericErr,
			},
		},
		"app error - 503 service unavailable": {
			handler: func(w http.ResponseWriter, r *http.Request) error {
				return apperror.New(apperror.CodeServiceUnavailable, "service temporarily unavailable")
			},
			wantStatusCode: http.StatusServiceUnavailable,
			wantLogLevel:   slog.LevelError,
			wantLogged:     true,
			wantMessage:    "backend error",
			wantFields: map[string]any{
				"method": "GET",
			},
		},
	}
	log, logCapture := testlogger.New()

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			// Create error handler wrapper
			errorHandler := middleware.Error(log)
			wrappedHandler := errorHandler(tt.handler)

			// Create test request and recorder
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rec := httptest.NewRecorder()

			// Execute handler
			wrappedHandler(rec, req)

			// Check status code
			assert.Equal(t, rec.Code, tt.wantStatusCode, "response status code")

			// Check if log was written
			logAssert := testlogger.Assert(t, logCapture.GetOutput())
			if tt.wantLogged {
				logAssert.
					Count(1, "should have 1 log after resolve the error").
					AtIndex(0, tt.wantLogLevel, tt.wantMessage, "log for resolved error")

				for fieldName, fieldValue := range tt.wantFields {
					logAssert.HasField(0, fieldName, fieldValue, "%s", fieldName)
				}
			} else {
				logAssert.Empty("empty logs for no error")
			}

			logCapture.Reset()
		})
	}
}

func TestErrorBoundary499(t *testing.T) {
	// Test the boundary between 4xx and 5xx errors (status 499 should log as warn)
	handler := func(w http.ResponseWriter, r *http.Request) error {
		return &apperror.AppError{Code: apperror.CodeTooManyRequests, Msg: "too many request", Err: nil}
	}

	log, logCapture := testlogger.New()
	errorHandler := middleware.Error(log)
	wrappedHandler := errorHandler(handler)

	req := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	rec := httptest.NewRecorder()

	wrappedHandler(rec, req)

	testlogger.Assert(t, logCapture.GetOutput()).
		AtIndex(0, slog.LevelWarn, "client error", "log message")
}

func TestErrorBoundary500(t *testing.T) {
	// Test the boundary between 4xx and 5xx errors (status 500 should log as error)
	handler := func(w http.ResponseWriter, r *http.Request) error {
		return apperror.New(apperror.CodeInternalError, "internal server error")
	}

	log, logCapture := testlogger.New()
	errorHandler := middleware.Error(log)
	wrappedHandler := errorHandler(handler)

	req := httptest.NewRequest(http.MethodDelete, "/api/resource/123", nil)
	rec := httptest.NewRecorder()

	wrappedHandler(rec, req)

	testlogger.Assert(t, logCapture.GetOutput()).
		AtIndex(0, slog.LevelError, "backend error", "log for error").
		HasField(0, "path", "/api/resource/123", "url")
}

func TestErrorMultipleCalls(t *testing.T) {
	// Test that multiple handler calls are logged correctly
	handler := func(w http.ResponseWriter, r *http.Request) error {
		return apperror.New(apperror.CodeInvalidInput, "bad request")
	}

	log, logCapture := testlogger.New()
	errorHandler := middleware.Error(log)
	wrappedHandler := errorHandler(handler)
	reqCount := 3

	// Call handler 3 times
	for range reqCount {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		wrappedHandler(rec, req)
	}

	// Check that 3 logs were written
	logAssert := testlogger.Assert(t, logCapture.GetOutput())
	logAssert.Count(3, "log counts for %d requests", reqCount)

	// Check each log level individually
	for i := range 3 {
		logAssert.AtIndex(i, slog.LevelWarn, "client error", "log for request %d", i)
	}
}

func TestErrorMixedLevels(t *testing.T) {
	// Test that different error types produce different log levels
	tests := []struct {
		name        string
		handler     func(w http.ResponseWriter, r *http.Request) error
		wantLevel   slog.Level
		wantMessage string
	}{
		{
			name: "client error",
			handler: func(w http.ResponseWriter, r *http.Request) error {
				return apperror.New(apperror.CodeInvalidFormat, "bad request")
			},
			wantLevel:   slog.LevelWarn,
			wantMessage: "client error",
		},
		{
			name: "server error",
			handler: func(w http.ResponseWriter, r *http.Request) error {
				return apperror.New(apperror.CodeInternalError, "internal error")
			},
			wantLevel:   slog.LevelError,
			wantMessage: "backend error",
		},
		{
			name: "another client error",
			handler: func(w http.ResponseWriter, r *http.Request) error {
				return apperror.New(apperror.CodeUnauthorized, "unauthorized")
			},
			wantLevel:   slog.LevelWarn,
			wantMessage: "client error",
		},
	}

	log, logCapture := testlogger.New()
	errorHandler := middleware.Error(log)

	// Execute all handlers
	for _, h := range tests {
		t.Run(h.name, func(t *testing.T) {
			wrappedHandler := errorHandler(h.handler)
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rec := httptest.NewRecorder()
			wrappedHandler(rec, req)
		})
	}

	logAssert := testlogger.Assert(t, logCapture.GetOutput())

	// Check total log count
	logAssert.Count(3, "log count for %d error requests", len(tests))

	// Verify each log
	for i, tt := range tests {
		logAssert.AtIndex(i, tt.wantLevel, tt.wantMessage, "log for request %d", i)
	}
}
