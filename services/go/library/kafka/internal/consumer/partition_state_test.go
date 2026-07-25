// services/go/library/kafka/internal/consumer/partition_state_test.go
package consumer_test

import (
	"context"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"go-services/library/assert"
	"go-services/library/kafka/internal/consumer"
	"go-services/library/testlogger"
)

// testSubscription returns a minimal subscription for tests.
func testSubscription() consumer.Subscription {
	return consumer.Subscription{
		Topic:         "test-topic",
		Handler:       func(context.Context, *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: consumer.FailurePolicy{},
		AckMode:       consumer.AckModeAtLeastOnce,
	}
}

// dequeueCtx returns a context with a generous timeout so tests never hang
// if a Dequeue would block unexpectedly.
func dequeueCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestPartitionState_New(t *testing.T) {
	ps := consumer.NewPartitionState(
		context.Background(),
		testlogger.NewLogger(),
		consumer.Key{Topic: "t", Partition: 1},
		testSubscription(),
		64,
	)
	assert.NotNil(t, ps, "PartitionState should not be nil")
	enqueued, _ := ps.TryEnqueue([]*kgo.Record{{Topic: "t", Partition: 1}})
	assert.Equal(t, enqueued, 1, "new state should accept records")
}

func TestPartitionState_Key(t *testing.T) {
	key := consumer.Key{Topic: "t", Partition: 2}
	ps := consumer.NewPartitionState(context.Background(), testlogger.NewLogger(), key, testSubscription(), 64)
	assert.Equal(t, key, ps.Key(), "Key() should return the constructor key")
}

func TestPartitionState_Done_NotClosedInitially(t *testing.T) {
	ps := consumer.NewPartitionState(
		context.Background(),
		testlogger.NewLogger(),
		consumer.Key{Topic: "t", Partition: 1},
		testSubscription(),
		64,
	)
	select {
	case <-ps.Done():
		t.Fatal("Done channel should not be closed initially")
	default:
	}
}

func TestPartitionState_TryEnqueue_Success(t *testing.T) {
	ps := consumer.NewPartitionState(
		context.Background(),
		testlogger.NewLogger(),
		consumer.Key{Topic: "t", Partition: 1},
		testSubscription(),
		10,
	)

	records := make([]*kgo.Record, 3)
	for i := range records {
		records[i] = &kgo.Record{Topic: "t", Partition: 1, Offset: int64(i)}
	}

	enqueued, buffered := ps.TryEnqueue(records)
	assert.Equal(t, enqueued, 3, "should enqueue all 3 records")
	assert.Equal(t, buffered, 3, "buffered count should be 3")

	// Drain the queue — must contain exactly the enqueued records.
	queued, ok := ps.Dequeue(dequeueCtx(t))
	assert.True(t, ok, "should dequeue successfully")
	assert.Equal(t, len(queued), 3, "queued slice should have 3 records")
	for i, r := range queued {
		assert.Equal(t, int64(i), r.Offset, "queued records should be in original order")
	}
}

func TestPartitionState_TryEnqueue_ExceedsCapacity(t *testing.T) {
	ps := consumer.NewPartitionState(
		context.Background(),
		testlogger.NewLogger(),
		consumer.Key{Topic: "t", Partition: 1},
		testSubscription(),
		2,
	)

	records := make([]*kgo.Record, 5)
	for i := range records {
		records[i] = &kgo.Record{Topic: "t", Partition: 1, Offset: int64(i)}
	}

	enqueued, buffered := ps.TryEnqueue(records)
	assert.Equal(t, enqueued, 2, "should enqueue only 2 (capacity)")
	assert.Equal(t, buffered, 2, "buffered count should be 2")

	// Drain — only the truncated prefix was enqueued.
	queued, ok := ps.Dequeue(dequeueCtx(t))
	assert.True(t, ok, "should dequeue successfully")
	assert.Equal(t, len(queued), 2, "queued slice should have 2 records")
	assert.Equal(t, queued[0].Offset, 0, "first queued offset")
	assert.Equal(t, queued[1].Offset, 1, "second queued offset")
}

func TestPartitionState_TryEnqueue_EmptyRecords(t *testing.T) {
	ps := consumer.NewPartitionState(
		context.Background(),
		testlogger.NewLogger(),
		consumer.Key{Topic: "t", Partition: 1},
		testSubscription(),
		10,
	)

	enqueued, buffered := ps.TryEnqueue(nil)
	assert.Equal(t, enqueued, 0, "nil slice should enqueue 0")
	assert.Equal(t, buffered, 0, "buffered should be 0")

	enqueued, buffered = ps.TryEnqueue([]*kgo.Record{})
	assert.Equal(t, enqueued, 0, "empty slice should enqueue 0")
	assert.Equal(t, buffered, 0, "buffered should be 0")

	// Queue should still be empty — nothing was enqueued.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, ok := ps.Dequeue(ctx)
	assert.False(t, ok, "should not dequeue from empty queue")
}

func TestPartitionState_TryEnqueue_QueueFull(t *testing.T) {
	ps := consumer.NewPartitionState(
		context.Background(),
		testlogger.NewLogger(),
		consumer.Key{Topic: "t", Partition: 1},
		testSubscription(),
		2,
	)

	// Fill the queue completely (2 single-record batches).
	n, b := ps.TryEnqueue([]*kgo.Record{{Topic: "t", Offset: 10}})
	assert.Equal(t, n, 1, "first enqueue")
	assert.Equal(t, b, 1, "buffered should be 1")
	n, b = ps.TryEnqueue([]*kgo.Record{{Topic: "t", Offset: 20}})
	assert.Equal(t, n, 1, "second enqueue")
	assert.Equal(t, b, 2, "buffered should be 2")

	// Now the queue is full — both channel slots and bufferedRecords at capacity.
	rejectedEnqueue, buffered := ps.TryEnqueue([]*kgo.Record{{Topic: "t", Offset: 99}})
	assert.Equal(t, rejectedEnqueue, 0, "should reject when queue is full")
	assert.Equal(t, buffered, 2, "buffered should still be 2")

	// Drain — only the first two batches made it in, in order.
	batch1, ok := ps.Dequeue(dequeueCtx(t))
	assert.True(t, ok, "first dequeue")
	assert.Equal(t, len(batch1), 1, "first batch should have 1 record")
	assert.Equal(t, batch1[0].Offset, 10, "first batch offset")

	batch2, ok := ps.Dequeue(dequeueCtx(t))
	assert.True(t, ok, "second dequeue")
	assert.Equal(t, len(batch2), 1, "second batch should have 1 record")
	assert.Equal(t, batch2[0].Offset, 20, "second batch offset")

	// Queue should be empty now — offset 99 was rejected and not enqueued.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, ok = ps.Dequeue(ctx)
	assert.False(t, ok, "should not dequeue from empty queue")
}

func TestPartitionState_Enqueue_AfterClosing_Fails(t *testing.T) {
	ps := consumer.NewPartitionState(
		context.Background(),
		testlogger.NewLogger(),
		consumer.Key{Topic: "t", Partition: 1},
		testSubscription(),
		10,
	)

	ps.BeginClosing()

	records := []*kgo.Record{{Topic: "t", Partition: 1, Offset: 0}}
	enqueued, _ := ps.TryEnqueue(records)
	assert.Equal(t, enqueued, 0, "should not enqueue after closing")
}

func TestPartitionState_Dequeue_ReturnsRecordsInOrder(t *testing.T) {
	ps := consumer.NewPartitionState(
		context.Background(),
		testlogger.NewLogger(),
		consumer.Key{Topic: "t", Partition: 1},
		testSubscription(),
		10,
	)

	records := make([]*kgo.Record, 5)
	for i := range records {
		records[i] = &kgo.Record{Topic: "t", Partition: 1, Offset: int64(i)}
	}
	ps.TryEnqueue(records)

	queued, ok := ps.Dequeue(dequeueCtx(t))
	assert.True(t, ok, "should dequeue successfully")
	assert.Equal(t, len(queued), 5, "should dequeue all 5 records")
	for i, r := range queued {
		assert.Equal(t, int64(i), r.Offset, "queued records should be in original order")
	}
}

func TestPartitionState_Dequeue_AfterBeginClosing(t *testing.T) {
	ps := consumer.NewPartitionState(
		context.Background(),
		testlogger.NewLogger(),
		consumer.Key{Topic: "t", Partition: 1},
		testSubscription(),
		10,
	)

	ps.TryEnqueue([]*kgo.Record{{Topic: "t", Offset: 0}})
	ps.BeginClosing()

	// After BeginClosing, both the queue is closed and the partition context
	// is cancelled — Dequeue must not hang regardless of which select case
	// fires first.
	done := make(chan struct{})
	go func() {
		ps.Dequeue(ps.Context())
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Dequeue should not hang after BeginClosing")
	}
}

func TestPartitionState_Dequeue_CtxWinsWhenBothReady(t *testing.T) {
	ps := consumer.NewPartitionState(
		context.Background(),
		testlogger.NewLogger(),
		consumer.Key{Topic: "t", Partition: 1},
		testSubscription(),
		10,
	)

	// Enqueue multiple batches.
	for i := range 3 {
		ps.TryEnqueue([]*kgo.Record{{Topic: "t", Offset: int64(i)}})
	}

	// Simulate BeginClosing: close the queue AND cancel the context.
	// Both signals are ready — Dequeue must deterministically return
	// via ctx.Done(), leaving records in the queue to be picked up by
	// the next consumer.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ps.BeginClosing()

	records, ok := ps.Dequeue(ctx)
	assert.False(t, ok, "ctx cancellation should win over buffered queue data")
	assert.Nil(t, records, "no records should be drained when ctx is cancelled")
}

func TestPartitionState_Dequeue_DoesNotHangWhenCtxCancelled(t *testing.T) {
	ps := consumer.NewPartitionState(
		context.Background(),
		testlogger.NewLogger(),
		consumer.Key{Topic: "t", Partition: 1},
		testSubscription(),
		10,
	)

	ps.TryEnqueue([]*kgo.Record{{Topic: "t", Offset: 42}})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Both queue and ctx are ready — Dequeue must not hang.
	done := make(chan struct{})
	go func() {
		ps.Dequeue(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Dequeue should not hang when ctx is cancelled")
	}
}

func TestPartitionState_TryPauseBackpressure_QueueFull(t *testing.T) {
	ps := consumer.NewPartitionState(
		context.Background(),
		testlogger.NewLogger(),
		consumer.Key{Topic: "t", Partition: 1},
		testSubscription(),
		10,
	)

	// Enqueue 9 batches (high watermark for capacity 10 is 9).
	for i := range 9 {
		n, _ := ps.TryEnqueue([]*kgo.Record{{Topic: "t", Offset: int64(i)}})
		assert.Equal(t, n, 1, "should enqueue record %d", i)
	}

	paused := ps.TryPauseBackpressure()
	assert.True(t, paused, "should pause when queue is at high watermark (9 of 10)")
}

func TestPartitionState_TryPauseBackpressure_NotFullEnough(t *testing.T) {
	ps := consumer.NewPartitionState(
		context.Background(),
		testlogger.NewLogger(),
		consumer.Key{Topic: "t", Partition: 1},
		testSubscription(),
		10,
	)

	// Enqueue only 8 batches (below high watermark of 9).
	for i := range 8 {
		n, _ := ps.TryEnqueue([]*kgo.Record{{Topic: "t", Offset: int64(i)}})
		assert.Equal(t, n, 1, "should enqueue record %d", i)
	}

	paused := ps.TryPauseBackpressure()
	assert.False(t, paused, "should not pause below high watermark (8 of 10)")
}

func TestPartitionState_TryPauseBackpressure_Idempotent(t *testing.T) {
	ps := consumer.NewPartitionState(
		context.Background(),
		testlogger.NewLogger(),
		consumer.Key{Topic: "t", Partition: 1},
		testSubscription(),
		10,
	)

	// Fill to high watermark.
	for i := range 9 {
		ps.TryEnqueue([]*kgo.Record{{Topic: "t", Offset: int64(i)}})
	}

	ps.TryPauseBackpressure()
	paused := ps.TryPauseBackpressure()
	assert.False(t, paused, "second pause should return false")
}

func TestPartitionState_TryPauseBackpressure_NotAccepting(t *testing.T) {
	ps := consumer.NewPartitionState(context.Background(), testlogger.NewLogger(), consumer.Key{Topic: "t", Partition: 1}, testSubscription(), 10)
	ps.BeginClosing()

	// Fill to high watermark.
	for i := range 9 {
		ps.TryEnqueue([]*kgo.Record{{Topic: "t", Offset: int64(i)}})
	}

	paused := ps.TryPauseBackpressure()
	assert.False(t, paused, "should not pause when not accepting")
}

func TestPartitionState_TryResumeBackpressure_DrainsLow(t *testing.T) {
	ps := consumer.NewPartitionState(context.Background(), testlogger.NewLogger(), consumer.Key{Topic: "t", Partition: 1}, testSubscription(), 10)

	// Fill to high watermark and pause.
	for i := range 9 {
		ps.TryEnqueue([]*kgo.Record{{Topic: "t", Offset: int64(i)}})
	}
	ps.TryPauseBackpressure()

	// Dequeue 5 batches: remaining = 4, which is <= capacity/2 = 5.
	for range 5 {
		ps.Dequeue(dequeueCtx(t))
	}

	resumed := ps.TryResumeBackpressure()
	assert.True(t, resumed, "should resume when queue drains to low watermark (4 of 10)")
}

func TestPartitionState_TryResumeBackpressure_NotLowEnough(t *testing.T) {
	ps := consumer.NewPartitionState(context.Background(), testlogger.NewLogger(), consumer.Key{Topic: "t", Partition: 1}, testSubscription(), 10)

	// Fill to high watermark and pause.
	for i := range 9 {
		ps.TryEnqueue([]*kgo.Record{{Topic: "t", Offset: int64(i)}})
	}
	ps.TryPauseBackpressure()

	// Dequeue only 2 batches: remaining = 7, which is above capacity/2 = 5.
	for range 2 {
		ps.Dequeue(dequeueCtx(t))
	}

	resumed := ps.TryResumeBackpressure()
	assert.False(t, resumed, "should not resume when above low watermark (7 of 10)")
}

func TestPartitionState_TryResumeBackpressure_NotPaused(t *testing.T) {
	ps := consumer.NewPartitionState(
		context.Background(),
		testlogger.NewLogger(),
		consumer.Key{Topic: "t", Partition: 1},
		testSubscription(),
		10,
	)

	resumed := ps.TryResumeBackpressure()
	assert.False(t, resumed, "resume without pause should return false")
}

func TestPartitionState_TryResumeBackpressure_NotAccepting(t *testing.T) {
	ps := consumer.NewPartitionState(
		context.Background(),
		testlogger.NewLogger(),
		consumer.Key{Topic: "t", Partition: 1},
		testSubscription(),
		10,
	)

	// Fill to high watermark and pause.
	for i := range 9 {
		ps.TryEnqueue([]*kgo.Record{{Topic: "t", Offset: int64(i)}})
	}
	ps.TryPauseBackpressure()

	ps.BeginClosing()

	resumed := ps.TryResumeBackpressure()
	assert.False(t, resumed, "should not resume when not accepting")
}

func TestPartitionState_AdvanceCommitOffset(t *testing.T) {
	ps := consumer.NewPartitionState(
		context.Background(),
		testlogger.NewLogger(),
		consumer.Key{Topic: "t", Partition: 1},
		testSubscription(),
		10,
	)

	record := &kgo.Record{Topic: "t", Partition: 1, Offset: 5, LeaderEpoch: 1}
	advanced := ps.AdvanceCommitOffset(record)
	assert.True(t, advanced, "should advance offset")

	offset, ok := ps.SnapshotDirtyOffset()
	assert.True(t, ok, "should be dirty after advance")
	assert.Equal(t, offset.Offset, 6, "offset should be record.Offset+1")
}

func TestPartitionState_AdvanceCommitOffset_RejectsStaleOffset(t *testing.T) {
	ps := consumer.NewPartitionState(
		context.Background(),
		testlogger.NewLogger(),
		consumer.Key{Topic: "t", Partition: 1},
		testSubscription(),
		10,
	)

	r1 := &kgo.Record{Topic: "t", Partition: 1, Offset: 5, LeaderEpoch: 1}
	ps.AdvanceCommitOffset(r1)

	r2 := &kgo.Record{Topic: "t", Partition: 1, Offset: 3, LeaderEpoch: 1}
	assert.False(t, ps.AdvanceCommitOffset(r2), "should reject stale offset")

	// State unchanged — still tracking offset 5.
	offset, ok := ps.SnapshotDirtyOffset()
	assert.True(t, ok, "should still be dirty")
	assert.Equal(t, offset.Offset, 6, "should still be original offset+1")
}

func TestPartitionState_AdvanceCommitOffset_TracksHighest(t *testing.T) {
	ps := consumer.NewPartitionState(
		context.Background(),
		testlogger.NewLogger(),
		consumer.Key{Topic: "t", Partition: 1},
		testSubscription(),
		10,
	)

	ps.AdvanceCommitOffset(&kgo.Record{Topic: "t", Partition: 1, Offset: 5, LeaderEpoch: 1})
	ps.AdvanceCommitOffset(&kgo.Record{Topic: "t", Partition: 1, Offset: 3, LeaderEpoch: 1}) // rejected
	ps.AdvanceCommitOffset(&kgo.Record{Topic: "t", Partition: 1, Offset: 9, LeaderEpoch: 1})

	offset, ok := ps.SnapshotDirtyOffset()
	assert.True(t, ok, "should have dirty offset")
	assert.Equal(t, offset.Offset, 10, "should track highest offset (9+1)")
}

func TestPartitionState_SnapshotDirtyOffset_AfterAdvance(t *testing.T) {
	ps := consumer.NewPartitionState(
		context.Background(),
		testlogger.NewLogger(),
		consumer.Key{Topic: "t", Partition: 1},
		testSubscription(),
		10,
	)

	_, ok := ps.SnapshotDirtyOffset()
	assert.False(t, ok, "should not be dirty before any advance")

	record := &kgo.Record{Topic: "t", Partition: 1, Offset: 5, LeaderEpoch: 1}
	ps.AdvanceCommitOffset(record)

	offset, ok := ps.SnapshotDirtyOffset()
	assert.True(t, ok, "should have dirty offset")
	assert.Equal(t, offset.Offset, 6, "offset should be record.Offset+1")
	assert.Equal(t, offset.Epoch, 1, "epoch should match")
}

func TestPartitionState_MarkCommitted_ClearsDirty(t *testing.T) {
	ps := consumer.NewPartitionState(
		context.Background(),
		testlogger.NewLogger(),
		consumer.Key{Topic: "t", Partition: 1},
		testSubscription(),
		10,
	)

	record := &kgo.Record{Topic: "t", Partition: 1, Offset: 5, LeaderEpoch: 1}
	ps.AdvanceCommitOffset(record)

	offset, _ := ps.SnapshotDirtyOffset()
	stillDirty := ps.MarkCommitted(offset)
	assert.False(t, stillDirty, "should not be dirty after commit catches up")

	_, ok := ps.SnapshotDirtyOffset()
	assert.False(t, ok, "no dirty offset after commit")
}

func TestPartitionState_MarkCommitted_StaysDirtyWhenBehind(t *testing.T) {
	ps := consumer.NewPartitionState(
		context.Background(),
		testlogger.NewLogger(),
		consumer.Key{Topic: "t", Partition: 1},
		testSubscription(),
		10,
	)

	// Advance to offset 10.
	ps.AdvanceCommitOffset(&kgo.Record{Topic: "t", Partition: 1, Offset: 9, LeaderEpoch: 1})

	// Commit only up to offset 7 — behind nextCommitOffset (10).
	stillDirty := ps.MarkCommitted(kgo.EpochOffset{Epoch: 1, Offset: 7})
	assert.True(t, stillDirty, "should still be dirty when commit is behind")

	offset, ok := ps.SnapshotDirtyOffset()
	assert.True(t, ok, "should still have dirty offset")
	assert.Equal(t, offset.Offset, 10, "next commit offset should still be 10")
}

func TestPartitionState_MarkCommitted_IgnoresStaleCommit(t *testing.T) {
	ps := consumer.NewPartitionState(
		context.Background(),
		testlogger.NewLogger(),
		consumer.Key{Topic: "t", Partition: 1},
		testSubscription(),
		10,
	)

	ps.AdvanceCommitOffset(&kgo.Record{Topic: "t", Partition: 1, Offset: 9, LeaderEpoch: 1})

	// Commit offset 10 (catches up).
	ps.MarkCommitted(kgo.EpochOffset{Epoch: 1, Offset: 10})

	// Then a stale commit arrives for offset 5 — must not regress committedOffset.
	stillDirty := ps.MarkCommitted(kgo.EpochOffset{Epoch: 1, Offset: 5})
	assert.False(t, stillDirty, "should stay clean when stale commit arrives")

	_, ok := ps.SnapshotDirtyOffset()
	assert.False(t, ok, "should still be clean after stale commit")
}

func TestPartitionState_BeginClosing_StopsAccepting(t *testing.T) {
	ps := consumer.NewPartitionState(
		context.Background(),
		testlogger.NewLogger(),
		consumer.Key{Topic: "t", Partition: 1},
		testSubscription(),
		10,
	)

	// Fill to high watermark first.
	for i := range 9 {
		ps.TryEnqueue([]*kgo.Record{{Topic: "t", Offset: int64(i)}})
	}

	ps.BeginClosing()

	enqueued, _ := ps.TryEnqueue([]*kgo.Record{{Topic: "t", Partition: 1}})
	assert.Equal(t, enqueued, 0, "should not accept records after closing")
	assert.False(t, ps.TryPauseBackpressure(), "backpressure pause should fail when not accepting")
}

func TestPartitionState_BeginClosing_WhenAlreadyClosing(t *testing.T) {
	ps := consumer.NewPartitionState(
		context.Background(),
		testlogger.NewLogger(),
		consumer.Key{Topic: "t", Partition: 1},
		testSubscription(),
		10,
	)

	ps.BeginClosing()
	// Second call while still in closing state (before MarkStopped).
	ps.BeginClosing()
}

func TestPartitionState_BeginClosing_CancelsContext(t *testing.T) {
	ps := consumer.NewPartitionState(
		context.Background(),
		testlogger.NewLogger(),
		consumer.Key{Topic: "t", Partition: 1},
		testSubscription(),
		10,
	)

	// Context should not be done before closing.
	ctx := ps.Context()
	select {
	case <-ctx.Done():
		t.Fatal("context should not be done before BeginClosing")
	default:
	}

	ps.BeginClosing()

	// Context should be cancelled immediately after BeginClosing.
	select {
	case <-ctx.Done():
	default:
		t.Fatal("context should be cancelled after BeginClosing")
	}
}

func TestPartitionState_MarkStopped(t *testing.T) {
	ps := consumer.NewPartitionState(context.Background(), testlogger.NewLogger(), consumer.Key{Topic: "t", Partition: 1}, testSubscription(), 10)

	ps.BeginClosing()
	ps.MarkStopped()

	enqueued, _ := ps.TryEnqueue([]*kgo.Record{{Topic: "t", Partition: 1}})
	assert.Equal(t, enqueued, 0, "should not accept records after stop")

	select {
	case <-ps.Done():
	default:
		t.Fatal("Done channel should be closed after MarkStopped")
	}
}

func TestPartitionState_BeginClosing_AfterStopped(t *testing.T) {
	ps := consumer.NewPartitionState(
		context.Background(),
		testlogger.NewLogger(),
		consumer.Key{Topic: "t", Partition: 1},
		testSubscription(),
		10,
	)

	ps.BeginClosing()
	ps.MarkStopped()

	// Double-close must not panic or deadlock.
	ps.BeginClosing()
}

func TestPartitionState_MarkStopped_WhenAlreadyStopped(t *testing.T) {
	ps := consumer.NewPartitionState(
		context.Background(),
		testlogger.NewLogger(),
		consumer.Key{Topic: "t", Partition: 1},
		testSubscription(),
		10,
	)

	ps.BeginClosing()
	ps.MarkStopped()
	// Second call must not panic.
	ps.MarkStopped()

	select {
	case <-ps.Done():
	default:
		t.Fatal("Done channel should be closed after MarkStopped")
	}
}
