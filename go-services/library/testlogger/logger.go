package testlogger

import (
	"bytes"
	"context"
	"log/slog"
	"sync"
)

// spyHandler implements slog.Handler to intercept log records.
// It maintains a reference to a shared LogCapture and tracks the evolution
// of attributes and groups as the logger is cloned.
type spyHandler struct {
	slog.Handler             // The underlying base handler (e.g., TextHandler)
	capture      *LogCapture // Shared state where all clones record their logs
	groupPath    string      // The current active group namespace (e.g., "request.user")
	preFields    []slog.Attr // Attributes accumulated through .With() calls
}

// Handle processes a single log record. It flattens pre-existing attributes
// from the logger's context and the record's specific attributes into
// a single Fields map for easy assertion.
func (h *spyHandler) Handle(ctx context.Context, r slog.Record) error {
	fields := make(map[string]any)
	// Add attributes accumulated from parent loggers (.With calls)
	for _, a := range h.preFields {
		fields[a.Key] = a.Value.Any()
	}

	// Add attributes provided specifically in this log line
	r.Attrs(func(a slog.Attr) bool {
		fields[a.Key] = a.Value.Any()
		return true
	})

	h.capture.mu.Lock()
	h.capture.Entries = append(h.capture.Entries, LogEntry{
		Level:  r.Level,
		Msg:    r.Message,
		Fields: fields,
	})
	h.capture.mu.Unlock()
	return h.Handler.Handle(ctx, r)
}

// WithAttrs creates a new spyHandler containing the provided attributes.
// This allows the spy to track context added to a logger via logger.With(...).
func (h *spyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &spyHandler{
		Handler:   h.Handler.WithAttrs(attrs),
		capture:   h.capture,
		groupPath: "",
		preFields: append(h.preFields, attrs...),
	}
}

// WithGroup creates a new spyHandler that prefixes future attributes with the group name.
// Note: In the current implementation, this updates the groupPath for tracking
// nested namespaces.
func (h *spyHandler) WithGroup(name string) slog.Handler {
	newPath := name
	if h.groupPath != "" {
		newPath = h.groupPath + "." + name
	}
	return &spyHandler{
		Handler:   h.Handler.WithGroup(name),
		capture:   h.capture,
		groupPath: newPath,
		preFields: h.preFields,
	}
}

// New initializes a new *slog.Logger and a corresponding *LogCapture.
// The logger is configured with a Debug level and a TextHandler backend
// that writes to the capture's internal buffer.
func New() (*slog.Logger, *LogCapture) {
	buf := &bytes.Buffer{}
	capture := &LogCapture{Buf: buf, Entries: []LogEntry{}, mu: sync.Mutex{}}

	baseHandler := slog.NewTextHandler(buf, &slog.HandlerOptions{
		Level:     slog.LevelDebug,
		AddSource: false,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			return a
		},
	})

	handler := &spyHandler{Handler: baseHandler, capture: capture, groupPath: "", preFields: []slog.Attr{}}
	return slog.New(handler), capture
}
