package consumer

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

// OffsetClient is the narrow interface Committer uses to commit offsets.
type OffsetClient interface {
	CommitOffsetsSync(ctx context.Context, offsets map[string]map[int32]kgo.EpochOffset) error
}

// OffsetClientFunc wraps a function as an OffsetClient for adapters.
type OffsetClientFunc func(ctx context.Context, offsets map[string]map[int32]kgo.EpochOffset) error

func (f OffsetClientFunc) CommitOffsetsSync(ctx context.Context, offsets map[string]map[int32]kgo.EpochOffset) error {
	return f(ctx, offsets)
}

// CommitConfig configures the commit loop timing and timeouts.
type CommitConfig struct {
	FlushInterval      time.Duration
	DebounceInterval   time.Duration
	FinalCommitTimeout time.Duration
}

func (c CommitConfig) withDefaults() CommitConfig {
	if c.FlushInterval <= 0 {
		c.FlushInterval = 500 * time.Millisecond
	}
	if c.DebounceInterval <= 0 {
		c.DebounceInterval = 100 * time.Millisecond
	}
	if c.FinalCommitTimeout <= 0 {
		c.FinalCommitTimeout = 30 * time.Second
	}
	return c
}

// Committer owns commit-loop timing, offset flushing, commit serialization,
// and final-commit behavior.
type Committer struct {
	registry *PartitionRegistry
	client   OffsetClient
	flushCh  chan struct{}
	commitMu chan struct{}
	cfg      CommitConfig
}

// NewCommitter creates a commit owner with the given dependencies.
func NewCommitter(
	registry *PartitionRegistry,
	client OffsetClient,
	cfg CommitConfig,
) *Committer {
	cfg = cfg.withDefaults()
	cm := &Committer{
		registry: registry,
		client:   client,
		cfg:      cfg,
		flushCh:  make(chan struct{}, 1),
		commitMu: make(chan struct{}, 1),
	}
	cm.commitMu <- struct{}{} // initial token — serialization is available
	return cm
}

// RequestFlush signals the commit loop to flush dirty offsets.
func (cm *Committer) RequestFlush() {
	select {
	case cm.flushCh <- struct{}{}:
	default:
	}
}

// Flush immediately flushes all dirty offsets.
func (cm *Committer) Flush(ctx context.Context) error {
	return cm.flush(ctx)
}

func (cm *Committer) flush(ctx context.Context) error {
	if err := cm.acquireCommitMu(ctx); err != nil {
		return err
	}
	defer cm.releaseCommitMu()

	offsets := cm.snapshotDirtyOffsets()
	if len(offsets) == 0 {
		return nil
	}

	if err := cm.client.CommitOffsetsSync(ctx, offsets); err != nil {
		return fmt.Errorf("failed to commit processed offsets: %w", err)
	}

	cm.markCommittedOffsets(offsets)
	return nil
}

// Run runs the commit loop until the context ends. It flushes dirty offsets
// on a periodic timer and on RequestFlush with debounce coalescing.
func (cm *Committer) Run(ctx context.Context) error {
	ticker := time.NewTicker(cm.cfg.FlushInterval)
	defer ticker.Stop()

	var (
		debounce  *time.Timer
		debounceC <-chan time.Time
	)
	defer func() {
		if debounce != nil {
			debounce.Stop()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := cm.flush(ctx); err != nil && !errors.Is(err, context.Canceled) {
				return err
			}
		case <-cm.flushCh:
			if debounce == nil {
				debounce = time.NewTimer(cm.cfg.DebounceInterval)
				debounceC = debounce.C
				continue
			}
			debounce.Stop()
			debounce.Reset(cm.cfg.DebounceInterval)
		case <-debounceC:
			debounce.Stop()
			debounce = nil
			debounceC = nil
			if err := cm.flush(ctx); err != nil && !errors.Is(err, context.Canceled) {
				return err
			}
		}
	}
}

