package kafka

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/twmb/franz-go/pkg/kgo"
)

// Run starts the consumer loop.
//
// Run blocks until the context is canceled or a fatal processing error occurs.
// Only one active Run call is allowed at a time for a given Consumer.
func (c *Consumer) Run(ctx context.Context) error {
	c.log.InfoContext(
		ctx,
		"Starting Kafka consumer loop",
		"groupId", c.cfg.groupId,
		"workers", c.cfg.workers,
	)

	if c.client == nil || c.client.kgoClient == nil {
		return fmt.Errorf("consumer client is not initialized")
	}

	runCtx, err := c.beginRun(ctx)
	if err != nil {
		return err
	}

	cl := c.client.kgoClient
	c.runWG.Add(1)
	go c.runCommitLoop(runCtx, cl)

	err = c.runClient(runCtx, cl)
	if err == nil && c.runFailure() == nil && ctx.Err() == nil {
		err = c.flushDirtyOffsetsWithTimeout(cl)
	}

	c.stopRun()
	c.runWG.Wait()

	runErr := c.runFailure()
	c.resetRunState()

	switch {
	case err == nil && runErr != nil:
		err = runErr
	case errors.Is(err, context.Canceled) && runErr != nil:
		err = runErr
	case errors.Is(err, context.Canceled) && ctx.Err() != nil:
		err = ctx.Err()
	case err == nil && ctx.Err() != nil:
		err = ctx.Err()
	}

	if err != nil {
		if ctx.Err() != nil && !errors.Is(err, runErr) {
			c.log.InfoContext(ctx, "Kafka consumer context cancelled, shutting down...")
			return ctx.Err()
		}
		c.log.Error("Kafka consumer loop error", "err", err)
		return err
	}

	return nil
}

func (c *Consumer) beginRun(parent context.Context) (context.Context, error) {
	c.runMu.Lock()
	defer c.runMu.Unlock()

	if c.runCtx != nil {
		return nil, fmt.Errorf("consumer run is already active")
	}

	runCtx, runCancel := context.WithCancel(parent)
	c.runCtx = runCtx
	c.runCancel = runCancel
	c.runErr = nil
	c.runErrOnce = sync.Once{}
	c.runWG = sync.WaitGroup{}
	c.commitSignal = make(chan struct{}, 1)
	c.dispatchSignal = make(chan struct{}, 1)
	c.processSem = make(chan struct{}, c.processConcurrency())
	c.workersMu.Lock()
	c.partitionStates = make(map[recordKey]*partitionState)
	c.dirtyStates = make(map[recordKey]*partitionState)
	c.workersMu.Unlock()

	return runCtx, nil
}

func (c *Consumer) stopRun() {
	c.runMu.RLock()
	runCancel := c.runCancel
	c.runMu.RUnlock()

	if runCancel != nil {
		runCancel()
	}
}

func (c *Consumer) resetRunState() {
	c.runMu.Lock()
	defer c.runMu.Unlock()

	c.runCtx = nil
	c.runCancel = nil
	c.runErr = nil
	c.commitSignal = nil
	c.dispatchSignal = nil
	c.processSem = nil
	c.workersMu.Lock()
	c.partitionStates = make(map[recordKey]*partitionState)
	c.dirtyStates = make(map[recordKey]*partitionState)
	c.workersMu.Unlock()
}

func (c *Consumer) fail(err error) {
	if err == nil {
		return
	}

	c.runMu.Lock()
	defer c.runMu.Unlock()

	c.runErrOnce.Do(func() {
		c.runErr = err
		if c.runCancel != nil {
			c.runCancel()
		}
	})
}

func (c *Consumer) runFailure() error {
	c.runMu.RLock()
	defer c.runMu.RUnlock()

	return c.runErr
}

func (c *Consumer) signalCommitLoop() {
	c.runMu.RLock()
	ch := c.commitSignal
	c.runMu.RUnlock()

	if ch == nil {
		return
	}

	select {
	case ch <- struct{}{}:
	default:
	}
}

func (c *Consumer) signalDispatchCapacity() {
	c.runMu.RLock()
	ch := c.dispatchSignal
	c.runMu.RUnlock()

	if ch == nil {
		return
	}

	select {
	case ch <- struct{}{}:
	default:
	}
}

func (c *Consumer) waitForDispatchCapacity(ctx context.Context) error {
	c.runMu.RLock()
	ch := c.dispatchSignal
	c.runMu.RUnlock()

	if ch == nil {
		return fmt.Errorf("dispatch signal is not initialized")
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-ch:
		return nil
	}
}

func (c *Consumer) acquireProcessSlot(ctx context.Context) error {
	c.runMu.RLock()
	sem := c.processSem
	c.runMu.RUnlock()

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
	c.runMu.RLock()
	sem := c.processSem
	c.runMu.RUnlock()

	if sem == nil {
		return
	}

	select {
	case <-sem:
	default:
	}
}

func (c *Consumer) processConcurrency() int {
	if c.cfg != nil && c.cfg.workers > 0 {
		return c.cfg.workers
	}
	return 1
}

func (c *Consumer) partitionQueueCapacity() int {
	capacity := defaultPartitionQueueCapacity
	if workers := c.processConcurrency(); workers > capacity {
		capacity = workers
	}
	return capacity
}

