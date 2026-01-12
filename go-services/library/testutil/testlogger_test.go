package testutil_test

import (
	"log/slog"
	"testing"

	"go-services/library/testutil"
)

func TestTestLogger(t *testing.T) {
	testLogger := testutil.NewTestLogger(t)

	testLogger.Logger.Info("hello", "key", "value")

	testLogger.AssertContains("hello", "should have generic message after execution")
	testLogger.AssertContains("key=value", "should have key value pair")
	testLogger.AssertNotContains("time=", "should not log time")
	testLogger.AssertLastLevel(slog.LevelInfo, "last log level")
}

func TestTestLoggerMultipleLevels(t *testing.T) {
	testLogger := testutil.NewTestLogger(t)

	testLogger.Logger.Debug("debug message")
	testLogger.Logger.Info("info message", "user", "alice")
	testLogger.Logger.Warn("warn message")
	testLogger.Logger.Error("error message", "err", "fail")

	testLogger.AssertContains("debug message", "should have debug message")
	testLogger.AssertContains("info message", "should have info message")
	testLogger.AssertContains("warn message", "should have warn message")
	testLogger.AssertContains("error message", "should have error message")

	// Check all levels were captured
	testLogger.AssertLogCount(4, "log count")

	// Check specific levels exist
	testLogger.AssertHasLevel(slog.LevelDebug, "should have debug level")
	testLogger.AssertHasLevel(slog.LevelInfo, "should have info level")
	testLogger.AssertHasLevel(slog.LevelWarn, "should have warn level")
	testLogger.AssertHasLevel(slog.LevelError, "should have error level")

	// Last log level should be ERROR
	testLogger.AssertLastLevel(slog.LevelError, "last level")

	// Check level counts
	testLogger.AssertLevelCount(slog.LevelDebug, 1, "debug level count")
	testLogger.AssertLevelCount(slog.LevelInfo, 1, "info level count")
	testLogger.AssertLevelCount(slog.LevelWarn, 1, "warn level count")
	testLogger.AssertLevelCount(slog.LevelError, 1, "error level count")
}

func TestTestLoggerReset(t *testing.T) {
	testLogger := testutil.NewTestLogger(t)

	testLogger.Logger.Info("first message")
	testLogger.Logger.Error("second message")

	testLogger.AssertNotEmpty("should contains logs")
	testLogger.AssertLogCount(2, "log count before reset")

	testLogger.Reset()

	testLogger.AssertEmpty("should be empty log after reset")
	testLogger.AssertLogCount(0, "should have no log after reset")

	testLogger.Logger.Warn("third message")

	testLogger.AssertNotContains("first message", "should not have the message that called before reset")
	testLogger.AssertNotContains("second message", "should not have the message that called before reset")
	testLogger.AssertContains("third message", "should have the message that called after reset")
	testLogger.AssertLastLevel(slog.LevelWarn, "last log level")
	testLogger.AssertLogCount(1, "log count after reset and logs")
}

func TestTestLoggerNoTimestamp(t *testing.T) {
	testLogger := testutil.NewTestLogger(t)

	testLogger.Logger.Debug("debug msg")
	testLogger.Logger.Info("info msg")
	testLogger.Logger.Warn("warn msg")
	testLogger.Logger.Error("error msg")

	testLogger.AssertNotContains("time=", "should not have timestamp")
}

func TestTestLoggerHelperMethods(t *testing.T) {
	testLogger := testutil.NewTestLogger(t)

	// Test empty on fresh logger
	testLogger.AssertEmpty("should not have log before execution")
	testLogger.AssertLogCount(0, "log count before execution")

	testLogger.Logger.Error("test error", "code", 500)
	testLogger.Logger.Error("another error", "code", 503)
	testLogger.Logger.Info("info log")

	// Test Contains
	testLogger.AssertContains("test error", "log message")
	testLogger.AssertContains("code=500", "log key value pair")
	testLogger.AssertNotContains("nonexistent", "logs")

	// Test HasLevel
	testLogger.AssertHasLevel(slog.LevelError, "should have error level")
	testLogger.AssertHasLevel(slog.LevelInfo, "should have info level")

	// Test LastLevel
	testLogger.AssertLastLevel(slog.LevelInfo, "last level should be info")

	// Test LevelCount
	testLogger.AssertLevelCount(slog.LevelError, 2, "error level count after execution")
	testLogger.AssertLevelCount(slog.LevelInfo, 1, "info level count after execution")
	testLogger.AssertLevelCount(slog.LevelWarn, 0, "warn level count after execution")

	// Test LogCount
	testLogger.AssertLogCount(3, "log count after execution")

	// Test not empty
	testLogger.AssertNotEmpty("logs should not be empty after execution")
}

func TestTestLoggerLevelsSlice(t *testing.T) {
	testLogger := testutil.NewTestLogger(t)

	testLogger.Logger.Info("first")
	testLogger.Logger.Warn("second")
	testLogger.Logger.Error("third")
	testLogger.Logger.Debug("fourth")

	testLogger.AssertLogCount(4, "log count")

	// Test each level at specific index
	testLogger.AssertLevelAt(0, slog.LevelInfo, "first log level")
	testLogger.AssertLevelAt(1, slog.LevelWarn, "second log level")
	testLogger.AssertLevelAt(2, slog.LevelError, "third log level")
	testLogger.AssertLevelAt(3, slog.LevelDebug, "fourth log level")
}

func TestTestLoggerResetPreservesLogger(t *testing.T) {
	testLogger := testutil.NewTestLogger(t)

	testLogger.Logger.Info("before reset")
	testLogger.AssertLogCount(1, "log count before reset")

	testLogger.Reset()
	testLogger.AssertLogCount(0, "log count after reset")

	// Logger should still work after reset
	testLogger.Logger.Warn("after reset")
	testLogger.AssertLogCount(1, "log count after reset and execution")
	testLogger.AssertLastLevel(slog.LevelWarn, "last log level")
	testLogger.AssertContains("after reset", "message")
	testLogger.AssertNotContains("before reset", "should not contain logs before the reset")
}
