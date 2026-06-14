// services/go/library/kafka/internal/consumer/partition_registry.go
package consumer

import (
	"context"
	"maps"
	"sync"

	"github.com/twmb/franz-go/pkg/kgo"
)

// PartitionRegistry owns the partition state map and dirty-state bookkeeping.
// It provides safe concurrent access to partition states for lookup, creation,
// lifecycle transitions (closing, abort), and cleanup.
//
// It does NOT start worker goroutines for new partitions — that is the
// responsibility of the dispatcher/worker runner (Step 7).
type PartitionRegistry struct {
	partitions map[Key]*PartitionState
	dirty      map[Key]*PartitionState
	mu         sync.Mutex
}

// NewPartitionRegistry creates an empty partition registry.
func NewPartitionRegistry() *PartitionRegistry {
	return &PartitionRegistry{
		partitions: make(map[Key]*PartitionState),
		dirty:      make(map[Key]*PartitionState),
		mu:         sync.Mutex{},
	}
}

// Get returns the partition state for a key, if one exists.
func (r *PartitionRegistry) Get(key Key) (*PartitionState, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ps, ok := r.partitions[key]
	return ps, ok
}

// GetOrCreate returns the existing partition state for a key, or creates a new
// one atomically under registry ownership. Returns created=true when a new
// state was created. The caller must start the worker goroutine separately.
func (r *PartitionRegistry) GetOrCreate(
	key Key,
	subscription Subscription,
	parent context.Context,
	queueCapacity int,
) (*PartitionState, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.partitions[key]; ok {
		return existing, false, nil
	}

	ps := NewPartitionState(parent, key, subscription, queueCapacity)
	r.partitions[key] = ps
	return ps, true, nil
}

// MarkDirty records that a partition has dirty commit progress.
// Returns false if the state is not tracked by this registry.
func (r *PartitionRegistry) MarkDirty(state *PartitionState) bool {
	if state == nil {
		return false
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	current, ok := r.partitions[state.Key()]
	if !ok || current != state {
		return false
	}
	r.dirty[state.Key()] = state
	return true
}

// ClearDirty removes dirty tracking for a partition. If state is non-nil,
// it only clears if the state pointer still matches the tracked one.
func (r *PartitionRegistry) ClearDirty(key Key, state *PartitionState) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if state == nil {
		delete(r.dirty, key)
		return
	}
	current, ok := r.dirty[key]
	if ok && current == state {
		delete(r.dirty, key)
	}
}

// SnapshotDirtyStates returns a stable snapshot of the currently dirty
// partition states. Returns nil if none are dirty.
func (r *PartitionRegistry) SnapshotDirtyStates() map[Key]*PartitionState {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.dirty) == 0 {
		return nil
	}

	snap := make(map[Key]*PartitionState, len(r.dirty))
	maps.Copy(snap, r.dirty)
	return snap
}

// BeginClosing marks the selected partitions as closing and returns the
// affected states. Partitions are matched by the given map of topic to
// partition IDs (the same format as Kafka rebalance callbacks).
func (r *PartitionRegistry) BeginClosing(partitions map[string][]int32) []*PartitionState {
	allowed := make(map[Key]struct{})
	for topic, partitionIDs := range partitions {
		for _, p := range partitionIDs {
			allowed[Key{Topic: topic, Partition: p}] = struct{}{}
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	states := make([]*PartitionState, 0, len(allowed))
	for key, state := range r.partitions {
		if _, ok := allowed[key]; !ok {
			continue
		}
		state.BeginClosing()
		states = append(states, state)
	}

	return states
}

// BeginClosingAll marks every tracked partition as closing and returns the
// affected states.
func (r *PartitionRegistry) BeginClosingAll() []*PartitionState {
	r.mu.Lock()
	defer r.mu.Unlock()

	states := make([]*PartitionState, 0, len(r.partitions))
	for _, state := range r.partitions {
		state.BeginClosing()
		states = append(states, state)
	}

	return states
}

// DropLost removes lost partitions and discards in-memory progress that can
// no longer be committed safely. The state is aborted (queue closed + context
// cancelled) and removed from the registry and dirty map.
func (r *PartitionRegistry) DropLost(partitions map[string][]int32) {
	allowed := make(map[Key]struct{})
	for topic, partitionIDs := range partitions {
		for _, p := range partitionIDs {
			allowed[Key{Topic: topic, Partition: p}] = struct{}{}
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for key, state := range r.partitions {
		if _, ok := allowed[key]; !ok {
			continue
		}
		state.Abort()
		delete(r.partitions, key)
		delete(r.dirty, key)
	}
}

// SnapshotOffsets builds the committable offset map for a set of states.
// Does not access registry internals — only calls SnapshotDirtyOffset on each state.
func (r *PartitionRegistry) SnapshotOffsets(states []*PartitionState) map[string]map[int32]kgo.EpochOffset {
	offsets := make(map[string]map[int32]kgo.EpochOffset)

	for _, state := range states {
		if state == nil {
			continue
		}
		offset, ok := state.SnapshotDirtyOffset()
		if !ok {
			continue
		}

		key := state.Key()
		partitionsByTopic, exists := offsets[key.Topic]
		if !exists {
			partitionsByTopic = make(map[int32]kgo.EpochOffset)
			offsets[key.Topic] = partitionsByTopic
		}
		partitionsByTopic[key.Partition] = offset
	}

	if len(offsets) == 0 {
		return nil
	}
	return offsets
}

// Cleanup removes stopped states from the registry and dirty-state map.
// Uses identity checks (pointer comparison) to avoid removing states that
// have been recreated under the same key.
func (r *PartitionRegistry) Cleanup(states []*PartitionState) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, state := range states {
		if state == nil {
			continue
		}

		key := state.Key()
		if current, ok := r.partitions[key]; ok && current == state {
			delete(r.partitions, key)
		}
		if current, ok := r.dirty[key]; ok && current == state {
			delete(r.dirty, key)
		}
	}
}
