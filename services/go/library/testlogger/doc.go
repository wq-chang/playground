// Package testlogger provides a spy implementation for slog.Handler that captures
// structured logs during unit tests. It enables precise assertions on log levels,
// messages, and attributes—including those nested via WithAttrs and WithGroup.
// The package features a fluent assertion API to simplify verifying log content
// and ensures that complex structured data remains easily testable.

package testlogger
