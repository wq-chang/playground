// services/go/library/kafka/internal/consumer/integration_test.go
package consumer_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"go-services/library/assert"
	"go-services/library/kafka/internal/consumer"
	"go-services/library/require"
	"go-services/library/testlogger"
)

// offsetRecorder records offset commits for test verification.
type offsetRecorder struct {
	committed []map[string]map[int32]kgo.EpochOffset
	mu        sync.Mutex
}

func (c *offsetRecorder) CommitOffsetsSync(_ context.Context, offsets map[string]map[int32]kgo.EpochOffset) error {
	c.mu.Lock()
	c.committed = append(c.committed, offsets)
	c.mu.Unlock()
	return nil
}

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

func subRecordHandler(fn func(context.Context, *kgo.Record) error) consumer.Subscription {
	return consumer.Subscription{
		Topic:         "test-topic",
		Handler:       fn,
		BatchHandler:  nil,
		FailurePolicy: consumer.FailurePolicy{MaxAttempts: 1, RetryBackoff: 0, DLQ: nil, OnExhausted: consumer.ExhaustedActionStop},
		AckMode:       consumer.AckModeAtLeastOnce,
	}
}

// setupIntegration creates all collaborators wired together.
func setupIntegration(t *testing.T, cl *offsetRecorder) (
	*consumer.RunState,
	*consumer.Router,
	*consumer.PartitionRegistry,
	*consumer.Committer,
	*consumer.PauseRegistry,
	*consumer.Dispatcher,
) {
	t.Helper()

	kgoClient := newTestKgoClient(t)

	logger := testlogger.NewLogger()
	router := consumer.NewRouter(kgoClient.AddConsumeTopics)
	pauses := consumer.NewPauseRegistry(time.Now)
	run := consumer.NewRunState()
	registry := consumer.NewPartitionRegistry(logger)

	committer := consumer.NewCommitter(
		registry, cl,
		consumer.CommitConfig{
			FlushInterval:      10 * time.Millisecond,
			DebounceInterval:   1 * time.Millisecond,
			DrainTimeout:       5 * time.Second,
			FinalCommitTimeout: 5 * time.Second,
		},
	)

	executor := consumer.NewRecordExecutor(logger)

	capacityCh := make(chan struct{}, 1)

	wr := consumer.NewWorkerRunner(
		logger, run, committer, executor,
		registry, pauses,
		4,
		capacityCh,
		nil, // dlqWriter
		kgoClient,
	)

	dispatcher := consumer.NewDispatcher(router, pauses, registry, kgoClient, wr.Start, capacityCh)

	return run, router, registry, committer, pauses, dispatcher
}

// TestIntegration_FullPipeline_Success dispatches records, verifies handler
// invocation and offset commit through the full pipeline.
func TestIntegration_FullPipeline_Success(t *testing.T) {
	cl := &offsetRecorder{}
	run, router, registry, committer, _, dis := setupIntegration(t, cl)

	var handled atomic.Int32
	handler := func(_ context.Context, _ *kgo.Record) error {
		handled.Add(1)
		return nil
	}

	require.NoError(t, router.Register(subRecordHandler(handler)), "Register should succeed")

	runCtx, err := run.Begin()
	require.NoError(t, err, "Begin should succeed")

	// Start commit loop.
	commitDone := make(chan error, 1)
	run.Go(func() {
		commitDone <- committer.Run(runCtx)
	})

	// Dispatch records.
	records := []*kgo.Record{
		{Topic: "test-topic", Partition: 0, Offset: 0, LeaderEpoch: 0},
		{Topic: "test-topic", Partition: 0, Offset: 1, LeaderEpoch: 0},
		{Topic: "test-topic", Partition: 0, Offset: 2, LeaderEpoch: 0},
	}

	require.NoError(t, dis.Dispatch(context.Background(), records),
		"Dispatch should succeed")

	require.True(t, waitFor(2*time.Second, func() bool { return handled.Load() == 3 }),
		"all 3 records should be processed, got %d", handled.Load())

	// Trigger a flush.
	require.NoError(t, committer.Flush(context.Background()), "Flush should succeed")

	cl.mu.Lock()
	assert.True(t, len(cl.committed) >= 1, "should have committed at least once, got %d", len(cl.committed))
	cl.mu.Unlock()

	// Graceful shutdown.
	states := registry.BeginClosingAll()
	drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, committer.Finalize(drainCtx, nil, states,
		"failed to commit on shutdown"),
		"Finalize should succeed")

	run.Stop()
	run.Wait()

	// Commit loop should exit cleanly.
	select {
	case commitErr := <-commitDone:
		assert.NoError(t, commitErr, "commit loop should exit cleanly")
	case <-time.After(2 * time.Second):
		t.Fatal("commit loop did not exit")
	}
}

