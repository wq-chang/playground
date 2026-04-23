// Package middleware provides reusable HTTP middleware components.
//
// This package includes common middleware used across HTTP servers,
// handling request lifecycle concerns such as recovery, error handling,
// logging, authentication, and request validation.
//
// Overview of available middleware:
//
//   - Recover: gracefully recovers from panics and returns a 500 response.
//   - Error: wraps handlers that return errors and converts them to JSON responses.
//   - Logging: logs request and response details using structured logging (slog).
//   - CORS: sets proper CORS headers for cross-origin requests.
//   - RequireJSON: ensures that incoming requests use the "application/json" Content-Type.
//   - Auth: performs authentication and authorization based on request headers or tokens.
//
// The middleware can be composed using the chain utility (chain.go)
// to build flexible, readable middleware pipelines.
//
// Example usage:
//
//	cors, err := middleware.CORS(log, "https://www.trustedorigin.com")
//	if err != nil {
//	    slog.Error("failed to initialize CORS middleware", "err", err)
//	    os.Exit(1)
//	}
//
//	errMw := middleware.Error(log)
//
//	chain := middleware.NewChain()
//	chain.Add(
//	    middleware.Logging(log),
//	    cors,
//	    middleware.RequireJSON,
//	    middleware.Recover(log),
//	)
//
//	mux := http.NewServeMux()
//
//	mux.HandleFunc("GET /api/", errMw(handlerWithError))
//
//	if err := http.ListenAndServe(addr, chain.Apply(mux)); err != nil {
//	    appl.Log.Error("server failed", "err", err)
//	    os.Exit(1)
//	}
//
// Each middleware is designed to be composable, context-aware,
// and to minimize coupling between application layers.
package middleware
