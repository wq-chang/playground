package kafka

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
)

var (
	commitOffsetsSyncFn       = commitOffsetsSync
	afterDirtyOffsetsSnapshot = func() {}
)

func (c *Consumer) runCommitLoop(ctx context.Context, cl *kgo.Client) {
	defer c.runWG.Done()

	ticker := time.NewTicker(commitFlushInterval)
	defer ticker.Stop()

	var (
		debounce  *time.Timer
		debounceC <-chan time.Time
	)

	stopDebounce := func() {
		if debounce == nil {
			return
		}
		debounce.Stop()
		debounce = nil
		debounceC = nil
	}
	defer stopDebounce()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.flushDirtyOffsets(ctx, cl); err != nil && !errors.Is(err, context.Canceled) {
				c.fail(err)
				return
			}
		case <-c.commitSignal:
			if debounce == nil {
				debounce = time.NewTimer(commitDebounceInterval)
				debounceC = debounce.C
				continue
			}

			debounce.Stop()
			debounce.Reset(commitDebounceInterval)
		case <-debounceC:
			stopDebounce()
			if err := c.flushDirtyOffsets(ctx, cl); err != nil && !errors.Is(err, context.Canceled) {
				c.fail(err)
				return
			}
		}
	}
}

func (c *Consumer) flushDirtyOffsets(ctx context.Context, cl *kgo.Client) error {
	if err := c.acquireCommitMu(ctx); err != nil {
		return err
	}
	defer c.releaseCommitMu()

	offsets := c.snapshotDirtyOffsets()
	if len(offsets) == 0 {
		return nil
	}
	afterDirtyOffsetsSnapshot()

	if err := c.commitOffsetsLocked(ctx, cl, offsets); err != nil {
		return fmt.Errorf("failed to commit processed offsets: %w", err)
	}

	c.markCommittedOffsets(offsets)
	return nil
}

func (c *Consumer) snapshotDirtyOffsets() map[string]map[int32]kgo.EpochOffset {
	dirtyStates := c.snapshotDirtyStates()
	if len(dirtyStates) == 0 {
		return nil
	}

	offsets := make(map[string]map[int32]kgo.EpochOffset)
	for key, state := range dirtyStates {
		offset, ok := state.snapshotDirtyOffset()
		if !ok {
			c.clearDirtyPartitionState(key, state)
			continue
		}

		partitionsByTopic, exists := offsets[key.topic]
		if !exists {
			partitionsByTopic = make(map[int32]kgo.EpochOffset)
			offsets[key.topic] = partitionsByTopic
		}
		partitionsByTopic[key.partition] = offset
	}

	if len(offsets) == 0 {
		return nil
	}
	return offsets
}

func (c *Consumer) markCommittedOffsets(offsets map[string]map[int32]kgo.EpochOffset) {
	if len(offsets) == 0 {
		return
	}

	for topic, partitions := range offsets {
		for partition, offset := range partitions {
			key := recordKey{topic: topic, partition: partition}
			c.workersMu.RLock()
			state, ok := c.partitionStates[key]
			c.workersMu.RUnlock()
			if !ok {
				c.clearDirtyPartitionState(key, nil)
				continue
			}
			if !state.markCommitted(offset) {
				c.clearDirtyPartitionState(key, state)
			}
		}
	}
}

func (c *Consumer) commitRecord(ctx context.Context, cl *kgo.Client, record *kgo.Record) error {
	return c.commitRecords(ctx, cl, record)
}

func (c *Consumer) commitRecords(ctx context.Context, cl *kgo.Client, records ...*kgo.Record) error {
	if len(records) == 0 {
		return nil
	}

	if err := c.acquireCommitMu(ctx); err != nil {
		return err
	}
	defer c.releaseCommitMu()

	return cl.CommitRecords(ctx, records...)
}

func (c *Consumer) commitOffsets(
	ctx context.Context,
	cl *kgo.Client,
	offsets map[string]map[int32]kgo.EpochOffset,
) error {
	if len(offsets) == 0 {
		return nil
	}

	if err := c.acquireCommitMu(ctx); err != nil {
		return err
	}
	defer c.releaseCommitMu()

	return c.commitOffsetsLocked(ctx, cl, offsets)
}

func (c *Consumer) acquireCommitMu(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if c.commitMu == nil {
		return fmt.Errorf("commit coordinator is not initialized")
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.commitMu:
		return nil
	}
}

func (c *Consumer) releaseCommitMu() {
	if c.commitMu == nil {
		return
	}

	c.commitMu <- struct{}{}
}

