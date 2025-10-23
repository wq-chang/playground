package testutil

import (
	"bytes"
	"log/slog"
	"slices"
)

type LogCapture struct {
	Buf    *bytes.Buffer
	Levels []slog.Level
}

// String returns the logged content as a string
func (c *LogCapture) String() string {
	return c.Buf.String()
}

// Reset clears the buffer and levels for reuse
func (c *LogCapture) Reset() {
	c.Buf.Reset()
	c.Levels = make([]slog.Level, 0)
}

// Contains checks if the log output contains the given substring
func (c *LogCapture) Contains(s string) bool {
	return bytes.Contains(c.Buf.Bytes(), []byte(s))
}

// HasLevel checks if any log entry has the specified level
func (c *LogCapture) HasLevel(level slog.Level) bool {
	return slices.Contains(c.Levels, level)
}

// LastLevel returns the level of the most recent log entry
// Returns LevelInfo if no logs have been written
func (c *LogCapture) LastLevel() slog.Level {
	if len(c.Levels) == 0 {
		return slog.LevelInfo
	}
	return c.Levels[len(c.Levels)-1]
}

// LevelCount returns the number of log entries at the specified level
func (c *LogCapture) LevelCount(level slog.Level) int {
	count := 0
	for _, l := range c.Levels {
		if l == level {
			count++
		}
	}
	return count
}

// IsEmpty checks if no log was written
func (c *LogCapture) IsEmpty() bool {
	return c.Buf.Len() == 0
}

// LogCount returns the total number of log entries
func (c *LogCapture) LogCount() int {
	return len(c.Levels)
}

// LevelAt returns the log level at the specified index
// Returns LevelInfo and false if index is out of bounds
func (c *LogCapture) LevelAt(index int) (slog.Level, bool) {
	if index < 0 || index >= len(c.Levels) {
		return slog.LevelInfo, false
	}
	return c.Levels[index], true
}
