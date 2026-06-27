// services/go/library/kafka/internal/consumer/worker_runner_test.go
package consumer_test

import (
	"context"
	"errors"
	"maps"
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
	resumedPartitions map[string][]int32
	committed         []map[string]map[int32]kgo.EpochOffset
	pausedTopics      []string
	mu                sync.Mutex
}

func (s *stubWorkerClient) CommitOffsetsSync(ctx context.Context, offsets map[string]map[int32]kgo.EpochOffset) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.committed = append(s.committed, offsets)
	return nil
}

func (s *stubWorkerClient) PauseFetchTopics(topics ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pausedTopics = append(s.pausedTopics, topics...)
}

func (s *stubWorkerClient) ResumeFetchPartitions(partitions map[string][]int32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.resumedPartitions == nil {
		s.resumedPartitions = make(map[string][]int32)
	}
	maps.Copy(s.resumedPartitions, partitions)
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

func setupWorker(run *consumer.RunState, reg *consumer.PartitionRegistry, pauses *consumer.PauseRegistry, client *stubWorkerClient, notify func()) *consumer.WorkerRunner {
	committer := consumer.NewCommitter(testlogger.NewLogger(), reg, pauses, client, consumer.CommitConfig{})
	executor := consumer.NewRecordExecutor(testlogger.NewLogger())
	return consumer.NewWorkerRunner(testlogger.NewLogger(), run, committer, executor, reg, pauses, 4, notify, nil)
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

	var notified atomic.Int32
	wr := setupWorker(run, reg, pauses, client, func() { notified.Add(1) })

	ps := getPS(t, reg, subAck(consumer.AckModeAtLeastOnce), runCtx)
	wr.Start(ps, client)

	time.Sleep(10 * time.Millisecond)
	ps.TryEnqueue([]*kgo.Record{{Topic: "t", Partition: 0, Offset: 1}})
	time.Sleep(50 * time.Millisecond)

	assert.True(t, notified.Load() > 0, "should have notified capacity at least once")
}

func TestWorkerRunner_SetNotifyCapacity(t *testing.T) {
	run, runCtx, reg, pauses, client := startWorker(t)
	defer stopWorker(run)

	wr := setupWorker(run, reg, pauses, client, func() {})

	var called atomic.Bool
	wr.SetNotifyCapacity(func() { called.Store(true) })

	ps := getPS(t, reg, subAck(consumer.AckModeAtLeastOnce), runCtx)
	wr.Start(ps, client)

	time.Sleep(10 * time.Millisecond)
	ps.TryEnqueue([]*kgo.Record{{Topic: "t", Partition: 0, Offset: 1}})
	time.Sleep(50 * time.Millisecond)

	assert.True(t, called.Load(), "SetNotifyCapacity should be effective")
}

func TestWorkerRunner_AtLeastOnce_MarksDirty(t *testing.T) {
	run, runCtx, reg, pauses, client := startWorker(t)
	defer stopWorker(run)

	wr := setupWorker(run, reg, pauses, client, func() {})

	ps := getPS(t, reg, subAck(consumer.AckModeAtLeastOnce), runCtx)
	wr.Start(ps, client)

	time.Sleep(10 * time.Millisecond)
	ps.TryEnqueue([]*kgo.Record{{Topic: "t", Partition: 0, Offset: 5, LeaderEpoch: 0}})
	time.Sleep(100 * time.Millisecond)

	snap := reg.SnapshotDirtyStates()
	assert.Equal(t, 1, len(snap), "partition should be dirty after at-least-once processing")
}

func TestWorkerRunner_AtMostOnce_CommitsBeforeProcessing(t *testing.T) {
	run, runCtx, reg, pauses, client := startWorker(t)
	defer stopWorker(run)

	wr := setupWorker(run, reg, pauses, client, func() {})

	ps := getPS(t, reg, subAck(consumer.AckModeAtMostOnce), runCtx)
	wr.Start(ps, client)

	time.Sleep(10 * time.Millisecond)
	ps.TryEnqueue([]*kgo.Record{{Topic: "t", Partition: 0, Offset: 5, LeaderEpoch: 0}})
	time.Sleep(100 * time.Millisecond)

	client.mu.Lock()
	assert.Equal(t, 1, len(client.committed), "should commit before processing in at-most-once mode")
	client.mu.Unlock()
}

func TestWorkerRunner_BatchProcessing(t *testing.T) {
	run, runCtx, reg, pauses, client := startWorker(t)
	defer stopWorker(run)

	wr := setupWorker(run, reg, pauses, client, func() {})

	ps := getPS(t, reg, subBatch(), runCtx)
	wr.Start(ps, client)

	time.Sleep(10 * time.Millisecond)
	ps.TryEnqueue([]*kgo.Record{{Topic: "t", Partition: 0, Offset: 5}})
	time.Sleep(100 * time.Millisecond)

	snap := reg.SnapshotDirtyStates()
	assert.Equal(t, 1, len(snap), "partition should be dirty after batch processing")
}

func TestWorkerRunner_TopicPauseOnFailure(t *testing.T) {
	run, runCtx, reg, pauses, client := startWorker(t)
	defer stopWorker(run)

	wr := setupWorker(run, reg, pauses, client, func() {})

	ps := getPS(t, reg, subFailsThenPause(), runCtx)
	wr.Start(ps, client)

	time.Sleep(10 * time.Millisecond)
	ps.TryEnqueue([]*kgo.Record{{Topic: "t", Partition: 0, Offset: 5}})
	time.Sleep(200 * time.Millisecond)

	assert.True(t, pauses.IsPaused("t"), "topic should be paused after handler exhaustion stop")

	client.mu.Lock()
	found := false
	for _, topic := range client.pausedTopics {
		if topic == "t" {
			found = true
		}
	}
	assert.True(t, found, "PauseFetchTopics should have been called for topic t")
	client.mu.Unlock()
}

func TestWorkerRunner_ProcessSemaphore_BoundsConcurrency(t *testing.T) {
	run, runCtx, reg, pauses, client := startWorker(t)
	defer stopWorker(run)

	committer := consumer.NewCommitter(testlogger.NewLogger(), reg, pauses, client, consumer.CommitConfig{})
	executor := consumer.NewRecordExecutor(testlogger.NewLogger())
	wr := consumer.NewWorkerRunner(testlogger.NewLogger(), run, committer, executor, reg, pauses, 1, func() {}, nil)

	ps := getPS(t, reg, subAck(consumer.AckModeAtLeastOnce), runCtx)
	wr.Start(ps, client)

	time.Sleep(10 * time.Millisecond)
	for i := range 5 {
		ps.TryEnqueue([]*kgo.Record{{Topic: "t", Partition: 0, Offset: int64(i)}})
	}
	time.Sleep(200 * time.Millisecond)

	snap := reg.SnapshotDirtyStates()
	assert.Equal(t, 1, len(snap), "partition should be dirty")
	if ps, ok := snap[consumer.Key{Topic: "t", Partition: 0}]; ok {
		off, hasOff := ps.SnapshotDirtyOffset()
		assert.True(t, hasOff, "should have dirty offset")
		assert.Equal(t, int64(5), off.Offset, "should reflect last offset+1")
	}
}