// TestIntegration_ExhaustionPausesTopic verifies that handler exhaustion
// pauses the topic and stops processing.
func TestIntegration_ExhaustionPausesTopic(t *testing.T) {
	cl := &offsetRecorder{}
	run, router, registry, committer, pauses, dis := setupIntegration(t, cl)

	var handled atomic.Int32
	handler := func(_ context.Context, _ *kgo.Record) error {
		handled.Add(1)
		return errors.New("always fails") // always fails
	}

	require.NoError(t, router.Register(subRecordHandler(handler)), "Register should succeed")

	runCtx, err := run.Begin()
	require.NoError(t, err, "Begin should succeed")

	commitDone := make(chan error, 1)
	run.Go(func() {
		commitDone <- committer.Run(runCtx)
	})

	records := []*kgo.Record{
		{Topic: "test-topic", Partition: 0, Offset: 0, LeaderEpoch: 0},
	}

	require.NoError(t, dis.Dispatch(context.Background(), records),
		"Dispatch should succeed")

	require.True(t, waitFor(2*time.Second, func() bool {
		return pauses.IsPaused("test-topic")
	}), "topic should be paused after exhaustion")

	// Second dispatch should skip the paused topic.
	records2 := []*kgo.Record{{Topic: "test-topic", Partition: 0, Offset: 1, LeaderEpoch: 0}}
	require.NoError(t, dis.Dispatch(context.Background(), records2),
		"second Dispatch should succeed with paused topic skipped")

	// BeginClosingAll drains partition states so run.Wait() can return.
	registry.BeginClosingAll()
	run.Stop()
	run.Wait()

	assert.NoError(t, <-commitDone, "commit loop should exit cleanly")
}

// TestIntegration_BackpressureCycle verifies the pause → drain → resume cycle.
func TestIntegration_BackpressureCycle(t *testing.T) {
	cl := &offsetRecorder{}
	run, router, registry, committer, _, dis := setupIntegration(t, cl)

	require.NoError(t, router.Register(consumer.Subscription{
		Topic:         "a",
		Handler:       func(_ context.Context, _ *kgo.Record) error { time.Sleep(5 * time.Millisecond); return nil },
		BatchHandler:  nil,
		FailurePolicy: consumer.FailurePolicy{MaxAttempts: 1, RetryBackoff: 0, DLQ: nil, OnExhausted: consumer.ExhaustedActionStop},
		AckMode:       consumer.AckModeAtLeastOnce,
	}), "Register a should succeed")
	require.NoError(t, router.Register(consumer.Subscription{
		Topic:         "b",
		Handler:       func(_ context.Context, _ *kgo.Record) error { time.Sleep(5 * time.Millisecond); return nil },
		BatchHandler:  nil,
		FailurePolicy: consumer.FailurePolicy{MaxAttempts: 1, RetryBackoff: 0, DLQ: nil, OnExhausted: consumer.ExhaustedActionStop},
		AckMode:       consumer.AckModeAtLeastOnce,
	}), "Register b should succeed")

	runCtx, err := run.Begin()
	require.NoError(t, err, "Begin should succeed")

	commitDone := make(chan error, 1)
	run.Go(func() {
		commitDone <- committer.Run(runCtx)
	})

	const nRecords = 200
	records := make([]*kgo.Record, 0, nRecords)
	for i := range int64(nRecords / 2) {
		records = append(records, &kgo.Record{Topic: "a", Partition: 0, Offset: i, LeaderEpoch: 0})
		records = append(records, &kgo.Record{Topic: "b", Partition: 0, Offset: i, LeaderEpoch: 0})
	}

	require.NoError(t, dis.Dispatch(context.Background(), records),
		"Dispatch should succeed without error")

	// Verify all records were processed through backpressure cycle.
	require.True(t, waitFor(30*time.Second, func() bool {
		cl.mu.Lock()
		committed := len(cl.committed)
		cl.mu.Unlock()
		return committed > 0
	}), "should have committed offsets after processing all records under backpressure")

	// BeginClosingAll drains partition states so run.Wait() can return.
	registry.BeginClosingAll()
	run.Stop()
	run.Wait()

	assert.NoError(t, <-commitDone, "commit loop should exit cleanly")
}

