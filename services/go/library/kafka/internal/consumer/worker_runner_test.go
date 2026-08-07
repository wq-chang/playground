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

type stubWorkerClient struct {
	committed []map[string]map[int32]kgo.EpochOffset
	mu        sync.Mutex
}

func (s *stubWorkerClient) CommitOffsetsSync(
	_ context.Context,
	offsets map[string]map[int32]kgo.EpochOffset,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.committed = append(s.committed, offsets)
	return nil
}

// stubResumer records ResumeFetchPartitions calls for backpressure-resume tests.
type stubResumer struct {
	resumed []map[string][]int32
	mu      sync.Mutex
}

func (s *stubResumer) ResumeFetchPartitions(topicPartitions map[string][]int32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resumed = append(s.resumed, topicPartitions)
}

// failingCommitClient makes every commit fail, simulating a Kafka outage.
type failingCommitClient struct{}

func (failingCommitClient) CommitOffsetsSync(
	_ context.Context,
	_ map[string]map[int32]kgo.EpochOffset,
) error {
	return errors.New("commit failed")
}

// blockingCommitClient blocks the commit until the context is cancelled, then
// returns the cancellation error — simulating a commit in flight during shutdown.
type blockingCommitClient struct {
	entered chan struct{}
}

func (c *blockingCommitClient) CommitOffsetsSync(
	ctx context.Context,
	_ map[string]map[int32]kgo.EpochOffset,
) error {
	close(c.entered)
	<-ctx.Done()
	return ctx.Err()
}

func subAck(mode consumer.AckMode) consumer.Subscription {
	return consumer.Subscription{
		Topic:        "t",
		Handler:      func(ctx context.Context, record *kgo.Record) error { return nil },
		BatchHandler: nil,
		FailurePolicy: consumer.FailurePolicy{
			MaxAttempts:  1,
			RetryBackoff: 0,
			DLQ:          nil,
			OnExhausted:  consumer.ExhaustedActionStop,
		},
		AckMode: mode,
	}
}

func subFailsThenPause() consumer.Subscription {
	return consumer.Subscription{
		Topic: "t",
		Handler: func(ctx context.Context, record *kgo.Record) error {
			return errors.New("test error")
		},
		BatchHandler: nil,
		FailurePolicy: consumer.FailurePolicy{
			MaxAttempts:  1,
			RetryBackoff: 0,
			DLQ:          nil,
			OnExhausted:  consumer.ExhaustedActionStop,
		},
		AckMode: consumer.AckModeAtLeastOnce,
	}
}

func subBatch() consumer.Subscription {
	return consumer.Subscription{
		Topic:   "t",
		Handler: nil,
		BatchHandler: func(
			ctx context.Context,
			records []*kgo.Record,
		) consumer.BatchResult {
			return consumer.BatchResult{}
		},
		FailurePolicy: consumer.FailurePolicy{
			MaxAttempts:  1,
			RetryBackoff: 0,
			DLQ:          nil,
			OnExhausted:  consumer.ExhaustedActionStop,
		},
		AckMode: consumer.AckModeAtLeastOnce,
	}
}

func setupWorker(
	run *consumer.RunState,
	reg *consumer.PartitionRegistry,
	pauses *consumer.PauseRegistry,
	client *stubWorkerClient,
	capacityCh chan struct{},
	resumer *stubResumer,
) *consumer.WorkerRunner {
	committer := consumer.NewCommitter(reg, client, consumer.CommitConfig{})
	executor := consumer.NewRecordExecutor(testlogger.NewLogger())
	return consumer.NewWorkerRunner(
		testlogger.NewLogger(),
		run,
		committer,
		executor,
		reg,
		pauses,
		4,
		capacityCh,
		nil,
		resumer,
	)
}

func startWorker(t *testing.T) (
	*consumer.RunState,
	context.Context,
	*consumer.PartitionRegistry,
	*consumer.PauseRegistry,
	*stubWorkerClient,
) {
	t.Helper()
	run := consumer.NewRunState()
	runCtx, err := run.Begin()
	require.NoError(t, err, "Begin should succeed")
	reg := consumer.NewPartitionRegistry(testlogger.NewLogger())
	pauses := consumer.NewPauseRegistry(time.Now, &stubTopicPauser{})
	client := &stubWorkerClient{}
	return run, runCtx, reg, pauses, client
}

func stopWorker(run *consumer.RunState) {
	run.Stop()
	run.Wait()
}

func getPS(
	t *testing.T,
	reg *consumer.PartitionRegistry,
	sub consumer.Subscription,
	runCtx context.Context,
) *consumer.PartitionState {
	t.Helper()
	ps, _, err := reg.GetOrCreate(runCtx, consumer.Key{Topic: "t", Partition: 0}, sub, 10)
	require.NoError(t, err, "GetOrCreate should succeed")
	return ps
}

// assertEventually polls cond until it returns true or the timeout elapses,
// replacing fixed-sleep assertions so loaded CI machines get a clear failure
// instead of a flaky timing miss.
func assertEventually(t *testing.T, timeout time.Duration, msg string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("condition not met within %v: %s", timeout, msg)
}

func TestWorkerRunner_Start_RunsPartitionLoop(t *testing.T) {
	run, runCtx, reg, pauses, client := startWorker(t)
	defer stopWorker(run)

	capacityCh := make(chan struct{}, 1)
	wr := setupWorker(run, reg, pauses, client, capacityCh, &stubResumer{})

	ps := getPS(t, reg, subAck(consumer.AckModeAtLeastOnce), runCtx)
	wr.Start(ps)

	ps.TryEnqueue([]*kgo.Record{{Topic: "t", Partition: 0, Offset: 1}})

	select {
	case <-capacityCh:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("capacity channel should receive after dequeue")
	}
}

