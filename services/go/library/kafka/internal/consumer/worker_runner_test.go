// services/go/library/kafka/internal/consumer/worker_runner_test.go
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

func (s *stubWorkerClient) CommitOffsetsSync(_ context.Context, offsets map[string]map[int32]kgo.EpochOffset) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.committed = append(s.committed, offsets)
	return nil
}

func subAck(mode consumer.AckMode) consumer.Subscription {
	return consumer.Subscription{
		Topic:         "t",
		Handler:       func(ctx context.Context, record *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: consumer.FailurePolicy{MaxAttempts: 1, RetryBackoff: 0, DLQ: nil, OnExhausted: consumer.ExhaustedActionStop},
		AckMode:       mode,
	}
}

func subFailsThenPause() consumer.Subscription {
	return consumer.Subscription{
		Topic:         "t",
		Handler:       func(ctx context.Context, record *kgo.Record) error { return errors.New("test error") },
		BatchHandler:  nil,
		FailurePolicy: consumer.FailurePolicy{MaxAttempts: 1, RetryBackoff: 0, DLQ: nil, OnExhausted: consumer.ExhaustedActionStop},
		AckMode:       consumer.AckModeAtLeastOnce,
	}
}

func subBatch() consumer.Subscription {
	return consumer.Subscription{
		Topic:         "t",
		Handler:       nil,
		BatchHandler:  func(ctx context.Context, records []*kgo.Record) consumer.BatchResult { return consumer.BatchResult{} },
		FailurePolicy: consumer.FailurePolicy{MaxAttempts: 1, RetryBackoff: 0, DLQ: nil, OnExhausted: consumer.ExhaustedActionStop},
		AckMode:       consumer.AckModeAtLeastOnce,
	}
}

func setupWorker(run *consumer.RunState, reg *consumer.PartitionRegistry, pauses *consumer.PauseRegistry, client *stubWorkerClient, capacityCh chan struct{}, kgoClient *kgo.Client) *consumer.WorkerRunner {
	committer := consumer.NewCommitter(reg, client, consumer.CommitConfig{})
	executor := consumer.NewRecordExecutor(testlogger.NewLogger())
	return consumer.NewWorkerRunner(testlogger.NewLogger(), run, committer, executor, reg, pauses, 4, capacityCh, nil, kgoClient)
}

func startWorker(t *testing.T) (*consumer.RunState, context.Context, *consumer.PartitionRegistry, *consumer.PauseRegistry, *stubWorkerClient) {
	t.Helper()
	run := consumer.NewRunState()
	runCtx, err := run.Begin()
	require.NoError(t, err, "Begin should succeed")
	reg := consumer.NewPartitionRegistry(testlogger.NewLogger())
	pauses := consumer.NewPauseRegistry(time.Now)
	client := &stubWorkerClient{}
	return run, runCtx, reg, pauses, client
}

func stopWorker(run *consumer.RunState) {
	run.Stop()
	run.Wait()
}

func getPS(t *testing.T, reg *consumer.PartitionRegistry, sub consumer.Subscription, runCtx context.Context) *consumer.PartitionState {
	t.Helper()
	ps, _, err := reg.GetOrCreate(consumer.Key{Topic: "t", Partition: 0}, sub, runCtx, 10)
	require.NoError(t, err, "GetOrCreate should succeed")
	return ps
}

func TestWorkerRunner_Start_RunsPartitionLoop(t *testing.T) {
	run, runCtx, reg, pauses, client := startWorker(t)
	defer stopWorker(run)

	capacityCh := make(chan struct{}, 1)
	wr := setupWorker(run, reg, pauses, client, capacityCh, newTestKgoClient(t))

	ps := getPS(t, reg, subAck(consumer.AckModeAtLeastOnce), runCtx)
	wr.Start(ps)

	time.Sleep(10 * time.Millisecond)
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

	wr := setupWorker(run, reg, pauses, client, make(chan struct{}, 1), newTestKgoClient(t))

	ps := getPS(t, reg, subAck(consumer.AckModeAtLeastOnce), runCtx)
	wr.Start(ps)

	time.Sleep(10 * time.Millisecond)
	ps.TryEnqueue([]*kgo.Record{{Topic: "t", Partition: 0, Offset: 5, LeaderEpoch: 0}})
	time.Sleep(100 * time.Millisecond)

	snap := reg.SnapshotDirtyStates()
	assert.Equal(t, len(snap), 1, "partition should be dirty after at-least-once processing")
}

func TestWorkerRunner_AtMostOnce_CommitsBeforeProcessing(t *testing.T) {
	run, runCtx, reg, pauses, client := startWorker(t)
	defer stopWorker(run)

	wr := setupWorker(run, reg, pauses, client, make(chan struct{}, 1), newTestKgoClient(t))

	ps := getPS(t, reg, subAck(consumer.AckModeAtMostOnce), runCtx)
	wr.Start(ps)

	time.Sleep(10 * time.Millisecond)
	ps.TryEnqueue([]*kgo.Record{{Topic: "t", Partition: 0, Offset: 5, LeaderEpoch: 0}})
	time.Sleep(100 * time.Millisecond)

	client.mu.Lock()
	assert.Equal(t, len(client.committed), 1, "should commit before processing in at-most-once mode")
	client.mu.Unlock()
}

func TestWorkerRunner_BatchProcessing(t *testing.T) {
	run, runCtx, reg, pauses, client := startWorker(t)
	defer stopWorker(run)

	wr := setupWorker(run, reg, pauses, client, make(chan struct{}, 1), newTestKgoClient(t))

	ps := getPS(t, reg, subBatch(), runCtx)
	wr.Start(ps)

	time.Sleep(10 * time.Millisecond)
	ps.TryEnqueue([]*kgo.Record{{Topic: "t", Partition: 0, Offset: 5}})
	time.Sleep(100 * time.Millisecond)

	snap := reg.SnapshotDirtyStates()
	assert.Equal(t, len(snap), 1, "partition should be dirty after batch processing")
}

func TestWorkerRunner_BatchAtMostOnce_CommitsBeforeProcessing(t *testing.T) {
	run, runCtx, reg, pauses, client := startWorker(t)
	defer stopWorker(run)

	sub := consumer.Subscription{
		Topic:         "t",
		Handler:       nil,
		BatchHandler:  func(ctx context.Context, records []*kgo.Record) consumer.BatchResult { return consumer.BatchResult{} },
		FailurePolicy: consumer.FailurePolicy{MaxAttempts: 1, RetryBackoff: 0, DLQ: nil, OnExhausted: consumer.ExhaustedActionStop},
		AckMode:       consumer.AckModeAtMostOnce,
	}

	wr := setupWorker(run, reg, pauses, client, make(chan struct{}, 1), newTestKgoClient(t))

	ps := getPS(t, reg, sub, runCtx)
	wr.Start(ps)

	time.Sleep(10 * time.Millisecond)
	ps.TryEnqueue([]*kgo.Record{{Topic: "t", Partition: 0, Offset: 5, LeaderEpoch: 0}})
	time.Sleep(100 * time.Millisecond)

	client.mu.Lock()
	assert.Equal(t, len(client.committed), 1, "should commit before processing in at-most-once batch mode")
	client.mu.Unlock()
}

func TestWorkerRunner_TopicPauseOnFailure(t *testing.T) {
	run, runCtx, reg, pauses, client := startWorker(t)
	defer stopWorker(run)

	wr := setupWorker(run, reg, pauses, client, make(chan struct{}, 1), newTestKgoClient(t))

	ps := getPS(t, reg, subFailsThenPause(), runCtx)
	wr.Start(ps)

	time.Sleep(10 * time.Millisecond)
	ps.TryEnqueue([]*kgo.Record{{Topic: "t", Partition: 0, Offset: 5}})
	time.Sleep(200 * time.Millisecond)

	assert.True(t, pauses.IsPaused("t"), "topic should be paused after handler exhaustion stop")
}

func TestWorkerRunner_ProcessSemaphore_BoundsConcurrency(t *testing.T) {
	run, runCtx, reg, pauses, client := startWorker(t)
	defer stopWorker(run)

	committer := consumer.NewCommitter(reg, client, consumer.CommitConfig{})
	executor := consumer.NewRecordExecutor(testlogger.NewLogger())
	wr := consumer.NewWorkerRunner(testlogger.NewLogger(), run, committer, executor, reg, pauses, 4, make(chan struct{}, 1), nil, newTestKgoClient(t))

	ps := getPS(t, reg, subAck(consumer.AckModeAtLeastOnce), runCtx)
	wr.Start(ps)

	time.Sleep(10 * time.Millisecond)
	for i := range 5 {
		ps.TryEnqueue([]*kgo.Record{{Topic: "t", Partition: 0, Offset: int64(i)}})
	}
	time.Sleep(200 * time.Millisecond)

	snap := reg.SnapshotDirtyStates()
	assert.Equal(t, len(snap), 1, "partition should be dirty")
	if ps, ok := snap[consumer.Key{Topic: "t", Partition: 0}]; ok {
		off, hasOff := ps.SnapshotDirtyOffset()
		assert.True(t, hasOff, "should have dirty offset")
		assert.Equal(t, off.Offset, 5, "should reflect last offset+1")
	}
}

func TestWorkerRunner_FlushResolvedBeforeFatalError(t *testing.T) {
	run, runCtx, reg, pauses, client := startWorker(t)
	defer stopWorker(run)

	// Handler fails → DLQ exhaustion → DLQ writer also fails → result.Cause != nil.
	var calls atomic.Int32
	sub := consumer.Subscription{
		Topic: "t",
		Handler: func(_ context.Context, _ *kgo.Record) error {
			if calls.Add(1) >= 3 {
				return errors.New("fatal")
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

	ps, _, err := reg.GetOrCreate(consumer.Key{Topic: "t", Partition: 0}, sub, runCtx, 10)
	require.NoError(t, err, "GetOrCreate should succeed")

	committer := consumer.NewCommitter(reg, client, consumer.CommitConfig{})
	executor := consumer.NewRecordExecutor(testlogger.NewLogger())
	failingDLQ := func(_ context.Context, _ *kgo.Record) error { return errors.New("dlq down") }
	wr := consumer.NewWorkerRunner(testlogger.NewLogger(), run, committer, executor, reg, pauses, 4, make(chan struct{}, 1), failingDLQ, newTestKgoClient(t))

	wr.Start(ps)
	time.Sleep(10 * time.Millisecond)

	ps.TryEnqueue([]*kgo.Record{
		{Topic: "t", Partition: 0, Offset: 0, LeaderEpoch: 0},
		{Topic: "t", Partition: 0, Offset: 1, LeaderEpoch: 0},
		{Topic: "t", Partition: 0, Offset: 2, LeaderEpoch: 0},
	})
	time.Sleep(200 * time.Millisecond)

	// Records 0,1 must be flushed before the error return.
	snap := reg.SnapshotDirtyStates()
	ps2, ok := snap[consumer.Key{Topic: "t", Partition: 0}]
	require.True(t, ok, "partition must be marked dirty after flushing resolved records")
	off, hasOff := ps2.SnapshotDirtyOffset()
	assert.True(t, hasOff, "should have dirty offset")
	assert.Equal(t, off.Offset, 2, "should commit up to offset 2")
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
		BatchHandler:  nil,
		FailurePolicy: consumer.FailurePolicy{MaxAttempts: 1, RetryBackoff: 0, DLQ: nil, OnExhausted: consumer.ExhaustedActionStop},
		AckMode:       consumer.AckModeAtLeastOnce,
	}

	ps, _, err := reg.GetOrCreate(consumer.Key{Topic: "t", Partition: 0}, sub, runCtx, 10)
	require.NoError(t, err, "GetOrCreate should succeed")

	committer := consumer.NewCommitter(reg, client, consumer.CommitConfig{})
	executor := consumer.NewRecordExecutor(testlogger.NewLogger())
	wr := consumer.NewWorkerRunner(testlogger.NewLogger(), run, committer, executor, reg, pauses, 4, make(chan struct{}, 1), nil, newTestKgoClient(t))
	wr.Start(ps)
	time.Sleep(10 * time.Millisecond)

	ps.TryEnqueue([]*kgo.Record{
		{Topic: "t", Partition: 0, Offset: 0, LeaderEpoch: 0},
		{Topic: "t", Partition: 0, Offset: 1, LeaderEpoch: 0},
	})

	<-record0Started
	ps.BeginClosing()
	close(record0Done)

	time.Sleep(200 * time.Millisecond)

	off, hasOff := ps.SnapshotDirtyOffset()
	require.True(t, hasOff, "record 0 must be flushed on context cancel")
	assert.Equal(t, off.Offset, 1, "should advance to offset 1")
}
