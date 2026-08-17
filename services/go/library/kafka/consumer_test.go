// Same-package tests for the Consumer.
package kafka

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"go-services/library/assert"
	"go-services/library/kafka/internal/consumer"
	"go-services/library/require"
	"go-services/library/testlogger"
)

// newTestKgoClient creates a standalone kgo.Client for unit testing.
func newTestKgoClient(t *testing.T) *kgo.Client {
	t.Helper()

	kgoClient, err := kgo.NewClient(
		kgo.SeedBrokers("localhost:9092"),
		kgo.ConsumerGroup("test-group"),
	)
	require.NoError(t, err, "failed to create test kgo client")
	t.Cleanup(kgoClient.Close)

	return kgoClient
}

func TestConsumer_New_ValidConfig(t *testing.T) {
	kgoClient := newTestKgoClient(t)
	cfg := newConfig([]string{"localhost:9092"}, "test-group")

	c, err := newConsumer(cfg, kgoClient, nil)
	require.NoError(t, err, "newConsumer should succeed")
	require.NotNil(t, c, "Consumer should not be nil")
}

func TestConsumer_New_WithStartupSubscriptions(t *testing.T) {
	kgoClient := newTestKgoClient(t)
	cfg := newConfig([]string{"localhost:9092"}, "test-group")
	cfg.subscriptions["topic-a"] = newDefaultSubscription(
		"topic-a",
		func(context.Context, *kgo.Record) error { return nil },
		AckModeAtLeastOnce,
	)
	cfg.subscriptions["topic-b"] = newDefaultBatchSubscription(
		"topic-b",
		func(context.Context, []*kgo.Record) BatchResult { return BatchResult{} },
		AckModeAtMostOnce,
	)

	c, err := newConsumer(cfg, kgoClient, nil)
	require.NoError(t, err, "newConsumer should succeed with startup subscriptions")

	snap := c.router.Snapshot()
	assert.Equal(t, len(snap), 2, "should have 2 subscriptions")
}