func TestWorkerRunner_AtLeastOnce_MarksDirty(t *testing.T) {
	run, runCtx, reg, pauses, client := startWorker(t)
	defer stopWorker(run)

	wr := setupWorker(run, reg, pauses, client, make(chan struct{}, 1), &stubResumer{})

	ps := getPS(t, reg, subAck(consumer.AckModeAtLeastOnce), runCtx)
	wr.Start(ps)

	ps.TryEnqueue([]*kgo.Record{{Topic: "t", Partition: 0, Offset: 5, LeaderEpoch: 0}})

	assertEventually(t, 2*time.Second, "partition should be dirty after at-least-once processing", func() bool {
		return len(reg.SnapshotDirtyStates()) == 1
	})
}

func TestWorkerRunner_AtMostOnce_CommitsBeforeProcessing(t *testing.T) {
	run, runCtx, reg, pauses, client := startWorker(t)
	defer stopWorker(run)

	wr := setupWorker(run, reg, pauses, client, make(chan struct{}, 1), &stubResumer{})

	ps := getPS(t, reg, subAck(consumer.AckModeAtMostOnce), runCtx)
	wr.Start(ps)

	ps.TryEnqueue([]*kgo.Record{{Topic: "t", Partition: 0, Offset: 5, LeaderEpoch: 0}})

	assertEventually(t, 2*time.Second, "should commit before processing in at-most-once mode", func() bool {
		client.mu.Lock()
		defer client.mu.Unlock()
		return len(client.committed) == 1
	})

	client.mu.Lock()
	require.Equal(t, client.committed[0]["t"][0].Offset, 6, "commit offset must be record offset + 1")
	require.Equal(t, client.committed[0]["t"][0].Epoch, 0, "commit epoch must be the record leader epoch")
	client.mu.Unlock()
}

func TestWorkerRunner_BatchProcessing(t *testing.T) {
	run, runCtx, reg, pauses, client := startWorker(t)
	defer stopWorker(run)

	wr := setupWorker(run, reg, pauses, client, make(chan struct{}, 1), &stubResumer{})

	ps := getPS(t, reg, subBatch(), runCtx)
	wr.Start(ps)

	ps.TryEnqueue([]*kgo.Record{{Topic: "t", Partition: 0, Offset: 5}})

	assertEventually(t, 2*time.Second, "partition should be dirty after batch processing", func() bool {
		return len(reg.SnapshotDirtyStates()) == 1
	})
}

func TestWorkerRunner_BatchAtMostOnce_CommitsBeforeProcessing(t *testing.T) {
	run, runCtx, reg, pauses, client := startWorker(t)
	defer stopWorker(run)

	sub := consumer.Subscription{
		Topic:   "t",
		Handler: nil,
		BatchHandler: func(ctx context.Context, records []*kgo.Record) consumer.BatchResult {
			return consumer.BatchResult{}
		},
		FailurePolicy: consumer.FailurePolicy{
			MaxAttempts:  1,
			RetryBackoff: 0,
			DLQ:          nil,
			OnExhausted:  consumer.ExhaustedActionStop,
		},
		AckMode: consumer.AckModeAtMostOnce,
	}

	wr := setupWorker(run, reg, pauses, client, make(chan struct{}, 1), &stubResumer{})

	ps := getPS(t, reg, sub, runCtx)
	wr.Start(ps)

	ps.TryEnqueue([]*kgo.Record{{Topic: "t", Partition: 0, Offset: 5, LeaderEpoch: 0}})

	assertEventually(t, 2*time.Second, "should commit before processing in at-most-once batch mode", func() bool {
		client.mu.Lock()
		defer client.mu.Unlock()
		return len(client.committed) == 1
	})

	client.mu.Lock()
	require.Equal(t, client.committed[0]["t"][0].Offset, 6, "commit offset must be record offset + 1")
	require.Equal(t, client.committed[0]["t"][0].Epoch, 0, "commit epoch must be the record leader epoch")
	client.mu.Unlock()
}

func TestWorkerRunner_TopicPauseOnFailure(t *testing.T) {
	run, runCtx, reg, pauses, client := startWorker(t)
	defer stopWorker(run)

	wr := setupWorker(run, reg, pauses, client, make(chan struct{}, 1), &stubResumer{})

	ps := getPS(t, reg, subFailsThenPause(), runCtx)
	wr.Start(ps)

	ps.TryEnqueue([]*kgo.Record{{Topic: "t", Partition: 0, Offset: 5}})

	assertEventually(t, 2*time.Second, "topic should be paused after handler exhaustion stop", func() bool {
		return pauses.IsPaused("t")
	})
}

func TestWorkerRunner_SequentialProcessing_AdvancesDirtyOffset(t *testing.T) {
	run, runCtx, reg, pauses, client := startWorker(t)
	defer stopWorker(run)

	// Records flow sequentially through one partition goroutine, so the
	// process semaphore is never contended here; this test pins the dirty
	// offset advance across a multi-record batch (semaphore bounding is
	// covered by TestWorkerRunner_MaxConcurrentZero_ClampsToOne).

	committer := consumer.NewCommitter(reg, client, consumer.CommitConfig{})
	executor := consumer.NewRecordExecutor(testlogger.NewLogger())
	wr := consumer.NewWorkerRunner(
		testlogger.NewLogger(),
		run,
		committer,
		executor,
		reg,
		pauses,
		4,
		make(chan struct{}, 1),
		nil,
		&stubResumer{},
	)

	ps := getPS(t, reg, subAck(consumer.AckModeAtLeastOnce), runCtx)
	wr.Start(ps)

	for i := range 5 {
		ps.TryEnqueue([]*kgo.Record{{Topic: "t", Partition: 0, Offset: int64(i)}})
	}

	assertEventually(t, 2*time.Second, "partition should be dirty", func() bool {
		return len(reg.SnapshotDirtyStates()) == 1
	})

	snap := reg.SnapshotDirtyStates()
	ps2, ok := snap[consumer.Key{Topic: "t", Partition: 0}]
	require.True(t, ok, "partition should be dirty")
	off, hasOff := ps2.SnapshotDirtyOffset()
	assert.True(t, hasOff, "should have dirty offset")
	assert.Equal(t, off.Offset, 5, "should reflect last offset+1")
}

