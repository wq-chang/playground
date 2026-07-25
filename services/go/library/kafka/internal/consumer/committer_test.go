package consumer_test

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

// stubOffsetClient implements consumer.OffsetClient for testing.
type stubOffsetClient struct {
	commitCh chan struct{}
	commits  []map[string]map[int32]kgo.EpochOffset
	mu       sync.Mutex
	fail     bool
}

func (s *stubOffsetClient) CommitOffsetsSync(
	ctx context.Context,
	offsets map[string]map[int32]kgo.EpochOffset,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return errors.New("commit failed")
	}
	s.commits = append(s.commits, offsets)
	if s.commitCh != nil {
		select {
		case s.commitCh <- struct{}{}:
		default:
		}
	}
	return nil
}

func newTestCommitter(
	t *testing.T,
	reg *consumer.PartitionRegistry,
	client consumer.OffsetClient,
) *consumer.Committer {
	t.Helper()
	return consumer.NewCommitter(
		reg,
		client,
		consumer.CommitConfig{
			FlushInterval:    50 * time.Millisecond,
			DebounceInterval: 10 * time.Millisecond,
		},
	)
}

func TestCommitter_RequestFlush_TriggersFlush(t *testing.T) {
	reg := consumer.NewPartitionRegistry(testlogger.NewLogger())
	client := &stubOffsetClient{fail: false, mu: sync.Mutex{}, commits: nil, commitCh: make(chan struct{}, 1)}
	cm := newTestCommitter(t, reg, client)

	ps, _, errGC := reg.GetOrCreate(
		consumer.Key{Topic: "t", Partition: 0},
		testSub("t"),
		context.Background(),
		10,
	)
	require.NoError(t, errGC, "GetOrCreate should succeed")
	reg.AdvanceStateCommitOffset(ps, &kgo.Record{Topic: "t", Offset: 5, LeaderEpoch: 0})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		err := cm.Run(ctx)
		require.NoError(t, err, "Run should exit without error")
		close(done)
	}()

	cm.RequestFlush()

	select {
	case <-client.commitCh:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for commit after RequestFlush")
	}

	// Stop the commit loop before checking the commit count so the ticker
	// cannot fire and produce a second commit between the signal and the
	// assertion.
	cancel()
	<-done

	client.mu.Lock()
	assert.Equal(t, len(client.commits), 1, "should have committed once")
	client.mu.Unlock()
}

func TestCommitter_RequestFlush_NonBlockingWhenFull(t *testing.T) {
	reg := consumer.NewPartitionRegistry(testlogger.NewLogger())
	client := &stubOffsetClient{}
	cm := newTestCommitter(t, reg, client)

	cm.RequestFlush() // fills the buffer (capacity 1)

	done := make(chan struct{})
	go func() {
		cm.RequestFlush() // must not block when buffer is full
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("RequestFlush blocked when buffer was full")
	}
}

func TestCommitter_Flush_CommitsDirtyOffsets(t *testing.T) {
	reg := consumer.NewPartitionRegistry(testlogger.NewLogger())
	client := &stubOffsetClient{}
	cm := newTestCommitter(t, reg, client)

	ps, _, errGC := reg.GetOrCreate(consumer.Key{Topic: "t", Partition: 0}, testSub("t"), context.Background(), 10)
	require.NoError(t, errGC, "GetOrCreate should succeed")
	reg.AdvanceStateCommitOffset(ps, &kgo.Record{Topic: "t", Offset: 5, LeaderEpoch: 0})

	err := cm.Flush(context.Background())
	require.NoError(t, err, "Flush should succeed")

	client.mu.Lock()
	assert.Equal(t, len(client.commits), 1, "should have committed once")
	assert.Equal(t, client.commits[0]["t"][0].Offset, 6, "should commit offset.Offset+1")
	client.mu.Unlock()
}

