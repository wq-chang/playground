// services/go/library/kafka/internal/consumer/pause_registry.go
package consumer

import (
	"maps"
	"sync"
	"time"

	"go-services/library/gsync"
	"go-services/library/kafka/ktype"
)

// PauseRegistry owns paused-topic state and provides immutable snapshots
// for concurrent readers. Reads (IsPaused, Snapshot) are lock-free via
// gsync.Value. It does not handle Kafka-level pause operations or offset
// commits — those belong to Committer and Dispatcher in later steps.
type PauseRegistry struct {
	snapshot     gsync.Value[map[string]ktype.PauseInfo]
	pausedTopics map[string]ktype.PauseInfo
	now          func() time.Time
	mu           sync.Mutex
}

// NewPauseRegistry creates a paused-topic registry with injectable clock.
func NewPauseRegistry(now func() time.Time) *PauseRegistry {
	pr := &PauseRegistry{
		pausedTopics: make(map[string]ktype.PauseInfo),
		now:          now,
		mu:           sync.Mutex{},
		snapshot:     gsync.Value[map[string]ktype.PauseInfo]{},
	}
	pr.snapshot.Store(make(map[string]ktype.PauseInfo))
	return pr
}

// Pause marks a topic as paused. Returns true only when this call applied
// the pause for the first time (idempotent on subsequent calls for the same topic).
func (pr *PauseRegistry) Pause(topic string, cause error) bool {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	if _, ok := pr.pausedTopics[topic]; ok {
		return false
	}

	pr.pausedTopics[topic] = ktype.PauseInfo{
		Cause:    cause,
		PausedAt: pr.now(),
	}
	pr.snapshot.Store(maps.Clone(pr.pausedTopics))
	return true
}

// IsPaused reports whether a topic is currently marked as paused.
// Lock-free — reads from the atomic snapshot.
func (pr *PauseRegistry) IsPaused(topic string) bool {
	snap := pr.snapshot.Load()
	_, ok := snap[topic]
	return ok
}

// Snapshot returns an immutable view of all paused topics and their info.
// Lock-free — returns the atomic snapshot directly.
func (pr *PauseRegistry) Snapshot() map[string]ktype.PauseInfo {
	return pr.snapshot.Load()
}
