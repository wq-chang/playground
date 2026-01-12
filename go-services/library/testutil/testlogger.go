package testutil

import (
	"bytes"
	"log/slog"
	"testing"

	"go-services/library/assert"
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
func (tl *TestLogger) AssertEmpty(msg string, msgArgs ...any) {
	tl.t.Helper()
	assert.Zero(tl.t, tl.Capture.String(), msg, msgArgs...)
}

// AssertNotEmpty asserts that at least one log was written
func (tl *TestLogger) AssertNotEmpty(msg string, msgArgs ...any) {
	tl.t.Helper()
	assert.NotZero(tl.t, tl.Capture.String(), msg, msgArgs...)
}

// AssertContains asserts that the log output contains the given substring
func (tl *TestLogger) AssertContains(substring string, msg string, msgArgs ...any) {
	tl.t.Helper()
	assert.StringContains(tl.t, tl.Capture.String(), substring, msg, msgArgs...)
}

// AssertContains asserts that the log output contains all the given substrings
func (tl *TestLogger) AssertContainsAll(substrings []string, msg string, msgArgs ...any) {
	tl.t.Helper()
	assert.StringContainsAll(tl.t, tl.Capture.String(), substrings, msg, msgArgs...)
}

// AssertNotContains asserts that the log output does not contain the given substring
func (tl *TestLogger) AssertNotContains(substring string, msg string, msgArgs ...any) {
	tl.t.Helper()
	assert.StringNotContains(tl.t, tl.Capture.String(), substring, msg, msgArgs...)
}

// AssertLastLevel asserts that the most recent log has the specified level
func (tl *TestLogger) AssertLastLevel(level slog.Level, msg string, msgArgs ...any) {
	tl.t.Helper()
	assert.Equal(tl.t, tl.Capture.LastLevel().String(), level.String(), msg, msgArgs...)
}

// AssertHasLevel asserts that at least one log has the specified level
func (tl *TestLogger) AssertHasLevel(level slog.Level, msg string, msgArgs ...any) {
	tl.t.Helper()
	assert.SliceContains(tl.t, tl.Capture.Levels, level, msg, msgArgs...)
}

// AssertLogCount asserts the total number of logs written
func (tl *TestLogger) AssertLogCount(count int, msg string, msgArgs ...any) {
	tl.t.Helper()
	assert.Equal(tl.t, tl.Capture.LogCount(), count, msg, msgArgs...)
}

// AssertLevelCount asserts the number of logs at a specific level
func (tl *TestLogger) AssertLevelCount(level slog.Level, count int, msg string, msgArgs ...any) {
	tl.t.Helper()
	assert.Equal(tl.t, tl.Capture.LevelCount(level), count, msg, msgArgs...)
}

// AssertLevelAt asserts the log level at a specific index
func (tl *TestLogger) AssertLevelAt(index int, level slog.Level, msg string, msgArgs ...any) {
	tl.t.Helper()
	assert.SliceIndex(tl.t, tl.Capture.Levels, index, level, msg, msgArgs...)
}

// Reset clears the captured log buffer and level history for reuse
func (tl *TestLogger) Reset() {
	tl.Capture.Reset()
}
