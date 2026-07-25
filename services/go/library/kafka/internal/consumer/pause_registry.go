package consumer

import (
	"maps"
	"sync"
	"time"

	"go-services/library/gsync"
)

// topicPauser is the subset of *kgo.Client used by PauseRegistry
// to tell Kafka to stop fetching a topic.
type topicPauser interface {
	PauseFetchTopics(topics ...string) []string
}

// PauseInfo records why and when a topic was paused.
type PauseInfo struct {
	Cause    error
	PausedAt time.Time
}

// PauseRegistry owns paused-topic state and provides immutable snapshots
// for concurrent readers. Reads (IsPaused, Snapshot) are lock-free via
// gsync.Value. When a topic is paused, PauseFetchTopics is called on
// pauser to stop fetching from that topic.
type PauseRegistry struct {
	snapshot     gsync.Value[map[string]PauseInfo]
	pausedTopics map[string]PauseInfo
	pauser       topicPauser
	now          func() time.Time
	mu           sync.Mutex
}

// NewPauseRegistry creates a paused-topic registry with injectable clock.
// pauser is called to PauseFetchTopics when a topic is paused.
func NewPauseRegistry(now func() time.Time, pauser topicPauser) *PauseRegistry {
	pr := &PauseRegistry{
		pausedTopics: make(map[string]PauseInfo),
		pauser:       pauser,
		now:          now,
		mu:           sync.Mutex{},
		snapshot:     gsync.Value[map[string]PauseInfo]{},
	}
	pr.snapshot.Store(make(map[string]PauseInfo))
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

	pr.pausedTopics[topic] = PauseInfo{
		Cause:    cause,
		PausedAt: pr.now(),
	}
	pr.snapshot.Store(maps.Clone(pr.pausedTopics))
	pr.pauser.PauseFetchTopics(topic)

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
func (pr *PauseRegistry) Snapshot() map[string]PauseInfo {
	return pr.snapshot.Load()
}
