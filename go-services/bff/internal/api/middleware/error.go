package middleware

import (
	"errors"
	"log/slog"
	"net/http"

	"go-services/bff/internal/api"
	"go-services/library/apperror"
)

type (
	errorHandlerWrapper  func(handler handlerFuncWithError) http.HandlerFunc
	handlerFuncWithError func(w http.ResponseWriter, r *http.Request) error
)

// Error returns a middleware wrapper that standardizes error handling
// for HTTP handlers that return an error.
//
// It wraps a handler function of the form:
//
//	func(w http.ResponseWriter, r *http.Request) error
//
// and converts returned errors into structured JSON responses
// using the api.SendErrorLog helper. The middleware distinguishes between
// expected client-side errors (4xx) and unexpected server-side errors (5xx),
// applying appropriate logging levels and response codes.
//
// Behavior:
//
//   - If the error is an *apperror.AppError*:
//
//   - Logs with level WARN for 4xx client errors.
//
//   - Logs with level ERROR for 5xx server errors.
//
//   - Sends an error response using the status code, error code, and message.
//
//   - If the error is not an *apperror.AppError*:
//
//   - Logs with level ERROR.
//
//   - Returns HTTP 500 Internal Server Error with a generic message.
//
// Example:
//
//	errMw := middleware.Error(log)
//
//	mux.HandleFunc("GET /auth/login", errMw(handlerFuncWithError))
//
// This pattern centralizes error handling and prevents repetitive
// response and logging logic across handlers.
//
// Parameters:
//   - log: the slog.Logger used for structured logging.
//
// Returns:
//   - A function that wraps a handler which returns an error into
//     a standard http.HandlerFunc with consistent error handling.
func Error(log *slog.Logger) errorHandlerWrapper {
	return func(handler handlerFuncWithError) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if err := handler(w, r); err != nil {
				var appErr *apperror.AppError
				if errors.As(err, &appErr) {
					if appErr.StatusCode >= 500 {
						log.Error("handler error",
							"method", r.Method,
							"path", r.URL.Path,
							"error", err,
						)
					} else {
						log.Warn("client error",
							"method", r.Method,
							"path", r.URL.Path,
							"status", appErr.StatusCode,
							"code", appErr.Code,
							"msg", appErr.Msg,
						)
					}

					api.SendErrorLog(log, w, appErr.StatusCode, appErr.Code, appErr.Msg)
					return
				}

				log.Error("unhandled error",
					"method", r.Method,
					"path", r.URL.Path,
					"error", err,
				)
				api.SendErrorLog(log, w,
					http.StatusInternalServerError,
					apperror.CodeInternalError,
					"internal server error",
				)
			}
		}
	}
}
