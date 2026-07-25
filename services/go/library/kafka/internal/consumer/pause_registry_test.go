package consumer_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"go-services/library/assert"
	"go-services/library/kafka/internal/consumer"
)

// stubTopicPauser records calls to PauseFetchTopics for test verification.
type stubTopicPauser struct {
	calls [][]string
	mu    sync.Mutex
}

func (s *stubTopicPauser) PauseFetchTopics(topics ...string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, topics)
	return nil
}

func (s *stubTopicPauser) pausedTopics() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var all []string
	for _, call := range s.calls {
		all = append(all, call...)
	}
	return all
}

func (s *stubTopicPauser) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

func TestPauseRegistry_Pause_CallsPauseFetchTopics(t *testing.T) {
	stub := &stubTopicPauser{}
	pr := consumer.NewPauseRegistry(time.Now, stub)

	wasPaused := pr.Pause("topic-a", errors.New("test error"))
	assert.True(t, wasPaused, "first pause should return true")
	assert.Equal(t, stub.callCount(), 1, "PauseFetchTopics should be called once")
	assert.Equal(t, stub.pausedTopics(), []string{"topic-a"}, "should pause topic-a")
}

func TestPauseRegistry_Pause_RepeatedPauseIsIdempotent(t *testing.T) {
	stub := &stubTopicPauser{}
	pr := consumer.NewPauseRegistry(time.Now, stub)

	pr.Pause("topic-a", errors.New("test error"))
	wasPaused := pr.Pause("topic-a", errors.New("test error"))
	assert.False(t, wasPaused, "second pause should return false")
	assert.Equal(t, stub.callCount(), 1, "PauseFetchTopics should only be called once")
}

func TestPauseRegistry_Pause_DistinctTopics(t *testing.T) {
	stub := &stubTopicPauser{}
	pr := consumer.NewPauseRegistry(time.Now, stub)

	pr.Pause("a", errors.New("e1"))
	pr.Pause("b", errors.New("e2"))

	assert.Equal(t, stub.callCount(), 2, "PauseFetchTopics should be called for each distinct topic")
	assert.Equal(t, len(pr.Snapshot()), 2, "snapshot should have both topics")
}

func TestPauseRegistry_Snapshot_InitiallyEmpty(t *testing.T) {
	pr := consumer.NewPauseRegistry(time.Now, &stubTopicPauser{})
	snap := pr.Snapshot()
	assert.NotNil(t, snap, "snapshot should be empty for a fresh registry")
	assert.Equal(t, len(snap), 0, "snapshot should be empty for a fresh registry")
}

func TestPauseRegistry_Snapshot_Immutable(t *testing.T) {
	stub := &stubTopicPauser{}
	pr := consumer.NewPauseRegistry(time.Now, stub)
	pr.Pause("topic-a", errors.New("test error"))

	snap := pr.Snapshot()
	assert.Equal(t, len(snap), 1, "snapshot should have 1 entry")

	pr.Pause("topic-b", errors.New("test error"))
	assert.Equal(t, len(snap), 1, "old snapshot should still have 1 entry")
}

func TestPauseRegistry_Snapshot_ContainsPauseInfo(t *testing.T) {
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }

	cause := errors.New("dlt error")
	pr := consumer.NewPauseRegistry(clock, &stubTopicPauser{})
	wasPaused := pr.Pause("topic-a", cause)
	assert.True(t, wasPaused, "first pause should succeed")

	snap := pr.Snapshot()
	info, ok := snap["topic-a"]
	assert.True(t, ok, "topic should be in snapshot")
	assert.ErrorIs(t, info.Cause, cause, "cause should match")
	assert.Equal(t, info.PausedAt, now, "timestamp should match")
}