func TestCommitter_Flush_NoDirty_NoCommit(t *testing.T) {
	reg := consumer.NewPartitionRegistry(testlogger.NewLogger())
	client := &stubOffsetClient{}
	cm := newTestCommitter(t, reg, client)

	err := cm.Flush(context.Background())
	require.NoError(t, err, "Flush should succeed")

	client.mu.Lock()
	assert.Equal(t, len(client.commits), 0, "should not commit when nothing is dirty")
	client.mu.Unlock()
}

func TestCommitter_Flush_CommitFailure(t *testing.T) {
	reg := consumer.NewPartitionRegistry(testlogger.NewLogger())
	client := &stubOffsetClient{fail: true, mu: sync.Mutex{}, commits: nil, commitCh: nil}
	cm := newTestCommitter(t, reg, client)

	ps, _, errGC := reg.GetOrCreate(consumer.Key{Topic: "t", Partition: 0}, testSub("t"), context.Background(), 10)
	require.NoError(t, errGC, "GetOrCreate should succeed")
	reg.AdvanceStateCommitOffset(ps, &kgo.Record{Topic: "t", Offset: 5, LeaderEpoch: 0})

	err := cm.Flush(context.Background())
	assert.ErrorContains(t, err, "commit failed", "Flush should propagate commit error")
}

func TestCommitter_Run_ReturnsNilOnContextCancel(t *testing.T) {
	reg := consumer.NewPartitionRegistry(testlogger.NewLogger())
	client := &stubOffsetClient{}
	cm := newTestCommitter(t, reg, client)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := cm.Run(ctx)
	require.NoError(t, err, "Run should return nil when context is cancelled")
}

func TestCommitter_Run_ReturnsErrorOnCommitFailure(t *testing.T) {
	reg := consumer.NewPartitionRegistry(testlogger.NewLogger())
	client := &stubOffsetClient{fail: true, mu: sync.Mutex{}, commits: nil, commitCh: nil}
	cm := newTestCommitter(t, reg, client)

	ps, _, errGC := reg.GetOrCreate(consumer.Key{Topic: "t", Partition: 0}, testSub("t"), context.Background(), 10)
	require.NoError(t, errGC, "GetOrCreate should succeed")
	reg.AdvanceStateCommitOffset(ps, &kgo.Record{Topic: "t", Offset: 5, LeaderEpoch: 0})

	ctx := t.Context()

	errCh := make(chan error, 1)
	go func() {
		errCh <- cm.Run(ctx)
	}()

	select {
	case err := <-errCh:
		assert.ErrorContains(t, err, "failed to commit processed offsets", "Run should return commit error")
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Run to return error")
	}
}

func TestCommitter_CommitRecords_CommitsRecords(t *testing.T) {
	reg := consumer.NewPartitionRegistry(testlogger.NewLogger())
	client := &stubOffsetClient{}
	cm := newTestCommitter(t, reg, client)

	records := []*kgo.Record{
		{Topic: "t", Partition: 0, Offset: 5, LeaderEpoch: 0},
		{Topic: "t", Partition: 1, Offset: 10, LeaderEpoch: 0},
	}

	err := cm.CommitRecords(context.Background(), records...)
	require.NoError(t, err, "CommitRecords should succeed")

	client.mu.Lock()
	assert.Equal(t, len(client.commits), 1, "should have committed once")
	assert.Equal(t, client.commits[0]["t"][0].Offset, 6, "first record offset should be +1")
	assert.Equal(t, client.commits[0]["t"][1].Offset, 11, "second record offset should be +1")
	client.mu.Unlock()
}

func TestCommitter_CommitRecords_EmptyNoOp(t *testing.T) {
	reg := consumer.NewPartitionRegistry(testlogger.NewLogger())
	client := &stubOffsetClient{}
	cm := newTestCommitter(t, reg, client)

	err := cm.CommitRecords(context.Background())
	require.NoError(t, err, "CommitRecords with no args should succeed")

	client.mu.Lock()
	assert.Equal(t, len(client.commits), 0, "no commit for empty records")
	client.mu.Unlock()
}