func TestWorkerRunner_DLQFailurePausesTopic_FlushesResolvedPrefix(t *testing.T) {
	run, runCtx, reg, pauses, client := startWorker(t)
	defer stopWorker(run)

	// Handler fails → DLQ exhaustion → DLQ writer also fails. The executor
	// returns PauseTopic:true for a failed DLQ publish, so the worker pauses
	// the topic and keeps the run alive — this is not the fatal-error path
	// (that one is covered by TestWorkerRunner_SingleRecordFatal_FlushesResolvedPrefix).
	var calls atomic.Int32
	sub := consumer.Subscription{
		Topic: "t",
		Handler: func(_ context.Context, _ *kgo.Record) error {
			if calls.Add(1) >= 3 {
				return errors.New("boom")
			}
			return nil
		},
		BatchHandler: nil,
		FailurePolicy: consumer.FailurePolicy{
			MaxAttempts:  1,
			RetryBackoff: 0,
			DLQ:          &consumer.DLQConfig{Topic: "dlq"},
			OnExhausted:  consumer.ExhaustedActionDLQThenCommit,
		},
		AckMode: consumer.AckModeAtLeastOnce,
	}

	ps, _, err := reg.GetOrCreate(runCtx, consumer.Key{Topic: "t", Partition: 0}, sub, 10)
	require.NoError(t, err, "GetOrCreate should succeed")

	committer := consumer.NewCommitter(reg, client, consumer.CommitConfig{})
	executor := consumer.NewRecordExecutor(testlogger.NewLogger())
	failingDLQ := func(_ context.Context, _ *kgo.Record) error { return errors.New("dlq down") }
	wr := consumer.NewWorkerRunner(
		testlogger.NewLogger(),
		run,
		committer,
		executor,
		reg,
		pauses,
		4,
		make(chan struct{}, 1),
		failingDLQ,
		&stubResumer{},
	)

	wr.Start(ps)

	ps.TryEnqueue([]*kgo.Record{
		{Topic: "t", Partition: 0, Offset: 0, LeaderEpoch: 0},
		{Topic: "t", Partition: 0, Offset: 1, LeaderEpoch: 0},
		{Topic: "t", Partition: 0, Offset: 2, LeaderEpoch: 0},
	})

	assertEventually(t, 2*time.Second, "topic must be paused after DLQ publish failure", func() bool {
		return pauses.IsPaused("t")
	})

	// Records 0,1 are flushed at loop exit after the topic pause.
	snap := reg.SnapshotDirtyStates()
	ps2, ok := snap[consumer.Key{Topic: "t", Partition: 0}]
	require.True(t, ok, "partition must be marked dirty after flushing resolved records")
	off, hasOff := ps2.SnapshotDirtyOffset()
	assert.True(t, hasOff, "should have dirty offset")
	assert.Equal(t, off.Offset, 2, "should commit up to offset 2")
	assert.Nil(t, run.Err(), "pause after DLQ failure must not fail the run")
}

func TestWorkerRunner_FlushResolvedBeforeContextCancel(t *testing.T) {
	run, runCtx, reg, pauses, client := startWorker(t)
	defer stopWorker(run)

	record0Started := make(chan struct{})
	record0Done := make(chan struct{})

	sub := consumer.Subscription{
		Topic: "t",
		Handler: func(_ context.Context, _ *kgo.Record) error {
			close(record0Started)
			<-record0Done
			return nil
		},
		BatchHandler: nil,
		FailurePolicy: consumer.FailurePolicy{
			MaxAttempts:  1,
			RetryBackoff: 0,
			DLQ:          nil,
			OnExhausted:  consumer.ExhaustedActionStop,
		},
		AckMode: consumer.AckModeAtLeastOnce,
	}

	ps, _, err := reg.GetOrCreate(runCtx, consumer.Key{Topic: "t", Partition: 0}, sub, 10)
	require.NoError(t, err, "GetOrCreate should succeed")

	committer := consumer.NewCommitter(reg, client, consumer.CommitConfig{})
	executor := consumer.NewRecordExecutor(testlogger.NewLogger())
	wr := consumer.NewWorkerRunner(
		testlogger.NewLogger(),
		run,
		committer,
		executor,
		reg,
		pauses,
		4,
		make(chan struct{}, 1),
		nil,
		&stubResumer{},
	)
	wr.Start(ps)

	ps.TryEnqueue([]*kgo.Record{
		{Topic: "t", Partition: 0, Offset: 0, LeaderEpoch: 0},
		{Topic: "t", Partition: 0, Offset: 1, LeaderEpoch: 0},
	})

	<-record0Started
	ps.BeginClosing()
	close(record0Done)

	assertEventually(t, 2*time.Second, "record 0 must be flushed on context cancel", func() bool {
		off, hasOff := ps.SnapshotDirtyOffset()
		return hasOff && off.Offset == 1
	})
	assert.Nil(t, run.Err(), "context cancel must not fail the run")
}