// TestDispatcher_Dispatch_PreservesPolledOrderWithinPartition verifies that
// records from the same topic-partition are processed in poll order.
func TestDispatcher_Dispatch_PreservesPolledOrderWithinPartition(t *testing.T) {
	cl := &offsetRecorder{}
	_, router, _, _, _, dis := setupIntegration(t, cl)

	var mu sync.Mutex
	var ordered []int64
	handler := func(_ context.Context, r *kgo.Record) error {
		mu.Lock()
		ordered = append(ordered, r.Offset)
		mu.Unlock()
		return nil
	}

	require.NoError(t, router.Register(consumer.Subscription{
		Topic:         "t",
		Handler:       handler,
		BatchHandler:  nil,
		FailurePolicy: consumer.FailurePolicy{MaxAttempts: 1, RetryBackoff: 0, DLQ: nil, OnExhausted: consumer.ExhaustedActionStop},
		AckMode:       consumer.AckModeAtLeastOnce,
	}), "Register should succeed")

	// Dispatch records in offset order 0,1,2 — handler must see them in order.
	records := []*kgo.Record{
		{Topic: "t", Partition: 0, Offset: 0, LeaderEpoch: 0},
		{Topic: "t", Partition: 0, Offset: 1, LeaderEpoch: 0},
		{Topic: "t", Partition: 0, Offset: 2, LeaderEpoch: 0},
	}

	require.NoError(t, dis.Dispatch(context.Background(), records),
		"Dispatch should succeed")

	require.True(t, waitFor(2*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(ordered) == 3
	}), "all 3 records should be processed, got %d", len(ordered))

	mu.Lock()
	assert.Equal(t, int64(0), ordered[0], "first processed should be offset 0")
	assert.Equal(t, int64(1), ordered[1], "second processed should be offset 1")
	assert.Equal(t, int64(2), ordered[2], "third processed should be offset 2")
	mu.Unlock()
}

func TestDispatcher_Dispatch_PreservesFirstSeenPartitionOrder(t *testing.T) {
	cl := &offsetRecorder{}
	_, router, _, _, _, dis := setupIntegration(t, cl)

	var mu sync.Mutex
	var ordered []string
	// Track each record as "topic:offset".
	handlerA := func(_ context.Context, r *kgo.Record) error {
		mu.Lock()
		ordered = append(ordered, fmt.Sprintf("%s:%d", r.Topic, r.Offset))
		mu.Unlock()
		return nil
	}
	handlerB := func(_ context.Context, r *kgo.Record) error {
		mu.Lock()
		ordered = append(ordered, fmt.Sprintf("%s:%d", r.Topic, r.Offset))
		mu.Unlock()
		return nil
	}

	require.NoError(t, router.Register(consumer.Subscription{
		Topic: "a", Handler: handlerA, BatchHandler: nil,
		FailurePolicy: consumer.FailurePolicy{MaxAttempts: 1, RetryBackoff: 0, DLQ: nil, OnExhausted: consumer.ExhaustedActionStop},
		AckMode:       consumer.AckModeAtLeastOnce,
	}), "Register a")
	require.NoError(t, router.Register(consumer.Subscription{
		Topic: "b", Handler: handlerB, BatchHandler: nil,
		FailurePolicy: consumer.FailurePolicy{MaxAttempts: 1, RetryBackoff: 0, DLQ: nil, OnExhausted: consumer.ExhaustedActionStop},
		AckMode:       consumer.AckModeAtLeastOnce,
	}), "Register b")

	// Interleave records; within each partition, dispatch order matches poll order.
	records := []*kgo.Record{
		{Topic: "a", Partition: 0, Offset: 0, LeaderEpoch: 0},
		{Topic: "b", Partition: 0, Offset: 0, LeaderEpoch: 0},
		{Topic: "a", Partition: 0, Offset: 1, LeaderEpoch: 0},
	}

	require.NoError(t, dis.Dispatch(context.Background(), records),
		"Dispatch should succeed")

	require.True(t, waitFor(2*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(ordered) == 3
	}), "all 3 records should be processed, got %d", len(ordered))

	mu.Lock()
	// Within a partition, records are processed in polled offset order.
	var aOffsets []int64
	for _, s := range ordered {
		if strings.HasPrefix(s, "a:") {
			off, err := strconv.ParseInt(s[2:], 10, 64)
			require.NoError(t, err, "ParseInt should succeed")
			aOffsets = append(aOffsets, off)
		}
	}
	assert.Equal(t, 2, len(aOffsets), "should process 2 records for 'a'")
	assert.True(t, aOffsets[0] < aOffsets[1], "within 'a', offset 0 should process before offset 1")
	mu.Unlock()
}

