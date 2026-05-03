package testlogger

import (
	"bytes"
	"log/slog"
	"sync"
)

// LogEntry represents a simplified, testable version of a slog record.
type LogEntry struct {
	Fields map[string]any
	Msg    string
	Level  slog.Level
}

// LogCapture is a concurrency-safe container for capturing logs during tests.
type LogCapture struct {
	Buf     *bytes.Buffer
	Entries []LogEntry
	mu      sync.Mutex
}

// Reset clears the captured log buffer and all log entries while retaining the
// underlying memory capacity. It is concurrency-safe and zeros out slice elements
// to prevent memory leaks from retained pointers.
func (c *LogCapture) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.Buf.Reset()

	clear(c.Entries)          // Sets all elements to their zero value
	c.Entries = c.Entries[:0] // Resets length to 0, keeps capacity
}

// GetOutput returns a point-in-time snapshot of all captured log entries.
// It returns a deep copy of the slice to ensure that assertions can be
// performed safely even if the background processes are still logging.
func (c *LogCapture) GetOutput() []LogEntry {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Using make/copy ensures the underlying array of the returned slice
	// is not shared with the active LogCapture.
	out := make([]LogEntry, len(c.Entries))
	copy(out, c.Entries)
	return out
}
