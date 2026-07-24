// services/go/library/kafka/internal/consumer/pause_registry_test.go
package consumer_test

import (
	"errors"
	"testing"
	"time"

	"go-services/library/assert"
	"go-services/library/kafka/internal/consumer"
	"go-services/library/require"
)

func TestPauseRegistry_Pause_FirstPauseReturnsTrue(t *testing.T) {
	pr := consumer.NewPauseRegistry(time.Now)
	wasPaused := pr.Pause("topic-a", errors.New("test error"))
	assert.True(t, wasPaused, "first pause should return true")

	snap := pr.Snapshot()
	_, ok := snap["topic-a"]
	assert.True(t, ok, "snapshot should contain paused topic")
}

func TestPauseRegistry_Pause_RepeatedPauseIsIdempotent(t *testing.T) {
	pr := consumer.NewPauseRegistry(time.Now)
	pr.Pause("topic-a", errors.New("test error"))
	wasPaused := pr.Pause("topic-a", errors.New("test error"))
	assert.False(t, wasPaused, "second pause should return false")
}

func TestPauseRegistry_IsPaused(t *testing.T) {
	pr := consumer.NewPauseRegistry(time.Now)
	require.False(t, pr.IsPaused("topic-a"), "not paused yet")

	pr.Pause("topic-a", errors.New("test error"))
	assert.True(t, pr.IsPaused("topic-a"), "should be paused now")
}

func TestPauseRegistry_Snapshot_InitiallyEmpty(t *testing.T) {
	pr := consumer.NewPauseRegistry(time.Now)
	snap := pr.Snapshot()
	assert.NotNil(t, snap, "snapshot should be empty for a fresh registry")
	assert.Equal(t, len(snap), 0, "snapshot should be empty for a fresh registry")
}

func TestPauseRegistry_Snapshot_Immutable(t *testing.T) {
	pr := consumer.NewPauseRegistry(time.Now)
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
	pr := consumer.NewPauseRegistry(clock)
	wasPaused := pr.Pause("topic-a", cause)
	assert.True(t, wasPaused, "first pause should succeed")

	snap := pr.Snapshot()
	info, ok := snap["topic-a"]
	assert.True(t, ok, "topic should be in snapshot")
	assert.ErrorIs(t, info.Cause, cause, "cause should match")
	assert.Equal(t, info.PausedAt, now, "timestamp should match")
}
