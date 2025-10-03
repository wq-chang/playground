package testutil

import (
	"strings"
	"testing"
)

func TestFakeLogger(t *testing.T) {
	logger, buf := NewFakeLogger()

	logger.Info("hello", "key", "value")
	logger.Error("oops", "err", "fail")

	logs := buf.String()

	if !strings.Contains(logs, "hello") {
		t.Errorf("expected log to contain 'hello', got %q", logs)
	}
	if strings.Contains(logs, "time=") {
		t.Errorf("unexpected timestamp in logs: %q", logs)
	}
}
