package testutil_test

import (
	"log/slog"
	"testing"

	"go-services/library/testutil"
)

func TestFakeLogger(t *testing.T) {
	testLogger := testutil.NewTestLogger(t)

	testLogger.Logger.Info("hello", "key", "value")

	testLogger.AssertContains("hello")
	testLogger.AssertContains("key=value")
	testLogger.AssertNotContains("time=")
	testLogger.AssertLastLevel(slog.LevelInfo)
}

func TestFakeLoggerMultipleLevels(t *testing.T) {
	testLogger := testutil.NewTestLogger(t)

	testLogger.Logger.Debug("debug message")
	testLogger.Logger.Info("info message", "user", "alice")
	testLogger.Logger.Warn("warn message")
	testLogger.Logger.Error("error message", "err", "fail")

	testLogger.AssertContains("debug message")
	testLogger.AssertContains("info message")
	testLogger.AssertContains("warn message")
	testLogger.AssertContains("error message")

	// Check all levels were captured
	testLogger.AssertLogCount(4)

	// Check specific levels exist
	testLogger.AssertHasLevel(slog.LevelDebug)
	testLogger.AssertHasLevel(slog.LevelInfo)
	testLogger.AssertHasLevel(slog.LevelWarn)
	testLogger.AssertHasLevel(slog.LevelError)

	// Last log level should be ERROR
	testLogger.AssertLastLevel(slog.LevelError)

	// Check level counts
	testLogger.AssertLevelCount(slog.LevelDebug, 1)
	testLogger.AssertLevelCount(slog.LevelInfo, 1)
	testLogger.AssertLevelCount(slog.LevelWarn, 1)
	testLogger.AssertLevelCount(slog.LevelError, 1)
}

func TestFakeLoggerReset(t *testing.T) {
	testLogger := testutil.NewTestLogger(t)

	testLogger.Logger.Info("first message")
	testLogger.Logger.Error("second message")

	testLogger.AssertNotEmpty()
	testLogger.AssertLogCount(2)

	testLogger.Reset()

	testLogger.AssertEmpty()
	testLogger.AssertLogCount(0)

	testLogger.Logger.Warn("third message")

	testLogger.AssertNotContains("first message")
	testLogger.AssertNotContains("second message")
	testLogger.AssertContains("third message")
	testLogger.AssertLastLevel(slog.LevelWarn)
	testLogger.AssertLogCount(1)
}

func TestFakeLoggerNoTimestamp(t *testing.T) {
	testLogger := testutil.NewTestLogger(t)

	testLogger.Logger.Debug("debug msg")
	testLogger.Logger.Info("info msg")
	testLogger.Logger.Warn("warn msg")
	testLogger.Logger.Error("error msg")

	testLogger.AssertNotContains("time=")
}

func TestFakeLoggerHelperMethods(t *testing.T) {
	testLogger := testutil.NewTestLogger(t)

	// Test empty on fresh logger
	testLogger.AssertEmpty()
	testLogger.AssertLogCount(0)

	testLogger.Logger.Error("test error", "code", 500)
	testLogger.Logger.Error("another error", "code", 503)
	testLogger.Logger.Info("info log")

	// Test Contains
	testLogger.AssertContains("test error")
	testLogger.AssertContains("code=500")
	testLogger.AssertNotContains("nonexistent")

	// Test HasLevel
	testLogger.AssertHasLevel(slog.LevelError)
	testLogger.AssertHasLevel(slog.LevelInfo)

	// Test LastLevel
	testLogger.AssertLastLevel(slog.LevelInfo)

	// Test LevelCount
	testLogger.AssertLevelCount(slog.LevelError, 2)
	testLogger.AssertLevelCount(slog.LevelInfo, 1)
	testLogger.AssertLevelCount(slog.LevelWarn, 0)

	// Test LogCount
	testLogger.AssertLogCount(3)

	// Test not empty
	testLogger.AssertNotEmpty()
}

func TestFakeLoggerLevelsSlice(t *testing.T) {
	testLogger := testutil.NewTestLogger(t)

	testLogger.Logger.Info("first")
	testLogger.Logger.Warn("second")
	testLogger.Logger.Error("third")
	testLogger.Logger.Debug("fourth")

	testLogger.AssertLogCount(4)

	// Test each level at specific index
	testLogger.AssertLevelAt(0, slog.LevelInfo)
	testLogger.AssertLevelAt(1, slog.LevelWarn)
	testLogger.AssertLevelAt(2, slog.LevelError)
	testLogger.AssertLevelAt(3, slog.LevelDebug)
}

func TestFakeLoggerResetPreservesLogger(t *testing.T) {
	testLogger := testutil.NewTestLogger(t)

	testLogger.Logger.Info("before reset")
	testLogger.AssertLogCount(1)

	testLogger.Reset()
	testLogger.AssertLogCount(0)

	// Logger should still work after reset
	testLogger.Logger.Warn("after reset")
	testLogger.AssertLogCount(1)
	testLogger.AssertLastLevel(slog.LevelWarn)
	testLogger.AssertContains("after reset")
	testLogger.AssertNotContains("before reset")
}