func TestWorkerRunner_ContextCancelDuringHandler_DoesNotFailRun(t *testing.T) {
	run, runCtx, reg, pauses, client := startWorker(t)
	defer stopWorker(run)

	handlerStarted := make(chan struct{})
	releaseHandler := make(chan struct{})

	sub := consumer.Subscription{
		Topic: "t",
		Handler: func(_ context.Context, _ *kgo.Record) error {
			close(handlerStarted)
			<-releaseHandler
			return errors.New("slow handler eventually fails")
		},
		BatchHandler: nil,
		FailurePolicy: consumer.FailurePolicy{
			MaxAttempts:  3,
			RetryBackoff: 0,
			DLQ:          nil,
			OnExhausted:  consumer.ExhaustedActionStop,
		},
		AckMode: consumer.AckModeAtLeastOnce,
	}

	ps, _, err := reg.GetOrCreate(runCtx, consumer.Key{Topic: "t", Partition: 0}, sub, 10)
	require.NoError(t, err, "GetOrCreate should succeed")

	wr := setupWorker(run, reg, pauses, client, make(chan struct{}, 1), &stubResumer{})
	wr.Start(ps)

	ps.TryEnqueue([]*kgo.Record{{Topic: "t", Partition: 0, Offset: 0, LeaderEpoch: 0}})

	<-handlerStarted
	ps.BeginClosing() // rebalance revoke / shutdown while handler is in flight
	close(releaseHandler)

	select {
	case <-ps.Done():
	case <-time.After(500 * time.Millisecond):
		t.Fatal("partition worker should stop after BeginClosing")
	}

	assert.Nil(t, run.Err(), "partition closing must not fail the run")
}

func TestWorkerRunner_BatchContextCancelDuringHandler_DoesNotFailRun(t *testing.T) {
	run, runCtx, reg, pauses, client := startWorker(t)
	defer stopWorker(run)

	handlerStarted := make(chan struct{})
	releaseHandler := make(chan struct{})

	sub := consumer.Subscription{
		Topic:   "t",
		Handler: nil,
		BatchHandler: func(_ context.Context, _ []*kgo.Record) consumer.BatchResult {
			close(handlerStarted)
			<-releaseHandler
			return consumer.BatchResult{Err: errors.New("slow batch handler eventually fails"), FailedAt: 0}
		},
		FailurePolicy: consumer.FailurePolicy{
			MaxAttempts:  3,
			RetryBackoff: 0,
			DLQ:          nil,
			OnExhausted:  consumer.ExhaustedActionStop,
		},
		AckMode: consumer.AckModeAtLeastOnce,
	}

	ps, _, err := reg.GetOrCreate(runCtx, consumer.Key{Topic: "t", Partition: 0}, sub, 10)
	require.NoError(t, err, "GetOrCreate should succeed")

	wr := setupWorker(run, reg, pauses, client, make(chan struct{}, 1), &stubResumer{})
	wr.Start(ps)

	ps.TryEnqueue([]*kgo.Record{{Topic: "t", Partition: 0, Offset: 0, LeaderEpoch: 0}})

	<-handlerStarted
	ps.BeginClosing() // rebalance revoke / shutdown while handler is in flight
	close(releaseHandler)

	select {
	case <-ps.Done():
	case <-time.After(500 * time.Millisecond):
		t.Fatal("partition worker should stop after BeginClosing")
	}

	assert.Nil(t, run.Err(), "partition closing must not fail the run")
}

func TestWorkerRunner_BatchPause_CommitsResolvedPrefix(t *testing.T) {
	run, runCtx, reg, pauses, client := startWorker(t)
	defer stopWorker(run)

	sub := consumer.Subscription{
		Topic:   "t",
		Handler: nil,
		BatchHandler: func(_ context.Context, _ []*kgo.Record) consumer.BatchResult {
			return consumer.BatchResult{Err: errors.New("batch boom"), FailedAt: 1}
		},
		FailurePolicy: consumer.FailurePolicy{
			MaxAttempts:  1,
			RetryBackoff: 0,
			DLQ:          nil,
			OnExhausted:  consumer.ExhaustedActionStop,
		},
		AckMode: consumer.AckModeAtLeastOnce,
	}

	ps, _, err := reg.GetOrCreate(runCtx, consumer.Key{Topic: "t", Partition: 0}, sub, 10)
	require.NoError(t, err, "GetOrCreate should succeed")

	wr := setupWorker(run, reg, pauses, client, make(chan struct{}, 1), &stubResumer{})
	wr.Start(ps)

	ps.TryEnqueue([]*kgo.Record{
		{Topic: "t", Partition: 0, Offset: 0, LeaderEpoch: 0},
		{Topic: "t", Partition: 0, Offset: 1, LeaderEpoch: 0},
	})

	assertEventually(t, 2*time.Second, "topic should be paused after exhaustion stop", func() bool {
		return pauses.IsPaused("t")
	})
	assert.Nil(t, run.Err(), "pause must not fail the run")

	off, hasOff := ps.SnapshotDirtyOffset()
	require.True(t, hasOff, "resolved prefix must be committed before pausing")
	assert.Equal(t, off.Offset, 1, "should commit up to record 0 (offset 0 + 1)")
}

func TestWorkerRunner_BatchFatal_CommitsResolvedPrefix(t *testing.T) {
	run, runCtx, reg, pauses, client := startWorker(t)
	defer stopWorker(run)

	var calls atomic.Int32
	sub := consumer.Subscription{
		Topic:   "t",
		Handler: nil,
		BatchHandler: func(_ context.Context, _ []*kgo.Record) consumer.BatchResult {
			if calls.Add(1) == 1 {
				return consumer.BatchResult{Err: errors.New("batch boom"), FailedAt: 1}
			}
			return consumer.BatchResult{Err: errors.New("bad index"), FailedAt: 99}
		},
		FailurePolicy: consumer.FailurePolicy{
			MaxAttempts:  2,
			RetryBackoff: 0,
			DLQ:          nil,
			OnExhausted:  consumer.ExhaustedActionStop,
		},
		AckMode: consumer.AckModeAtLeastOnce,
	}

	ps, _, err := reg.GetOrCreate(runCtx, consumer.Key{Topic: "t", Partition: 0}, sub, 10)
	require.NoError(t, err, "GetOrCreate should succeed")

	wr := setupWorker(run, reg, pauses, client, make(chan struct{}, 1), &stubResumer{})
	wr.Start(ps)

	ps.TryEnqueue([]*kgo.Record{
		{Topic: "t", Partition: 0, Offset: 0, LeaderEpoch: 0},
		{Topic: "t", Partition: 0, Offset: 1, LeaderEpoch: 0},
	})

	assertEventually(t, 2*time.Second, "invalid failed index is a fatal error", func() bool {
		return run.Err() != nil
	})
	assert.ErrorContains(t, run.Err(), "failed index", "fatal error should be the invalid index error")

	off, hasOff := ps.SnapshotDirtyOffset()
	require.True(t, hasOff, "resolved prefix must be committed before fatal return")
	assert.Equal(t, off.Offset, 1, "should commit up to record 0 (offset 0 + 1)")
}

