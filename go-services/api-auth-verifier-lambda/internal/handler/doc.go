// Package handler provides AWS Lambda handlers for API requests.
//
// This package focuses on request processing, authentication, and
// token validation. It includes utilities to validate JWT tokens,
// extract Authorization headers (case-insensitive), and generate
// appropriate API Gateway responses.
//
// The primary responsibilities of this package include:
//   - Handling API Gateway requests and responses.
//   - Extracting and validating Bearer tokens from Authorization headers.
//   - Logging errors or failed validation using slog.Logger
package handler
