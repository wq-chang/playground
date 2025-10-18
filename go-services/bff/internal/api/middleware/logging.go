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

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	rw.body.Write(b)
	return rw.ResponseWriter.Write(b)
}

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
					"status code", rw.statusCode,
					"body", strings.TrimSuffix(rw.body.String(), "\n"),
				)
			}
		})
	}
}