func (c *Consumer) commitOffsetsLocked(
	ctx context.Context,
	cl *kgo.Client,
	offsets map[string]map[int32]kgo.EpochOffset,
) error {
	return commitOffsetsSyncFn(ctx, cl, offsets)
}

func commitOffsetsSync(
	ctx context.Context,
	cl *kgo.Client,
	offsets map[string]map[int32]kgo.EpochOffset,
) error {
	if len(offsets) == 0 {
		return nil
	}

	var commitErr error
	cl.CommitOffsetsSync(
		ctx,
		offsets,
		func(
			_ *kgo.Client,
			_ *kmsg.OffsetCommitRequest,
			_ *kmsg.OffsetCommitResponse,
			err error,
		) {
			commitErr = err
		},
	)
	return commitErr
}

func (c *Consumer) pauseTopic(cl *kgo.Client, topic string, cause error) error {
	c.pausedTopicsMu.Lock()
	if _, ok := c.pausedTopics[topic]; ok {
		c.pausedTopicsMu.Unlock()
		return nil
	}
	c.pausedTopics[topic] = pausedTopic{
		cause:    cause,
		pausedAt: time.Now(),
	}
	c.pausedTopicsState.Store(maps.Clone(c.pausedTopics))
	c.pausedTopicsMu.Unlock()

	c.workersMu.Lock()
	offsets := c.abortPartitionStatesLocked(func(key recordKey) bool {
		return key.topic == topic
	})
	c.workersMu.Unlock()

	if cl != nil {
		cl.PauseFetchTopics(topic)
	}
	if err := c.commitOffsets(context.Background(), cl, offsets); err != nil {
		return fmt.Errorf("failed to commit paused topic offsets: %w", err)
	}

	c.log.Warn("Kafka topic paused after retry exhaustion",
		"topic", topic,
		"err", cause)
	return nil
}

func (c *Consumer) isTopicPaused(topic string) bool {
	_, ok := c.pausedTopicSnapshot()[topic]
	return ok
}

func (c *Consumer) abortPartitionStatesLocked(match func(recordKey) bool) map[string]map[int32]kgo.EpochOffset {
	offsets := make(map[string]map[int32]kgo.EpochOffset)

	for key, state := range c.partitionStates {
		if !match(key) {
			continue
		}

		offset, ok := state.abort()
		delete(c.partitionStates, key)
		delete(c.dirtyStates, key)
		if !ok {
			continue
		}

		partitionsByTopic, exists := offsets[key.topic]
		if !exists {
			partitionsByTopic = make(map[int32]kgo.EpochOffset)
			offsets[key.topic] = partitionsByTopic
		}
		partitionsByTopic[key.partition] = offset
	}

	if len(offsets) == 0 {
		return nil
	}
	return offsets
}

func (c *Consumer) beginClosingPartitionStatesForPartitions(partitions map[string][]int32) []*partitionState {
	allowed := make(map[recordKey]struct{})
	for topic, partitionIDs := range partitions {
		for _, partition := range partitionIDs {
			allowed[recordKey{topic: topic, partition: partition}] = struct{}{}
		}
	}

	c.workersMu.Lock()
	defer c.workersMu.Unlock()

	states := make([]*partitionState, 0, len(allowed))
	for key, state := range c.partitionStates {
		if _, ok := allowed[key]; !ok {
			continue
		}
		state.beginClosing()
		states = append(states, state)
	}

	return states
}

func (c *Consumer) beginClosingAllPartitionStates() []*partitionState {
	c.workersMu.Lock()
	defer c.workersMu.Unlock()

	states := make([]*partitionState, 0, len(c.partitionStates))
	for _, state := range c.partitionStates {
		state.beginClosing()
		states = append(states, state)
	}

	return states
}

func (c *Consumer) waitForPartitionStates(ctx context.Context, states []*partitionState) error {
	if len(states) == 0 {
		return nil
	}

	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), partitionDrainTimeout)
		defer cancel()
	}

	for _, state := range states {
		select {
		case <-state.done:
		case <-ctx.Done():
			return fmt.Errorf(
				"failed waiting for topic %q partition %d to drain: %w",
				state.key.topic,
				state.key.partition,
				ctx.Err(),
			)
		}
	}

	return nil
}

