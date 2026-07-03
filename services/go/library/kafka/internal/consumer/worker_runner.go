package consumer

import (
	"context"
	"log/slog"

	"github.com/twmb/franz-go/pkg/kgo"
)

// WorkerRunner manages per-partition worker goroutines and their lifecycle.
// It owns the process semaphore for bounding concurrent handler execution.
type WorkerRunner struct {
	run        *RunState
	committer  *Committer
	executor   *RecordExecutor
	registry   *PartitionRegistry
	pauses     *PauseRegistry
	logger     *slog.Logger
	processSem chan struct{}
	notifyCap  func()
	dlqWriter  func(ctx context.Context, record *kgo.Record) error
}

// NewWorkerRunner creates a worker runner with the given dependencies.
// notifyCapacity is called when a batch is dequeued (wired to Dispatcher.NotifyCapacity).
// dlqWriter publishes enriched records to the DLQ topic.
func NewWorkerRunner(
	logger *slog.Logger,
	run *RunState,
	committer *Committer,
	executor *RecordExecutor,
	registry *PartitionRegistry,
	pauses *PauseRegistry,
	maxConcurrent int,
	notifyCapacity func(),
	dlqWriter func(ctx context.Context, record *kgo.Record) error,
) *WorkerRunner {
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	return &WorkerRunner{
		run:        run,
		committer:  committer,
		executor:   executor,
		registry:   registry,
		pauses:     pauses,
		logger:     logger,
		processSem: make(chan struct{}, maxConcurrent),
		notifyCap:  notifyCapacity,
		dlqWriter:  dlqWriter,
	}
}

// SetNotifyCapacity updates the capacity notification callback after construction.
// Used to break circular dependency between WorkerRunner and Dispatcher.
func (wr *WorkerRunner) SetNotifyCapacity(fn func()) {
	wr.notifyCap = fn
}

// Start launches a goroutine for this partition under the run's wait group.
// Must be called before the first enqueue to avoid NotifyCapacity deadlock.
func (wr *WorkerRunner) Start(state *PartitionState, client WorkerClient) {
	wr.run.Go(func() {
		wr.partitionLoop(state, client)
	})
}

func (wr *WorkerRunner) partitionLoop(state *PartitionState, client WorkerClient) {
	defer state.MarkStopped()

	for {
		select {
		case <-state.ctx.Done():
			return
		case records, ok := <-state.queue:
			if !ok {
				return
			}

			state.OnDequeue(records)
			wr.notifyCap()

			// Resume partition if backpressure is cleared and topic is not paused.
			// The low-watermark check is handled inside TryResumeBackpressure to
			// provide hysteresis against rapid pause/resume cycles.
			if !wr.pauses.IsPaused(state.Key().Topic) {
				if state.TryResumeBackpressure() {
					client.ResumeFetchPartitions(map[string][]int32{
						state.Key().Topic: {state.Key().Partition},
					})
				}
			}

			if state.subscription.BatchHandler != nil {
				wr.processBatch(state.ctx, state, records, client)
			} else {
				wr.processRecords(state.ctx, state, records, client)
			}
		}
	}
}

func (wr *WorkerRunner) processRecords(
	ctx context.Context,
	state *PartitionState,
	records []*kgo.Record,
	client WorkerClient,
) {
	var lastResolved *kgo.Record

	for _, record := range records {
		if wr.pauses.IsPaused(state.Key().Topic) {
			continue
		}

		if state.subscription.AckMode == AckModeAtMostOnce {
			offsets := recordsToOffsets([]*kgo.Record{record})
			if err := client.CommitOffsetsSync(ctx, offsets); err != nil {
				wr.run.Fail(err)
				return
			}
		}

		if err := wr.acquireProcessSlot(ctx); err != nil {
			if lastResolved != nil && state.AdvanceCommitOffset(lastResolved) && wr.registry.MarkDirty(state) {
				wr.committer.RequestFlush()
			}
			return
		}
		result := wr.executor.ExecuteRecord(ctx, state.subscription, record, wr.dlqWriter)
		wr.releaseProcessSlot()

		if result.PauseTopic {
			wr.logger.WarnContext(ctx, "Kafka topic paused after retry exhaustion",
				"topic", record.Topic, "err", result.Cause)
			wr.pauses.Pause(record.Topic, result.Cause)
			client.PauseFetchTopics(record.Topic)
			continue
		}

		if result.Cause != nil {
			if lastResolved != nil && state.AdvanceCommitOffset(lastResolved) && wr.registry.MarkDirty(state) {
				wr.committer.RequestFlush()
			}
			wr.run.Fail(result.Cause)
			return
		}

		if state.subscription.AckMode == AckModeAtLeastOnce && result.Resolved {
			lastResolved = record
		}
	}

	if lastResolved != nil && state.AdvanceCommitOffset(lastResolved) && wr.registry.MarkDirty(state) {
		wr.committer.RequestFlush()
	}
}

func (wr *WorkerRunner) processBatch(
	ctx context.Context,
	state *PartitionState,
	records []*kgo.Record,
	client WorkerClient,
) {
	if len(records) == 0 {
		return
	}

	if wr.pauses.IsPaused(state.Key().Topic) {
		return
	}

	if state.subscription.AckMode == AckModeAtMostOnce {
		offsets := recordsToOffsets(records)
		if err := client.CommitOffsetsSync(ctx, offsets); err != nil {
			wr.run.Fail(err)
			return
		}
	}

	if err := wr.acquireProcessSlot(ctx); err != nil {
		return
	}
	resolvedCount, cause, pauseTopic := wr.executor.ExecuteBatch(ctx, state.subscription, records, wr.dlqWriter)
	wr.releaseProcessSlot()

	if pauseTopic {
		wr.logger.WarnContext(ctx, "Kafka topic paused after retry exhaustion",
			"topic", records[0].Topic, "err", cause)
		wr.pauses.Pause(records[0].Topic, cause)
		client.PauseFetchTopics(records[0].Topic)
		return
	}

	if cause != nil {
		wr.run.Fail(cause)
		return
	}

	if state.subscription.AckMode == AckModeAtLeastOnce && resolvedCount > 0 {
		lastResolved := records[resolvedCount-1]
		if state.AdvanceCommitOffset(lastResolved) && wr.registry.MarkDirty(state) {
			wr.committer.RequestFlush()
		}
	}
}

func (wr *WorkerRunner) acquireProcessSlot(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case wr.processSem <- struct{}{}:
		return nil
	}
}

func (wr *WorkerRunner) releaseProcessSlot() {
	select {
	case <-wr.processSem:
	default:
	}
}