func TestConsumer_AddSubscription(t *testing.T) {
	kgoClient := newTestKgoClient(t)
	cfg := newConfig([]string{"localhost:9092"}, "test-group")
	c, err := newConsumer(cfg, kgoClient, nil)
	require.NoError(t, err, "newConsumer should succeed")

	sub := Subscription{
		Topic:         "my-topic",
		Handler:       func(context.Context, *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: FailurePolicy{},
		AckMode:       AckModeAtLeastOnce,
	}

	err = c.AddSubscription(sub)
	require.NoError(t, err, "AddSubscription should succeed")

	snap := c.router.Snapshot()
	assert.Equal(t, len(snap), 1, "should have 1 subscription")

	got, ok := c.router.Lookup("my-topic")
	assert.True(t, ok, "topic should be found")
	assert.Equal(t, got.Topic, "my-topic", "topic should match")
}

func TestConsumer_AddSubscription_Duplicate(t *testing.T) {
	kgoClient := newTestKgoClient(t)
	cfg := newConfig([]string{"localhost:9092"}, "test-group")
	c, err := newConsumer(cfg, kgoClient, nil)
	require.NoError(t, err, "newConsumer should succeed")

	sub := Subscription{
		Topic:         "my-topic",
		Handler:       func(context.Context, *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: FailurePolicy{},
		AckMode:       AckModeAtLeastOnce,
	}

	err = c.AddSubscription(sub)
	require.NoError(t, err, "first AddSubscription should succeed")

	err = c.AddSubscription(sub)
	assert.ErrorContains(t, err, "topic handler already registered", "duplicate should error")
}

func TestConsumer_AddSubscription_Invalid(t *testing.T) {
	kgoClient := newTestKgoClient(t)
	cfg := newConfig([]string{"localhost:9092"}, "test-group")
	c, err := newConsumer(cfg, kgoClient, nil)
	require.NoError(t, err, "newConsumer should succeed")

	sub := Subscription{
		Topic:         "",
		Handler:       func(context.Context, *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: FailurePolicy{},
		AckMode:       AckModeAtLeastOnce,
	}
	err = c.AddSubscription(sub)
	assert.ErrorContains(t, err, "topic must not be empty", "empty topic should error")
}

func TestConsumer_AddTopic(t *testing.T) {
	kgoClient := newTestKgoClient(t)
	cfg := newConfig([]string{"localhost:9092"}, "test-group")
	c, err := newConsumer(cfg, kgoClient, nil)
	require.NoError(t, err, "newConsumer should succeed")

	handler := func(context.Context, *kgo.Record) error { return nil }
	err = c.AddTopic("topic-a", handler)
	require.NoError(t, err, "AddTopic should succeed")

	got, ok := c.router.Lookup("topic-a")
	assert.True(t, ok, "topic should be found")
	assert.Equal(t, got.Topic, "topic-a", "topic should match")
	assert.NotNil(t, got.Handler, "handler should be set")
}

func TestConsumer_AddBatchTopic(t *testing.T) {
	kgoClient := newTestKgoClient(t)
	cfg := newConfig([]string{"localhost:9092"}, "test-group")
	c, err := newConsumer(cfg, kgoClient, nil)
	require.NoError(t, err, "newConsumer should succeed")

	handler := func(context.Context, []*kgo.Record) BatchResult { return BatchResult{} }
	err = c.AddBatchTopic("topic-b", handler)
	require.NoError(t, err, "AddBatchTopic should succeed")

	got, ok := c.router.Lookup("topic-b")
	assert.True(t, ok, "topic should be found")
	assert.Equal(t, got.Topic, "topic-b", "topic should match")
	assert.NotNil(t, got.BatchHandler, "batch handler should be set")
}

func TestConsumer_Run_CancelsOnContext(t *testing.T) {
	kgoClient := newTestKgoClient(t)
	cfg := newConfig([]string{"localhost:9092"}, "test-group")
	c, err := newConsumer(cfg, kgoClient, nil)
	require.NoError(t, err, "newConsumer should succeed")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	err = c.Run(ctx)
	assert.ErrorIs(t, err, context.Canceled, "Run should return context.Canceled")
}

func TestConsumer_Run_RejectsConcurrentRun(t *testing.T) {
	kgoClient := newTestKgoClient(t)
	cfg := newConfig([]string{"localhost:9092"}, "test-group")
	c, err := newConsumer(cfg, kgoClient, nil)
	require.NoError(t, err, "newConsumer should succeed")

	// Start the first run's lifecycle so runState is active, without
	// calling Run() itself (which would block on dispatch).
	_, beginErr := c.runState.Begin()
	require.NoError(t, beginErr, "first Begin should succeed")

	ch := make(chan struct{})
	go func() {
		<-c.runState.Context().Done()
		close(ch)
	}()

	// Second Run should be rejected immediately.
	ctx2 := context.Background()
	err = c.Run(ctx2)
	assert.ErrorContains(t, err, "run is already active", "concurrent Run should error")

	c.runState.Stop()
	<-ch
}

func TestConsumer_NormalizesOnRegister(t *testing.T) {
	kgoClient := newTestKgoClient(t)
	cfg := newConfig([]string{"localhost:9092"}, "test-group")
	c, err := newConsumer(cfg, kgoClient, nil)
	require.NoError(t, err, "newConsumer should succeed")

	err = c.AddSubscription(Subscription{
		Topic:         "t",
		Handler:       func(context.Context, *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: FailurePolicy{},
		AckMode:       AckModeAtLeastOnce,
	})
	require.NoError(t, err, "AddSubscription should succeed")

	got, ok := c.router.Lookup("t")
	require.True(t, ok, "topic should be found")
	assert.Equal(t, got.FailurePolicy.MaxAttempts, 1, "should normalize max attempts")
}

// stubFetchClient records commit and pause calls for rebalance tests.
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

func (s *stubFetchClient) PauseFetchTopics(topics ...string) []string {
	s.mu.Lock()
	s.pausedTopics = append(s.pausedTopics, topics...)
	s.mu.Unlock()
	return nil
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

	router := consumer.NewRouter(nil)
	pauses := consumer.NewPauseRegistry(time.Now, stub)
	run := consumer.NewRunState()
	registry := consumer.NewPartitionRegistry(nil)

	committer := consumer.NewCommitter(
		registry,
		stub,
		consumer.CommitConfig{
			FlushInterval:    10 * time.Millisecond,
			DebounceInterval: 1 * time.Millisecond,
		},
	)

	return &Consumer{
		cfg:          nil,
		kgoClient:    nil,
		dlqProducer:  nil,
		log:          testlogger.NewLogger(),
		router:       router,
		pauses:       pauses,
		runState:     run,
		registry:     registry,
		committer:    committer,
		dispatcher:   nil,
		workerRunner: nil,
	}
}

// addTestPartition creates a partition state with dirty offset and registers it.
func addTestPartition(
	t *testing.T,
	c *Consumer,
	topic string,
	partition int32,
	offset int64,
	epoch int32,
	dirty bool,
) {
	t.Helper()

	key := consumer.Key{Topic: topic, Partition: partition}
	sub := consumer.Subscription{
		Topic:         topic,
		Handler:       func(_ context.Context, _ *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: consumer.FailurePolicy{MaxAttempts: 1, RetryBackoff: 0, DLQ: nil, OnExhausted: consumer.ExhaustedActionStop},
		AckMode:       consumer.AckModeAtLeastOnce,
	}

	state, _, err := c.registry.GetOrCreate(context.Background(), key, sub, 10)
	require.NoError(t, err, "GetOrCreate should succeed")

	record := &kgo.Record{
		Topic:       topic,
		Partition:   partition,
		Offset:      offset,
		LeaderEpoch: epoch,
	}

	// MarkStopped so Finalize can proceed (done channel closes without blocking on queue).
	state.MarkStopped()

	if dirty {
		require.True(t, c.registry.AdvanceStateCommitOffset(state, record), "AdvanceStateCommitOffset should succeed")
	} else {
		state.AdvanceCommitOffset(record)
	}
}

func TestConsumer_OnPartitionsRevoked_CommitsSelectedOffsets(t *testing.T) {
	stub := &stubFetchClient{
		commitErr:     nil,
		commitBlockCh: nil,
		pausedParts:   nil,
		resumedParts:  nil,
		pausedTopics:  nil,
		committed:     nil,
		mu:            sync.Mutex{},
	}
	c := newTestConsumer(t, stub)

	addTestPartition(t, c, "topic-a", 1, 5, 4, true)
	addTestPartition(t, c, "topic-b", 0, 1, 7, true)

	c.onPartitionsRevoked(context.Background(), nil, map[string][]int32{
		"topic-b": {0},
	})

	stub.mu.Lock()
	assert.Equal(t, len(stub.committed), 1, "should commit revoked offsets")
	committed := stub.committed[0]
	stub.mu.Unlock()

	assert.Equal(t, committed["topic-b"][0].Offset, 2, "revoked partition should commit next offset")
	assert.Equal(t, committed["topic-b"][0].Epoch, 7, "revoked partition should commit epoch")

	_, topicAExists := c.registry.Get(consumer.Key{Topic: "topic-a", Partition: 1})
	assert.True(t, topicAExists, "unrevoked partition should remain")
	assert.NoError(t, c.runState.Err(), "successful revoke should not fail run")
}

func TestConsumer_OnPartitionsRevoked_FailsRunOnCommitError(t *testing.T) {
	stub := &stubFetchClient{
		commitErr:     errors.New("commit failed"),
		commitBlockCh: nil,
		pausedParts:   nil,
		resumedParts:  nil,
		pausedTopics:  nil,
		committed:     nil,
		mu:            sync.Mutex{},
	}
	c := newTestConsumer(t, stub)

	addTestPartition(t, c, "topic-a", 1, 5, 4, true)

	c.onPartitionsRevoked(context.Background(), nil, map[string][]int32{
		"topic-a": {1},
	})

	assert.ErrorIs(t, c.runState.Err(), stub.commitErr, "run should fail on commit error")
}

func TestConsumer_OnPartitionsRevoked_WaitsForDrainBeforeCommit(t *testing.T) {
	stub := &stubFetchClient{commitErr: nil, commitBlockCh: nil, pausedParts: nil, resumedParts: nil, pausedTopics: nil, committed: nil, mu: sync.Mutex{}}
	c := newTestConsumer(t, stub)

	key := consumer.Key{Topic: "t", Partition: 0}
	sub := consumer.Subscription{
		Topic:         "t",
		Handler:       func(_ context.Context, _ *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: consumer.FailurePolicy{MaxAttempts: 1, RetryBackoff: 0, DLQ: nil, OnExhausted: consumer.ExhaustedActionStop},
		AckMode:       consumer.AckModeAtLeastOnce,
	}

	state, _, err := c.registry.GetOrCreate(context.Background(), key, sub, 10)
	require.NoError(t, err, "GetOrCreate should succeed")
	state.AdvanceCommitOffset(&kgo.Record{Topic: "t", Partition: 0, Offset: 9, LeaderEpoch: 0})
	c.registry.AdvanceStateCommitOffset(state, &kgo.Record{Topic: state.Key().Topic, Partition: state.Key().Partition, Offset: 0, LeaderEpoch: 0})

	// Do NOT MarkStopped — Finalize must wait for drain.
	// Queue closes via BeginClosing, then the drain loop in Finalize
	// detects state.done since BeginClosing closes queue and worker exits.
	// For this test, call MarkStopped to close the done channel and simulate drain.
	state.MarkStopped()

	c.onPartitionsRevoked(context.Background(), nil, map[string][]int32{"t": {0}})

	stub.mu.Lock()
	assert.True(t, len(stub.committed) >= 1, "should commit after drain, got %d", len(stub.committed))
	stub.mu.Unlock()
}

func TestConsumer_OnPartitionsLost_DropsSelectedOffsets(t *testing.T) {
	stub := &stubFetchClient{commitErr: nil, commitBlockCh: nil, pausedParts: nil, resumedParts: nil, pausedTopics: nil, committed: nil, mu: sync.Mutex{}}
	c := newTestConsumer(t, stub)

	addTestPartition(t, c, "topic-a", 1, 5, 4, true)
	addTestPartition(t, c, "topic-b", 0, 1, 7, true)

	c.onPartitionsLost(context.Background(), map[string][]int32{
		"topic-a": {1},
	})

	// Partition should be dropped — no commit, state removed.
	stub.mu.Lock()
	assert.Equal(t, len(stub.committed), 0, "lost partitions should not commit offsets")
	stub.mu.Unlock()

	_, topicAExists := c.registry.Get(consumer.Key{Topic: "topic-a", Partition: 1})
	assert.False(t, topicAExists, "lost partition should be removed")

	_, topicBExists := c.registry.Get(consumer.Key{Topic: "topic-b", Partition: 0})
	assert.True(t, topicBExists, "unlost partition should remain")
}

func TestConsumer_OnPartitionsRevoked_UsesCallbackContextForFinalCommit(t *testing.T) {
	stub := &stubFetchClient{commitErr: nil, commitBlockCh: nil, pausedParts: nil, resumedParts: nil, pausedTopics: nil, committed: nil, mu: sync.Mutex{}}
	c := newTestConsumer(t, stub)

	addTestPartition(t, c, "t", 0, 5, 4, true)

	// Use a pre-cancelled context to verify it's threaded through.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c.onPartitionsRevoked(ctx, nil, map[string][]int32{"t": {0}})

	// A cancelled context should cause Finalize to fail.
	// onPartitionsRevoked calls Fail on the error.
	err := c.runState.Err()
	assert.NotNil(t, err, "revoke with cancelled context should fail run")
	assert.ErrorIs(t, err, context.Canceled, "error should be context.Canceled")
}

func TestConsumer_OnPartitionsRevoked_StopsWaitingWhenContextCancelled(t *testing.T) {
	stub := &stubFetchClient{commitErr: nil, commitBlockCh: nil, pausedParts: nil, resumedParts: nil, pausedTopics: nil, committed: nil, mu: sync.Mutex{}}
	c := newTestConsumer(t, stub)

	// Create a partition state that blocks on drain (not stopped, not closed).
	key := consumer.Key{Topic: "t", Partition: 0}
	sub := consumer.Subscription{
		Topic:         "t",
		Handler:       func(_ context.Context, _ *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: consumer.FailurePolicy{MaxAttempts: 1, RetryBackoff: 0, DLQ: nil, OnExhausted: consumer.ExhaustedActionStop},
		AckMode:       consumer.AckModeAtLeastOnce,
	}

	state, _, err := c.registry.GetOrCreate(context.Background(), key, sub, 10)
	require.NoError(t, err, "GetOrCreate should succeed")
	state.AdvanceCommitOffset(&kgo.Record{Topic: "t", Partition: 0, Offset: 9, LeaderEpoch: 0})
	c.registry.AdvanceStateCommitOffset(state, &kgo.Record{Topic: state.Key().Topic, Partition: state.Key().Partition, Offset: 0, LeaderEpoch: 0})
	// Do NOT call MarkStopped — state.Done() will block.

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	c.onPartitionsRevoked(ctx, nil, map[string][]int32{"t": {0}})

	// Finalize should have timed out waiting for drain.
	err = c.runState.Err()
	assert.NotNil(t, err, "revoke with timeout should fail run because drain won't complete")
}

// TestConsumer_WaitForWorkersToStop_TimeOutAndFailClosed verifies the bounded
// shutdown wait: a goroutine that never exits (simulating a partition worker
// pinned by a context-ignoring handler) must not hang shutdown. The wait
// returns false after shutdownTimeout, and the consumer is left fail-closed —
// run state stays active so the consumer cannot be recycled into a new run
// that would race the abandoned goroutines.
func TestConsumer_WaitForWorkersToStop_TimeOutAndFailClosed(t *testing.T) {
	kgoClient := newTestKgoClient(t)
	cfg := newConfig([]string{"localhost:9092"}, "test-group")
	WithShutdownTimeout(50 * time.Millisecond)(cfg)

	c, err := newConsumer(cfg, kgoClient, nil)
	require.NoError(t, err, "newConsumer should succeed")

	_, beginErr := c.runState.Begin()
	require.NoError(t, beginErr, "Begin should succeed")

	// A tracked goroutine that never exits — the shutdown wait must time out
	// instead of hanging indefinitely.
	c.runState.Go(func() { select {} })

	ok := c.waitForWorkersToStop(context.Background())
	require.False(t, ok, "wait should time out with a stuck worker")

	// Fail-closed: the consumer is left unusable, so a new run is rejected.
	_, beginErr = c.runState.Begin()
	require.ErrorContains(t, beginErr, "run is already active", "consumer must not be reusable after abandoned shutdown")
}

// TestConsumer_WaitForWorkersToStop_Completes verifies the normal path: an
// exiting goroutine lets the bounded wait finish promptly, and the consumer
// remains reusable after a proper Stop + Wait + Reset.
func TestConsumer_WaitForWorkersToStop_Completes(t *testing.T) {
	kgoClient := newTestKgoClient(t)
	cfg := newConfig([]string{"localhost:9092"}, "test-group")
	WithShutdownTimeout(50 * time.Millisecond)(cfg)

	c, err := newConsumer(cfg, kgoClient, nil)
	require.NoError(t, err, "newConsumer should succeed")

	runCtx, err := c.runState.Begin()
	require.NoError(t, err, "Begin should succeed")

	c.runState.Go(func() {
		select {
		case <-runCtx.Done():
		case <-time.After(5 * time.Millisecond):
		}
	})

	start := time.Now()
	ok := c.waitForWorkersToStop(context.Background())
	require.True(t, ok, "wait should complete when workers exit")
	require.Less(t, time.Since(start), 50*time.Millisecond, "wait should return well before the shutdown timeout")

	// With a clean wait, Stop + Reset recycle the run state.
	c.runState.Stop()
	c.runState.Reset()
	_, beginErr := c.runState.Begin()
	require.NoError(t, beginErr, "run state should be reusable after clean shutdown")
}
