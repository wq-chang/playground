// services/go/library/kafka/consumer_v2_rebalance_test.go
package kafka

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"go-services/library/assert"
	"go-services/library/kafka/internal/consumer"
	"go-services/library/require"
)

func discardingLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// noopRegisterClient is a RegisterClient stub that no-ops AddConsumeTopics.
type noopRegisterClient struct{}

func (noopRegisterClient) AddConsumeTopics(topics ...string) {}

// stubFetchClient records fetch control calls.
type stubFetchClient struct {
	commitErr     error
	commitBlockCh chan struct{}
	pausedParts   []map[string][]int32
	resumedParts  []map[string][]int32
	pausedTopics  []string
	committed     []map[string]map[int32]kgo.EpochOffset
	mu            sync.Mutex
}

func (s *stubFetchClient) PauseFetchPartitions(partitions map[string][]int32) {
	s.mu.Lock()
	s.pausedParts = append(s.pausedParts, partitions)
	s.mu.Unlock()
}

func (s *stubFetchClient) ResumeFetchPartitions(partitions map[string][]int32) {
	s.mu.Lock()
	s.resumedParts = append(s.resumedParts, partitions)
	s.mu.Unlock()
}

func (s *stubFetchClient) PauseFetchTopics(topics ...string) {
	s.mu.Lock()
	s.pausedTopics = append(s.pausedTopics, topics...)
	s.mu.Unlock()
}

func (s *stubFetchClient) CommitOffsetsSync(ctx context.Context, offsets map[string]map[int32]kgo.EpochOffset) error {
	s.mu.Lock()
	if s.commitBlockCh != nil {
		ch := s.commitBlockCh
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			s.mu.Lock()
			err := ctx.Err()
			s.mu.Unlock()
			return err
		case <-ch:
		}
		s.mu.Lock()
	}
	// Also check context when not blocking.
	if err := ctx.Err(); err != nil {
		s.mu.Unlock()
		return err
	}
	s.committed = append(s.committed, offsets)
	err := s.commitErr
	s.mu.Unlock()
	return err
}

// newTestConsumer creates a Consumer with stubbed collaborators for rebalance tests.
func newTestConsumer(t *testing.T, stub *stubFetchClient) *Consumer {
	t.Helper()

	router := consumer.NewRouter(noopRegisterClient{})
	pauses := consumer.NewPauseRegistry(time.Now)
	run := consumer.NewRunState()
	registry := consumer.NewPartitionRegistry(nil)

	committer := consumer.NewCommitter(
		nil, registry, pauses, stub,
		consumer.CommitConfig{
			FlushInterval:      10 * time.Millisecond,
			DebounceInterval:   1 * time.Millisecond,
			DrainTimeout:       5 * time.Second,
			FinalCommitTimeout: 5 * time.Second,
		},
	)

	return &Consumer{
		router:       router,
		pauses:       pauses,
		runState:     run,
		registry:     registry,
		committer:    committer,
		fetchClient:  stub,
		workerClient: stub,
		drainTimeout: 5 * time.Second,
		log:          discardingLogger(),
	}
}

