package middleware

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go-services/library/testutil"
)

func TestRecoverMiddleware(t *testing.T) {
	logger := testutil.NewTestLogger(t)

	// --- test handler that panics ---
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("something went wrong")
	})

	mw := Recover(logger.Logger)
	handler := mw(panicHandler)

	t.Run("should recover from panic and call SendErrorLog", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
		}

		resBody := rec.Body.String()

		logger.AssertLevelCount(slog.LevelError, 1)
		logger.AssertContains("panic recovered")
		logger.AssertContains("something went wrong")
		if !strings.Contains(resBody, "internal server error") {
			t.Errorf("got %q, want it to contain %q", resBody, "internal server error")
		}
	})

	t.Run("should not trigger recovery for normal requests", func(t *testing.T) {
		logger.Reset()
		normalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte("ok"))
			if err != nil {
				t.Fatal("failed to write ok")
			}
		})

		handler := Recover(logger.Logger)(normalHandler)
		req := httptest.NewRequest(http.MethodGet, "/ok", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if got, want := rec.Code, http.StatusOK; got != want {
			t.Errorf("got status = %d, want %d", http.StatusOK, rec.Code)
		}

		logger.AssertLevelCount(slog.LevelError, 0)
		logger.AssertEmpty()
	})
}
