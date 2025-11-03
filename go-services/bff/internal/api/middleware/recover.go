package middleware

import (
	"log/slog"
	"net/http"

	"go-services/bff/internal/api"
	"go-services/library/apperror"
)

// Recover returns an HTTP middleware that recovers from panics
// in downstream handlers, logs the panic, and sends a standardized
// internal server error response to the client.
//
// This middleware prevents the application from crashing due to
// unexpected panics in request handlers. When a panic occurs,
// it performs the following steps:
//
//  1. Logs the panic message and stack trace at the error level
//     using the provided slog.Logger.
//  2. Sends a structured error response with HTTP 500 (Internal Server Error)
//     using api.SendErrorLog, with the error code set to
//     apperror.CodeInternalError.
//
// Example:
//
//		logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
//
//		mux := http.NewServeMux()
//		mux.HandleFunc("/panic", func(w http.ResponseWriter, r *http.Request) {
//			panic("unexpected error")
//		})
//
//		handler := middleware.Recover(logger)(mux)
//		http.ListenAndServe(":8080", handler)
//	}
//
// Example output (JSON):
//
//	{
//	  "time": "2025-11-04T00:00:00Z",
//	  "level": "ERROR",
//	  "msg": "panic recovered",
//	  "err": "unexpected error"
//	}
//
// If a panic occurs, the client receives:
//
//	HTTP/1.1 500 Internal Server Error
//	Content-Type: application/json
//
//	{"code":"INTERNAL_ERROR","message":"internal server error"}
//
// This middleware should typically be one of the outermost layers
// in the HTTP middleware chain, ensuring that all panics from inner
// handlers are caught and logged.
func Recover(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.Error("panic recovered", "err", rec)
					api.SendErrorLog(
						log,
						w,
						http.StatusInternalServerError,
						apperror.CodeInternalError,
						"internal server error",
					)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}
