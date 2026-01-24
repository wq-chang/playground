package middleware

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"go-services/library/assert"
	"go-services/library/testlogger"
)

func TestRecoverMiddleware(t *testing.T) {
	log, logCapture := testlogger.New()

	// --- test handler that panics ---
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("something went wrong")
	})

	mw := Recover(log)
	handler := mw(panicHandler)

	t.Run("should recover from panic and call SendErrorLog", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, rec.Code, http.StatusInternalServerError, "http status")

		resBody := rec.Body.String()

		testlogger.Assert(t, logCapture.GetOutput()).
			Count(1, "should have 1 error logs after recovered from panic").
			AtIndex(0, slog.LevelError, "panic recovered", "panic recovered message").
			HasField(0, "err", "something went wrong", "error message")

		assert.StringContains(
			t,
			resBody,
			"internal server error",
			"want error resonse return a generic body",
		)
	})

	t.Run("should not trigger recovery for normal requests", func(t *testing.T) {
		logCapture.Reset()
		normalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte("ok"))
			if err != nil {
				t.Fatal("failed to write ok")
			}
		})

		handler := Recover(log)(normalHandler)
		req := httptest.NewRequest(http.MethodGet, "/ok", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, rec.Code, http.StatusOK, "http status for normal request")
		testlogger.Assert(t, logCapture.GetOutput()).
			Count(0, "should not log for normal request")
	})
}
