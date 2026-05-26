package kafka

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/twmb/franz-go/pkg/kgo"
)

type partitionWorker struct {
	ctx                context.Context
	queue              chan *kgo.Record
	cancel             context.CancelFunc
	done               chan struct{}
	key                recordKey
	subscription       Subscription
	nextCommitOffset   kgo.EpochOffset
	committedOffset    kgo.EpochOffset
	stopOnce           sync.Once
	mu                 sync.Mutex
	accepting          bool
	backpressurePaused bool
	dirty              bool
}

func newPartitionWorker(
	parent context.Context,
	key recordKey,
	subscription Subscription,
	queueCapacity int,
) *partitionWorker {
	workerCtx, cancel := context.WithCancel(parent)
	return &partitionWorker{
		ctx:                workerCtx,
		queue:              make(chan *kgo.Record, queueCapacity),
		cancel:             cancel,
		done:               make(chan struct{}),
		key:                key,
		subscription:       subscription,
		nextCommitOffset:   kgo.EpochOffset{},
		committedOffset:    kgo.EpochOffset{},
		stopOnce:           sync.Once{},
		mu:                 sync.Mutex{},
		accepting:          true,
		backpressurePaused: false,
		dirty:              false,
	}
}

func (c *Consumer) enqueueRecord(cl *kgo.Client, worker *partitionWorker, record *kgo.Record) (bool, error) {
	if err := worker.ctx.Err(); err != nil {
		return false, err
	}

	enqueued, queueLen := worker.tryEnqueue(record)
	if enqueued {
		c.maybePausePartitionForBackpressure(cl, worker, queueLen)
		return true, nil
	}

	c.maybePausePartitionForBackpressure(cl, worker, queueLen)
	return false, nil
}

func (c *Consumer) maybePausePartitionForBackpressure(cl *kgo.Client, worker *partitionWorker, queueLen int) {
	if queueLen < c.partitionQueueHighWatermark() {
		return
	}
	if !worker.markBackpressurePaused() {
		return
	}
	if cl == nil {
		return
	}

	cl.PauseFetchPartitions(map[string][]int32{
		worker.key.topic: {worker.key.partition},
	})
}

func (c *Consumer) maybeResumePartitionAfterDrain(cl *kgo.Client, worker *partitionWorker, queueLen int) {
	if queueLen > c.partitionQueueLowWatermark() || c.isTopicPaused(worker.key.topic) {
		return
	}
	if !worker.clearBackpressurePaused() {
		return
	}
	if cl == nil {
		return
	}

	cl.ResumeFetchPartitions(map[string][]int32{
		worker.key.topic: {worker.key.partition},
	})
}

func (w *partitionWorker) run(c *Consumer, cl *kgo.Client) {
	defer close(w.done)

	for {
		select {
		case <-w.ctx.Done():
			return
		case record := <-w.queue:
			queueLen := w.onDequeue()
			c.signalDispatchCapacity()
			c.maybeResumePartitionAfterDrain(cl, w, queueLen)

			if c.isTopicPaused(record.Topic) {
				continue
			}

			if err := c.acquireProcessSlot(w.ctx); err != nil {
				return
			}

			err := c.processWorkerRecord(w.ctx, cl, w, record)
			c.releaseProcessSlot()
			if err != nil && !errors.Is(err, context.Canceled) {
				c.fail(err)
				return
			}
		}
	}
}

func (c *Consumer) acquireProcessSlot(ctx context.Context) error {
	c.mu.RLock()
	sem := c.processSem
	c.mu.RUnlock()

	if sem == nil {
		return fmt.Errorf("process semaphore is not initialized")
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case sem <- struct{}{}:
		return nil
	}
}

func (c *Consumer) releaseProcessSlot() {
	c.mu.RLock()
	sem := c.processSem
	c.mu.RUnlock()

	if sem == nil {
		return
	}

	select {
	case <-sem:
	default:
	}
}

func (c *Consumer) processWorkerRecord(
	ctx context.Context,
	cl *kgo.Client,
	worker *partitionWorker,
	record *kgo.Record,
) error {
	switch worker.subscription.AckMode {
	case AckModeAtMostOnce:
		if err := c.commitRecord(ctx, cl, record); err != nil {
			return fmt.Errorf("failed to commit record before handling: %w", err)
		}
	case AckModeAtLeastOnce:
		// Manual offset commit happens after successful processing.
	default:
		return fmt.Errorf("unsupported ack mode: %d", worker.subscription.AckMode)
	}

	result, err := c.executeRecord(ctx, worker.subscription, record)
	if err != nil {
		return err
	}
	if result.pauseTopic {
		return c.pauseTopic(cl, record.Topic, result.cause)
	}
	if worker.subscription.AckMode == AckModeAtLeastOnce && result.resolved {
		if worker.advanceCommitOffset(record) {
			c.signalCommitLoop()
		}
	}

	return nil
}

func (w *partitionWorker) tryEnqueue(record *kgo.Record) (bool, int) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.accepting {
		return false, len(w.queue)
	}

	select {
	case w.queue <- record:
		return true, len(w.queue)
	default:
		return false, len(w.queue)
	}
}

func (w *partitionWorker) onDequeue() int {
	w.mu.Lock()
	defer w.mu.Unlock()

	return len(w.queue)
}

func (w *partitionWorker) markBackpressurePaused() bool {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.accepting || w.backpressurePaused {
		return false
	}

	w.backpressurePaused = true
	return true
}

func (w *partitionWorker) clearBackpressurePaused() bool {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.accepting || !w.backpressurePaused {
		return false
	}

	w.backpressurePaused = false
	return true
}

func (w *partitionWorker) advanceCommitOffset(record *kgo.Record) bool {
	nextOffset := kgo.EpochOffset{
		Epoch:  record.LeaderEpoch,
		Offset: record.Offset + 1,
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.nextCommitOffset.Less(nextOffset) {
		return false
	}

	w.nextCommitOffset = nextOffset
	w.dirty = w.committedOffset.Less(nextOffset)
	return true
}

func (w *partitionWorker) snapshotDirtyOffset() (kgo.EpochOffset, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.dirty || !w.committedOffset.Less(w.nextCommitOffset) {
		return kgo.EpochOffset{}, false
	}

	return w.nextCommitOffset, true
}

func (w *partitionWorker) markCommitted(offset kgo.EpochOffset) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.committedOffset.Less(offset) {
		w.committedOffset = offset
	}
	if !w.committedOffset.Less(w.nextCommitOffset) {
		w.dirty = false
	}
}

func (w *partitionWorker) stop() (kgo.EpochOffset, bool) {
	var (
		offset kgo.EpochOffset
		ok     bool
	)

	w.stopOnce.Do(func() {
		w.mu.Lock()
		w.accepting = false
		w.backpressurePaused = false
		offset = w.nextCommitOffset
		ok = w.committedOffset.Less(offset)
		w.mu.Unlock()

		w.cancel()
	})

	return offset, ok
}
