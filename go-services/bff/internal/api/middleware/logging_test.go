package middleware_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go-services/bff/internal/api/middleware"
	"go-services/library/assert"
	"go-services/library/testlogger"
)

func TestLoggingMiddleware(t *testing.T) {
	log, logCapture := testlogger.New()

	mw := middleware.Logging(log)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		defer func() {
			err := r.Body.Close()
			if err != nil {
				t.Fatal("failed to close body")
			}
		}()

		if string(body) == "error" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("ok"))
		if err != nil {
			t.Fatal("failed to write ok")
		}
	})

	ts := httptest.NewServer(mw(testHandler))
	defer ts.Close()

	// --- define test cases ---
	tests := map[string]struct {
		method     string
		path       string
		body       string
		wantStatus int
		wantLogs   []testlogger.LogEntry
	}{
		"successful request": {
			method:     http.MethodPost,
			path:       "/foo",
			body:       "hello",
			wantStatus: http.StatusOK,
			wantLogs: []testlogger.LogEntry{
				{
					Msg:   "incoming request",
					Level: slog.LevelInfo,
					Fields: map[string]any{
						"method": "POST",
						"path":   "/foo",
						"body":   "hello",
					},
				},
				{
					Msg:   "response",
					Level: slog.LevelInfo,
					Fields: map[string]any{
						"status_code": int64(200),
						"body":        "ok",
					},
				},
			},
		},
		"bad request": {
			method:     http.MethodPost,
			path:       "/foo",
			body:       "error",
			wantStatus: http.StatusBadRequest,
			wantLogs: []testlogger.LogEntry{
				{
					Msg:   "incoming request",
					Level: slog.LevelInfo,
					Fields: map[string]any{
						"method": "POST",
						"path":   "/foo",
						"body":   "error",
					},
				},
				{
					Msg:   "response",
					Level: slog.LevelInfo,
					Fields: map[string]any{
						"status_code": int64(400),
						"body":        "bad request",
					},
				},
			},
		},
		"OPTIONS method should not log": {
			method:     http.MethodOptions,
			path:       "/foo",
			body:       "",
			wantStatus: http.StatusOK,
			wantLogs:   nil,
		},
	}

	// --- run test cases ---
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			logCapture.Reset()

			reqBody := strings.NewReader(tt.body)
			req, _ := http.NewRequest(tt.method, ts.URL+tt.path, reqBody)
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer func() {
				err := res.Body.Close()
				if err != nil {
					t.Fatal("failed to close body")
				}
			}()

			assert.Equal(t, res.StatusCode, tt.wantStatus, "wrong status code")

			logAssert := testlogger.Assert(t, logCapture.GetOutput())
			if tt.wantLogs != nil {
				for i, wantLog := range tt.wantLogs {
					logAssert.AtIndex(i, wantLog.Level, wantLog.Msg, "log")

					for name, value := range wantLog.Fields {
						logAssert.HasField(i, name, value, "field for log %d field %q", i, name)
					}
				}
			} else {
				logAssert.Empty("option request should not have log")
			}
		})
	}
}
