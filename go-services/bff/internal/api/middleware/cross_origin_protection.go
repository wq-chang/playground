package middleware

import (
	"log/slog"
	"net/http"

	"go-services/bff/internal/api"
	"go-services/library/apperror"
)

func CrossOriginProtection(log *slog.Logger, trustedOrigin string) (func(http.Handler) http.Handler, error) {
	csrp := http.NewCrossOriginProtection()
	if err := csrp.AddTrustedOrigin(trustedOrigin); err != nil {
		return nil, err
	}

	csrp.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		api.SendErrorLog(log, w, http.StatusForbidden, apperror.CodeForbidden, "CORS origin not allowed")
	}))

	return csrp.Handler, nil
}