func TestCommitter_CommitRecords_CommitFailure(t *testing.T) {
	reg := consumer.NewPartitionRegistry(testlogger.NewLogger())
	client := &stubOffsetClient{fail: true, mu: sync.Mutex{}, commits: nil, commitCh: nil}
	cm := newTestCommitter(t, reg, client)

	err := cm.CommitRecords(context.Background(), &kgo.Record{Topic: "t", Offset: 1, LeaderEpoch: 0})
	assert.ErrorContains(t, err, "commit failed", "CommitRecords should propagate commit error")
}

func TestCommitter_CommitRecords_NilRecordSkipped(t *testing.T) {
	reg := consumer.NewPartitionRegistry(testlogger.NewLogger())
	client := &stubOffsetClient{}
	cm := newTestCommitter(t, reg, client)

	err := cm.CommitRecords(
		context.Background(),
		nil,
		&kgo.Record{Topic: "t", Partition: 0, Offset: 5, LeaderEpoch: 0},
		nil,
	)
	require.NoError(t, err, "CommitRecords should skip nil records")

	client.mu.Lock()
	require.Equal(t, len(client.commits), 1, "should have committed once")
	assert.Equal(t, len(client.commits[0]["t"]), 1, "only one partition should be committed")
	client.mu.Unlock()
}

func TestCommitter_Finalize_WaitsAndCommits(t *testing.T) {
	reg := consumer.NewPartitionRegistry(testlogger.NewLogger())
	client := &stubOffsetClient{}
	cm := newTestCommitter(t, reg, client)

	ps, _, errGC := reg.GetOrCreate(
		consumer.Key{Topic: "t", Partition: 0},
		testSub("t"),
		context.Background(),
		10,
	)
	require.NoError(t, errGC, "GetOrCreate should succeed")
	reg.AdvanceStateCommitOffset(ps, &kgo.Record{Topic: "t", Offset: 5, LeaderEpoch: 0})

	ps.BeginClosing()
	ps.MarkStopped()

	drainCtx := context.Background()
	commitCtx := context.Background()
	err := cm.Finalize(drainCtx, commitCtx, []*consumer.PartitionState{ps}, "final commit error")
	require.NoError(t, err, "Finalize should succeed")

	client.mu.Lock()
	assert.Equal(t, len(client.commits), 1, "should have committed final offsets")
	client.mu.Unlock()
}

func TestCommitter_Finalize_EmptyStatesNoOp(t *testing.T) {
	reg := consumer.NewPartitionRegistry(testlogger.NewLogger())
	client := &stubOffsetClient{}
	cm := newTestCommitter(t, reg, client)

	err := cm.Finalize(context.Background(), context.Background(), nil, "err msg")
	require.NoError(t, err, "Finalize with empty states should be a no-op")

	client.mu.Lock()
	assert.Equal(t, len(client.commits), 0, "no commit for empty states")
	client.mu.Unlock()
}

func TestCommitter_Finalize_DrainContextTimeout(t *testing.T) {
	reg := consumer.NewPartitionRegistry(testlogger.NewLogger())
	client := &stubOffsetClient{}
	cm := newTestCommitter(t, reg, client)

	ps, _, errGC := reg.GetOrCreate(
		consumer.Key{Topic: "t", Partition: 0},
		testSub("t"),
		context.Background(),
		10,
	)
	require.NoError(t, errGC, "GetOrCreate should succeed")
	ps.BeginClosing()
	// NOT calling MarkStopped — Done() channel stays open, drain will block forever.

	drainCtx, cancel := context.WithCancel(context.Background())
	cancel()

	err := cm.Finalize(drainCtx, context.Background(), []*consumer.PartitionState{ps}, "drain failed")
	assert.ErrorContains(t, err, "failed waiting for topic", "Finalize should propagate drain error")
}

