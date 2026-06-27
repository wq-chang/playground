// services/go/library/kafka/internal/consumer/committer_test.go
package consumer_test

import (
	"go-services/library/testlogger"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go-services/library/assert"
	"go-services/library/kafka/internal/consumer"
	"go-services/library/require"

	"github.com/twmb/franz-go/pkg/kgo"
)

// stubOffsetClient implements consumer.OffsetClient for testing.
type stubOffsetClient struct {
	commits []map[string]map[int32]kgo.EpochOffset
	mu      sync.Mutex
	fail    bool
}

func (s *stubOffsetClient) CommitOffsetsSync(ctx context.Context, offsets map[string]map[int32]kgo.EpochOffset) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return errors.New("commit failed")
	}
	s.commits = append(s.commits, offsets)
	return nil
}

func newTestCommitter(t *testing.T, reg *consumer.PartitionRegistry, pauses *consumer.PauseRegistry, client consumer.OffsetClient) *consumer.Committer {
	t.Helper()
	return consumer.NewCommitter(testlogger.NewLogger(), reg, pauses, client, consumer.CommitConfig{
		FlushInterval:      50 * time.Millisecond,
		DebounceInterval:   10 * time.Millisecond,
		DrainTimeout:       100 * time.Millisecond,
		FinalCommitTimeout: 100 * time.Millisecond,
	})
}

func TestCommitter_RequestFlush_TriggersFlush(t *testing.T) {
	reg := consumer.NewPartitionRegistry(testlogger.NewLogger())
	pauses := consumer.NewPauseRegistry(time.Now)
	client := &stubOffsetClient{}
	cm := newTestCommitter(t, reg, pauses, client)

	ps, _, errGC := reg.GetOrCreate(consumer.Key{Topic: "t", Partition: 0}, testSub("t"), context.Background(), 10)
	require.NoError(t, errGC, "GetOrCreate should succeed")
	ps.AdvanceCommitOffset(&kgo.Record{Topic: "t", Offset: 5, LeaderEpoch: 0})
	reg.MarkDirty(ps)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		err := cm.Run(ctx) // expected to exit when ctx is cancelled
		require.NoError(t, err, "Run should exit without error")
		close(done)
	}()

	cm.RequestFlush()
	time.Sleep(100 * time.Millisecond)

	client.mu.Lock()
	assert.Equal(t, 1, len(client.commits), "should have committed once")
	client.mu.Unlock()

	cancel()
	<-done
}

func TestCommitter_Flush_CommitsDirtyOffsets(t *testing.T) {
	reg := consumer.NewPartitionRegistry(testlogger.NewLogger())
	pauses := consumer.NewPauseRegistry(time.Now)
	client := &stubOffsetClient{}
	cm := newTestCommitter(t, reg, pauses, client)

	ps, _, errGC := reg.GetOrCreate(consumer.Key{Topic: "t", Partition: 0}, testSub("t"), context.Background(), 10)
	require.NoError(t, errGC, "GetOrCreate should succeed")
	ps.AdvanceCommitOffset(&kgo.Record{Topic: "t", Offset: 5, LeaderEpoch: 0})
	reg.MarkDirty(ps)

	err := cm.Flush(context.Background())
	require.NoError(t, err, "Flush should succeed")

	client.mu.Lock()
	assert.Equal(t, 1, len(client.commits), "should have committed once")
	assert.Equal(t, int64(6), client.commits[0]["t"][0].Offset, "should commit offset.Offset+1")
	client.mu.Unlock()
}

func TestCommitter_CommitRecords_CommitsRecords(t *testing.T) {
	reg := consumer.NewPartitionRegistry(testlogger.NewLogger())
	pauses := consumer.NewPauseRegistry(time.Now)
	client := &stubOffsetClient{}
	cm := newTestCommitter(t, reg, pauses, client)

	records := []*kgo.Record{
		{Topic: "t", Partition: 0, Offset: 5, LeaderEpoch: 0},
		{Topic: "t", Partition: 1, Offset: 10, LeaderEpoch: 0},
	}

	err := cm.CommitRecords(context.Background(), records...)
	require.NoError(t, err, "CommitRecords should succeed")

	client.mu.Lock()
	assert.Equal(t, 1, len(client.commits), "should have committed once")
	assert.Equal(t, int64(6), client.commits[0]["t"][0].Offset, "first record offset should be +1")
	assert.Equal(t, int64(11), client.commits[0]["t"][1].Offset, "second record offset should be +1")
	client.mu.Unlock()
}

