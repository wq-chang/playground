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
	"go-services/library/testutil"
)

func TestError(t *testing.T) {
	tests := map[string]struct {
		handler         func(w http.ResponseWriter, r *http.Request) error
		wantStatusCode  int
		wantLogLevel    slog.Level
		wantLogged      bool
		wantLogContains []string
	}{
		"no error - handler succeeds": {
			handler: func(w http.ResponseWriter, r *http.Request) error {
				w.WriteHeader(http.StatusOK)
				return nil
			},
			wantStatusCode:  http.StatusOK,
			wantLogLevel:    0,
			wantLogged:      false,
			wantLogContains: []string{},
		},
		"app error - 4xx client error": {
			handler: func(w http.ResponseWriter, r *http.Request) error {
				return apperror.New(apperror.CodeInvalidInput, "invalid input")
			},
			wantStatusCode: http.StatusBadRequest,
			wantLogLevel:   slog.LevelWarn,
			wantLogged:     true,
			wantLogContains: []string{
				"client error",
				"method=GET",
				"path=/test",
				"status=400",
				"code=INVALID_INPUT",
				"msg=\"invalid input\"",
			},
		},
		"app error - 5xx server error": {
			handler: func(w http.ResponseWriter, r *http.Request) error {
				return apperror.New(apperror.CodeDBConnection, "database connection failed")
			},
			wantStatusCode: http.StatusInternalServerError,
			wantLogLevel:   slog.LevelError,
			wantLogged:     true,
			wantLogContains: []string{
				"backend error",
				"method=GET",
				"path=/test",
			},
		},
		"app error - 404 not found": {
			handler: func(w http.ResponseWriter, r *http.Request) error {
				return apperror.New(apperror.CodeNotFound, "resource not found")
			},
			wantStatusCode: http.StatusNotFound,
			wantLogLevel:   slog.LevelWarn,
			wantLogged:     true,
			wantLogContains: []string{
				"client error",
				"status=404",
				"code=NOT_FOUND",
			},
		},
		"unhandled error - generic error": {
			handler: func(w http.ResponseWriter, r *http.Request) error {
				return errors.New("unexpected error")
			},
			wantStatusCode: http.StatusInternalServerError,
			wantLogLevel:   slog.LevelError,
			wantLogged:     true,
			wantLogContains: []string{
				"unhandled error",
				"method=GET",
				"path=/test",
				"error=\"unexpected error\"",
			},
		},
		"app error - 503 service unavailable": {
			handler: func(w http.ResponseWriter, r *http.Request) error {
				return apperror.New(apperror.CodeServiceUnavailable, "service temporarily unavailable")
			},
			wantStatusCode: http.StatusServiceUnavailable,
			wantLogLevel:   slog.LevelError,
			wantLogged:     true,
			wantLogContains: []string{
				"backend error",
				"method=GET",
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			testLogger := testutil.NewTestLogger(t)

			// Create error handler wrapper
			errorHandler := middleware.Error(testLogger.Logger)
			wrappedHandler := errorHandler(tt.handler)

			// Create test request and recorder
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rec := httptest.NewRecorder()

			// Execute handler
			wrappedHandler(rec, req)

			// Check status code
			assert.Equal(t, rec.Code, tt.wantStatusCode, "response status code")

			// Check if log was written
			if tt.wantLogged {
				testLogger.AssertNotEmpty("logs")
				testLogger.AssertLastLevel(tt.wantLogLevel, "log level")

				// Check log contents
				testLogger.AssertContainsAll(
					tt.wantLogContains,
					"logs for level %s",
					tt.wantLogLevel.String(),
				)
			} else {
				testLogger.AssertEmpty("logs")
			}
		})
	}
}

