package testutil

import (
	"bytes"
	"log/slog"
	"testing"
)

// TestLogger wraps LogCapture and provides assertion helpers
// for verifying log output and levels during tests.
//
// It is designed for use with Go’s testing package, enabling
// expressive and reliable logging assertions.
//
// Typical workflow:
//  1. Create a TestLogger using NewTestLogger(t).
//  2. Use the returned slog.Logger for your code under test.
//  3. Use the provided Assert* methods to check the logged output.
//
// Example:
//
//	tl := testutil.NewTestLogger(t)
//	log := tl.Logger
//
//	log.Info("initialized service")
//	log.Error("failed to connect")
//
//	tl.AssertHasLevel(slog.LevelError)
//	tl.AssertContains("failed to connect")
//	tl.AssertLevelCount(slog.LevelInfo, 1)
type TestLogger struct {
	Logger  *slog.Logger // The logger instance used in tests; emits logs into the in-memory buffer.
	Capture *LogCapture  // Captures log output and levels for later inspection or assertions.
	t       *testing.T   // The testing context, used to report assertion failures.
}

// NewTestLogger creates a new TestLogger with a configured slog.Logger.
//
// The returned logger writes to an in-memory buffer and tracks
// emitted log levels for later inspection. The logger uses a
// TextHandler configured to:
//   - Disable time and source attributes for stable test output.
//   - Record log levels in the Capture.Levels slice.
//
// Example:
//
//	tl := testutil.NewTestLogger(t)
//	log := tl.Logger
//
//	log.Warn("deprecated call")
//	tl.AssertLastLevel(slog.LevelWarn)
func NewTestLogger(t *testing.T) *TestLogger {
	t.Helper()

	buf := &bytes.Buffer{}
	capture := &LogCapture{
		Buf:    buf,
		Levels: make([]slog.Level, 0),
	}

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
	if got := tl.Capture.String(); !tl.Capture.IsEmpty() {
		tl.t.Errorf("got log output %q, want none", got)
	}
}

// AssertNotEmpty asserts that at least one log was written
func (tl *TestLogger) AssertNotEmpty() {
	tl.t.Helper()
	if tl.Capture.IsEmpty() {
		tl.t.Error("got no log output, want at least one log entry")
	}
}

// AssertContains asserts that the log output contains the given substring
func (tl *TestLogger) AssertContains(substring string) {
	tl.t.Helper()
	if got := tl.Capture.String(); !tl.Capture.Contains(substring) {
		tl.t.Errorf("got log output %q, want it to contain %q", got, substring)
	}
}

// AssertNotContains asserts that the log output does not contain the given substring
func (tl *TestLogger) AssertNotContains(substring string) {
	tl.t.Helper()
	if got := tl.Capture.String(); tl.Capture.Contains(substring) {
		tl.t.Errorf("got log output %q, want it not to contain %q", got, substring)
	}
}

// AssertLastLevel asserts that the most recent log has the specified level
func (tl *TestLogger) AssertLastLevel(level slog.Level) {
	tl.t.Helper()
	if got, want := tl.Capture.LastLevel(), level; got != want {
		tl.t.Errorf("last log level = %v, want %v", got, want)
	}
}

// AssertHasLevel asserts that at least one log has the specified level
func (tl *TestLogger) AssertHasLevel(level slog.Level) {
	tl.t.Helper()
	if !tl.Capture.HasLevel(level) {
		tl.t.Errorf("got no log with level %v, want at least one", level)
	}
}

// AssertLogCount asserts the total number of logs written
func (tl *TestLogger) AssertLogCount(count int) {
	tl.t.Helper()
	if got, want := tl.Capture.LogCount(), count; got != want {
		tl.t.Errorf("got %d logs, want %d", got, want)
	}
}

// AssertLevelCount asserts the number of logs at a specific level
func (tl *TestLogger) AssertLevelCount(level slog.Level, count int) {
	tl.t.Helper()
	if got, want := tl.Capture.LevelCount(level), count; got != want {
		tl.t.Errorf("got %d logs at level %v, want %d", got, level, want)
	}
}

// AssertLevelAt asserts the log level at a specific index
func (tl *TestLogger) AssertLevelAt(index int, level slog.Level) {
	tl.t.Helper()
	got, ok := tl.Capture.LevelAt(index)
	want := level
	if !ok {
		tl.t.Errorf("no log at index %d, want level %v", index, want)
		return
	}
	if got != want {
		tl.t.Errorf("log[%d] level = %v, want %v", index, got, want)
	}
}

// Reset clears the captured log buffer and level history for reuse
func (tl *TestLogger) Reset() {
	tl.Capture.Reset()
}
