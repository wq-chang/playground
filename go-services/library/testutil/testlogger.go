package testutil

import (
	"bytes"
	"log/slog"
	"testing"
)

// TestLogger wraps LogCapture with test assertions
type TestLogger struct {
	Logger  *slog.Logger
	Capture *LogCapture
	t       *testing.T
}

// NewTestLogger creates a new test logger with assertion helpers
func NewTestLogger(t *testing.T) *TestLogger {
	t.Helper()

	buf := &bytes.Buffer{}
	capture := &LogCapture{
		Buf:    buf,
		Levels: make([]slog.Level, 0),
	}

	// Use a TextHandler but disable time and level formatting
	handler := slog.NewTextHandler(buf,
		&slog.HandlerOptions{
			AddSource: false,
			Level:     slog.LevelDebug,
			ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
				// Remove time attribute
				if a.Key == slog.TimeKey {
					return slog.Attr{}
				}
				if a.Key == slog.LevelKey {
					capture.Levels = append(capture.Levels, a.Value.Any().(slog.Level))
				}
				return a
			},
		},
	)

	logger := slog.New(handler)
	return &TestLogger{
		Logger:  logger,
		Capture: capture,
		t:       t,
	}
}

// AssertEmpty asserts that no logs were written
func (tl *TestLogger) AssertEmpty() {
	tl.t.Helper()
	if !tl.Capture.IsEmpty() {
		tl.t.Errorf("expected no log output but got: %s", tl.Capture.String())
	}
}

// AssertNotEmpty asserts that at least one log was written
func (tl *TestLogger) AssertNotEmpty() {
	tl.t.Helper()
	if tl.Capture.IsEmpty() {
		tl.t.Error("expected log output but none was written")
	}
}

// AssertContains asserts that the log output contains the given substring
func (tl *TestLogger) AssertContains(substring string) {
	tl.t.Helper()
	if !tl.Capture.Contains(substring) {
		tl.t.Errorf("log output missing %q\nGot: %s", substring, tl.Capture.String())
	}
}

// AssertNotContains asserts that the log output does not contain the given substring
func (tl *TestLogger) AssertNotContains(substring string) {
	tl.t.Helper()
	if tl.Capture.Contains(substring) {
		tl.t.Errorf("log output should not contain %q\nGot: %s", substring, tl.Capture.String())
	}
}

// AssertLastLevel asserts that the most recent log has the specified level
func (tl *TestLogger) AssertLastLevel(level slog.Level) {
	tl.t.Helper()
	if tl.Capture.LastLevel() != level {
		tl.t.Errorf("last log level = %v, want %v", tl.Capture.LastLevel(), level)
	}
}

// AssertHasLevel asserts that at least one log has the specified level
func (tl *TestLogger) AssertHasLevel(level slog.Level) {
	tl.t.Helper()
	if !tl.Capture.HasLevel(level) {
		tl.t.Errorf("expected to have log with level %v", level)
	}
}

// AssertLogCount asserts the total number of logs written
func (tl *TestLogger) AssertLogCount(count int) {
	tl.t.Helper()
	if tl.Capture.LogCount() != count {
		tl.t.Errorf("expected %d logs, got %d", count, tl.Capture.LogCount())
	}
}

// AssertLevelCount asserts the number of logs at a specific level
func (tl *TestLogger) AssertLevelCount(level slog.Level, count int) {
	tl.t.Helper()
	actual := tl.Capture.LevelCount(level)
	if actual != count {
		tl.t.Errorf("expected %d logs at level %v, got %d", count, level, actual)
	}
}

// AssertLevelAt asserts the log level at a specific index
func (tl *TestLogger) AssertLevelAt(index int, level slog.Level) {
	tl.t.Helper()
	actual, ok := tl.Capture.LevelAt(index)
	if !ok {
		tl.t.Errorf("expected log at index %d but none exists", index)
		return
	}
	if actual != level {
		tl.t.Errorf("log[%d] level = %v, want %v", index, actual, level)
	}
}

// Reset clears the captured logs
func (tl *TestLogger) Reset() {
	tl.Capture.Reset()
}