func TestErrorBoundary499(t *testing.T) {
	// Test the boundary between 4xx and 5xx errors (status 499 should log as warn)
	handler := func(w http.ResponseWriter, r *http.Request) error {
		return &apperror.AppError{Code: apperror.CodeTooManyRequests, Msg: "too many request", Err: nil}
	}

	testLogger := testutil.NewTestLogger(t)
	errorHandler := middleware.Error(testLogger.Logger)
	wrappedHandler := errorHandler(handler)

	req := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	rec := httptest.NewRecorder()

	wrappedHandler(rec, req)

	testLogger.AssertLastLevel(slog.LevelWarn, "log level")
	testLogger.AssertContains("client error", "log message")
}

func TestErrorBoundary500(t *testing.T) {
	// Test the boundary between 4xx and 5xx errors (status 500 should log as error)
	handler := func(w http.ResponseWriter, r *http.Request) error {
		return apperror.New(apperror.CodeInternalError, "internal server error")
	}

	testLogger := testutil.NewTestLogger(t)
	errorHandler := middleware.Error(testLogger.Logger)
	wrappedHandler := errorHandler(handler)

	req := httptest.NewRequest(http.MethodDelete, "/api/resource/123", nil)
	rec := httptest.NewRecorder()

	wrappedHandler(rec, req)

	testLogger.AssertLastLevel(slog.LevelError, "log level")
	testLogger.AssertContains("backend error", "log message")
	testLogger.AssertContains("path=/api/resource/123", "request url path")
}

func TestErrorMultipleCalls(t *testing.T) {
	// Test that multiple handler calls are logged correctly
	handler := func(w http.ResponseWriter, r *http.Request) error {
		return apperror.New(apperror.CodeInvalidInput, "bad request")
	}

	testLogger := testutil.NewTestLogger(t)
	errorHandler := middleware.Error(testLogger.Logger)
	wrappedHandler := errorHandler(handler)

	// Call handler 3 times
	for range 3 {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		wrappedHandler(rec, req)
	}

	// Check that 3 logs were written
	testLogger.AssertLogCount(3, "log count")
	testLogger.AssertLevelCount(slog.LevelWarn, 3, "warn level count")

	// Check each log level individually
	for i := range 3 {
		testLogger.AssertLevelAt(i, slog.LevelWarn, "log level")
	}
}

func TestErrorMixedLevels(t *testing.T) {
	// Test that different error types produce different log levels
	tests := []struct {
		name      string
		handler   func(w http.ResponseWriter, r *http.Request) error
		wantLevel slog.Level
	}{
		{
			name: "client error",
			handler: func(w http.ResponseWriter, r *http.Request) error {
				return apperror.New(apperror.CodeInvalidFormat, "bad request")
			},
			wantLevel: slog.LevelWarn,
		},
		{
			name: "server error",
			handler: func(w http.ResponseWriter, r *http.Request) error {
				return apperror.New(apperror.CodeInternalError, "internal error")
			},
			wantLevel: slog.LevelError,
		},
		{
			name: "another client error",
			handler: func(w http.ResponseWriter, r *http.Request) error {
				return apperror.New(apperror.CodeUnauthorized, "unauthorized")
			},
			wantLevel: slog.LevelWarn,
		},
	}

	testLogger := testutil.NewTestLogger(t)
	errorHandler := middleware.Error(testLogger.Logger)

	// Execute all handlers
	for _, h := range tests {
		t.Run(h.name, func(t *testing.T) {
			wrappedHandler := errorHandler(h.handler)
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rec := httptest.NewRecorder()
			wrappedHandler(rec, req)
		})
	}

	// Check total log count
	testLogger.AssertLogCount(3, "log count")

	// Check level counts
	testLogger.AssertLevelCount(slog.LevelWarn, 2, "warn level count")
	testLogger.AssertLevelCount(slog.LevelError, 1, "error level count")

	// Verify each log level
	testLogger.AssertLevelAt(0, slog.LevelWarn, "log level for first request")
	testLogger.AssertLevelAt(1, slog.LevelError, "log level for second request")
	testLogger.AssertLevelAt(2, slog.LevelWarn, "log level for third request")
}
