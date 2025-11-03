package middleware

import (
	"log/slog"
	"net/http"

	"go-services/bff/internal/api"
	"go-services/library/apperror"
)

// CORS returns a middleware that enforces Cross-Origin Resource Sharing (CORS)
// and protects against cross-origin request forgery (CSRF-like) attacks.
//
// It performs two main functions:
//  1. Sets CORS headers on every response to allow trusted origins.
//  2. Wraps the request handler with additional origin validation using
//     http.NewCrossOriginProtection(), denying requests from untrusted origins.
//
// The middleware automatically handles OPTIONS preflight requests
// by returning HTTP 204 (No Content).
//
// If a request originates from an untrusted domain, it responds with
// HTTP 403 Forbidden and a JSON error payload using api.SendErrorLog.
//
// Example:
//
//	cors, err := middleware.CORS(log, "https://frontend.example.com")
//	if err != nil {
//	    log.Error("failed to initialize CORS middleware", "err", err)
//	    os.Exit(1)
//	}
//
//	chain := middleware.NewChain()
//	chain.Add(cors, middleware.Logging(log))
//
//	http.ListenAndServe(":8080", chain.Apply(mux))
//
// Parameters:
//   - log: the structured logger (slog.Logger) used for error reporting.
//   - trustedOrigin: the allowed frontend base URL for cross-origin requests.
//
// Returns:
//   - A middleware function that wraps an http.Handler to apply CORS rules.
//   - An error if the provided trustedOrigin is invalid.
//
// The returned middleware sets these headers:
//
//	Access-Control-Allow-Origin: <trustedOrigin>
//	Access-Control-Allow-Methods: GET, POST, PUT, PATCH, DELETE, OPTIONS
//	Access-Control-Allow-Headers: Content-Type, Authorization
//	Access-Control-Allow-Credentials: true
//
// Example response when the origin is not allowed:
//
//	HTTP/1.1 403 Forbidden
//	Content-Type: application/json
//	{
//	  "code": "FORBIDDEN",
//	  "message": "CORS origin not allowed"
//	}
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
