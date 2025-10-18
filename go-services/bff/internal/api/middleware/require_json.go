package middleware

import (
	"encoding/json"
	"net/http"
)

func RequireJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}

		if r.Header.Get("Content-Type") != "application/json" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnsupportedMediaType)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "Content-Type must be application/json",
			})
			return
		}

		// TODO: move media type to constants
		if accept := r.Header.Get("Accept"); accept != "" && accept != "application/json" && accept != "*/*" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotAcceptable)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "Accept header must include application/json",
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}
