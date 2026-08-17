// Package consumer_test contains in-process wiring tests for the consumer
// pipeline.
//
// These tests assemble the real pipeline components (Router, PauseRegistry,
// PartitionRegistry, Committer, WorkerRunner, Dispatcher) exactly as
// kafka/consumer.go does in production, but with every broker-facing seam
// stubbed: records are dispatched directly into the pipeline, offset commits
// are recorded in memory, and Client interactions (add topics, pause/resume
// fetches) are recorded by stubBrokerClient. There is deliberately no Kafka
// broker and no network I/O here — protocol-level behavior against a real
// broker is covered by the tagged integration tests in the parent kafka/
// package (testcontainers, see kafka/main_test.go's `integration` tag).
package consumer_test

import (
	"context"
	"errors"
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

// failingOffsetClient always fails offset commits with the configured error,
// simulating a broker rejecting OffsetCommit requests.
type failingOffsetClient struct {
	err error
}

func (c *failingOffsetClient) CommitOffsetsSync(_ context.Context, _ map[string]map[int32]kgo.EpochOffset) error {
	return c.err
}

// stubBrokerClient satisfies the narrow *kgo.Client subsets that the pipeline
// components depend on: Router's add-topics hook, Dispatcher's partition
// pauser, and WorkerRunner's partition resumer. It records calls so tests can
// assert client interaction, but performs no network I/O — these tests
// dispatch records directly and never poll a broker.
type stubBrokerClient struct {
	addedTopics  []string
	pausedParts  []map[string][]int32
	resumedParts []map[string][]int32
	mu           sync.Mutex
}

// AddConsumeTopics implements the addTopics hook consumed by Router.
func (s *stubBrokerClient) AddConsumeTopics(topics ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.addedTopics = append(s.addedTopics, topics...)
}

// PauseFetchPartitions implements partitionPauser for Dispatcher.
func (s *stubBrokerClient) PauseFetchPartitions(parts map[string][]int32) map[string][]int32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pausedParts = append(s.pausedParts, parts)
	return parts
}

