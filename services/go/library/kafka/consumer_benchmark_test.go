package kafka

import (
	"context"
	"sync"
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"
)

// These benchmarks isolate the partition-queue orchestration change from the
// handler / commit path so we can compare the old single-record queue behavior
// against the current batched queue behavior under the same dispatch flow.
func BenchmarkPartitionQueueModes(b *testing.B) {
	cases := []struct {
		name                string
		partitions          int
		recordsPerPartition int
		queueCapacity       int
	}{
		{
			name:                "hot-partition",
			partitions:          1,
			recordsPerPartition: 1024,
			queueCapacity:       64,
		},
		{
			name:                "balanced-8-partitions",
			partitions:          8,
			recordsPerPartition: 256,
			queueCapacity:       64,
		},
		{
			name:                "small-balanced-fetch",
			partitions:          8,
			recordsPerPartition: 8,
			queueCapacity:       64,
		},
	}

	for _, tc := range cases {
		tc := tc
		b.Run(tc.name+"/legacy-single-record", func(b *testing.B) {
			benchmarkPartitionQueueMode(b, tc.partitions, tc.recordsPerPartition, tc.queueCapacity, benchmarkQueueModeLegacy)
		})
		b.Run(tc.name+"/batched", func(b *testing.B) {
			benchmarkPartitionQueueMode(b, tc.partitions, tc.recordsPerPartition, tc.queueCapacity, benchmarkQueueModeBatched)
		})
	}
}

type benchmarkQueueMode int

const (
	benchmarkQueueModeLegacy benchmarkQueueMode = iota
	benchmarkQueueModeBatched
)

type benchmarkPendingBatch struct {
	batch partitionRecordBatch
	next  int
}

type legacyPartitionQueueState struct {
	queue     chan *kgo.Record
	mu        sync.Mutex
	accepting bool
}

func newLegacyPartitionQueueState(queueCapacity int) *legacyPartitionQueueState {
	return &legacyPartitionQueueState{
		queue:     make(chan *kgo.Record, queueCapacity),
		mu:        sync.Mutex{},
		accepting: true,
	}
}

func (s *legacyPartitionQueueState) tryEnqueue(record *kgo.Record) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.accepting {
		return false
	}

	select {
	case s.queue <- record:
		return true
	default:
		return false
	}
}

func (s *legacyPartitionQueueState) drainOne() bool {
	select {
	case <-s.queue:
		return true
	default:
		return false
	}
}

func benchmarkPartitionQueueMode(
	b *testing.B,
	partitions int,
	recordsPerPartition int,
	queueCapacity int,
	mode benchmarkQueueMode,
) {
	consumer := newTestConsumer()
	if err := consumer.registerSubscription(testSubscription("topic-a"), false); err != nil {
		b.Fatalf("failed to register benchmark subscription: %v", err)
	}

	records := benchmarkRecords(partitions, recordsPerPartition)
	batches, err := consumer.partitionBatches(records)
	if err != nil {
		b.Fatalf("failed to create partition batches: %v", err)
	}

	b.ReportAllocs()

	legacyStates := make(map[recordKey]*legacyPartitionQueueState, len(batches))
	batchedStates := make(map[recordKey]*partitionState, len(batches))
	for _, batch := range batches {
		legacyStates[batch.key] = newLegacyPartitionQueueState(queueCapacity)
		batchedStates[batch.key] = newPartitionState(context.Background(), batch.key, batch.subscription, queueCapacity)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		switch mode {
		case benchmarkQueueModeLegacy:
			benchmarkLegacyPartitionQueueDispatch(batches, legacyStates)
		case benchmarkQueueModeBatched:
			benchmarkBatchedPartitionQueueDispatch(batches, batchedStates)
		default:
			b.Fatalf("unsupported benchmark queue mode: %d", mode)
		}
	}
}

func benchmarkLegacyPartitionQueueDispatch(batches []partitionRecordBatch, states map[recordKey]*legacyPartitionQueueState) {
	pending := make([]benchmarkPendingBatch, 0, len(batches))
	for _, batch := range batches {
		pending = append(pending, benchmarkPendingBatch{
			batch: batch,
			next:  0,
		})
	}

	for len(pending) > 0 {
		progressed := false
		nextPending := make([]benchmarkPendingBatch, 0, len(pending))

		for _, cursor := range pending {
			state := states[cursor.batch.key]

			for cursor.next < len(cursor.batch.records) {
				if !state.tryEnqueue(cursor.batch.records[cursor.next]) {
					nextPending = append(nextPending, cursor)
					break
				}

				progressed = true
				cursor.next++
			}
		}

		if len(nextPending) == 0 {
			break
		}
		if !progressed && !drainLegacyPartitionQueueStates(states) {
			panic("legacy benchmark dispatch made no progress")
		}

		pending = nextPending
	}

	for drainLegacyPartitionQueueStates(states) {
	}
}

func benchmarkBatchedPartitionQueueDispatch(batches []partitionRecordBatch, states map[recordKey]*partitionState) {
	pending := make([]benchmarkPendingBatch, 0, len(batches))
	for _, batch := range batches {
		pending = append(pending, benchmarkPendingBatch{
			batch: batch,
			next:  0,
		})
	}

	for len(pending) > 0 {
		progressed := false
		nextPending := make([]benchmarkPendingBatch, 0, len(pending))

		for _, cursor := range pending {
			state := states[cursor.batch.key]

			for cursor.next < len(cursor.batch.records) {
				enqueued, _ := state.tryEnqueueRecords(cursor.batch.records[cursor.next:])
				if enqueued == 0 {
					nextPending = append(nextPending, cursor)
					break
				}

				progressed = true
				cursor.next += enqueued
			}
		}

		if len(nextPending) == 0 {
			break
		}
		if !progressed && !drainBatchedPartitionQueueStates(states) {
			panic("batched benchmark dispatch made no progress")
		}

		pending = nextPending
	}

	for drainBatchedPartitionQueueStates(states) {
	}
}

func drainLegacyPartitionQueueStates(states map[recordKey]*legacyPartitionQueueState) bool {
	progressed := false

	for _, state := range states {
		if state.drainOne() {
			progressed = true
		}
	}

	return progressed
}

func drainBatchedPartitionQueueStates(states map[recordKey]*partitionState) bool {
	progressed := false

	for _, state := range states {
		select {
		case records := <-state.queue:
			state.onDequeueBatch(records)
			progressed = true
		default:
		}
	}

	return progressed
}

func benchmarkRecords(partitions, recordsPerPartition int) []*kgo.Record {
	records := make([]*kgo.Record, 0, partitions*recordsPerPartition)

	for offset := 0; offset < recordsPerPartition; offset++ {
		for partition := 0; partition < partitions; partition++ {
			records = append(records, &kgo.Record{
				Topic:     "topic-a",
				Partition: int32(partition),
				Offset:    int64(offset),
			})
		}
	}

	return records
}
