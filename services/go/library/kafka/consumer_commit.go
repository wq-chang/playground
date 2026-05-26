package kafka

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
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
		if !debounce.Stop() {
			select {
			case <-debounce.C:
			default:
			}
		}
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

			if !debounce.Stop() {
				select {
				case <-debounce.C:
				default:
				}
			}
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

func (c *Consumer) flushDirtyOffsetsWithTimeout(cl *kgo.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), commitFlushInterval)
	defer cancel()

	return c.flushDirtyOffsets(ctx, cl)
}

func (c *Consumer) flushDirtyOffsets(ctx context.Context, cl *kgo.Client) error {
	offsets := c.snapshotDirtyOffsets()
	if len(offsets) == 0 {
		return nil
	}

	if err := c.commitOffsets(ctx, cl, offsets); err != nil {
		return fmt.Errorf("failed to commit processed offsets: %w", err)
	}

	c.markCommittedOffsets(offsets)
	return nil
}

func (c *Consumer) snapshotDirtyOffsets() map[string]map[int32]kgo.EpochOffset {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.workers) == 0 {
		return nil
	}

	offsets := make(map[string]map[int32]kgo.EpochOffset)
	for key, worker := range c.workers {
		offset, ok := worker.snapshotDirtyOffset()
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

func (c *Consumer) markCommittedOffsets(offsets map[string]map[int32]kgo.EpochOffset) {
	if len(offsets) == 0 {
		return
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	for topic, partitions := range offsets {
		for partition, offset := range partitions {
			worker, ok := c.workers[recordKey{topic: topic, partition: partition}]
			if !ok {
				continue
			}
			worker.markCommitted(offset)
		}
	}
}

func (c *Consumer) commitRecord(ctx context.Context, cl *kgo.Client, record *kgo.Record) error {
	c.commitMu.Lock()
	defer c.commitMu.Unlock()

	return cl.CommitRecords(ctx, record)
}

func (c *Consumer) commitOffsets(
	ctx context.Context,
	cl *kgo.Client,
	offsets map[string]map[int32]kgo.EpochOffset,
) error {
	if len(offsets) == 0 {
		return nil
	}

	c.commitMu.Lock()
	defer c.commitMu.Unlock()

	return commitOffsetsSync(ctx, cl, offsets)
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
	cl.CommitOffsetsSync(ctx, offsets, func(_ *kgo.Client, _ *kmsg.OffsetCommitRequest, _ *kmsg.OffsetCommitResponse, err error) {
		commitErr = err
	})
	return commitErr
}

func (c *Consumer) pauseTopic(cl *kgo.Client, topic string, cause error) error {
	c.mu.Lock()
	if _, ok := c.pausedTopics[topic]; ok {
		c.mu.Unlock()
		return nil
	}
	c.pausedTopics[topic] = pausedTopic{
		cause:    cause,
		pausedAt: time.Now(),
	}
	offsets := c.stopWorkersLocked(func(key recordKey) bool {
		return key.topic == topic
	})
	c.mu.Unlock()

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
	c.mu.RLock()
	defer c.mu.RUnlock()

	_, ok := c.pausedTopics[topic]
	return ok
}

func (c *Consumer) stopWorkersLocked(match func(recordKey) bool) map[string]map[int32]kgo.EpochOffset {
	offsets := make(map[string]map[int32]kgo.EpochOffset)

	for key, worker := range c.workers {
		if !match(key) {
			continue
		}

		offset, ok := worker.stop()
		delete(c.workers, key)
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

func (c *Consumer) stopWorkersForPartitions(partitions map[string][]int32) map[string]map[int32]kgo.EpochOffset {
	allowed := make(map[recordKey]struct{})
	for topic, partitionIDs := range partitions {
		for _, partition := range partitionIDs {
			allowed[recordKey{topic: topic, partition: partition}] = struct{}{}
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	return c.stopWorkersLocked(func(key recordKey) bool {
		_, ok := allowed[key]
		return ok
	})
}

func (c *Consumer) stopWorkersForLostPartitions(partitions map[string][]int32) {
	allowed := make(map[recordKey]struct{})
	for topic, partitionIDs := range partitions {
		for _, partition := range partitionIDs {
			allowed[recordKey{topic: topic, partition: partition}] = struct{}{}
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	_ = c.stopWorkersLocked(func(key recordKey) bool {
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

	offsets := c.stopWorkersForPartitions(partitions)
	if len(offsets) == 0 {
		return
	}

	if err := c.commitOffsets(ctx, cl, offsets); err != nil {
		commitErr := fmt.Errorf("failed to commit processed offsets on revoke: %w", err)
		c.log.ErrorContext(ctx, "failed to commit processed offsets on revoke", "err", commitErr)
		c.fail(commitErr)
	}
}

func (c *Consumer) onPartitionsLost(ctx context.Context, partitions map[string][]int32) {
	c.log.WarnContext(ctx, "Kafka partitions lost; dropping in-memory commit progress", "partitions", partitions)
	c.stopWorkersForLostPartitions(partitions)
}
