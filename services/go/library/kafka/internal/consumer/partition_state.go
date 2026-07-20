package consumer

import (
	"context"
	"log/slog"
	"sync"

	"github.com/twmb/franz-go/pkg/kgo"
)

type partitionLifecycle int

const (
	partitionLifecycleRunning partitionLifecycle = iota
	partitionLifecycleClosing
	partitionLifecycleStopped
)

// PartitionState owns the per-partition runtime state: a buffered record queue,
// backpressure flags, commit-progress tracking, and lifecycle management.
//
// The queue and lifecycle transitions are thread-safe under a private mutex.
// The done channel is exposed read-only via Done() — closing it is the
// responsibility of the worker runner.
type PartitionState struct {
	ctx                context.Context
	log                *slog.Logger
	queue              chan []*kgo.Record
	cancel             context.CancelFunc
	done               chan struct{}
	key                Key
	subscription       Subscription
	nextCommitOffset   kgo.EpochOffset
	committedOffset    kgo.EpochOffset
	lifecycle          partitionLifecycle
	queueCloseOnce     sync.Once
	closeDoneOnce      sync.Once
	mu                 sync.Mutex
	bufferedRecords    int
	maxBufferedRecords int
	accepting          bool
	backpressurePaused bool
	dirty              bool
}

// NewPartitionState creates one partition runtime state with a private queue,
// cancel function, done channel, and commit-progress state.
func NewPartitionState(
	parent context.Context,
	logger *slog.Logger,
	key Key,
	subscription Subscription,
	queueCapacity int,
) *PartitionState {
	stateCtx, cancel := context.WithCancel(parent)
	return &PartitionState{
		ctx:                stateCtx,
		queue:              make(chan []*kgo.Record, queueCapacity),
		cancel:             cancel,
		done:               make(chan struct{}),
		key:                key,
		subscription:       subscription,
		nextCommitOffset:   kgo.EpochOffset{Epoch: -1, Offset: -1},
		committedOffset:    kgo.EpochOffset{Epoch: -1, Offset: -1},
		queueCloseOnce:     sync.Once{},
		closeDoneOnce:      sync.Once{},
		mu:                 sync.Mutex{},
		log:                logger,
		maxBufferedRecords: queueCapacity,
		bufferedRecords:    0,
		accepting:          true,
		backpressurePaused: false,
		dirty:              false,
		lifecycle:          partitionLifecycleRunning,
	}
}

// Key returns the topic-partition key for this state.
func (s *PartitionState) Key() Key {
	return s.key
}

// Context returns the partition's context, which is cancelled when
// BeginClosing is called.
func (s *PartitionState) Context() context.Context {
	return s.ctx
}

// Subscription returns the subscription configuration for this partition.
func (s *PartitionState) Subscription() Subscription {
	return s.subscription
}

// Done returns a read-only channel that is closed when the worker runner
// has finished processing this partition.
func (s *PartitionState) Done() <-chan struct{} {
	return s.done
}

// TryEnqueue tries to enqueue a suffix of records without blocking.
// Returns the number of records enqueued and the total buffered count.
func (s *PartitionState) TryEnqueue(records []*kgo.Record) (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.accepting || len(records) == 0 {
		return 0, s.bufferedRecords
	}

	available := s.maxBufferedRecords - s.bufferedRecords
	if available <= 0 {
		return 0, s.bufferedRecords
	}
	if len(records) > available {
		records = records[:available]
	}

	// The channel send is guaranteed to succeed here: available > 0 means
	// bufferedRecords < maxBufferedRecords, and since each queued slice
	// contributes ≥1 to bufferedRecords, len(queue) < cap(queue) always holds.
	// The default branch exists only as defense against future bugs.
	select {
	case s.queue <- records:
		s.bufferedRecords += len(records)
		return len(records), s.bufferedRecords
	default:
		return 0, s.bufferedRecords
	}
}

