// services/go/library/kafka/internal/consumer/partition_registry_test.go
package consumer_test

import (
	"context"
	"sync"
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"

	"go-services/library/assert"
	"go-services/library/kafka/internal/consumer"
	"go-services/library/require"
	"go-services/library/testlogger"
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

func TestPartitionRegistry_Get_Success(t *testing.T) {
	r := consumer.NewPartitionRegistry(testlogger.NewLogger())
	key := consumer.Key{Topic: "t", Partition: 0}

	psCreated, _, err := r.GetOrCreate(key, testSub("t"), context.Background(), 10)
	require.NoError(t, err, "GetOrCreate should succeed")

	ps, ok := r.Get(key)
	assert.True(t, ok, "Get should find existing key")
	assert.True(t, psCreated == ps, "Get should return the same state pointer")
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
	assert.NotNil(t, ps, "returned state should not be nil")
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

func TestPartitionRegistry_ClearDirty(t *testing.T) {
	r := consumer.NewPartitionRegistry(testlogger.NewLogger())
	key := consumer.Key{Topic: "t", Partition: 1}
	ps, _, errGC := r.GetOrCreate(key, testSub("t"), context.Background(), 10)
	require.NoError(t, errGC, "GetOrCreate should succeed")
	r.AdvanceStateCommitOffset(ps, &kgo.Record{
		Topic:       ps.Key().Topic,
		Partition:   ps.Key().Partition,
		Offset:      0,
		LeaderEpoch: 0,
	})
	snap := r.SnapshotDirtyStates()
	require.Equal(t, 1, len(snap), "should have dirty states before clear")

	r.ClearDirty(key)

	snap = r.SnapshotDirtyStates()
	assert.Equal(t, 0, len(snap), "no dirty states after clear")
}

func TestPartitionRegistry_SnapshotDirtyStates(t *testing.T) {
	r := consumer.NewPartitionRegistry(testlogger.NewLogger())
	ps1, _, err := r.GetOrCreate(consumer.Key{Topic: "a", Partition: 0}, testSub("a"), context.Background(), 10)
	require.NoError(t, err, "GetOrCreate should succeed")
	ps2, _, err := r.GetOrCreate(consumer.Key{Topic: "b", Partition: 0}, testSub("b"), context.Background(), 10)
	require.NoError(t, err, "GetOrCreate should succeed")

	r.AdvanceStateCommitOffset(
		ps1,
		&kgo.Record{Topic: ps1.Key().Topic, Partition: ps1.Key().Partition, Offset: 0, LeaderEpoch: 0},
	)
	r.AdvanceStateCommitOffset(
		ps2,
		&kgo.Record{Topic: ps2.Key().Topic, Partition: ps2.Key().Partition, Offset: 0, LeaderEpoch: 0},
	)

	snap := r.SnapshotDirtyStates()
	assert.Equal(t, 2, len(snap), "snapshot should have 2 entries")
}

func TestPartitionRegistry_MarkStateCommitted_ClearsWhenClean(t *testing.T) {
	r := consumer.NewPartitionRegistry(testlogger.NewLogger())
	key := consumer.Key{Topic: "t", Partition: 0}
	ps, _, err := r.GetOrCreate(key, testSub("t"), context.Background(), 10)
	require.NoError(t, err, "GetOrCreate should succeed")

	// Advance offset and mark dirty.
	r.AdvanceStateCommitOffset(ps, &kgo.Record{Topic: "t", Partition: 0, Offset: 5, LeaderEpoch: 1})

	// Mark committed — no further progress, dirty should be cleared.
	r.MarkStateCommitted(ps, kgo.EpochOffset{Epoch: 1, Offset: 6})

	snap := r.SnapshotDirtyStates()
	assert.Equal(t, 0, len(snap), "dirty should be cleared after commit catches up")
}

func TestPartitionRegistry_MarkStateCommitted_KeepsWhenStillDirty(t *testing.T) {
	r := consumer.NewPartitionRegistry(testlogger.NewLogger())
	key := consumer.Key{Topic: "t", Partition: 0}
	ps, _, err := r.GetOrCreate(key, testSub("t"), context.Background(), 10)
	require.NoError(t, err, "GetOrCreate should succeed")

	// Advance offset past what we'll commit (simulating worker progress
	// between snapshot and commit).
	ps.AdvanceCommitOffset(&kgo.Record{Topic: "t", Partition: 0, Offset: 5, LeaderEpoch: 1})
	r.AdvanceStateCommitOffset(ps, &kgo.Record{Topic: "t", Partition: 0, Offset: 6, LeaderEpoch: 1})

	// Commit only up to offset 6 — state is still dirty at offset 7.
	r.MarkStateCommitted(ps, kgo.EpochOffset{Epoch: 1, Offset: 6})

	snap := r.SnapshotDirtyStates()
	assert.Equal(t, 1, len(snap), "dirty should remain when more progress exists")
}

func TestPartitionRegistry_MarkStateCommitted_AtomicWithAdvanceStateCommitOffset(t *testing.T) {
	r := consumer.NewPartitionRegistry(testlogger.NewLogger())
	key := consumer.Key{Topic: "t", Partition: 0}
	ps, _, err := r.GetOrCreate(key, testSub("t"), context.Background(), 10)
	require.NoError(t, err, "GetOrCreate should succeed")

	r.AdvanceStateCommitOffset(ps, &kgo.Record{Topic: "t", Partition: 0, Offset: 5, LeaderEpoch: 1})

	var markDirtyDone, commitDone sync.WaitGroup
	markDirtyDone.Add(1)
	commitDone.Add(1)

	// Worker goroutine: wait until commit starts, then sneak in a MarksDirty.
	go func() {
		markDirtyDone.Done() // signal readiness
		commitDone.Wait()    // wait for commit to enter the locked region
		// This AdvanceStateCommitOffset will block until MarkStateCommitted releases r.mu.
		// After it acquires the lock, the dirty map has already been inspected.
		r.AdvanceStateCommitOffset(ps, &kgo.Record{Topic: "t", Partition: 0, Offset: 6, LeaderEpoch: 1})
	}()

	// Commit goroutine: commit offset 6, then verify the worker's progress
	// wasn't lost even though it arrived mid-commit.
	go func() {
		markDirtyDone.Wait() // ensure worker is ready
		r.MarkStateCommitted(ps, kgo.EpochOffset{Epoch: 1, Offset: 6})
		commitDone.Done()
	}()

	commitDone.Wait()

	// After commit + worker update, there should still be dirty progress.
	snap := r.SnapshotDirtyStates()
	assert.Equal(t, 1, len(snap), "worker's dirty update should survive atomic commit")
}

func TestPartitionRegistry_AdvanceStateCommitOffset_AdvancesAndMarks(t *testing.T) {
	r := consumer.NewPartitionRegistry(testlogger.NewLogger())
	key := consumer.Key{Topic: "t", Partition: 0}
	ps, _, err := r.GetOrCreate(key, testSub("t"), context.Background(), 10)
	require.NoError(t, err, "GetOrCreate should succeed")

	advanced := r.AdvanceStateCommitOffset(ps, &kgo.Record{Topic: "t", Partition: 0, Offset: 5, LeaderEpoch: 1})
	assert.True(t, advanced, "first record should advance offset")

	snap := r.SnapshotDirtyStates()
	assert.Equal(t, 1, len(snap), "state should be dirty after advance")
}

func TestPartitionRegistry_AdvanceStateCommitOffset_StaleOffset(t *testing.T) {
	r := consumer.NewPartitionRegistry(testlogger.NewLogger())
	key := consumer.Key{Topic: "t", Partition: 0}
	ps, _, err := r.GetOrCreate(key, testSub("t"), context.Background(), 10)
	require.NoError(t, err, "GetOrCreate should succeed")

	r.AdvanceStateCommitOffset(ps, &kgo.Record{Topic: "t", Partition: 0, Offset: 10, LeaderEpoch: 1})

	// Older offset should not advance.
	advanced := r.AdvanceStateCommitOffset(ps, &kgo.Record{Topic: "t", Partition: 0, Offset: 5, LeaderEpoch: 1})
	assert.False(t, advanced, "stale offset should not advance")
}

func TestPartitionRegistry_AdvanceStateCommitOffset_NilState(t *testing.T) {
	r := consumer.NewPartitionRegistry(testlogger.NewLogger())
	advanced := r.AdvanceStateCommitOffset(nil, &kgo.Record{Topic: "t", Partition: 0, Offset: 0, LeaderEpoch: 0})
	assert.False(t, advanced, "nil state should not advance")
}

func TestPartitionRegistry_AdvanceStateCommitOffset_Unregistered(t *testing.T) {
	r := consumer.NewPartitionRegistry(testlogger.NewLogger())
	orphan := consumer.NewPartitionState(
		context.Background(), testlogger.NewLogger(),
		consumer.Key{Topic: "orphan", Partition: 0},
		testSub("orphan"), 10,
	)
	advanced := r.AdvanceStateCommitOffset(
		orphan,
		&kgo.Record{Topic: "orphan", Partition: 0, Offset: 0, LeaderEpoch: 0},
	)
	assert.False(t, advanced, "unregistered state should not advance")
}

func TestPartitionRegistry_BeginClosing_Selected(t *testing.T) {
	r := consumer.NewPartitionRegistry(testlogger.NewLogger())
	ps1, _, errGC := r.GetOrCreate(consumer.Key{Topic: "t", Partition: 0}, testSub("t"), context.Background(), 10)
	require.NoError(t, errGC, "GetOrCreate should succeed")
	ps2, _, errGC := r.GetOrCreate(consumer.Key{Topic: "t", Partition: 1}, testSub("t"), context.Background(), 10)
	require.NoError(t, errGC, "GetOrCreate should succeed")
	ps3, _, errGC := r.GetOrCreate(consumer.Key{Topic: "other", Partition: 0}, testSub("other"), context.Background(), 10)
	require.NoError(t, errGC, "GetOrCreate should succeed")

	states := r.BeginClosing(map[string][]int32{"t": {0, 1}})
	assert.Equal(t, 2, len(states), "should close 2 partitions")
	enqueued1, _ := ps1.TryEnqueue([]*kgo.Record{{Topic: "t", Partition: 0}})
	assert.Equal(t, 0, enqueued1, "ps1 should not accept records after closing")
	enqueued2, _ := ps2.TryEnqueue([]*kgo.Record{{Topic: "t", Partition: 1}})
	assert.Equal(t, 0, enqueued2, "ps2 should not accept records after closing")
	enqueued3, _ := ps3.TryEnqueue([]*kgo.Record{{Topic: "other", Partition: 0}})
	assert.Equal(t, 1, enqueued3, "ps3 should still accept records — not selected for closing")
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
	lostKey := consumer.Key{Topic: "t", Partition: 0}
	survivorKey := consumer.Key{Topic: "t", Partition: 1}

	ps, _, errGC := r.GetOrCreate(lostKey, testSub("t"), context.Background(), 10)
	require.NoError(t, errGC, "GetOrCreate should succeed")
	survivor, _, errGC := r.GetOrCreate(survivorKey, testSub("t"), context.Background(), 10)
	require.NoError(t, errGC, "GetOrCreate should succeed")

	r.AdvanceStateCommitOffset(ps, &kgo.Record{Topic: ps.Key().Topic, Partition: ps.Key().Partition, Offset: 0, LeaderEpoch: 0})
	r.DropLost(map[string][]int32{"t": {0}})

	_, ok := r.Get(lostKey)
	assert.False(t, ok, "lost partition should be removed")

	got, ok := r.Get(survivorKey)
	assert.True(t, ok, "unaffected partition should still be in registry")
	assert.True(t, got == survivor, "unaffected partition pointer should be intact")
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

func TestPartitionRegistry_SnapshotOffsets_NilAndEmpty(t *testing.T) {
	r := consumer.NewPartitionRegistry(testlogger.NewLogger())

	// Nil state is skipped.
	offsets := r.SnapshotOffsets([]*consumer.PartitionState{nil})
	assert.Nil(t, offsets, "nil state should produce nil offsets")

	// State with no dirty offset is skipped.
	ps, _, err := r.GetOrCreate(consumer.Key{Topic: "t", Partition: 0}, testSub("t"), context.Background(), 10)
	require.NoError(t, err, "GetOrCreate should succeed")
	offsets = r.SnapshotOffsets([]*consumer.PartitionState{ps})
	assert.Nil(t, offsets, "clean state should produce nil offsets")
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