func TestWorkerRunner_ResumesFetchAfterBackpressureDrain(t *testing.T) {
	run, runCtx, reg, pauses, client := startWorker(t)
	defer stopWorker(run)

	resumer := &stubResumer{}
	committer := consumer.NewCommitter(reg, client, consumer.CommitConfig{})
	executor := consumer.NewRecordExecutor(testlogger.NewLogger())
	wr := consumer.NewWorkerRunner(
		testlogger.NewLogger(),
		run,
		committer,
		executor,
		reg,
		pauses,
		4,
		make(chan struct{}, 1),
		nil,
		resumer,
	)

	ps, _, err := reg.GetOrCreate(
		runCtx,
		consumer.Key{Topic: "t", Partition: 0},
		subAck(consumer.AckModeAtLeastOnce),
		4,
	)
	require.NoError(t, err, "GetOrCreate should succeed")

	// Fill the queue to the high watermark and apply the backpressure pause
	// (normally done by the dispatcher).
	for i := range 3 {
		ps.TryEnqueue([]*kgo.Record{{Topic: "t", Partition: 0, Offset: int64(i)}})
	}
	require.True(t, ps.TryPauseBackpressure(), "backpressure pause should apply")

	wr.Start(ps)

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		resumer.mu.Lock()
		n := len(resumer.resumed)
		resumer.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	resumer.mu.Lock()
	defer resumer.mu.Unlock()
	require.Equal(t, len(resumer.resumed), 1, "fetch should resume once after backpressure drains")
	assert.Equal(t, resumer.resumed[0], map[string][]int32{"t": {0}}, "resume should target the drained partition")
}

func TestWorkerRunner_NoResumeWhenTopicPaused(t *testing.T) {
	run, runCtx, reg, pauses, client := startWorker(t)
	defer stopWorker(run)

	pauses.Pause("t", errors.New("exhausted"))

	resumer := &stubResumer{}
	committer := consumer.NewCommitter(reg, client, consumer.CommitConfig{})
	executor := consumer.NewRecordExecutor(testlogger.NewLogger())
	wr := consumer.NewWorkerRunner(
		testlogger.NewLogger(),
		run,
		committer,
		executor,
		reg,
		pauses,
		4,
		make(chan struct{}, 1),
		nil,
		resumer,
	)

	ps, _, err := reg.GetOrCreate(
		runCtx,
		consumer.Key{Topic: "t", Partition: 0},
		subAck(consumer.AckModeAtLeastOnce),
		4,
	)
	require.NoError(t, err, "GetOrCreate should succeed")

	for i := range 3 {
		ps.TryEnqueue([]*kgo.Record{{Topic: "t", Partition: 0, Offset: int64(i)}})
	}
	require.True(t, ps.TryPauseBackpressure(), "backpressure pause should apply")

	wr.Start(ps)
	// Settle window for a negative assertion: no resume may ever arrive.
	time.Sleep(200 * time.Millisecond)

	resumer.mu.Lock()
	defer resumer.mu.Unlock()
	assert.Equal(t, len(resumer.resumed), 0, "exhaustion-paused topic must not be resumed")
}

func TestWorkerRunner_PauseMidBatch_SkipsRemainingUncommitted(t *testing.T) {
	run, runCtx, reg, pauses, client := startWorker(t)
	defer stopWorker(run)

	var calls atomic.Int32
	sub := consumer.Subscription{
		Topic: "t",
		Handler: func(_ context.Context, record *kgo.Record) error {
			calls.Add(1)
			if record.Offset == 1 {
				return errors.New("boom")
			}
			return nil
		},
		BatchHandler: nil,
		FailurePolicy: consumer.FailurePolicy{
			MaxAttempts:  1,
			RetryBackoff: 0,
			DLQ:          nil,
			OnExhausted:  consumer.ExhaustedActionStop,
		},
		AckMode: consumer.AckModeAtLeastOnce,
	}

	ps, _, err := reg.GetOrCreate(runCtx, consumer.Key{Topic: "t", Partition: 0}, sub, 10)
	require.NoError(t, err, "GetOrCreate should succeed")

	wr := setupWorker(run, reg, pauses, client, make(chan struct{}, 1), &stubResumer{})
	wr.Start(ps)

	ps.TryEnqueue([]*kgo.Record{
		{Topic: "t", Partition: 0, Offset: 0, LeaderEpoch: 0},
		{Topic: "t", Partition: 0, Offset: 1, LeaderEpoch: 0},
		{Topic: "t", Partition: 0, Offset: 2, LeaderEpoch: 0},
	})

	assertEventually(t, 2*time.Second, "topic should be paused after exhaustion stop", func() bool {
		return pauses.IsPaused("t")
	})
	assert.Equal(t, calls.Load(), 2, "record after the pause point must be skipped, not delivered")
	assert.Nil(t, run.Err(), "pause must not fail the run")

	off, hasOff := ps.SnapshotDirtyOffset()
	require.True(t, hasOff, "resolved prefix must be committed")
	assert.Equal(t, off.Offset, 1, "only record 0 may be committed; the skipped record stays uncommitted")
}

