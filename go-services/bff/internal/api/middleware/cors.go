package middleware

import (
	"log/slog"
	"net/http"

	"go-services/bff/internal/api"
	"go-services/library/apperror"
)

func CORS(log *slog.Logger, trustedOrigin string) (func(http.Handler) http.Handler, error) {
	csrp := http.NewCrossOriginProtection()
	if err := csrp.AddTrustedOrigin(trustedOrigin); err != nil {
		return nil, err
	}
	csrp.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		api.SendErrorLog(log, w, http.StatusForbidden, apperror.CodeForbidden, "CORS origin not allowed")
	}))

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Set CORS headers
			w.Header().Set("Access-Control-Allow-Origin", trustedOrigin)
			w.Header().Set("Access-Control-Allow-Methods",
				"GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Credentials", "true")

			// Handle preflight requests
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			// Apply cross-origin protection, then continue to next handler
			csrp.Handler(next).ServeHTTP(w, r)
		})
	}, nil
}
