package middleware

import (
	"log/slog"
	"net/http"

	"go-services/bff/internal/api"
	"go-services/library/apperror"
)

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
