package middleware

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

// responseWriter is a wrapper to capture status code and body
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	body       *bytes.Buffer
}

// WriteHeader overrides the default WriteHeader method to record the
// HTTP status code before writing it to the underlying ResponseWriter.
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Write overrides the default Write method to capture the response body
// while still passing it through to the underlying ResponseWriter.
//
// It stores the written bytes in the internal buffer for later logging.
func (rw *responseWriter) Write(b []byte) (int, error) {
	rw.body.Write(b)
	return rw.ResponseWriter.Write(b)
}

// Logging returns an HTTP middleware that logs both incoming requests
// and outgoing responses using the provided slog.Logger.
//
// The middleware logs:
//   - Request method, path, query parameters, and body
//   - Response status code and body
//
// Request and response bodies are captured safely without consuming
// them, ensuring that downstream handlers can still read the request body.
//
// OPTIONS requests are skipped to reduce log noise (commonly used for CORS preflight).
//
// Example:
//
//	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
//	mux := http.NewServeMux()
//	mux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
//		w.Write([]byte("world"))
//	})
//
//	http.ListenAndServe(":8080", middleware.Logging(logger)(mux))
//
// Example output (JSON):
//
//	{
//	  "time": "2025-11-04T00:00:00Z",
//	  "level": "INFO",
//	  "msg": "incoming request",
//	  "method": "POST",
//	  "path": "/hello",
//	  "query": "",
//	  "body": "{\"foo\":\"bar\"}"
//	}
//	{
//	  "time": "2025-11-04T00:00:00Z",
//	  "level": "INFO",
//	  "msg": "response",
//	  "status_code": 200,
//	  "body": "world"
//	}
func Logging(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// --- Log request ---
			var requestBody []byte
			if r.Body != nil {
				bodyBytes, err := io.ReadAll(r.Body)
				if err != nil {
					log.Error("failed to read request body", "err", err)
				} else {
					requestBody = bodyBytes
				}
				// Replace the body so the handler can read it
				r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			}

			if r.Method != http.MethodOptions {
				log.Info(
					"incoming request",
					"method", r.Method,
					"path", r.URL.Path,
					"query", r.URL.RawQuery,
					"body", string(requestBody),
				)
			}

			rw := &responseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
				body:           &bytes.Buffer{},
			}

			next.ServeHTTP(rw, r)

			if r.Method != http.MethodOptions {
				log.Info(
					"response",
					"status_code", rw.statusCode,
					"body", strings.TrimSuffix(rw.body.String(), "\n"),
				)
			}
		})
	}
}
