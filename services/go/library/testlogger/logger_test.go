package testlogger_test

import (
	"log/slog"
	"testing"

	"go-services/library/assert"
	"go-services/library/testlogger"
)

func TestLogger(t *testing.T) {
	l, capture := testlogger.New()

	// Test WithAttrs logic
	authLogger := l.With("module", "auth")
	authLogger.Info("login attempt", "user", "alice")

	results := capture.GetOutput()

	testlogger.Assert(t, results).
		AtIndex(0, slog.LevelInfo, "login attempt", "check message").
		HasField(0, "module", "auth", "check WithAttr field").
		HasField(0, "user", "alice", "check record field")
}

func TestLogCapture_WithGroup(t *testing.T) {
	logger, capture := testlogger.New()

	// 1. Create a grouped logger
	requestLogger := logger.WithGroup("request").With("id", "req-123")

	// 2. Add a nested group
	dbLogger := requestLogger.WithGroup("database")
	dbLogger.Info("query executed", "duration_ms", 150)

	// 3. Get captured output
	entries := capture.GetOutput()

	// 4. Assertions
	testlogger.Assert(t, entries).
		Count(1, "should capture exactly one log").
		AtIndex(0, slog.LevelInfo, "query executed", "log entry").
		HasField(0, "duration_ms", any(int64(150)), "field from log line")

	entry := entries[0]
	assert.Equal(t, entry.Msg, "query executed", "Message mismatch")

	// Verify fields from both parent logger and specific log line
	assert.Equal(t, entry.Fields["id"], "req-123", "missing field from parent group")
	assert.Equal(t, entry.Fields["duration_ms"], any(int64(150)), "missing field from log line")
}
