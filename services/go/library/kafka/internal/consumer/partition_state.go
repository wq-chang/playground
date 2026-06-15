// services/go/library/kafka/internal/consumer/partition_state.go
package consumer

import (
	"context"
	"sync"

	"github.com/twmb/franz-go/pkg/kgo"

	"go-services/library/kafka/ktype"
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
// responsibility of the worker runner (Step 7).
type PartitionState struct {
	ctx                context.Context
	queue              chan []*kgo.Record
	cancel             context.CancelFunc
	done               chan struct{}
	key                Key
	subscription       ktype.Subscription
	nextCommitOffset   kgo.EpochOffset
	committedOffset    kgo.EpochOffset
	lifecycle          partitionLifecycle
	queueCloseOnce     sync.Once
	closeDoneOnce      sync.Once
	mu                 sync.Mutex
	bufferedRecords    int32
	maxBufferedRecords int32
	accepting          bool
	backpressurePaused bool
	dirty              bool
}

// NewPartitionState creates one partition runtime state with a private queue,
// cancel function, done channel, and commit-progress state.
func NewPartitionState(
	parent context.Context,
	key Key,
	subscription ktype.Subscription,
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
		maxBufferedRecords: int32(queueCapacity),
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
		return 0, int(s.bufferedRecords)
	}

	available := int(s.maxBufferedRecords - s.bufferedRecords)
	if available <= 0 {
		return 0, int(s.bufferedRecords)
	}
	if len(records) > available {
		records = records[:available]
	}

	select {
	case s.queue <- records:
		s.bufferedRecords += int32(len(records))
		return len(records), int(s.bufferedRecords)
	default:
		return 0, int(s.bufferedRecords)
	}
}

// OnDequeue records that a batch left the queue and returns the remaining
// buffered-record count.
func (s *PartitionState) OnDequeue(records []*kgo.Record) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.bufferedRecords -= int32(len(records))
	if s.bufferedRecords < 0 {
		s.bufferedRecords = 0
	}
	return int(s.bufferedRecords)
}

// MarkBackpressurePaused marks the partition as paused-by-backpressure if it
// was still accepting work. Returns true if this call applied the pause.
func (s *PartitionState) MarkBackpressurePaused() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.accepting || s.backpressurePaused {
		return false
	}

	s.backpressurePaused = true
	return true
}

// ClearBackpressurePaused clears the backpressure pause flag when the buffered
// count drains low enough. Returns true if the flag was cleared.
func (s *PartitionState) ClearBackpressurePaused() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.accepting || !s.backpressurePaused {
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

// BeginClosing moves the state into closing mode and stops accepting new work.
func (s *PartitionState) BeginClosing() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.beginClosingLocked()
}

func (s *PartitionState) beginClosingLocked() {
	if s.lifecycle == partitionLifecycleStopped {
		return
	}

	s.lifecycle = partitionLifecycleClosing
	s.accepting = false
	s.backpressurePaused = false
	s.closeQueueLocked()
}

func (s *PartitionState) closeQueueLocked() {
	s.queueCloseOnce.Do(func() {
		close(s.queue)
	})
}

// Abort aborts the state, cancels its context, and reports the last committable
// offset if one exists.
func (s *PartitionState) Abort() (kgo.EpochOffset, bool) {
	s.mu.Lock()
	s.beginClosingLocked()
	offset := s.nextCommitOffset
	ok := s.committedOffset.Less(offset)
	s.mu.Unlock()

	s.cancel()
	return offset, ok
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

// IsRunning reports whether the state is still in running mode.
func (s *PartitionState) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.lifecycle == partitionLifecycleRunning
}