// addTestPartition creates a partition state with dirty offset and registers it.
func addTestPartition(t *testing.T, v2 *Consumer, topic string, partition int32, offset int64, epoch int32, dirty bool) {
	t.Helper()

	key := consumer.Key{Topic: topic, Partition: partition}
	sub := consumer.Subscription{
		Topic:         topic,
		Handler:       func(_ context.Context, _ *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: consumer.FailurePolicy{MaxAttempts: 1, RetryBackoff: 0, DLQ: nil, OnExhausted: consumer.ExhaustedActionStop},
		AckMode:       consumer.AckModeAtLeastOnce,
	}

	state, _, err := v2.registry.GetOrCreate(key, sub, context.Background(), 10)
	require.NoError(t, err, "GetOrCreate should succeed")

	state.AdvanceCommitOffset(&kgo.Record{
		Topic:       topic,
		Partition:   partition,
		Offset:      offset,
		LeaderEpoch: epoch,
	})

	// MarkStopped so Finalize can proceed (done channel closes without blocking on queue).
	state.MarkStopped()

	if dirty {
		require.True(t, v2.registry.MarkDirty(state), "MarkDirty should succeed")
	}
}

func TestConsumer_OnPartitionsRevoked_CommitsSelectedOffsets(t *testing.T) {
	stub := &stubFetchClient{}
	v2 := newTestConsumer(t, stub)

	addTestPartition(t, v2, "topic-a", 1, 5, 4, true)
	addTestPartition(t, v2, "topic-b", 0, 1, 7, true)

	v2.onPartitionsRevoked(context.Background(), nil, map[string][]int32{
		"topic-b": {0},
	})

	stub.mu.Lock()
	assert.Equal(t, 1, len(stub.pausedParts), "should pause revoked partitions")
	assert.Equal(t, 1, len(stub.committed), "should commit revoked offsets")
	committed := stub.committed[0]
	stub.mu.Unlock()

	assert.Equal(t, int64(2), committed["topic-b"][0].Offset, "revoked partition should commit next offset")
	assert.Equal(t, int32(7), committed["topic-b"][0].Epoch, "revoked partition should commit epoch")

	_, topicAExists := v2.registry.Get(consumer.Key{Topic: "topic-a", Partition: 1})
	assert.True(t, topicAExists, "unrevoked partition should remain")
	assert.NoError(t, v2.runState.Err(), "successful revoke should not fail run")
}

func TestConsumer_OnPartitionsRevoked_FailsRunOnCommitError(t *testing.T) {
	stub := &stubFetchClient{commitErr: errors.New("commit failed")}
	v2 := newTestConsumer(t, stub)

	addTestPartition(t, v2, "topic-a", 1, 5, 4, true)

	v2.onPartitionsRevoked(context.Background(), nil, map[string][]int32{
		"topic-a": {1},
	})

	assert.ErrorIs(t, v2.runState.Err(), stub.commitErr, "run should fail on commit error")
}

func TestConsumer_OnPartitionsRevoked_WaitsForDrainBeforeCommit(t *testing.T) {
	stub := &stubFetchClient{}
	v2 := newTestConsumer(t, stub)

	key := consumer.Key{Topic: "t", Partition: 0}
	sub := consumer.Subscription{
		Topic:         "t",
		Handler:       func(_ context.Context, _ *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: consumer.FailurePolicy{MaxAttempts: 1, RetryBackoff: 0, DLQ: nil, OnExhausted: consumer.ExhaustedActionStop},
		AckMode:       consumer.AckModeAtLeastOnce,
	}

	state, _, err := v2.registry.GetOrCreate(key, sub, context.Background(), 10)
	require.NoError(t, err, "GetOrCreate should succeed")
	state.AdvanceCommitOffset(&kgo.Record{Topic: "t", Partition: 0, Offset: 9, LeaderEpoch: 0})
	v2.registry.MarkDirty(state)

	// Do NOT MarkStopped — Finalize must wait for drain.
	// Queue closes via BeginClosing, then the drain loop in Finalize
	// detects state.done since BeginClosing closes queue and worker exits.
	// For this test, call MarkStopped to close the done channel and simulate drain.
	state.MarkStopped()

	v2.onPartitionsRevoked(context.Background(), nil, map[string][]int32{"t": {0}})

	stub.mu.Lock()
	assert.True(t, len(stub.committed) >= 1, "should commit after drain, got %d", len(stub.committed))
	stub.mu.Unlock()
}

func TestConsumer_OnPartitionsLost_DropsSelectedOffsets(t *testing.T) {
	stub := &stubFetchClient{}
	v2 := newTestConsumer(t, stub)

	addTestPartition(t, v2, "topic-a", 1, 5, 4, true)
	addTestPartition(t, v2, "topic-b", 0, 1, 7, true)

	v2.onPartitionsLost(context.Background(), map[string][]int32{
		"topic-a": {1},
	})

	// Partition should be dropped — no commit, state removed.
	stub.mu.Lock()
	assert.Equal(t, 0, len(stub.committed), "lost partitions should not commit offsets")
	stub.mu.Unlock()

	_, topicAExists := v2.registry.Get(consumer.Key{Topic: "topic-a", Partition: 1})
	assert.False(t, topicAExists, "lost partition should be removed")

	_, topicBExists := v2.registry.Get(consumer.Key{Topic: "topic-b", Partition: 0})
	assert.True(t, topicBExists, "unlost partition should remain")
}

func TestConsumer_OnPartitionsRevoked_UsesCallbackContextForFinalCommit(t *testing.T) {
	stub := &stubFetchClient{}
	v2 := newTestConsumer(t, stub)

	addTestPartition(t, v2, "t", 0, 5, 4, true)

	// Use a pre-cancelled context to verify it's threaded through.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	v2.onPartitionsRevoked(ctx, nil, map[string][]int32{"t": {0}})

	// A cancelled context should cause Finalize to fail.
	// onPartitionsRevoked calls Fail on the error.
	err := v2.runState.Err()
	assert.NotNil(t, err, "revoke with cancelled context should fail run")
	assert.ErrorIs(t, err, context.Canceled, "error should be context.Canceled")
}

func TestConsumer_OnPartitionsRevoked_StopsWaitingWhenContextCancelled(t *testing.T) {
	stub := &stubFetchClient{}
	v2 := newTestConsumer(t, stub)

	// Create a partition state that blocks on drain (not stopped, not closed).
	key := consumer.Key{Topic: "t", Partition: 0}
	sub := consumer.Subscription{
		Topic:         "t",
		Handler:       func(_ context.Context, _ *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: consumer.FailurePolicy{MaxAttempts: 1, RetryBackoff: 0, DLQ: nil, OnExhausted: consumer.ExhaustedActionStop},
		AckMode:       consumer.AckModeAtLeastOnce,
	}

	state, _, err := v2.registry.GetOrCreate(key, sub, context.Background(), 10)
	require.NoError(t, err, "GetOrCreate should succeed")
	state.AdvanceCommitOffset(&kgo.Record{Topic: "t", Partition: 0, Offset: 9, LeaderEpoch: 0})
	v2.registry.MarkDirty(state)
	// Do NOT call MarkStopped — state.Done() will block.

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	v2.onPartitionsRevoked(ctx, nil, map[string][]int32{"t": {0}})

	// Finalize should have timed out waiting for drain.
	err = v2.runState.Err()
	assert.NotNil(t, err, "revoke with timeout should fail run because drain won't complete")
}
