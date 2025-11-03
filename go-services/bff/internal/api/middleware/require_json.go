package middleware

import (
	"log/slog"
	"net/http"

	"go-services/bff/internal/api"
	"go-services/library/apperror"
)

// RequireJSON returns an HTTP middleware that validates requests
// to ensure they conform to JSON expectations.
//
// Behavior:
//  1. OPTIONS, GET, and HEAD requests are passed through without checks.
//  2. Other methods (POST, PUT, PATCH, etc.) require a "Content-Type" header of "application/json".
//     - If the Content-Type is invalid, responds with 415 Unsupported Media Type using api.SendErrorLog.
//  3. If an "Accept" header is provided and does not allow "application/json" or "*/*",
//     responds with 406 Not Acceptable.
//  4. Otherwise, the request proceeds to the next handler.
//
// This middleware helps enforce proper content negotiation and ensures
// clients are sending and expecting JSON payloads where appropriate.
//
// Example:
//
//	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
//
//	mux := http.NewServeMux()
//	mux.HandleFunc("/data", func(w http.ResponseWriter, r *http.Request) {
//		w.Write([]byte(`{"message":"ok"}`))
//	})
//
//	handler := middleware.RequireJSON(logger)(mux)
//	http.ListenAndServe(":8080", handler)
//
// Example behaviors:
//
//  1. POST with "Content-Type: text/plain"
//     -> HTTP 415 Unsupported Media Type
//
//  2. PATCH with "Accept: text/html"
//     -> HTTP 406 Not Acceptable
//
//  3. GET request
//     -> Passes through without validation
func RequireJSON(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodOptions || r.Method == http.MethodGet || r.Method == http.MethodHead {
				next.ServeHTTP(w, r)
				return
			}

			if r.Header.Get("Content-Type") != api.ContentTypeJSON {
				api.SendErrorLog(log, w, http.StatusUnsupportedMediaType,
					apperror.CodeUnsupportedMediaType, "Content-Type must be application/json string")
				return
			}

			// TODO: move media type to constants
			if accept := r.Header.Get("Accept"); accept != "" &&
				accept != api.ContentTypeJSON && accept != "*/*" {
				api.SendErrorLog(log, w, http.StatusNotAcceptable,
					apperror.CodeNotAcceptable, "Accept header must include application/json")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