func (c *Consumer) partitionQueueHighWatermark() int {
	capacity := c.partitionQueueCapacity()
	if capacity <= 1 {
		return 1
	}
	return capacity - 1
}

func (c *Consumer) partitionQueueLowWatermark() int {
	return c.partitionQueueCapacity() / 2
}

func (c *Consumer) runPartitionState(state *partitionState, cl *kgo.Client) {
	defer close(state.done)

	for {
		select {
		case <-state.ctx.Done():
			return
		case records := <-state.queue:
			bufferedRecords := state.onDequeueBatch(records)
			c.signalDispatchCapacity()
			c.maybeResumePartitionAfterDrain(cl, state, bufferedRecords)

			if state.subscription.BatchHandler != nil {
				if len(records) == 0 || c.isTopicPaused(state.key.topic) {
					continue
				}
				if err := c.acquireProcessSlot(state.ctx); err != nil {
					return
				}

				err := func() error {
					defer c.releaseProcessSlot()
					return c.processPartitionBatch(state.ctx, cl, state, records)
				}()
				if err != nil && !errors.Is(err, context.Canceled) {
					c.fail(err)
					return
				}
				continue
			}

			for _, record := range records {
				if c.isTopicPaused(record.Topic) {
					continue
				}

				if err := c.acquireProcessSlot(state.ctx); err != nil {
					return
				}

				err := func() error {
					defer c.releaseProcessSlot()
					return c.processPartitionRecord(state.ctx, cl, state, record)
				}()
				if err != nil && !errors.Is(err, context.Canceled) {
					c.fail(err)
					return
				}
			}
		}
	}
}

func (c *Consumer) runClient(ctx context.Context, cl *kgo.Client) error {
	for {
		fetches := cl.PollRecords(ctx, -1)
		if fetches.IsClientClosed() {
			return nil
		}

		if err := fetches.Err(); err != nil {
			cl.AllowRebalance()
			if ctx.Err() != nil {
				return nil
			}
			c.log.Warn("Kafka poll error", "err", err)
			continue
		}

		records := fetches.Records()
		var dispatchErr error
		if len(records) > 0 {
			dispatchErr = c.dispatchRecords(ctx, cl, records)
		}
		cl.AllowRebalance()

		if dispatchErr != nil {
			return dispatchErr
		}
	}
}

func (c *Consumer) dispatchRecords(ctx context.Context, cl *kgo.Client, records []*kgo.Record) error {
	batches, err := c.partitionBatches(records)
	if err != nil {
		return err
	}
	if len(batches) == 0 {
		return nil
	}

	type pendingBatch struct {
		batch partitionRecordBatch
		next  int
	}

	pending := make([]pendingBatch, 0, len(batches))
	for _, batch := range batches {
		pending = append(pending, pendingBatch{
			batch: batch,
			next:  0,
		})
	}

	for len(pending) > 0 {
		progressed := false
		nextPending := make([]pendingBatch, 0, len(pending))

		for _, cursor := range pending {
			if c.isTopicPaused(cursor.batch.key.topic) {
				progressed = true
				continue
			}

			state, err := c.partitionStateFor(cursor.batch.key, cursor.batch.subscription)
			if err != nil {
				return err
			}
			if state == nil {
				progressed = true
				continue
			}

			for cursor.next < len(cursor.batch.records) {
				enqueued, err := c.enqueuePartitionRecords(cl, state, cursor.batch.records[cursor.next:])
				if err != nil {
					return err
				}
				if enqueued == 0 {
					nextPending = append(nextPending, cursor)
					break
				}

				progressed = true
				cursor.next += enqueued
			}
		}

		if len(nextPending) == 0 {
			return nil
		}
		if !progressed {
			if err := c.waitForDispatchCapacity(ctx); err != nil {
				return err
			}
		}
		pending = nextPending
	}

	return nil
}

func (c *Consumer) partitionStateFor(key recordKey, subscription Subscription) (*partitionState, error) {
	c.runMu.RLock()
	runCtx := c.runCtx
	c.runMu.RUnlock()
	if runCtx == nil {
		return nil, fmt.Errorf("consumer run is not active")
	}

	c.workersMu.RLock()
	if state, ok := c.partitionStates[key]; ok {
		c.workersMu.RUnlock()
		return state, nil
	}
	c.workersMu.RUnlock()

	c.workersMu.Lock()
	defer c.workersMu.Unlock()

	if state, ok := c.partitionStates[key]; ok {
		return state, nil
	}
	if c.isTopicPaused(key.topic) {
		return nil, nil
	}

	c.runMu.RLock()
	runCtx = c.runCtx
	c.runMu.RUnlock()
	if runCtx == nil {
		return nil, fmt.Errorf("consumer run is not active")
	}

	state := newPartitionState(runCtx, key, subscription, c.partitionQueueCapacity())
	c.partitionStates[key] = state

	var kgoClient *kgo.Client
	if c.client != nil {
		kgoClient = c.client.kgoClient
	}

	c.runWG.Add(1)
	go func() {
		defer c.runWG.Done()
		c.runPartitionState(state, kgoClient)
	}()

	return state, nil
}