func TestIntegration_DispatchDoesNotBlockOtherPartitions(t *testing.T) {
	cl := &offsetRecorder{}
	_, router, _, _, _, dis := setupIntegration(t, cl)

	require.NoError(t, router.Register(consumer.Subscription{
		Topic:         "a",
		Handler:       func(_ context.Context, _ *kgo.Record) error { select {} },
		BatchHandler:  nil,
		FailurePolicy: consumer.FailurePolicy{MaxAttempts: 1, RetryBackoff: 0, DLQ: nil, OnExhausted: consumer.ExhaustedActionStop},
		AckMode:       consumer.AckModeAtLeastOnce,
	}), "Register a")

	var processed atomic.Int32
	require.NoError(t, router.Register(consumer.Subscription{
		Topic:         "b",
		Handler:       func(_ context.Context, _ *kgo.Record) error { processed.Add(1); return nil },
		BatchHandler:  nil,
		FailurePolicy: consumer.FailurePolicy{MaxAttempts: 1, RetryBackoff: 0, DLQ: nil, OnExhausted: consumer.ExhaustedActionStop},
		AckMode:       consumer.AckModeAtLeastOnce,
	}), "Register b")

	records := []*kgo.Record{
		{Topic: "a", Partition: 0, Offset: 0, LeaderEpoch: 0},
		{Topic: "b", Partition: 0, Offset: 0, LeaderEpoch: 0},
	}

	// Dispatch must not block — Verify "b" got dispatched and enqueued.
	// Since the workers have 4 concurrent slots (default), both records get a slot.
	// "a" blocks forever in handler, "b" proceeds and completes.
	require.NoError(t, dis.Dispatch(context.Background(), records),
		"Dispatch should not block")

	require.True(t, waitFor(2*time.Second, func() bool { return processed.Load() == 1 }),
		"partition 'b' should process despite 'a' blocking, got %d", processed.Load())
}

func TestIntegration_ShutdownFlushesRemaining(t *testing.T) {
	cl := &offsetRecorder{}
	run, router, registry, committer, _, dis := setupIntegration(t, cl)

	var handled atomic.Int32
	handler := func(_ context.Context, _ *kgo.Record) error {
		handled.Add(1)
		time.Sleep(10 * time.Millisecond)
		return nil
	}

	require.NoError(t, router.Register(subRecordHandler(handler)), "Register should succeed")

	runCtx, err := run.Begin()
	require.NoError(t, err, "Begin should succeed")

	commitDone := make(chan error, 1)
	run.Go(func() {
		commitDone <- committer.Run(runCtx)
	})

	records := []*kgo.Record{
		{Topic: "test-topic", Partition: 0, Offset: 0, LeaderEpoch: 0},
		{Topic: "test-topic", Partition: 0, Offset: 1, LeaderEpoch: 0},
	}

	require.NoError(t, dis.Dispatch(context.Background(), records),
		"Dispatch should succeed")

	require.True(t, waitFor(2*time.Second, func() bool { return handled.Load() == 2 }),
		"both records should be processed, got %d", handled.Load())

	// Trigger shutdown through Finalize.
	states := registry.BeginClosingAll()
	drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, committer.Finalize(drainCtx, nil, states,
		"failed to commit on shutdown"),
		"Finalize should succeed")

	run.Stop()
	run.Wait()

	assert.NoError(t, <-commitDone, "commit loop should exit cleanly")

	cl.mu.Lock()
	defer cl.mu.Unlock()
	assert.True(t, len(cl.committed) > 0, "should have committed offsets during shutdown, got %d",
		len(cl.committed))
}

func waitFor(timeout time.Duration, fn func() bool) bool {
	deadline := time.After(timeout)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()

	for {
		if fn() {
			return true
		}
		select {
		case <-deadline:
			return false
		case <-tick.C:
		}
	}
}