func TestWorkerRunner_AtMostOnceCommitFailure_FailsRun(t *testing.T) {
	run, runCtx, reg, pauses, _ := startWorker(t)
	defer stopWorker(run)

	var handled atomic.Int32
	sub := consumer.Subscription{
		Topic:        "t",
		Handler:      func(_ context.Context, _ *kgo.Record) error { handled.Add(1); return nil },
		BatchHandler: nil,
		FailurePolicy: consumer.FailurePolicy{
			MaxAttempts:  1,
			RetryBackoff: 0,
			DLQ:          nil,
			OnExhausted:  consumer.ExhaustedActionStop,
		},
		AckMode: consumer.AckModeAtMostOnce,
	}

	ps, _, err := reg.GetOrCreate(runCtx, consumer.Key{Topic: "t", Partition: 0}, sub, 10)
	require.NoError(t, err, "GetOrCreate should succeed")

	committer := consumer.NewCommitter(reg, failingCommitClient{}, consumer.CommitConfig{})
	executor := consumer.NewRecordExecutor(testlogger.NewLogger())
	wr := consumer.NewWorkerRunner(
		testlogger.NewLogger(),
		run,
		committer,
		executor,
		reg,
		pauses,
		4,
		make(chan struct{}, 1),
		nil,
		&stubResumer{},
	)
	wr.Start(ps)

	ps.TryEnqueue([]*kgo.Record{{Topic: "t", Partition: 0, Offset: 5, LeaderEpoch: 0}})

	assertEventually(t, 2*time.Second, "commit failure must fail the run", func() bool {
		return run.Err() != nil
	})
	assert.ErrorContains(t, run.Err(), "commit failed", "fatal error should be the commit error")
	assert.Equal(t, handled.Load(), 0, "record must not be processed when its commit fails")
}

func TestWorkerRunner_BatchAtMostOnceCommitFailure_FailsRun(t *testing.T) {
	run, runCtx, reg, pauses, _ := startWorker(t)
	defer stopWorker(run)

	var handled atomic.Int32
	sub := consumer.Subscription{
		Topic:   "t",
		Handler: nil,
		BatchHandler: func(_ context.Context, _ []*kgo.Record) consumer.BatchResult {
			handled.Add(1)
			return consumer.BatchResult{}
		},
		FailurePolicy: consumer.FailurePolicy{
			MaxAttempts:  1,
			RetryBackoff: 0,
			DLQ:          nil,
			OnExhausted:  consumer.ExhaustedActionStop,
		},
		AckMode: consumer.AckModeAtMostOnce,
	}

	ps, _, err := reg.GetOrCreate(runCtx, consumer.Key{Topic: "t", Partition: 0}, sub, 10)
	require.NoError(t, err, "GetOrCreate should succeed")

	committer := consumer.NewCommitter(reg, failingCommitClient{}, consumer.CommitConfig{})
	executor := consumer.NewRecordExecutor(testlogger.NewLogger())
	wr := consumer.NewWorkerRunner(
		testlogger.NewLogger(),
		run,
		committer,
		executor,
		reg,
		pauses,
		4,
		make(chan struct{}, 1),
		nil,
		&stubResumer{},
	)
	wr.Start(ps)

	ps.TryEnqueue([]*kgo.Record{{Topic: "t", Partition: 0, Offset: 5, LeaderEpoch: 0}})

	assertEventually(t, 2*time.Second, "commit failure must fail the run", func() bool {
		return run.Err() != nil
	})
	assert.ErrorContains(t, run.Err(), "commit failed", "fatal error should be the commit error")
	assert.Equal(t, handled.Load(), 0, "batch must not be processed when its commit fails")
}

func TestWorkerRunner_AtMostOnceCommitCancel_DoesNotFailRun(t *testing.T) {
	run, runCtx, reg, pauses, _ := startWorker(t)
	defer stopWorker(run)

	var handled atomic.Int32
	sub := consumer.Subscription{
		Topic:        "t",
		Handler:      func(_ context.Context, _ *kgo.Record) error { handled.Add(1); return nil },
		BatchHandler: nil,
		FailurePolicy: consumer.FailurePolicy{
			MaxAttempts:  1,
			RetryBackoff: 0,
			DLQ:          nil,
			OnExhausted:  consumer.ExhaustedActionStop,
		},
		AckMode: consumer.AckModeAtMostOnce,
	}

	ps, _, err := reg.GetOrCreate(runCtx, consumer.Key{Topic: "t", Partition: 0}, sub, 10)
	require.NoError(t, err, "GetOrCreate should succeed")

	commitClient := &blockingCommitClient{entered: make(chan struct{})}
	committer := consumer.NewCommitter(reg, commitClient, consumer.CommitConfig{})
	executor := consumer.NewRecordExecutor(testlogger.NewLogger())
	wr := consumer.NewWorkerRunner(
		testlogger.NewLogger(),
		run,
		committer,
		executor,
		reg,
		pauses,
		4,
		make(chan struct{}, 1),
		nil,
		&stubResumer{},
	)
	wr.Start(ps)

	ps.TryEnqueue([]*kgo.Record{{Topic: "t", Partition: 0, Offset: 5, LeaderEpoch: 0}})
	<-commitClient.entered // commit is in flight, blocked on the context
	ps.BeginClosing()      // shutdown / revoke cancels the partition context

	select {
	case <-ps.Done():
	case <-time.After(500 * time.Millisecond):
		t.Fatal("partition worker should stop after BeginClosing")
	}

	assert.Nil(t, run.Err(), "commit cancelled by closing must not fail the run")
	assert.Equal(t, handled.Load(), 0, "record must not be processed when its commit is cancelled")
}

