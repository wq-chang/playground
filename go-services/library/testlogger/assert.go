package testlogger

import (
	"fmt"
	"log/slog"
	"testing"

	"go-services/library/assert"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// LogAssert wraps a slice of LogEntry to provide chainable assertion methods.
type LogAssert struct {
	t       *testing.T
	entries []LogEntry
}

var IgnoreLogFieldsOpt = []cmp.Option{cmpopts.IgnoreFields(LogEntry{}, "Fields")}

// Assert initializes a new LogAssert with the provided test context and entries.
// It returns a pointer to LogAssert to allow for fluent method chaining.
func Assert(t *testing.T, entries []LogEntry) *LogAssert {
	return &LogAssert{t: t, entries: entries}
}

// Count asserts that the total number of captured log entries matches the expected count.
// It fails the test immediately if the lengths do not match.
func (a *LogAssert) Count(expected int, msg string, msgArgs ...any) *LogAssert {
	a.t.Helper()

	formattedMsg := fmt.Sprintf(msg, msgArgs...)
	assert.Equal(a.t, len(a.entries), expected, "log count mismatch :: %s", formattedMsg)
	return a
}

// Empty asserts that no log entries were captured.
func (a *LogAssert) Empty(msg string, msgArgs ...any) *LogAssert {
	a.t.Helper()

	formattedMsg := fmt.Sprintf(msg, msgArgs...)
	assert.SliceEmpty(a.t, a.entries, "unexpected log :: %s", formattedMsg)

	return a
}

// NotEmpty asserts that at least one log entry was captured.
func (a *LogAssert) NotEmpty(msg string, msgArgs ...any) *LogAssert {
	a.t.Helper()

	formattedMsg := fmt.Sprintf(msg, msgArgs...)
	assert.SliceNotEmpty(a.t, a.entries, "missing log :: %s", formattedMsg)

	return a
}

// AtIndex verifies the slog.Level and Message of a log entry at a specific index.
// It ignores the 'Fields' map during comparison, focusing only on the core log properties.
func (a *LogAssert) AtIndex(index int, level slog.Level, logMsg string, msg string, msgArgs ...any) *LogAssert {
	a.t.Helper()

	want := LogEntry{Level: level, Msg: logMsg, Fields: nil}
	formattedMsg := fmt.Sprintf(msg, msgArgs...)
	assert.SliceAtOpt(
		a.t,
		a.entries,
		index,
		want,
		IgnoreLogFieldsOpt,
		"log property mismatch :: %s",
		formattedMsg,
	)

	return a
}

// HasField verifies that a specific key-value pair exists within the 'Fields' map
// of the log entry at the given index.
// It first performs a bounds check on the index before attempting to access the map.
func (a *LogAssert) HasField(index int, key string, want any, msg string, msgArgs ...any) *LogAssert {
	a.t.Helper()

	formattedMsg := fmt.Sprintf(msg, msgArgs...)
	if !assert.SliceIndex(a.t, a.entries, index, "log index out of bounds :: %s", formattedMsg) {
		return a
	}

	assert.MapAt(a.t, a.entries[index].Fields, key, want, "log field mismatch :: %s", formattedMsg)

	return a
}
