package kafka

import (
	"context"
	"fmt"
	"sync"

	"github.com/twmb/franz-go/pkg/kgo"
)

type partitionState struct {
	ctx                context.Context
	queue              chan []*kgo.Record
	cancel             context.CancelFunc
	done               chan struct{}
	key                recordKey
	subscription       Subscription
	nextCommitOffset   kgo.EpochOffset
	committedOffset    kgo.EpochOffset
	queueCloseOnce     sync.Once
	mu                 sync.Mutex
	maxBufferedRecords int32
	bufferedRecords    int32
	accepting          bool
	backpressurePaused bool
	dirty              bool
	lifecycle          partitionLifecycle
}

type partitionLifecycle int

const (
	partitionLifecycleRunning partitionLifecycle = iota
	partitionLifecycleClosing
	partitionLifecycleStopped
)

func newPartitionState(
	parent context.Context,
	key recordKey,
	subscription Subscription,
	queueCapacity int,
) *partitionState {
	stateCtx, cancel := context.WithCancel(parent)
	return &partitionState{
		ctx:                stateCtx,
		queue:              make(chan []*kgo.Record, queueCapacity),
		cancel:             cancel,
		done:               make(chan struct{}),
		key:                key,
		subscription:       subscription,
		nextCommitOffset:   kgo.EpochOffset{Epoch: -1, Offset: -1},
		committedOffset:    kgo.EpochOffset{Epoch: -1, Offset: -1},
		queueCloseOnce:     sync.Once{},
		mu:                 sync.Mutex{},
		maxBufferedRecords: int32(queueCapacity),
		bufferedRecords:    0,
		accepting:          true,
		backpressurePaused: false,
		dirty:              false,
		lifecycle:          partitionLifecycleRunning,
	}
}

func (c *Consumer) enqueuePartitionRecords(
	cl *kgo.Client,
	state *partitionState,
	records []*kgo.Record,
) (int, error) {
	if err := state.ctx.Err(); err != nil {
		return 0, err
	}

	enqueued, bufferedRecords := state.tryEnqueueRecords(records)
	if enqueued > 0 {
		c.maybePausePartitionForBackpressure(cl, state, bufferedRecords)
		return enqueued, nil
	}

	c.maybePausePartitionForBackpressure(cl, state, bufferedRecords)
	return 0, nil
}

func (c *Consumer) maybePausePartitionForBackpressure(cl *kgo.Client, state *partitionState, bufferedRecords int) {
	if bufferedRecords < c.partitionQueueHighWatermark() {
		return
	}
	if !state.markBackpressurePaused() {
		return
	}
	if cl == nil {
		return
	}

	cl.PauseFetchPartitions(map[string][]int32{
		state.key.topic: {state.key.partition},
	})
}

func (c *Consumer) maybeResumePartitionAfterDrain(cl *kgo.Client, state *partitionState, bufferedRecords int) {
	if bufferedRecords > c.partitionQueueLowWatermark() || c.isTopicPaused(state.key.topic) {
		return
	}
	if !state.clearBackpressurePaused() {
		return
	}
	if cl == nil {
		return
	}

	cl.ResumeFetchPartitions(map[string][]int32{
		state.key.topic: {state.key.partition},
	})
}

func (c *Consumer) processPartitionRecord(
	ctx context.Context,
	cl *kgo.Client,
	state *partitionState,
	record *kgo.Record,
) error {
	switch state.subscription.AckMode {
	case AckModeAtMostOnce:
		if err := c.commitRecord(ctx, cl, record); err != nil {
			return fmt.Errorf("failed to commit record before handling: %w", err)
		}
	case AckModeAtLeastOnce:
		// Manual offset commit happens after successful processing.
	default:
		return fmt.Errorf("unsupported ack mode: %d", state.subscription.AckMode)
	}

	result, err := c.executeRecord(ctx, state.subscription, record)
	if err != nil {
		return err
	}
	if result.pauseTopic {
		return c.pauseTopic(cl, record.Topic, result.cause)
	}
	if state.subscription.AckMode == AckModeAtLeastOnce && result.resolved {
		if state.advanceCommitOffset(record) && c.markDirtyPartitionState(state) {
			c.signalCommitLoop()
		}
	}

	return nil
}

func (c *Consumer) processPartitionBatch(
	ctx context.Context,
	cl *kgo.Client,
	state *partitionState,
	records []*kgo.Record,
) error {
	if len(records) == 0 {
		return nil
	}

	switch state.subscription.AckMode {
	case AckModeAtMostOnce:
		if err := c.commitRecords(ctx, cl, records...); err != nil {
			return fmt.Errorf("failed to commit batch before handling: %w", err)
		}
	case AckModeAtLeastOnce:
		// Manual offset commit happens after successful processing.
	default:
		return fmt.Errorf("unsupported ack mode: %d", state.subscription.AckMode)
	}

	result, err := c.executeBatch(ctx, state.subscription, records)
	if err != nil {
		return err
	}
	if state.subscription.AckMode == AckModeAtLeastOnce && result.resolvedCount > 0 {
		lastResolvedRecord := records[result.resolvedCount-1]
		if state.advanceCommitOffset(lastResolvedRecord) && c.markDirtyPartitionState(state) {
			c.signalCommitLoop()
		}
	}
	if result.pauseTopic {
		return c.pauseTopic(cl, records[0].Topic, result.cause)
	}

	return nil
}

func (s *partitionState) tryEnqueueRecords(records []*kgo.Record) (int, int) {
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

func (s *partitionState) onDequeueBatch(records []*kgo.Record) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.bufferedRecords -= int32(len(records))
	if s.bufferedRecords < 0 {
		s.bufferedRecords = 0
	}
	return int(s.bufferedRecords)
}

func (s *partitionState) markBackpressurePaused() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.accepting || s.backpressurePaused {
		return false
	}

	s.backpressurePaused = true
	return true
}

func (s *partitionState) clearBackpressurePaused() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.accepting || !s.backpressurePaused {
		return false
	}

	s.backpressurePaused = false
	return true
}

func (s *partitionState) advanceCommitOffset(record *kgo.Record) bool {
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
	s.dirty = s.committedOffset.Less(nextOffset)
	return true
}

func (s *partitionState) snapshotDirtyOffset() (kgo.EpochOffset, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.dirty || !s.committedOffset.Less(s.nextCommitOffset) {
		return kgo.EpochOffset{}, false
	}

	return s.nextCommitOffset, true
}

func (s *partitionState) markCommitted(offset kgo.EpochOffset) bool {
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

func (s *partitionState) beginClosing() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.beginClosingLocked()
}

func (s *partitionState) beginClosingLocked() {
	if s.lifecycle == partitionLifecycleStopped {
		return
	}

	s.lifecycle = partitionLifecycleClosing
	s.accepting = false
	s.backpressurePaused = false
	s.closeQueueLocked()
}

func (s *partitionState) closeQueueLocked() {
	s.queueCloseOnce.Do(func() {
		close(s.queue)
	})
}

func (s *partitionState) abort() (kgo.EpochOffset, bool) {
	s.mu.Lock()
	s.beginClosingLocked()
	offset := s.nextCommitOffset
	ok := s.committedOffset.Less(offset)
	s.mu.Unlock()

	s.cancel()
	return offset, ok
}

func (s *partitionState) markStopped() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.lifecycle = partitionLifecycleStopped
	s.accepting = false
	s.backpressurePaused = false
}

func (s *partitionState) isRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.lifecycle == partitionLifecycleRunning
}
