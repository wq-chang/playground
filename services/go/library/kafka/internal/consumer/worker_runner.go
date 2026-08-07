package consumer

import (
	"context"
	"log/slog"

	"github.com/twmb/franz-go/pkg/kgo"
)

// partitionResumer is the subset of *kgo.Client used by WorkerRunner to
// resume Kafka fetches for a partition when backpressure clears.
type partitionResumer interface {
	ResumeFetchPartitions(topicPartitions map[string][]int32)
}

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
	capacityCh chan struct{}
	dlqWriter  func(ctx context.Context, record *kgo.Record) error
	resumer    partitionResumer
}

// NewWorkerRunner creates a worker runner with the given dependencies.
// capacityCh is signaled (non-blocking) when a batch is dequeued so the
// dispatcher can retry stalled dispatches. dlqWriter publishes enriched
// records to the DLQ topic.
func NewWorkerRunner(
	logger *slog.Logger,
	run *RunState,
	committer *Committer,
	executor *RecordExecutor,
	registry *PartitionRegistry,
	pauses *PauseRegistry,
	maxConcurrent int,
	capacityCh chan struct{},
	dlqWriter func(ctx context.Context, record *kgo.Record) error,
	resumer partitionResumer,
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
		capacityCh: capacityCh,
		dlqWriter:  dlqWriter,
		resumer:    resumer,
	}
}

// Start launches a goroutine for this partition under the run's wait group.
func (wr *WorkerRunner) Start(state *PartitionState) {
	wr.run.Go(func() {
		wr.partitionLoop(state)
	})
}

func (wr *WorkerRunner) partitionLoop(state *PartitionState) {
	defer state.MarkStopped()

	for {
		records, ok := state.Dequeue(state.Context())
		if !ok {
			return
		}

		select {
		case wr.capacityCh <- struct{}{}:
		default:
		}

		// Resume partition if backpressure is cleared and topic is not paused.
		// The low-watermark check is handled inside TryResumeBackpressure to
		// provide hysteresis against rapid pause/resume cycles.
		if !wr.pauses.IsPaused(state.Key().Topic) {
			if state.TryResumeBackpressure() {
				wr.resumer.ResumeFetchPartitions(map[string][]int32{
					state.Key().Topic: {state.Key().Partition},
				})
			}
		}

		if state.Subscription().BatchHandler != nil {
			wr.processBatch(state.Context(), state, records)
		} else {
			wr.processRecords(state.Context(), state, records)
		}
	}
}

func (wr *WorkerRunner) processRecords(
	ctx context.Context,
	state *PartitionState,
	records []*kgo.Record,
) {
	var lastResolved *kgo.Record

	for _, record := range records {
		if wr.pauses.IsPaused(state.Key().Topic) {
			continue
		}

		if state.Subscription().AckMode == AckModeAtMostOnce {
			if err := wr.committer.CommitRecords(ctx, record); err != nil {
				if ctx.Err() != nil {
					// Partition is closing (rebalance revoke or shutdown) — the
					// commit failed because of cancellation, not Kafka.
					return
				}
				wr.run.Fail(err)
				return
			}
		}

		if err := wr.acquireProcessSlot(ctx); err != nil {
			if lastResolved != nil && wr.registry.AdvanceStateCommitOffset(state, lastResolved) {
				wr.committer.RequestFlush()
			}
			return
		}
		result := wr.executor.ExecuteRecord(ctx, state.Subscription(), record, wr.dlqWriter)
		wr.releaseProcessSlot()

		if result.PauseTopic {
			wr.logger.WarnContext(
				ctx,
				"Kafka topic paused after retry exhaustion",
				"topic", record.Topic,
				"err", result.Cause,
			)
			wr.pauses.Pause(record.Topic, result.Cause)
			continue
		}

		if result.Cause != nil {
			if lastResolved != nil && wr.registry.AdvanceStateCommitOffset(state, lastResolved) {
				wr.committer.RequestFlush()
			}
			if ctx.Err() != nil {
				// Partition is closing (rebalance revoke or shutdown); abort
				// the partition loop without failing the run so Finalize can
				// commit and the consumer keeps running.
				return
			}
			wr.run.Fail(result.Cause)
			return
		}

		if state.Subscription().AckMode == AckModeAtLeastOnce && result.Resolved {
			lastResolved = record
		}
	}

	if lastResolved != nil && wr.registry.AdvanceStateCommitOffset(state, lastResolved) {
		wr.committer.RequestFlush()
	}
}

func (wr *WorkerRunner) processBatch(
	ctx context.Context,
	state *PartitionState,
	records []*kgo.Record,
) {
	if len(records) == 0 {
		return
	}

	if wr.pauses.IsPaused(state.Key().Topic) {
		return
	}

	if state.Subscription().AckMode == AckModeAtMostOnce {
		if err := wr.committer.CommitRecords(ctx, records...); err != nil {
			if ctx.Err() != nil {
				// Partition is closing (rebalance revoke or shutdown) — the
				// commit failed because of cancellation, not Kafka.
				return
			}
			wr.run.Fail(err)
			return
		}
	}

	if err := wr.acquireProcessSlot(ctx); err != nil {
		return
	}
	resolvedCount, cause, pauseTopic := wr.executor.ExecuteBatch(ctx, state.Subscription(), records, wr.dlqWriter)
	wr.releaseProcessSlot()

	if pauseTopic {
		wr.logger.WarnContext(
			ctx,
			"Kafka topic paused after retry exhaustion",
			"topic", records[0].Topic,
			"err", cause,
		)
		wr.pauses.Pause(records[0].Topic, cause)
		wr.commitResolvedPrefix(state, records, resolvedCount)
		return
	}

	if cause != nil {
		wr.commitResolvedPrefix(state, records, resolvedCount)
		if ctx.Err() != nil {
			// Partition is closing (rebalance revoke or shutdown); abort
			// the partition loop without failing the run so Finalize can
			// commit and the consumer keeps running.
			return
		}
		wr.run.Fail(cause)
		return
	}

	wr.commitResolvedPrefix(state, records, resolvedCount)
}

// commitResolvedPrefix advances the commit offset for a successfully handled
// prefix of a batch, mirroring the single-record path's lastResolved handling.
// No-op outside at-least-once mode (at-most-once commits before processing).
func (wr *WorkerRunner) commitResolvedPrefix(
	state *PartitionState,
	records []*kgo.Record,
	resolvedCount int,
) {
	if resolvedCount <= 0 || state.Subscription().AckMode != AckModeAtLeastOnce {
		return
	}
	lastResolved := records[resolvedCount-1]
	if wr.registry.AdvanceStateCommitOffset(state, lastResolved) {
		wr.committer.RequestFlush()
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
