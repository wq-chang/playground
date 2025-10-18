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

func Error(log *slog.Logger) errorHandlerWrapper {
	return func(handler handlerFuncWithError) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if err := handler(w, r); err != nil {
				log.Error("handler error",
					"method", r.Method,
					"path", r.URL.Path,
					"error", err,
				)

				var statusErr *apperror.AppError
				if errors.As(err, &statusErr) {
					api.SendErrorLog(log, w, statusErr.StatusCode, statusErr.Code, statusErr.Msg)
				} else {
					api.SendErrorLog(
						log,
						w,
						http.StatusInternalServerError,
						apperror.CodeInternalError,
						"internal server error",
					)
				}
			}
		}
	}
}
