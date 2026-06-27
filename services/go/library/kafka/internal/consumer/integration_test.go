// services/go/library/kafka/internal/consumer/integration_test.go
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

// integrationClient implements OffsetClient, WorkerClient, and FetchControlClient.
type integrationClient struct {
	committed      []map[string]map[int32]kgo.EpochOffset
	pausedByTopics []string
	pausedParts    []map[string][]int32
	resumedParts   []map[string][]int32
	mu             sync.Mutex
}

func (c *integrationClient) CommitOffsetsSync(_ context.Context, offsets map[string]map[int32]kgo.EpochOffset) error {
	c.mu.Lock()
	c.committed = append(c.committed, offsets)
	c.mu.Unlock()
	return nil
}

func (c *integrationClient) PauseFetchTopics(topics ...string) {
	c.mu.Lock()
	c.pausedByTopics = append(c.pausedByTopics, topics...)
	c.mu.Unlock()
}

func (c *integrationClient) PauseFetchPartitions(partitions map[string][]int32) {
	c.mu.Lock()
	c.pausedParts = append(c.pausedParts, partitions)
	c.mu.Unlock()
}

func (c *integrationClient) ResumeFetchPartitions(partitions map[string][]int32) {
	c.mu.Lock()
	c.resumedParts = append(c.resumedParts, partitions)
	c.mu.Unlock()
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
func setupIntegration(t *testing.T, cl *integrationClient) (
	*consumer.RunState,
	*consumer.Router,
	*consumer.PartitionRegistry,
	*consumer.Committer,
	*consumer.Dispatcher,
) {
	t.Helper()

	logger := testlogger.NewLogger()
	router := consumer.NewRouter(nil)
	pauses := consumer.NewPauseRegistry(time.Now)
	run := consumer.NewRunState()
	registry := consumer.NewPartitionRegistry(logger)

	committer := consumer.NewCommitter(
		logger, registry, pauses, cl,
		consumer.CommitConfig{
			FlushInterval:      10 * time.Millisecond,
			DebounceInterval:   1 * time.Millisecond,
			DrainTimeout:       5 * time.Second,
			FinalCommitTimeout: 5 * time.Second,
		},
	)

	executor := consumer.NewRecordExecutor(logger)

	wr := consumer.NewWorkerRunner(
		logger, run, committer, executor,
		registry, pauses,
		4,
		nil, // notifyCapacity — wired below
		nil, // dlqWriter
	)

	dispatcher := consumer.NewDispatcher(router, pauses, registry, wr)
	wr.SetNotifyCapacity(dispatcher.NotifyCapacity)

	return run, router, registry, committer, dispatcher
}

// TestIntegration_FullPipeline_Success dispatches records, verifies handler
// invocation and offset commit through the full pipeline.
func TestIntegration_FullPipeline_Success(t *testing.T) {
	cl := &integrationClient{}
	run, router, registry, committer, dis := setupIntegration(t, cl)

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

	require.NoError(t, dis.Dispatch(context.Background(), records, cl, cl),
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
	cl := &integrationClient{}
	run, router, registry, committer, dis := setupIntegration(t, cl)

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

	require.NoError(t, dis.Dispatch(context.Background(), records, cl, cl),
		"Dispatch should succeed")

	require.True(t, waitFor(2*time.Second, func() bool {
		cl.mu.Lock()
		defer cl.mu.Unlock()
		return len(cl.pausedByTopics) >= 1
	}), "topic should be paused after exhaustion")

	// Second dispatch should skip the paused topic.
	records2 := []*kgo.Record{{Topic: "test-topic", Partition: 0, Offset: 1, LeaderEpoch: 0}}
	require.NoError(t, dis.Dispatch(context.Background(), records2, cl, cl),
		"second Dispatch should succeed with paused topic skipped")

	// BeginClosingAll drains partition states so run.Wait() can return.
	registry.BeginClosingAll()
	run.Stop()
	run.Wait()

	assert.NoError(t, <-commitDone, "commit loop should exit cleanly")
}

// TestIntegration_BackpressureCycle verifies the pause → drain → resume cycle.
func TestIntegration_BackpressureCycle(t *testing.T) {
	cl := &integrationClient{}
	run, router, registry, committer, dis := setupIntegration(t, cl)

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

	require.NoError(t, dis.Dispatch(context.Background(), records, cl, cl),
		"Dispatch should succeed without error")

	require.True(t, waitFor(30*time.Second, func() bool {
		cl.mu.Lock()
		resumed := len(cl.resumedParts)
		cl.mu.Unlock()
		return resumed > 0
	}), "should have resumed partitions after backpressure drained")

	// BeginClosingAll drains partition states so run.Wait() can return.
	registry.BeginClosingAll()
	run.Stop()
	run.Wait()

	assert.NoError(t, <-commitDone, "commit loop should exit cleanly")
}

// TestIntegration_ShutdownFlushesRemaining verifies finalization during graceful shutdown.
func TestIntegration_ShutdownFlushesRemaining(t *testing.T) {
	cl := &integrationClient{}
	run, router, registry, committer, dis := setupIntegration(t, cl)

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

	require.NoError(t, dis.Dispatch(context.Background(), records, cl, cl),
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
