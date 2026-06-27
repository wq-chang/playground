// services/go/library/kafka/internal/consumer/partition_registry_test.go
package consumer_test

import (
	"go-services/library/testlogger"
	"context"
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"

	"go-services/library/assert"
	"go-services/library/kafka/internal/consumer"
	"go-services/library/require"
)

func testSub(topic string) consumer.Subscription {
	return consumer.Subscription{
		Topic:         topic,
		Handler:       func(context.Context, *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: consumer.FailurePolicy{},
		AckMode:       consumer.AckModeAtLeastOnce,
	}
}

func TestPartitionRegistry_New(t *testing.T) {
	r := consumer.NewPartitionRegistry(testlogger.NewLogger())
	assert.NotNil(t, r, "NewPartitionRegistry should not return nil")
}

func TestPartitionRegistry_Get_Missing(t *testing.T) {
	r := consumer.NewPartitionRegistry(testlogger.NewLogger())
	_, ok := r.Get(consumer.Key{Topic: "t", Partition: 0})
	assert.False(t, ok, "Get for missing key should return false")
}

func TestPartitionRegistry_GetOrCreate_Creates(t *testing.T) {
	r := consumer.NewPartitionRegistry(testlogger.NewLogger())
	key := consumer.Key{Topic: "t", Partition: 1}

	ps, created, err := r.GetOrCreate(key, testSub("t"), context.Background(), 10)
	require.NoError(t, err, "GetOrCreate should succeed")
	assert.True(t, created, "should report created=true for new key")
	assert.True(t, ps.IsRunning(), "new state should be running")
}

func TestPartitionRegistry_GetOrCreate_ReturnsExisting(t *testing.T) {
	r := consumer.NewPartitionRegistry(testlogger.NewLogger())
	key := consumer.Key{Topic: "t", Partition: 1}

	ps1, created1, err := r.GetOrCreate(key, testSub("t"), context.Background(), 10)
	require.NoError(t, err, "first GetOrCreate should succeed")
	assert.True(t, created1, "first GetOrCreate should return created=true")

	ps2, created2, err := r.GetOrCreate(key, testSub("t"), context.Background(), 10)
	require.NoError(t, err, "second GetOrCreate should succeed")
	assert.False(t, created2, "second call should report created=false")
	assert.True(t, ps1 == ps2, "should return the same state pointer")
}

func TestPartitionRegistry_Get_AfterCreate(t *testing.T) {
	r := consumer.NewPartitionRegistry(testlogger.NewLogger())
	key := consumer.Key{Topic: "t", Partition: 1}

	_, _, err := r.GetOrCreate(key, testSub("t"), context.Background(), 10)
	require.NoError(t, err, "GetOrCreate should succeed")
	ps, ok := r.Get(key)
	assert.True(t, ok, "Get should find existing key")
	assert.NotNil(t, ps, "returned state should not be nil")
}

func TestPartitionRegistry_MarkDirty(t *testing.T) {
	r := consumer.NewPartitionRegistry(testlogger.NewLogger())
	key := consumer.Key{Topic: "t", Partition: 1}
	ps, _, err := r.GetOrCreate(key, testSub("t"), context.Background(), 10)
	require.NoError(t, err, "GetOrCreate should succeed")

	dirty := r.MarkDirty(ps)
	assert.True(t, dirty, "MarkDirty should return true")
}

func TestPartitionRegistry_MarkDirty_WrongState(t *testing.T) {
	r := consumer.NewPartitionRegistry(testlogger.NewLogger())
	_, _, errGC := r.GetOrCreate(consumer.Key{Topic: "t", Partition: 0}, testSub("t"), context.Background(), 10)
	require.NoError(t, errGC, "GetOrCreate should succeed")

	orphan := consumer.NewPartitionState(context.Background(), testlogger.NewLogger(), consumer.Key{Topic: "x", Partition: 9}, testSub("x"), 10)
	dirty := r.MarkDirty(orphan)
	assert.False(t, dirty, "MarkDirty for unknown state should return false")

	dirty = r.MarkDirty(nil)
	assert.False(t, dirty, "MarkDirty for nil should return false")
}

func TestPartitionRegistry_ClearDirty(t *testing.T) {
	r := consumer.NewPartitionRegistry(testlogger.NewLogger())
	key := consumer.Key{Topic: "t", Partition: 1}
	ps, _, errGC := r.GetOrCreate(key, testSub("t"), context.Background(), 10)
	require.NoError(t, errGC, "GetOrCreate should succeed")

	r.MarkDirty(ps)
	r.ClearDirty(key, ps)

	snap := r.SnapshotDirtyStates()
	assert.Equal(t, 0, len(snap), "no dirty states after clear")
}

func TestPartitionRegistry_SnapshotDirtyStates(t *testing.T) {
	r := consumer.NewPartitionRegistry(testlogger.NewLogger())
	ps1, _, err := r.GetOrCreate(consumer.Key{Topic: "a", Partition: 0}, testSub("a"), context.Background(), 10)
	require.NoError(t, err, "GetOrCreate should succeed")
	ps2, _, err := r.GetOrCreate(consumer.Key{Topic: "b", Partition: 0}, testSub("b"), context.Background(), 10)
	require.NoError(t, err, "GetOrCreate should succeed")

	r.MarkDirty(ps1)
	r.MarkDirty(ps2)

	snap := r.SnapshotDirtyStates()
	assert.Equal(t, 2, len(snap), "snapshot should have 2 entries")
}

func TestPartitionRegistry_BeginClosing_Selected(t *testing.T) {
	r := consumer.NewPartitionRegistry(testlogger.NewLogger())
	ps1, _, errGC := r.GetOrCreate(consumer.Key{Topic: "t", Partition: 0}, testSub("t"), context.Background(), 10)
	require.NoError(t, errGC, "GetOrCreate should succeed")
	ps2, _, errGC := r.GetOrCreate(consumer.Key{Topic: "t", Partition: 1}, testSub("t"), context.Background(), 10)
	require.NoError(t, errGC, "GetOrCreate should succeed")
	_, _, errGC = r.GetOrCreate(consumer.Key{Topic: "other", Partition: 0}, testSub("other"), context.Background(), 10)
	require.NoError(t, errGC, "GetOrCreate should succeed")

	states := r.BeginClosing(map[string][]int32{"t": {0, 1}})
	assert.Equal(t, 2, len(states), "should close 2 partitions")
	assert.False(t, ps1.IsRunning(), "ps1 should no longer be running")
	assert.False(t, ps2.IsRunning(), "ps2 should no longer be running")
}

func TestPartitionRegistry_BeginClosingAll(t *testing.T) {
	r := consumer.NewPartitionRegistry(testlogger.NewLogger())
	_, _, errGC := r.GetOrCreate(consumer.Key{Topic: "a", Partition: 0}, testSub("a"), context.Background(), 10)
	require.NoError(t, errGC, "GetOrCreate should succeed")
	_, _, errGC = r.GetOrCreate(consumer.Key{Topic: "b", Partition: 0}, testSub("b"), context.Background(), 10)
	require.NoError(t, errGC, "GetOrCreate should succeed")

	states := r.BeginClosingAll()
	assert.Equal(t, 2, len(states), "should close all partitions")
}

func TestPartitionRegistry_DropLost(t *testing.T) {
	r := consumer.NewPartitionRegistry(testlogger.NewLogger())
	key := consumer.Key{Topic: "t", Partition: 0}
	ps, _, errGC := r.GetOrCreate(key, testSub("t"), context.Background(), 10)
	require.NoError(t, errGC, "GetOrCreate should succeed")

	r.MarkDirty(ps)
	r.DropLost(map[string][]int32{"t": {0}})

	_, ok := r.Get(key)
	assert.False(t, ok, "lost partition should be removed")
}

func TestPartitionRegistry_SnapshotOffsets(t *testing.T) {
	r := consumer.NewPartitionRegistry(testlogger.NewLogger())
	ps, _, err := r.GetOrCreate(consumer.Key{Topic: "t", Partition: 0}, testSub("t"), context.Background(), 10)
	require.NoError(t, err, "GetOrCreate should succeed")

	ps.AdvanceCommitOffset(&kgo.Record{Topic: "t", Partition: 0, Offset: 5, LeaderEpoch: 1})

	offsets := r.SnapshotOffsets([]*consumer.PartitionState{ps})
	assert.Equal(t, 1, len(offsets), "should have 1 topic")
	assert.Equal(t, int64(6), offsets["t"][0].Offset, "offset should be record.Offset+1")
}

func TestPartitionRegistry_Cleanup_RemovesStopped(t *testing.T) {
	r := consumer.NewPartitionRegistry(testlogger.NewLogger())
	key := consumer.Key{Topic: "t", Partition: 0}
	ps, _, err := r.GetOrCreate(key, testSub("t"), context.Background(), 10)
	require.NoError(t, err, "GetOrCreate should succeed")

	ps.BeginClosing()
	ps.MarkStopped()

	r.Cleanup([]*consumer.PartitionState{ps})

	_, ok := r.Get(key)
	assert.False(t, ok, "partition should be removed after cleanup")
}

func TestPartitionRegistry_Cleanup_IgnoresStalePointer(t *testing.T) {
	r := consumer.NewPartitionRegistry(testlogger.NewLogger())
	key := consumer.Key{Topic: "t", Partition: 0}

	oldPs, _, errGC := r.GetOrCreate(key, testSub("t"), context.Background(), 10)
	require.NoError(t, errGC, "GetOrCreate should succeed")
	r.DropLost(map[string][]int32{"t": {0}})

	newPs, _, errGC := r.GetOrCreate(key, testSub("t"), context.Background(), 10)
	require.NoError(t, errGC, "GetOrCreate should succeed")

	r.Cleanup([]*consumer.PartitionState{oldPs})

	got, ok := r.Get(key)
	assert.True(t, ok, "new state should still be in registry")
	assert.True(t, newPs == got, "new state pointer should be intact")
}

func TestPartitionRegistry_Cleanup_NilEntry(t *testing.T) {
	r := consumer.NewPartitionRegistry(testlogger.NewLogger())
	r.Cleanup([]*consumer.PartitionState{nil})
}