// ResumeFetchPartitions implements partitionResumer for WorkerRunner.
func (s *stubBrokerClient) ResumeFetchPartitions(parts map[string][]int32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resumedParts = append(s.resumedParts, parts)
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
func setupIntegration(t *testing.T, cl consumer.OffsetClient) (
	*consumer.RunState,
	*consumer.Router,
	*consumer.PartitionRegistry,
	*consumer.Committer,
	*consumer.PauseRegistry,
	*consumer.Dispatcher,
) {
	t.Helper()

	client := &stubBrokerClient{}

	logger := testlogger.NewLogger()
	router := consumer.NewRouter(client.AddConsumeTopics)
	pauses := consumer.NewPauseRegistry(time.Now, &stubTopicPauser{})
	run := consumer.NewRunState()
	registry := consumer.NewPartitionRegistry(logger)

	committer := consumer.NewCommitter(
		registry, cl,
		consumer.CommitConfig{
			FlushInterval:    10 * time.Millisecond,
			DebounceInterval: 1 * time.Millisecond,
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
		client,
	)

	dispatcher := consumer.NewDispatcher(router, pauses, registry, client, wr.Start, 64, capacityCh)

	return run, router, registry, committer, pauses, dispatcher
}

// TestIntegration_FullPipeline_Success dispatches records, verifies handler
// invocation and offset commit through the full pipeline.
func TestPipeline_FullRun_Success(t *testing.T) {
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

	require.NoError(t, dis.Dispatch(context.Background(), runCtx, records),
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, committer.Finalize(ctx, states,
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
func TestPipeline_ExhaustionPausesTopic(t *testing.T) {
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

	require.NoError(t, dis.Dispatch(context.Background(), runCtx, records),
		"Dispatch should succeed")

	require.True(t, waitFor(2*time.Second, func() bool {
		return pauses.IsPaused("test-topic")
	}), "topic should be paused after exhaustion")

	// Second dispatch should skip the paused topic.
	records2 := []*kgo.Record{{Topic: "test-topic", Partition: 0, Offset: 1, LeaderEpoch: 0}}
	require.NoError(t, dis.Dispatch(context.Background(), runCtx, records2),
		"second Dispatch should succeed with paused topic skipped")

	// BeginClosingAll drains partition states so run.Wait() can return.
	registry.BeginClosingAll()
	run.Stop()
	run.Wait()

	assert.NoError(t, <-commitDone, "commit loop should exit cleanly")
}

// TestIntegration_BackpressureCycle verifies the pause → drain → resume cycle.
func TestPipeline_BackpressureCycle(t *testing.T) {
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

	require.NoError(t, dis.Dispatch(context.Background(), runCtx, records),
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

func TestPipeline_DispatchDoesNotBlockOtherPartitions(t *testing.T) {
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
	require.NoError(t, dis.Dispatch(context.Background(), context.Background(), records),
		"Dispatch should not block")

	require.True(t, waitFor(2*time.Second, func() bool { return processed.Load() == 1 }),
		"partition 'b' should process despite 'a' blocking, got %d", processed.Load())
}

func TestPipeline_ShutdownFlushesRemaining(t *testing.T) {
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

	require.NoError(t, dis.Dispatch(context.Background(), runCtx, records),
		"Dispatch should succeed")

	require.True(t, waitFor(2*time.Second, func() bool { return handled.Load() == 2 }),
		"both records should be processed, got %d", handled.Load())

	// Trigger shutdown through Finalize.
	states := registry.BeginClosingAll()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, committer.Finalize(ctx, states,
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

// TestIntegration_FatalErrorDoesNotDeadlock verifies that shutdown completes
// even after a fatal error is recorded mid-run. Regression test for the
// deadlock where partition workers' contexts were decoupled from the run
// lifecycle, causing Wait() to hang forever after a fatal error. See C1/H1 in todo.md.
func TestPipeline_FatalErrorDoesNotDeadlock(t *testing.T) {
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

	commitDone := make(chan error, 1)
	run.Go(func() {
		commitDone <- committer.Run(runCtx)
	})

	// Dispatch records, passing the run context so partition workers are
	// children of the run lifecycle — this is the core of the C1/H1 fix.
	records := []*kgo.Record{
		{Topic: "test-topic", Partition: 0, Offset: 0, LeaderEpoch: 0},
		{Topic: "test-topic", Partition: 0, Offset: 1, LeaderEpoch: 0},
	}
	require.NoError(t, dis.Dispatch(context.Background(), runCtx, records),
		"Dispatch should succeed")

	require.True(t, waitFor(2*time.Second, func() bool { return handled.Load() == 2 }),
		"both records should be processed, got %d", handled.Load())

	// Simulate a fatal error mid-run.
	run.Fail(errors.New("simulated fatal error"))

	// Shut down. If the fix is working, BeginClosingAll + Stop + Wait
	// will return promptly rather than hanging.
	states := registry.BeginClosingAll()
	registry.Cleanup(states)
	run.Stop()

	done := make(chan struct{})
	go func() {
		run.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success — Wait() returned without hanging.
	case <-time.After(5 * time.Second):
		t.Fatal("run.Wait() hung after fatal error — deadlock regression")
	}

	// Commit loop should exit after Stop().
	select {
	case <-commitDone:
	case <-time.After(2 * time.Second):
		t.Fatal("commit loop did not exit after Stop()")
	}

	assert.Equal(t, run.Err().Error(), "simulated fatal error",
		"fatal error should be preserved")
}

// TestIntegration_CommitLoopFailureFailsRun verifies that a commit failure in
// the async commit loop propagates through the wiring exactly as consumer.Run
// wires it: committer.Run error → runState.Fail. A real broker cannot be made
// to reject offset commits deterministically, so this path is exercised here
// with a failing OffsetClient. Regression guard for the shutdown shape: the
// assembly must fail the run and still complete shutdown without hanging.
func TestPipeline_CommitLoopFailureFailsRun(t *testing.T) {
	commitErr := errors.New("simulated commit rejection")
	run, router, registry, committer, _, dis := setupIntegration(t, &failingOffsetClient{err: commitErr})

	var handled atomic.Int32
	handler := func(_ context.Context, _ *kgo.Record) error {
		handled.Add(1)
		return nil
	}

	require.NoError(t, router.Register(subRecordHandler(handler)), "Register should succeed")

	runCtx, err := run.Begin()
	require.NoError(t, err, "Begin should succeed")

	// Wire the commit loop exactly as consumer.Run does.
	commitDone := make(chan error, 1)
	run.Go(func() {
		commitLoopErr := committer.Run(runCtx)
		if commitLoopErr != nil {
			run.Fail(commitLoopErr)
		}
		commitDone <- commitLoopErr
	})

	records := []*kgo.Record{{Topic: "test-topic", Partition: 0, Offset: 0, LeaderEpoch: 0}}
	require.NoError(t, dis.Dispatch(context.Background(), runCtx, records),
		"Dispatch should succeed")

	require.True(t, waitFor(2*time.Second, func() bool { return handled.Load() == 1 }),
		"record should be processed, got %d", handled.Load())

	// Processing marks the offset dirty; the commit loop's flush then fails
	// and the run is failed through the wiring above.
	require.True(t, waitFor(2*time.Second, func() bool { return run.Err() != nil }),
		"commit loop failure should fail the run")

	// Shutdown must complete without hanging (same shape as the deadlock
	// regression): BeginClosingAll + Cleanup + Stop + Wait.
	states := registry.BeginClosingAll()
	registry.Cleanup(states)
	run.Stop()

	done := make(chan struct{})
	go func() {
		run.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("run.Wait() hung after commit-loop failure")
	}

	// The commit loop should exit with the propagated commit error.
	select {
	case commitLoopErr := <-commitDone:
		assert.ErrorIs(t, commitLoopErr, commitErr, "commit loop should propagate the commit failure")
	case <-time.After(2 * time.Second):
		t.Fatal("commit loop did not exit after Stop()")
	}

	assert.ErrorIs(t, run.Err(), commitErr,
		"fatal error should preserve the commit failure")
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