func TestCommitter_CommitRecords_EmptyNoOp(t *testing.T) {
	reg := consumer.NewPartitionRegistry(testlogger.NewLogger())
	pauses := consumer.NewPauseRegistry(time.Now)
	client := &stubOffsetClient{}
	cm := newTestCommitter(t, reg, pauses, client)

	err := cm.CommitRecords(context.Background())
	require.NoError(t, err, "CommitRecords with no args should succeed")

	client.mu.Lock()
	assert.Equal(t, 0, len(client.commits), "no commit for empty records")
	client.mu.Unlock()
}

func TestCommitter_Flush_NoDirty_NoCommit(t *testing.T) {
	reg := consumer.NewPartitionRegistry(testlogger.NewLogger())
	pauses := consumer.NewPauseRegistry(time.Now)
	client := &stubOffsetClient{}
	cm := newTestCommitter(t, reg, pauses, client)

	err := cm.Flush(context.Background())
	require.NoError(t, err, "Flush should succeed")

	client.mu.Lock()
	assert.Equal(t, 0, len(client.commits), "should not commit when nothing is dirty")
	client.mu.Unlock()
}

func TestCommitter_Flush_CommitFailure(t *testing.T) {
	reg := consumer.NewPartitionRegistry(testlogger.NewLogger())
	pauses := consumer.NewPauseRegistry(time.Now)
	client := &stubOffsetClient{fail: true, mu: sync.Mutex{}, commits: nil}
	cm := newTestCommitter(t, reg, pauses, client)

	ps, _, errGC := reg.GetOrCreate(consumer.Key{Topic: "t", Partition: 0}, testSub("t"), context.Background(), 10)
	require.NoError(t, errGC, "GetOrCreate should succeed")
	ps.AdvanceCommitOffset(&kgo.Record{Topic: "t", Offset: 5, LeaderEpoch: 0})
	reg.MarkDirty(ps)

	err := cm.Flush(context.Background())
	assert.ErrorContains(t, err, "commit failed", "Flush should propagate commit error")
}

func TestCommitter_Finalize_WaitsAndCommits(t *testing.T) {
	reg := consumer.NewPartitionRegistry(testlogger.NewLogger())
	pauses := consumer.NewPauseRegistry(time.Now)
	client := &stubOffsetClient{}
	cm := newTestCommitter(t, reg, pauses, client)

	ps, _, errGC := reg.GetOrCreate(consumer.Key{Topic: "t", Partition: 0}, testSub("t"), context.Background(), 10)
	require.NoError(t, errGC, "GetOrCreate should succeed")
	ps.AdvanceCommitOffset(&kgo.Record{Topic: "t", Offset: 5, LeaderEpoch: 0})
	reg.MarkDirty(ps)

	ps.BeginClosing()
	ps.MarkStopped()

	drainCtx := context.Background()
	commitCtx := context.Background()
	err := cm.Finalize(drainCtx, commitCtx, []*consumer.PartitionState{ps}, "final commit error")
	require.NoError(t, err, "Finalize should succeed")

	client.mu.Lock()
	assert.Equal(t, 1, len(client.commits), "should have committed final offsets")
	client.mu.Unlock()
}

func TestCommitter_PauseTopic_CommitsOffsets(t *testing.T) {
	reg := consumer.NewPartitionRegistry(testlogger.NewLogger())
	pauses := consumer.NewPauseRegistry(time.Now)
	client := &stubOffsetClient{}
	cm := newTestCommitter(t, reg, pauses, client)

	offsets := map[string]map[int32]kgo.EpochOffset{
		"t": {0: {Epoch: 0, Offset: 6}},
	}

	err := cm.PauseTopic(context.Background(), offsets)
	require.NoError(t, err, "PauseTopic should succeed")

	client.mu.Lock()
	assert.Equal(t, 1, len(client.commits), "should have committed the offsets for the paused topic")
	client.mu.Unlock()
}