func TestWorkerRunner_BatchPausedAtEntry_DoesNotProcess(t *testing.T) {
	run, runCtx, reg, pauses, client := startWorker(t)
	defer stopWorker(run)

	pauses.Pause("t", errors.New("exhausted"))

	var calls atomic.Int32
	sub := consumer.Subscription{
		Topic:   "t",
		Handler: nil,
		BatchHandler: func(_ context.Context, _ []*kgo.Record) consumer.BatchResult {
			calls.Add(1)
			return consumer.BatchResult{}
		},
		FailurePolicy: consumer.FailurePolicy{
			MaxAttempts:  1,
			RetryBackoff: 0,
			DLQ:          nil,
			OnExhausted:  consumer.ExhaustedActionStop,
		},
		AckMode: consumer.AckModeAtLeastOnce,
	}

	ps, _, err := reg.GetOrCreate(runCtx, consumer.Key{Topic: "t", Partition: 0}, sub, 10)
	require.NoError(t, err, "GetOrCreate should succeed")

	wr := setupWorker(run, reg, pauses, client, make(chan struct{}, 1), &stubResumer{})
	wr.Start(ps)

	ps.TryEnqueue([]*kgo.Record{{Topic: "t", Partition: 0, Offset: 5, LeaderEpoch: 0}})
	// Settle window for negative assertions: nothing may be processed.
	time.Sleep(200 * time.Millisecond)

	assert.Equal(t, calls.Load(), 0, "paused batch must not be processed")
	assert.Nil(t, run.Err(), "skipping a paused batch must not fail the run")
	snap := reg.SnapshotDirtyStates()
	assert.Equal(t, len(snap), 0, "paused batch must not mark the partition dirty")
}

func TestWorkerRunner_BatchSlotAcquireCancel_StopsCleanly(t *testing.T) {
	run, runCtx, reg, pauses, client := startWorker(t)
	defer stopWorker(run)

	aStarted := make(chan struct{})
	aRelease := make(chan struct{})
	// Release the blocked handler on every path (including t.Fatal) so the
	// deferred stopWorker/run.Wait never deadlocks on partition A's goroutine.
	defer close(aRelease)
	subA := consumer.Subscription{
		Topic: "t",
		Handler: func(_ context.Context, _ *kgo.Record) error {
			close(aStarted)
			<-aRelease
			return nil
		},
		BatchHandler: nil,
		FailurePolicy: consumer.FailurePolicy{
			MaxAttempts:  1,
			RetryBackoff: 0,
			DLQ:          nil,
			OnExhausted:  consumer.ExhaustedActionStop,
		},
		AckMode: consumer.AckModeAtLeastOnce,
	}

	var batchCalls atomic.Int32
	subB := consumer.Subscription{
		Topic:   "t",
		Handler: nil,
		BatchHandler: func(_ context.Context, _ []*kgo.Record) consumer.BatchResult {
			batchCalls.Add(1)
			return consumer.BatchResult{}
		},
		FailurePolicy: consumer.FailurePolicy{
			MaxAttempts:  1,
			RetryBackoff: 0,
			DLQ:          nil,
			OnExhausted:  consumer.ExhaustedActionStop,
		},
		AckMode: consumer.AckModeAtLeastOnce,
	}

	psA, _, err := reg.GetOrCreate(runCtx, consumer.Key{Topic: "t", Partition: 0}, subA, 10)
	require.NoError(t, err, "GetOrCreate should succeed")
	psB, _, err := reg.GetOrCreate(runCtx, consumer.Key{Topic: "t", Partition: 1}, subB, 10)
	require.NoError(t, err, "GetOrCreate should succeed")

	committer := consumer.NewCommitter(reg, client, consumer.CommitConfig{})
	executor := consumer.NewRecordExecutor(testlogger.NewLogger())
	wr := consumer.NewWorkerRunner(
		testlogger.NewLogger(),
		run,
		committer,
		executor,
		reg,
		pauses,
		1,
		make(chan struct{}, 1),
		nil,
		&stubResumer{},
	)
	wr.Start(psA)
	wr.Start(psB)

	psA.TryEnqueue([]*kgo.Record{{Topic: "t", Partition: 0, Offset: 0, LeaderEpoch: 0}})
	<-aStarted // partition A holds the only process slot

	psB.TryEnqueue([]*kgo.Record{{Topic: "t", Partition: 1, Offset: 0, LeaderEpoch: 0}})
	psB.BeginClosing() // revoke while B waits for the slot

	select {
	case <-psB.Done():
	case <-time.After(500 * time.Millisecond):
		t.Fatal("partition B worker should stop after BeginClosing")
	}

	assert.Equal(t, batchCalls.Load(), 0, "batch must not run without a process slot")
	assert.Nil(t, run.Err(), "slot acquire cancelled by closing must not fail the run")
}

