// Package config provides application configuration utilities.
//
// This package is responsible for loading and managing configuration
// values for the application, primarily from environment variables.
//
// It includes functionality to:
//   - Read Keycloak-related configuration from environment variables.
//   - Construct URLs such as the JWKS endpoint from the Keycloak URL and realm.
//   - Provide a Config struct that can be used throughout the application.
//
// Usage of this package ensures that all configuration values are
// validated at startup, and any missing critical environment variables
// result in an error.
package config