func TestCommitter_Finalize_NoDirtyOffsets(t *testing.T) {
	reg := consumer.NewPartitionRegistry(testlogger.NewLogger())
	client := &stubOffsetClient{}
	cm := newTestCommitter(t, reg, client)

	ps, _, errGC := reg.GetOrCreate(consumer.Key{Topic: "t", Partition: 0}, testSub("t"), context.Background(), 10)
	require.NoError(t, errGC, "GetOrCreate should succeed")
	// NOT calling AdvanceStateCommitOffset — no dirty offsets.
	ps.BeginClosing()
	ps.MarkStopped()

	err := cm.Finalize(context.Background(), context.Background(), []*consumer.PartitionState{ps}, "final commit error")
	require.NoError(t, err, "Finalize should succeed without committing")

	client.mu.Lock()
	assert.Equal(t, len(client.commits), 0, "no commit when nothing dirty")
	client.mu.Unlock()

	_, ok := reg.Get(consumer.Key{Topic: "t", Partition: 0})
	assert.False(t, ok, "state should be cleaned up even without dirty offsets")
}

func TestCommitter_Finalize_CommitFailure(t *testing.T) {
	reg := consumer.NewPartitionRegistry(testlogger.NewLogger())
	client := &stubOffsetClient{fail: true, mu: sync.Mutex{}, commits: nil, commitCh: nil}
	cm := newTestCommitter(t, reg, client)

	ps, _, errGC := reg.GetOrCreate(consumer.Key{Topic: "t", Partition: 0}, testSub("t"), context.Background(), 10)
	require.NoError(t, errGC, "GetOrCreate should succeed")
	reg.AdvanceStateCommitOffset(ps, &kgo.Record{Topic: "t", Offset: 5, LeaderEpoch: 0})
	ps.BeginClosing()
	ps.MarkStopped()

	err := cm.Finalize(context.Background(), context.Background(), []*consumer.PartitionState{ps}, "final commit error")
	assert.ErrorContains(t, err, "final commit error", "Finalize should wrap the error message")
	assert.ErrorContains(t, err, "commit failed", "Finalize should include the underlying error")
}

func TestCommitter_Finalize_NilCommitCtx(t *testing.T) {
	reg := consumer.NewPartitionRegistry(testlogger.NewLogger())
	client := &stubOffsetClient{}
	cm := newTestCommitter(t, reg, client)

	ps, _, errGC := reg.GetOrCreate(consumer.Key{Topic: "t", Partition: 0}, testSub("t"), context.Background(), 10)
	require.NoError(t, errGC, "GetOrCreate should succeed")
	reg.AdvanceStateCommitOffset(ps, &kgo.Record{Topic: "t", Offset: 5, LeaderEpoch: 0})
	ps.BeginClosing()
	ps.MarkStopped()

	// intentionally testing nil commitCtx guard
	// nolint:staticcheck
	err := cm.Finalize(context.Background(), nil, []*consumer.PartitionState{ps}, "final commit error")
	assert.ErrorContains(t, err, "commitCtx must not be nil", "Finalize should reject nil commitCtx")

	client.mu.Lock()
	assert.Equal(t, len(client.commits), 0, "no commit when commitCtx is nil")
	client.mu.Unlock()
}

func TestCommitter_Finalize_NilDrainCtx(t *testing.T) {
	reg := consumer.NewPartitionRegistry(testlogger.NewLogger())
	client := &stubOffsetClient{}
	cm := newTestCommitter(t, reg, client)

	ps, _, errGC := reg.GetOrCreate(consumer.Key{Topic: "t", Partition: 0}, testSub("t"), context.Background(), 10)
	require.NoError(t, errGC, "GetOrCreate should succeed")
	reg.AdvanceStateCommitOffset(ps, &kgo.Record{Topic: "t", Offset: 5, LeaderEpoch: 0})
	ps.BeginClosing()
	ps.MarkStopped()

	// intentionally testing nil drainCtx guard
	// nolint:staticcheck
	err := cm.Finalize(nil, context.Background(), []*consumer.PartitionState{ps}, "final commit error")
	assert.ErrorContains(t, err, "drainCtx must not be nil", "Finalize should reject nil drainCtx")

	client.mu.Lock()
	assert.Equal(t, len(client.commits), 0, "no commit when drainCtx is nil")
	client.mu.Unlock()
}