func TestWorkerRunner_MaxConcurrentZero_ClampsToOne(t *testing.T) {
	run, runCtx, reg, pauses, client := startWorker(t)
	defer stopWorker(run)

	var running, maxRunning atomic.Int32
	handler := func(_ context.Context, _ *kgo.Record) error {
		n := running.Add(1)
		defer running.Add(-1)
		for {
			cur := maxRunning.Load()
			if n <= cur || maxRunning.CompareAndSwap(cur, n) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		return nil
	}
	sub := consumer.Subscription{
		Topic:        "t",
		Handler:      handler,
		BatchHandler: nil,
		FailurePolicy: consumer.FailurePolicy{
			MaxAttempts:  1,
			RetryBackoff: 0,
			DLQ:          nil,
			OnExhausted:  consumer.ExhaustedActionStop,
		},
		AckMode: consumer.AckModeAtLeastOnce,
	}

	psA, _, err := reg.GetOrCreate(runCtx, consumer.Key{Topic: "t", Partition: 0}, sub, 10)
	require.NoError(t, err, "GetOrCreate should succeed")
	psB, _, err := reg.GetOrCreate(runCtx, consumer.Key{Topic: "t", Partition: 1}, sub, 10)
	require.NoError(t, err, "GetOrCreate should succeed")

	committer := consumer.NewCommitter(reg, client, consumer.CommitConfig{})
	executor := consumer.NewRecordExecutor(testlogger.NewLogger())
	wr := consumer.NewWorkerRunner(
		testlogger.NewLogger(),
		run,
		committer,
		executor,
		reg,
		pauses,
		0,
		make(chan struct{}, 1),
		nil,
		&stubResumer{},
	)
	wr.Start(psA)
	wr.Start(psB)

	psA.TryEnqueue([]*kgo.Record{{Topic: "t", Partition: 0, Offset: 0, LeaderEpoch: 0}})
	psB.TryEnqueue([]*kgo.Record{{Topic: "t", Partition: 1, Offset: 0, LeaderEpoch: 0}})

	assertEventually(t, 2*time.Second, "handler should start", func() bool {
		return maxRunning.Load() >= 1
	})
	assertEventually(t, 2*time.Second, "handlers should finish", func() bool {
		return running.Load() == 0
	})
	assert.Equal(t, maxRunning.Load(), 1, "maxConcurrent 0 should clamp to a single process slot")
}

func TestWorkerRunner_SingleRecordFatal_FlushesResolvedPrefix(t *testing.T) {
	run, runCtx, reg, pauses, client := startWorker(t)
	defer stopWorker(run)

	// An unsupported OnExhausted value makes the executor return a fatal
	// (non-pause) Cause — the only way to reach the single-record fatal path
	// in the worker, since every other exhausted action either pauses the
	// topic or implies a cancelled context.
	sub := consumer.Subscription{
		Topic: "t",
		Handler: func(_ context.Context, record *kgo.Record) error {
			if record.Offset == 2 {
				return errors.New("boom")
			}
			return nil
		},
		BatchHandler: nil,
		FailurePolicy: consumer.FailurePolicy{
			MaxAttempts:  1,
			RetryBackoff: 0,
			DLQ:          nil,
			OnExhausted:  consumer.ExhaustedAction(127),
		},
		AckMode: consumer.AckModeAtLeastOnce,
	}

	ps, _, err := reg.GetOrCreate(runCtx, consumer.Key{Topic: "t", Partition: 0}, sub, 10)
	require.NoError(t, err, "GetOrCreate should succeed")

	wr := setupWorker(run, reg, pauses, client, make(chan struct{}, 1), &stubResumer{})
	wr.Start(ps)

	ps.TryEnqueue([]*kgo.Record{
		{Topic: "t", Partition: 0, Offset: 0, LeaderEpoch: 0},
		{Topic: "t", Partition: 0, Offset: 1, LeaderEpoch: 0},
		{Topic: "t", Partition: 0, Offset: 2, LeaderEpoch: 0},
	})

	assertEventually(t, 2*time.Second, "unsupported exhausted action is a fatal error", func() bool {
		return run.Err() != nil
	})
	assert.ErrorContains(t, run.Err(), "unsupported exhausted action", "fatal error should be the executor error")
	assert.False(t, pauses.IsPaused("t"), "fatal error must not pause the topic")

	// Records 0,1 must be flushed before the fatal return.
	off, hasOff := ps.SnapshotDirtyOffset()
	require.True(t, hasOff, "resolved prefix must be committed before fatal return")
	assert.Equal(t, off.Offset, 2, "should commit up to record 1 (offset 1 + 1)")
}

func TestWorkerRunner_BatchAtMostOnceCommitCancel_DoesNotFailRun(t *testing.T) {
	run, runCtx, reg, pauses, _ := startWorker(t)
	defer stopWorker(run)

	var handled atomic.Int32
	sub := consumer.Subscription{
		Topic:   "t",
		Handler: nil,
		BatchHandler: func(_ context.Context, _ []*kgo.Record) consumer.BatchResult {
			handled.Add(1)
			return consumer.BatchResult{}
		},
		FailurePolicy: consumer.FailurePolicy{
			MaxAttempts:  1,
			RetryBackoff: 0,
			DLQ:          nil,
			OnExhausted:  consumer.ExhaustedActionStop,
		},
		AckMode: consumer.AckModeAtMostOnce,
	}

	ps, _, err := reg.GetOrCreate(runCtx, consumer.Key{Topic: "t", Partition: 0}, sub, 10)
	require.NoError(t, err, "GetOrCreate should succeed")

	commitClient := &blockingCommitClient{entered: make(chan struct{})}
	committer := consumer.NewCommitter(reg, commitClient, consumer.CommitConfig{})
	executor := consumer.NewRecordExecutor(testlogger.NewLogger())
	wr := consumer.NewWorkerRunner(
		testlogger.NewLogger(),
		run,
		committer,
		executor,
		reg,
		pauses,
		4,
		make(chan struct{}, 1),
		nil,
		&stubResumer{},
	)
	wr.Start(ps)

	ps.TryEnqueue([]*kgo.Record{{Topic: "t", Partition: 0, Offset: 5, LeaderEpoch: 0}})
	<-commitClient.entered // commit is in flight, blocked on the context
	ps.BeginClosing()      // shutdown / revoke cancels the partition context

	select {
	case <-ps.Done():
	case <-time.After(500 * time.Millisecond):
		t.Fatal("partition worker should stop after BeginClosing")
	}

	assert.Nil(t, run.Err(), "commit cancelled by closing must not fail the run")
	assert.Equal(t, handled.Load(), 0, "batch must not be processed when its commit is cancelled")
}

func TestWorkerRunner_CapacitySignalNonBlockingWhenFull(t *testing.T) {
	run, runCtx, reg, pauses, client := startWorker(t)
	defer stopWorker(run)

	capacityCh := make(chan struct{}, 1)
	capacityCh <- struct{}{} // pre-fill: a token is already pending for the dispatcher

	wr := setupWorker(run, reg, pauses, client, capacityCh, &stubResumer{})

	ps := getPS(t, reg, subAck(consumer.AckModeAtLeastOnce), runCtx)
	wr.Start(ps)

	ps.TryEnqueue([]*kgo.Record{{Topic: "t", Partition: 0, Offset: 5, LeaderEpoch: 0}})

	assertEventually(t, 2*time.Second, "record must still be processed while the capacity signal is dropped", func() bool {
		return len(reg.SnapshotDirtyStates()) == 1
	})
	assert.Equal(t, len(capacityCh), 1, "full capacity channel must not be pushed into again")
}
