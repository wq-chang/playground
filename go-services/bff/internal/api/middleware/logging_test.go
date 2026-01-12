package middleware_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go-services/bff/internal/api/middleware"
	"go-services/library/assert"
	"go-services/library/testutil"
)

func TestLoggingMiddleware(t *testing.T) {
	logger := testutil.NewTestLogger(t)

	mw := middleware.Logging(logger.Logger)

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
		method         string
		path           string
		body           string
		expectedStatus int
		expectInLog    []string
	}{
		"successful request": {
			method:         http.MethodPost,
			path:           "/foo",
			body:           "hello",
			expectedStatus: http.StatusOK,
			expectInLog: []string{
				"incoming request",
				"method=POST",
				"path=/foo",
				"body=hello",
				"response",
				"status_code=200",
				"body=ok",
			},
		},
		"bad request": {
			method:         http.MethodPost,
			path:           "/foo",
			body:           "error",
			expectedStatus: http.StatusBadRequest,
			expectInLog: []string{
				"incoming request",
				"method=POST",
				"body=error",
				"response",
				"status_code=400",
				"body=\"bad request\"",
			},
		},
		"OPTIONS method should not log": {
			method:         http.MethodOptions,
			path:           "/foo",
			body:           "",
			expectedStatus: http.StatusOK,
			expectInLog:    nil, // should be empty
		},
	}

	// --- run test cases ---
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			logger.Reset()

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

			assert.Equal(t, res.StatusCode, tt.expectedStatus, "wrong status code")

			logOutput := logger.Capture.String()

			if tt.expectInLog == nil {
				assert.Equal(t, logOutput, "", "expected no logs for OPTIONS request, got:\n%s", logOutput)
			} else {
				logger.AssertContainsAll(tt.expectInLog, "logs")
			}
		})
	}
}