// Dequeue blocks until a batch is available, the queue is closed, or ctx is
// done. It atomically updates the buffered count — callers do not need a
// separate OnDequeue step.
func (s *PartitionState) Dequeue(ctx context.Context) ([]*kgo.Record, bool) {
	select {
	case <-ctx.Done():
		return nil, false
	case records, ok := <-s.queue:
		if !ok {
			return nil, false
		}

		s.mu.Lock()
		s.bufferedRecords -= len(records)
		if s.bufferedRecords < 0 {
			s.log.WarnContext(
				s.ctx,
				"partition bufferedRecords went negative on dequeue — accounting bug",
				"topic", s.key.Topic,
				"partition", s.key.Partition,
				"dequeued", len(records),
				"buffered", s.bufferedRecords,
			)
			s.bufferedRecords = 0
		}
		s.mu.Unlock()

		return records, true
	}
}

// TryPauseBackpressure marks the partition as pause-by-backpressure if the
// queue is at the high watermark (capacity - 1 slots filled) and it was still
// accepting work. Returns true if this call applied the pause, indicating
// the caller should pause Kafka fetches for this partition.
func (s *PartitionState) TryPauseBackpressure() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.accepting || s.backpressurePaused {
		return false
	}
	if s.bufferedRecords < s.maxBufferedRecords-1 {
		return false
	}

	s.backpressurePaused = true
	return true
}

// TryResumeBackpressure clears the backpressure pause flag if the queue has
// drained to the low watermark (capacity / 2 remaining) and was currently
// paused by backpressure. Returns true if the flag was cleared, indicating
// the caller should resume Kafka fetches for this partition.
func (s *PartitionState) TryResumeBackpressure() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.accepting || !s.backpressurePaused {
		return false
	}
	if s.bufferedRecords > s.maxBufferedRecords/2 {
		return false
	}

	s.backpressurePaused = false
	return true
}

// AdvanceCommitOffset advances the next commit offset based on a successfully
// resolved record. Returns true if the offset was advanced.
func (s *PartitionState) AdvanceCommitOffset(record *kgo.Record) bool {
	nextOffset := kgo.EpochOffset{
		Epoch:  record.LeaderEpoch,
		Offset: record.Offset + 1,
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.nextCommitOffset.Less(nextOffset) {
		return false
	}

	s.nextCommitOffset = nextOffset
	s.dirty = true
	return true
}

// SnapshotDirtyOffset returns the current dirty offset if there is uncommitted
// progress. Returns false if no dirty offset exists.
func (s *PartitionState) SnapshotDirtyOffset() (kgo.EpochOffset, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.dirty || !s.committedOffset.Less(s.nextCommitOffset) {
		return kgo.EpochOffset{}, false
	}

	return s.nextCommitOffset, true
}

// MarkCommitted records a successful commit and returns whether the state is
// still dirty afterward.
func (s *PartitionState) MarkCommitted(offset kgo.EpochOffset) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.committedOffset.Less(offset) {
		s.committedOffset = offset
	}
	if !s.committedOffset.Less(s.nextCommitOffset) {
		s.dirty = false
	}
	return s.dirty
}

// BeginClosing moves the state into closing mode, stops accepting new work,
// and cancels the context so in-flight handlers detect the signal and exit.
// This prevents goroutine leaks when a handler hangs during drain timeout.
func (s *PartitionState) BeginClosing() {
	s.mu.Lock()

	if s.lifecycle == partitionLifecycleStopped {
		s.mu.Unlock()
		return
	}

	s.lifecycle = partitionLifecycleClosing
	s.accepting = false
	s.backpressurePaused = false
	s.queueCloseOnce.Do(func() {
		close(s.queue)
	})

	s.mu.Unlock()

	s.cancel()
}

// MarkStopped marks the state as fully stopped and closes the done channel
// to signal observers (such as Finalize) that the partition has drained.
func (s *PartitionState) MarkStopped() {
	s.mu.Lock()
	s.lifecycle = partitionLifecycleStopped
	s.accepting = false
	s.backpressurePaused = false
	s.mu.Unlock()

	s.closeDoneOnce.Do(func() {
		close(s.done)
	})
}
