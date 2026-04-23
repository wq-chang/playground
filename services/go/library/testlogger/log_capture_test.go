package testlogger

import (
	"bytes"
	"log/slog"
	"sync"
	"testing"

	"go-services/library/assert"
)

func TestLogCapture(t *testing.T) {
	t.Run("Reset clears data but maintains capacity", func(t *testing.T) {
		lc := &LogCapture{
			Buf:     bytes.NewBuffer(make([]byte, 0, 1024)),
			Entries: make([]LogEntry, 0, 10),
			mu:      sync.Mutex{},
		}

		// 1. Fill with data
		lc.Buf.WriteString("log data")
		lc.Entries = append(lc.Entries, LogEntry{
			Level:  slog.LevelInfo,
			Msg:    "test message",
			Fields: map[string]any{"key": "val"},
		})

		initialCap := cap(lc.Entries)
		initialBufCap := lc.Buf.Cap()

		// 2. Reset
		lc.Reset()

		// 3. Assertions
		assert.Equal(t, lc.Buf.Len(), 0, "buffer length after reset")
		assert.SliceLen(t, lc.Entries, 0, "log entries length after reset")

		// Verify capacity is preserved for performance
		assert.Equal(t, lc.Buf.Cap(), initialBufCap, "buffer cap after reset")
		assert.Equal(t, cap(lc.Entries), initialCap, "log entries cap after reset")

		// Verify clear() actually zeroed the underlying array
		// Re-slice to old capacity to "peek" at the memory
		peeking := lc.Entries[:initialCap]
		assert.True(
			t,
			peeking[0].Msg == "" && peeking[0].Fields == nil,
			"clear() failed to zero out the underlying array",
		)
	})

	t.Run("GetOutput returns a deep copy", func(t *testing.T) {
		lc := &LogCapture{
			Buf: &bytes.Buffer{},
			Entries: []LogEntry{
				{Msg: "original", Level: slog.LevelInfo, Fields: map[string]any{}},
			},
			mu: sync.Mutex{},
		}

		output := lc.GetOutput()

		// Modify the original
		lc.mu.Lock()
		lc.Entries[0].Msg = "modified"
		lc.mu.Unlock()

		// Verify the snapshot stayed the same
		assert.Equal(t, output[0].Msg, "original", "GetOutput did not return a deep copy; found modification")
	})

	t.Run("Concurrency Safety", func(t *testing.T) {
		lc := &LogCapture{
			Buf:     &bytes.Buffer{},
			Entries: make([]LogEntry, 0),
			mu:      sync.Mutex{},
		}

		var wg sync.WaitGroup
		iterations := 100

		// Simulating concurrent writes
		for i := range iterations {
			wg.Add(1)
			go func(val int) {
				defer wg.Done()
				lc.mu.Lock()
				lc.Entries = append(lc.Entries, LogEntry{
					Msg:    "log",
					Level:  slog.LevelInfo,
					Fields: map[string]any{},
				})
				lc.mu.Unlock()
			}(i)
		}

		// Simulating concurrent reads (GetOutput)
		for range 10 {
			wg.Go(func() {
				_ = lc.GetOutput()
			})
		}

		wg.Wait()

		assert.SliceLen(t, lc.Entries, iterations, "logs count")
	})
}
