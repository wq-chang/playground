// services/go/library/kafka/internal/consumer/partition_state_test.go
package consumer_test

import (
	"context"
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"

	"go-services/library/assert"
	"go-services/library/kafka/internal/consumer"
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

func TestPartitionState_New(t *testing.T) {
	ps := consumer.NewPartitionState(
		context.Background(),
		consumer.Key{Topic: "t", Partition: 1},
		testSubscription(),
		64,
	)
	assert.NotNil(t, ps, "PartitionState should not be nil")
	assert.True(t, ps.IsRunning(), "new state should be running")
}

func TestPartitionState_Key(t *testing.T) {
	key := consumer.Key{Topic: "t", Partition: 2}
	ps := consumer.NewPartitionState(context.Background(), key, testSubscription(), 64)
	assert.Equal(t, key, ps.Key(), "Key() should return the constructor key")
}

func TestPartitionState_Done_NotClosedInitially(t *testing.T) {
	ps := consumer.NewPartitionState(context.Background(), consumer.Key{Topic: "t", Partition: 1}, testSubscription(), 64)
	select {
	case <-ps.Done():
		t.Fatal("Done channel should not be closed initially")
	default:
	}
}

func TestPartitionState_TryEnqueue_Success(t *testing.T) {
	ps := consumer.NewPartitionState(context.Background(), consumer.Key{Topic: "t", Partition: 1}, testSubscription(), 10)

	records := make([]*kgo.Record, 3)
	for i := range records {
		records[i] = &kgo.Record{Topic: "t", Partition: 1, Offset: int64(i)}
	}

	enqueued, buffered := ps.TryEnqueue(records)
	assert.Equal(t, 3, enqueued, "should enqueue all 3 records")
	assert.Equal(t, 3, buffered, "buffered count should be 3")
}

func TestPartitionState_TryEnqueue_ExceedsCapacity(t *testing.T) {
	ps := consumer.NewPartitionState(context.Background(), consumer.Key{Topic: "t", Partition: 1}, testSubscription(), 2)

	records := make([]*kgo.Record, 5)
	for i := range records {
		records[i] = &kgo.Record{Topic: "t", Partition: 1, Offset: int64(i)}
	}

	enqueued, buffered := ps.TryEnqueue(records)
	assert.Equal(t, 2, enqueued, "should enqueue only 2 (capacity)")
	assert.Equal(t, 2, buffered, "buffered count should be 2")
}

func TestPartitionState_OnDequeue_DecRemovesRecords(t *testing.T) {
	ps := consumer.NewPartitionState(context.Background(), consumer.Key{Topic: "t", Partition: 1}, testSubscription(), 10)

	records := make([]*kgo.Record, 5)
	for i := range records {
		records[i] = &kgo.Record{Topic: "t", Partition: 1, Offset: int64(i)}
	}
	ps.TryEnqueue(records)

	buffered := ps.OnDequeue(records)
	assert.Equal(t, 0, buffered, "buffered should be 0 after dequeue")
}

func TestPartitionState_MarkBackpressurePaused_Success(t *testing.T) {
	ps := consumer.NewPartitionState(context.Background(), consumer.Key{Topic: "t", Partition: 1}, testSubscription(), 10)

	paused := ps.MarkBackpressurePaused()
	assert.True(t, paused, "first pause should succeed")
}

func TestPartitionState_MarkBackpressurePaused_Idempotent(t *testing.T) {
	ps := consumer.NewPartitionState(context.Background(), consumer.Key{Topic: "t", Partition: 1}, testSubscription(), 10)

	ps.MarkBackpressurePaused()
	paused := ps.MarkBackpressurePaused()
	assert.False(t, paused, "second pause should return false")
}

func TestPartitionState_ClearBackpressurePaused(t *testing.T) {
	ps := consumer.NewPartitionState(context.Background(), consumer.Key{Topic: "t", Partition: 1}, testSubscription(), 10)

	ps.MarkBackpressurePaused()
	cleared := ps.ClearBackpressurePaused()
	assert.True(t, cleared, "clear should succeed after pause")

	assert.False(t, ps.ClearBackpressurePaused(), "second clear should return false")
}

func TestPartitionState_ClearBackpressurePaused_WithoutPause(t *testing.T) {
	ps := consumer.NewPartitionState(context.Background(), consumer.Key{Topic: "t", Partition: 1}, testSubscription(), 10)

	cleared := ps.ClearBackpressurePaused()
	assert.False(t, cleared, "clear without pause should return false")
}

func TestPartitionState_AdvanceCommitOffset(t *testing.T) {
	ps := consumer.NewPartitionState(context.Background(), consumer.Key{Topic: "t", Partition: 1}, testSubscription(), 10)

	record := &kgo.Record{Topic: "t", Partition: 1, Offset: 5, LeaderEpoch: 1}
	advanced := ps.AdvanceCommitOffset(record)
	assert.True(t, advanced, "should advance offset")
}

func TestPartitionState_SnapshotDirtyOffset_AfterAdvance(t *testing.T) {
	ps := consumer.NewPartitionState(context.Background(), consumer.Key{Topic: "t", Partition: 1}, testSubscription(), 10)

	record := &kgo.Record{Topic: "t", Partition: 1, Offset: 5, LeaderEpoch: 1}
	ps.AdvanceCommitOffset(record)

	offset, ok := ps.SnapshotDirtyOffset()
	assert.True(t, ok, "should have dirty offset")
	assert.Equal(t, int64(6), offset.Offset, "offset should be record.Offset+1")
	assert.Equal(t, int32(1), offset.Epoch, "epoch should match")
}

func TestPartitionState_MarkCommitted_ClearsDirty(t *testing.T) {
	ps := consumer.NewPartitionState(context.Background(), consumer.Key{Topic: "t", Partition: 1}, testSubscription(), 10)

	record := &kgo.Record{Topic: "t", Partition: 1, Offset: 5, LeaderEpoch: 1}
	ps.AdvanceCommitOffset(record)

	offset, _ := ps.SnapshotDirtyOffset()
	stillDirty := ps.MarkCommitted(offset)
	assert.False(t, stillDirty, "should not be dirty after commit")

	_, ok := ps.SnapshotDirtyOffset()
	assert.False(t, ok, "no dirty offset after commit")
}

func TestPartitionState_BeginClosing_StopsAccepting(t *testing.T) {
	ps := consumer.NewPartitionState(context.Background(), consumer.Key{Topic: "t", Partition: 1}, testSubscription(), 10)

	ps.BeginClosing()

	assert.False(t, ps.IsRunning(), "should not be running after closing")
	assert.False(t, ps.MarkBackpressurePaused(), "backpressure pause should fail when not accepting")
}

func TestPartitionState_Abort_ReturnsOffset(t *testing.T) {
	ps := consumer.NewPartitionState(context.Background(), consumer.Key{Topic: "t", Partition: 1}, testSubscription(), 10)

	record := &kgo.Record{Topic: "t", Partition: 1, Offset: 5, LeaderEpoch: 1}
	ps.AdvanceCommitOffset(record)

	offset, ok := ps.Abort()
	assert.True(t, ok, "should return offset on abort")
	assert.Equal(t, int64(6), offset.Offset, "abort offset should be record.Offset+1")

	assert.False(t, ps.IsRunning(), "should not be running after abort")
	enqueued, _ := ps.TryEnqueue([]*kgo.Record{{Topic: "t", Partition: 1}})
	assert.Equal(t, 0, enqueued, "should not enqueue after abort")
}

func TestPartitionState_MarkStopped(t *testing.T) {
	ps := consumer.NewPartitionState(context.Background(), consumer.Key{Topic: "t", Partition: 1}, testSubscription(), 10)

	ps.BeginClosing()
	ps.MarkStopped()

	assert.False(t, ps.IsRunning(), "should not be running after stop")
}

func TestPartitionState_Enqueue_AfterClosing_Fails(t *testing.T) {
	ps := consumer.NewPartitionState(context.Background(), consumer.Key{Topic: "t", Partition: 1}, testSubscription(), 10)

	ps.BeginClosing()

	records := []*kgo.Record{{Topic: "t", Partition: 1, Offset: 0}}
	enqueued, _ := ps.TryEnqueue(records)
	assert.Equal(t, 0, enqueued, "should not enqueue after closing")
}
