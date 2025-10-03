package testutil

import (
	"bytes"
	"log/slog"
)

// NewFakeLogger creates a logger that writes messages to an in-memory buffer
// without timestamps, suitable for testing.
func NewFakeLogger() (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}

	// Use a TextHandler but disable time and level formatting
	handler := slog.NewTextHandler(buf,
		&slog.HandlerOptions{
			AddSource: false, // optional: skip file/line info
			Level:     slog.LevelDebug,
			ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
				// Remove time attribute
				if a.Key == slog.TimeKey {
					return slog.Attr{}
				}
				return a
			},
		},
	)

	logger := slog.New(handler)
	return logger, buf
}
