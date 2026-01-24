package testlogger_test

import (
	"log/slog"
	"testing"

	"go-services/library/testlogger"
)

func TestLogAssert(t *testing.T) {
	entries := []testlogger.LogEntry{
		{
			Level: slog.LevelInfo,
			Msg:   "request started",
			Fields: map[string]any{
				"path": "/users",
				"id":   123,
			},
		},
		{
			Level: slog.LevelError,
			Msg:   "database timeout",
			Fields: map[string]any{
				"retry": true,
			},
		},
	}

	t.Run("Count matches total entries", func(t *testing.T) {
		// Should pass
		testlogger.Assert(t, entries).Count(2, "expecting two logs")
	})

	t.Run("AtIndex validates level and message ignoring fields", func(t *testing.T) {
		// Index 0 check
		testlogger.Assert(t, entries).AtIndex(0, slog.LevelInfo, "request started", "check start log")

		// Index 1 check
		testlogger.Assert(t, entries).AtIndex(1, slog.LevelError, "database timeout", "check error log")
	})

	t.Run("HasField validates metadata at specific index", func(t *testing.T) {
		// Check path in the first log
		testlogger.Assert(t, entries).HasField(0, "path", "/users", "verify request path")

		// Check retry flag in the second log
		testlogger.Assert(t, entries).HasField(1, "retry", true, "verify retry policy")
	})

	t.Run("Full Chain validation", func(t *testing.T) {
		// Validates everything in one fluent chain
		testlogger.Assert(t, entries).
			Count(2, "total logs recorded").
			AtIndex(0, slog.LevelInfo, "request started", "first entry").
			HasField(0, "id", 123, "check ID").
			AtIndex(1, slog.LevelError, "database timeout", "second entry")
	})
}