func (c *Consumer) snapshotOffsetsForStates(states []*partitionState) map[string]map[int32]kgo.EpochOffset {
	offsets := make(map[string]map[int32]kgo.EpochOffset)

	for _, state := range states {
		offset, ok := state.snapshotDirtyOffset()
		if !ok {
			continue
		}

		partitionsByTopic, exists := offsets[state.key.topic]
		if !exists {
			partitionsByTopic = make(map[int32]kgo.EpochOffset)
			offsets[state.key.topic] = partitionsByTopic
		}
		partitionsByTopic[state.key.partition] = offset
	}

	if len(offsets) == 0 {
		return nil
	}

	return offsets
}

func (c *Consumer) cleanupStoppedPartitionStates(states []*partitionState) {
	c.workersMu.Lock()
	defer c.workersMu.Unlock()

	for _, state := range states {
		current, ok := c.partitionStates[state.key]
		if ok && current == state {
			delete(c.partitionStates, state.key)
		}
		current, ok = c.dirtyStates[state.key]
		if ok && current == state {
			delete(c.dirtyStates, state.key)
		}
	}
}

func (c *Consumer) finalizeClosingPartitionStates(
	drainCtx context.Context,
	commitCtx context.Context,
	cl *kgo.Client,
	states []*partitionState,
	commitErrMessage string,
) error {
	if len(states) == 0 {
		return nil
	}
	if err := c.waitForPartitionStates(drainCtx, states); err != nil {
		return err
	}

	if commitCtx == nil {
		var cancel context.CancelFunc
		commitCtx, cancel = context.WithTimeout(context.Background(), finalCommitTimeout)
		defer cancel()
	}

	if err := c.acquireCommitMu(commitCtx); err != nil {
		return fmt.Errorf("%s: %w", commitErrMessage, err)
	}
	defer c.releaseCommitMu()

	offsets := c.snapshotOffsetsForStates(states)
	if len(offsets) == 0 {
		c.cleanupStoppedPartitionStates(states)
		return nil
	}

	err := c.commitOffsetsLocked(commitCtx, cl, offsets)
	c.cleanupStoppedPartitionStates(states)
	if err != nil {
		return fmt.Errorf("%s: %w", commitErrMessage, err)
	}

	return nil
}

func (c *Consumer) stopPartitionStatesForLostPartitions(partitions map[string][]int32) {
	allowed := make(map[recordKey]struct{})
	for topic, partitionIDs := range partitions {
		for _, partition := range partitionIDs {
			allowed[recordKey{topic: topic, partition: partition}] = struct{}{}
		}
	}

	c.workersMu.Lock()
	defer c.workersMu.Unlock()

	_ = c.abortPartitionStatesLocked(func(key recordKey) bool {
		_, ok := allowed[key]
		return ok
	})
}

func (c *Consumer) onPartitionsRevoked(ctx context.Context, cl *kgo.Client, partitions map[string][]int32) {
	if len(partitions) == 0 {
		return
	}

	if cl != nil {
		cl.PauseFetchPartitions(partitions)
	}

	states := c.beginClosingPartitionStatesForPartitions(partitions)
	if err := c.finalizeClosingPartitionStates(ctx, ctx, cl, states, "failed to commit processed offsets on revoke"); err != nil {
		commitErr := err
		c.log.ErrorContext(ctx, "failed to commit processed offsets on revoke", "err", commitErr)
		c.fail(commitErr)
	}
}

func (c *Consumer) onPartitionsLost(ctx context.Context, partitions map[string][]int32) {
	c.log.WarnContext(ctx, "Kafka partitions lost; dropping in-memory commit progress", "partitions", partitions)
	c.stopPartitionStatesForLostPartitions(partitions)
}

func (c *Consumer) markDirtyPartitionState(state *partitionState) bool {
	c.workersMu.Lock()
	defer c.workersMu.Unlock()

	current, ok := c.partitionStates[state.key]
	if !ok || current != state {
		return false
	}
	c.dirtyStates[state.key] = state
	return true
}

func (c *Consumer) clearDirtyPartitionState(key recordKey, state *partitionState) {
	c.workersMu.Lock()
	defer c.workersMu.Unlock()

	if state == nil {
		delete(c.dirtyStates, key)
		return
	}
	current, ok := c.dirtyStates[key]
	if ok && current == state {
		delete(c.dirtyStates, key)
	}
}

func (c *Consumer) snapshotDirtyStates() map[recordKey]*partitionState {
	c.workersMu.RLock()
	defer c.workersMu.RUnlock()

	if len(c.dirtyStates) == 0 {
		return nil
	}

	dirtyStates := make(map[recordKey]*partitionState, len(c.dirtyStates))
	maps.Copy(dirtyStates, c.dirtyStates)
	return dirtyStates
}
