package consumer

import (
	"context"
	"fmt"

	"github.com/twmb/franz-go/pkg/kgo"
)

// Dispatcher owns partition batching, dispatch coordination, and backpressure
// management. It groups polled records by topic-partition, enqueues them into
// partition workers, and applies backpressure pauses when queues fill up.
type Dispatcher struct {
	router   *Router
	pauses   *PauseRegistry
	registry *PartitionRegistry
	runner   *WorkerRunner
	dispCh   chan struct{}
}

// NewDispatcher creates a dispatcher with the given dependencies.
func NewDispatcher(
	router *Router,
	pauses *PauseRegistry,
	registry *PartitionRegistry,
	runner *WorkerRunner,
) *Dispatcher {
	return &Dispatcher{
		router:   router,
		pauses:   pauses,
		registry: registry,
		runner:   runner,
		dispCh:   make(chan struct{}, 1),
	}
}

// Dispatch partitions a poll result and enqueues records into partition workers.
// Returns an error if a fatal dispatch error occurs (not context.Canceled).
func (d *Dispatcher) Dispatch(
	ctx context.Context,
	records []*kgo.Record,
	client FetchControlClient,
	workerClient WorkerClient,
) error {
	batches, err := d.groupByPartition(records)
	if err != nil {
		return err
	}
	if len(batches) == 0 {
		return nil
	}

	type pendingBatch struct {
		batch PartitionBatch
		next  int
	}

	pending := make([]pendingBatch, 0, len(batches))
	for _, batch := range batches {
		pending = append(pending, pendingBatch{batch: batch, next: 0})
	}

	for len(pending) > 0 {
		progressed := false
		nextPending := make([]pendingBatch, 0, len(pending))

		for _, cursor := range pending {
			if d.pauses.IsPaused(cursor.batch.Key.Topic) {
				progressed = true
				continue
			}

			state, created, err := d.registry.GetOrCreate(
				cursor.batch.Key,
				cursor.batch.Subscription,
				context.Background(),
				defaultQueueCapacity,
			)
			if err != nil {
				return err
			}
			if created {
				d.runner.Start(state, workerClient)
			}

			for cursor.next < len(cursor.batch.Records) {
				enqueued, _ := state.TryEnqueue(cursor.batch.Records[cursor.next:])
				if enqueued == 0 {
					nextPending = append(nextPending, cursor)
					break
				}
				progressed = true
				cursor.next += enqueued

				// Apply backpressure pause if queue is at high watermark.
				if state.TryPauseBackpressure() {
					client.PauseFetchPartitions(map[string][]int32{
						cursor.batch.Key.Topic: {cursor.batch.Key.Partition},
					})
				}
			}
		}

		if len(nextPending) == 0 {
			return nil
		}
		if !progressed {
			if err := d.WaitForCapacity(ctx); err != nil {
				return err
			}
		}
		pending = nextPending
	}

	return nil
}

// groupByPartition builds PartitionBatches from polled records, skipping
// paused topics and looking up subscriptions from the router.
func (d *Dispatcher) groupByPartition(records []*kgo.Record) ([]PartitionBatch, error) {
	subscriptions := d.router.Snapshot()
	pausedTopics := d.pauses.Snapshot()
	indexByKey := make(map[Key]int)
	batches := make([]PartitionBatch, 0)

	for _, record := range records {
		if _, paused := pausedTopics[record.Topic]; paused {
			continue
		}

		subscription, ok := subscriptions[record.Topic]
		if !ok {
			return nil, fmt.Errorf("failed to map topic to subscription: %s", record.Topic)
		}

		key := Key{Topic: record.Topic, Partition: record.Partition}
		index, ok := indexByKey[key]
		if !ok {
			index = len(batches)
			indexByKey[key] = index
			batches = append(batches, PartitionBatch{
				Key:          key,
				Subscription: subscription,
				Records:      make([]*kgo.Record, 0, 1),
			})
		}
		batches[index].Records = append(batches[index].Records, record)
	}

	return batches, nil
}

// NotifyCapacity signals that some worker consumed a batch, so dispatch can retry.
func (d *Dispatcher) NotifyCapacity() {
	select {
	case d.dispCh <- struct{}{}:
	default:
	}
}

// WaitForCapacity blocks until capacity becomes available or the context is done.
func (d *Dispatcher) WaitForCapacity(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-d.dispCh:
		return nil
	}
}

const defaultQueueCapacity = 64