func (cm *Committer) snapshotDirtyOffsets() map[string]map[int32]kgo.EpochOffset {
	dirtyStates := cm.registry.SnapshotDirtyStates()
	if len(dirtyStates) == 0 {
		return nil
	}

	states := make([]*PartitionState, 0, len(dirtyStates))
	for _, state := range dirtyStates {
		states = append(states, state)
	}
	return cm.registry.SnapshotOffsets(states)
}

func (cm *Committer) markCommittedOffsets(offsets map[string]map[int32]kgo.EpochOffset) {
	if len(offsets) == 0 {
		return
	}

	for topic, partitions := range offsets {
		for partition, offset := range partitions {
			key := Key{Topic: topic, Partition: partition}
			state, ok := cm.registry.Get(key)
			if !ok {
				cm.registry.ClearDirty(key)
				continue
			}
			cm.registry.MarkStateCommitted(state, offset)
		}
	}
}

func (cm *Committer) acquireCommitMu(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-cm.commitMu:
		return nil
	}
}

func (cm *Committer) releaseCommitMu() {
	cm.commitMu <- struct{}{}
}

// CommitRecords commits one or more records for at-most-once processing.
func (cm *Committer) CommitRecords(ctx context.Context, records ...*kgo.Record) error {
	if len(records) == 0 {
		return nil
	}

	if err := cm.acquireCommitMu(ctx); err != nil {
		return err
	}
	defer cm.releaseCommitMu()

	return cm.client.CommitOffsetsSync(ctx, recordsToOffsets(records))
}

// recordsToOffsets builds an offset commit map from a slice of records.
func recordsToOffsets(records []*kgo.Record) map[string]map[int32]kgo.EpochOffset {
	offsets := make(map[string]map[int32]kgo.EpochOffset)
	for _, record := range records {
		if record == nil {
			continue
		}
		topic := record.Topic
		partition := record.Partition
		offset := kgo.EpochOffset{
			Epoch:  record.LeaderEpoch,
			Offset: record.Offset + 1,
		}
		if _, ok := offsets[topic]; !ok {
			offsets[topic] = make(map[int32]kgo.EpochOffset)
		}
		offsets[topic][partition] = offset
	}
	if len(offsets) == 0 {
		return nil
	}
	return offsets
}

// Finalize waits for the selected states to drain, then commits the final
// offsets and cleans up the states from the registry.
func (cm *Committer) Finalize(
	drainCtx context.Context,
	commitCtx context.Context,
	states []*PartitionState,
	errMessage string,
) error {
	if len(states) == 0 {
		return nil
	}

	if drainCtx == nil {
		return fmt.Errorf("%s: drainCtx must not be nil", errMessage)
	}

	if err := cm.waitForPartitions(drainCtx, states); err != nil {
		return err
	}

	if commitCtx == nil {
		var cancel context.CancelFunc
		commitCtx, cancel = context.WithTimeout(context.Background(), cm.cfg.FinalCommitTimeout)
		defer cancel()
	}

	if err := cm.acquireCommitMu(commitCtx); err != nil {
		return fmt.Errorf("%s: %w", errMessage, err)
	}
	defer cm.releaseCommitMu()

	offsets := cm.registry.SnapshotOffsets(states)
	if len(offsets) == 0 {
		cm.registry.Cleanup(states)
		return nil
	}

	err := cm.client.CommitOffsetsSync(commitCtx, offsets)
	cm.registry.Cleanup(states)
	if err != nil {
		return fmt.Errorf("%s: %w", errMessage, err)
	}
	return nil
}

func (cm *Committer) waitForPartitions(ctx context.Context, states []*PartitionState) error {
	if len(states) == 0 {
		return nil
	}

	for _, state := range states {
		select {
		case <-state.Done():
		case <-ctx.Done():
			return fmt.Errorf(
				"failed waiting for topic %q partition %d to drain: %w",
				state.Key().Topic,
				state.Key().Partition,
				ctx.Err(),
			)
		}
	}

	return nil
}
