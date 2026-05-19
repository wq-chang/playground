// Package idp defines backend-private identity-provider adapter contracts for
// backend flows that need provider-specific admin or user lookup capabilities.
//
// Shared JWT/JWKS token validation lives in library/auth so Go services can
// validate tokens without depending on backend-only admin APIs.
package idp
