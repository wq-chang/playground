// services/go/library/kafka/internal/consumer/dispatcher_test.go
package consumer_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"go-services/library/assert"
	"go-services/library/kafka/internal/consumer"
	"go-services/library/require"
	"go-services/library/testlogger"
)

// sub returns a Subscription with a no-op record handler.
func sub(topic string) consumer.Subscription {
	return consumer.Subscription{
		Topic:        topic,
		Handler:      func(ctx context.Context, record *kgo.Record) error { return nil },
		BatchHandler: nil,
		FailurePolicy: consumer.FailurePolicy{
			MaxAttempts:  1,
			RetryBackoff: 0,
			DLQ:          nil,
			OnExhausted:  consumer.ExhaustedActionStop,
		},
		AckMode: consumer.AckModeAtLeastOnce,
	}
}

func makeRecord(topic string, partition int32, offset int64) *kgo.Record {
	return &kgo.Record{
		Topic:       topic,
		Partition:   partition,
		Offset:      offset,
		LeaderEpoch: 0,
	}
}

// stubFetchPauser records calls to PauseFetchPartitions for test verification.
type stubFetchPauser struct {
	calls []map[string][]int32
}

func (s *stubFetchPauser) PauseFetchPartitions(p map[string][]int32) map[string][]int32 {
	s.calls = append(s.calls, p)
	return nil
}

func waitForDispatcherToBlock(t *testing.T) {
	t.Helper()
	time.Sleep(50 * time.Millisecond) // crude but effective for test purposes
}

func TestDispatcher_New(t *testing.T) {
	router := consumer.NewRouter(nil)
	pauses := consumer.NewPauseRegistry(time.Now, &stubTopicPauser{})
	registry := consumer.NewPartitionRegistry(testlogger.NewLogger())
	d := consumer.NewDispatcher(router, pauses, registry, nil, nil, 64, make(chan struct{}, 1))
	assert.NotNil(t, d, "NewDispatcher should not return nil")
}

func TestDispatcher_Dispatch_EmptyRecords(t *testing.T) {
	router := consumer.NewRouter(nil)
	pauses := consumer.NewPauseRegistry(time.Now, &stubTopicPauser{})
	registry := consumer.NewPartitionRegistry(testlogger.NewLogger())
	d := consumer.NewDispatcher(router, pauses, registry, nil, nil, 64, make(chan struct{}, 1))

	err := d.Dispatch(context.Background(), context.Background(), nil)
	assert.NoError(t, err, "Dispatch nil records should succeed")

	err = d.Dispatch(context.Background(), context.Background(), []*kgo.Record{})
	assert.NoError(t, err, "Dispatch empty records should succeed")
}

func TestDispatcher_Dispatch_UnregisteredTopic_Error(t *testing.T) {
	router := consumer.NewRouter(nil)
	pauses := consumer.NewPauseRegistry(time.Now, &stubTopicPauser{})
	registry := consumer.NewPartitionRegistry(testlogger.NewLogger())
	d := consumer.NewDispatcher(router, pauses, registry, nil, nil, 64, make(chan struct{}, 1))

	err := d.Dispatch(context.Background(), context.Background(), []*kgo.Record{
		makeRecord("no-such-topic", 0, 0),
	})
	assert.ErrorContains(t, err, "failed to map topic to subscription", "unregistered topic should error")
}

func TestDispatcher_Dispatch_UnregisteredTopic_AmongValid(t *testing.T) {
	router := consumer.NewRouter(nil)
	pauses := consumer.NewPauseRegistry(time.Now, &stubTopicPauser{})
	registry := consumer.NewPartitionRegistry(testlogger.NewLogger())

	require.NoError(t, router.Register(sub("valid")), "should register valid topic")

	d := consumer.NewDispatcher(router, pauses, registry, nil, nil, 64, make(chan struct{}, 1))

	err := d.Dispatch(context.Background(), context.Background(), []*kgo.Record{
		makeRecord("valid", 0, 0),
		makeRecord("unknown", 0, 1),
	})
	assert.ErrorContains(t, err, "failed to map topic to subscription", "should error on first unregistered topic")
}

func TestDispatcher_Dispatch_PausedTopic_SkipsAllRecords(t *testing.T) {
	router := consumer.NewRouter(nil)
	pauses := consumer.NewPauseRegistry(time.Now, &stubTopicPauser{})
	registry := consumer.NewPartitionRegistry(testlogger.NewLogger())

	require.NoError(t, router.Register(sub("t")), "should register topic")
	pauses.Pause("t", nil)

	d := consumer.NewDispatcher(router, pauses, registry, nil, nil, 64, make(chan struct{}, 1))

	err := d.Dispatch(context.Background(), context.Background(), []*kgo.Record{
		makeRecord("t", 0, 0),
		makeRecord("t", 0, 1),
	})
	require.NoError(t, err, "Dispatch with paused topic should succeed")

	_, ok := registry.Get(consumer.Key{Topic: "t", Partition: 0})
	assert.False(t, ok, "no partition state should be created for paused topic")
}

func TestDispatcher_Dispatch_MixedPausedAndActive(t *testing.T) {
	router := consumer.NewRouter(nil)
	pauses := consumer.NewPauseRegistry(time.Now, &stubTopicPauser{})
	registry := consumer.NewPartitionRegistry(testlogger.NewLogger())

	require.NoError(t, router.Register(sub("paused")), "should register paused topic")
	require.NoError(t, router.Register(sub("active")), "should register active topic")
	pauses.Pause("paused", nil)

	var started atomic.Int32
	startFn := func(ps *consumer.PartitionState) {
		started.Add(1)
	}

	d := consumer.NewDispatcher(router, pauses, registry, nil, startFn, 64, make(chan struct{}, 1))

	err := d.Dispatch(context.Background(), context.Background(), []*kgo.Record{
		makeRecord("paused", 0, 0),
		makeRecord("active", 0, 1),
	})
	require.NoError(t, err, "Dispatch with mixed paused/active should succeed")

	_, ok := registry.Get(consumer.Key{Topic: "paused", Partition: 0})
	assert.False(t, ok, "no state for paused topic")

	state, ok := registry.Get(consumer.Key{Topic: "active", Partition: 0})
	require.True(t, ok, "state should exist for active topic")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	dequeued, okDequeue := state.Dequeue(ctx)
	assert.True(t, okDequeue, "should dequeue record from active topic")
	assert.Equal(t, len(dequeued), 1, "should have one record")
	assert.Equal(t, dequeued[0].Topic, "active", "record should be from active topic")

	assert.Equal(t, started.Load(), 1, "startFn should have been called once")
}

func TestDispatcher_Dispatch_SingleRecord_CreatesStateAndEnqueues(t *testing.T) {
	router := consumer.NewRouter(nil)
	pauses := consumer.NewPauseRegistry(time.Now, &stubTopicPauser{})
	registry := consumer.NewPartitionRegistry(testlogger.NewLogger())

	require.NoError(t, router.Register(sub("t")), "should register topic")

	var started atomic.Int32
	startFn := func(ps *consumer.PartitionState) {
		started.Add(1)
	}

	d := consumer.NewDispatcher(router, pauses, registry, nil, startFn, 64, make(chan struct{}, 1))

	err := d.Dispatch(context.Background(), context.Background(), []*kgo.Record{
		makeRecord("t", 0, 42),
	})
	require.NoError(t, err, "Dispatch single record should succeed")

	state, ok := registry.Get(consumer.Key{Topic: "t", Partition: 0})
	require.True(t, ok, "partition state should exist after dispatch")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	dequeued, okDequeue := state.Dequeue(ctx)
	assert.True(t, okDequeue, "should dequeue the record")
	assert.Equal(t, len(dequeued), 1, "should have exactly one record")
	assert.Equal(t, dequeued[0].Offset, 42, "record offset should match")

	assert.Equal(t, started.Load(), 1, "startFn should be called when state is created")
}

func TestDispatcher_Dispatch_MultipleRecords_SamePartition(t *testing.T) {
	router := consumer.NewRouter(nil)
	pauses := consumer.NewPauseRegistry(time.Now, &stubTopicPauser{})
	registry := consumer.NewPartitionRegistry(testlogger.NewLogger())

	require.NoError(t, router.Register(sub("t")), "should register topic")

	var started atomic.Int32
	startFn := func(ps *consumer.PartitionState) {
		started.Add(1)
	}

	d := consumer.NewDispatcher(router, pauses, registry, nil, startFn, 64, make(chan struct{}, 1))

	err := d.Dispatch(context.Background(), context.Background(), []*kgo.Record{
		makeRecord("t", 0, 0),
		makeRecord("t", 0, 1),
		makeRecord("t", 0, 2),
	})
	require.NoError(t, err, "Dispatch multiple records should succeed")

	state, ok := registry.Get(consumer.Key{Topic: "t", Partition: 0})
	require.True(t, ok, "partition state should exist")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	dequeued, okDequeue := state.Dequeue(ctx)
	assert.True(t, okDequeue, "should dequeue records")
	assert.Equal(t, len(dequeued), 3, "all 3 records should be in one batch")

	assert.Equal(t, started.Load(), 1, "startFn should be called exactly once")
}

func TestDispatcher_Dispatch_DifferentPartitions(t *testing.T) {
	router := consumer.NewRouter(nil)
	pauses := consumer.NewPauseRegistry(time.Now, &stubTopicPauser{})
	registry := consumer.NewPartitionRegistry(testlogger.NewLogger())

	require.NoError(t, router.Register(sub("t")), "should register topic")

	var started atomic.Int32
	startFn := func(ps *consumer.PartitionState) {
		started.Add(1)
	}

	d := consumer.NewDispatcher(router, pauses, registry, nil, startFn, 64, make(chan struct{}, 1))

	err := d.Dispatch(context.Background(), context.Background(), []*kgo.Record{
		makeRecord("t", 0, 0),
		makeRecord("t", 1, 0),
		makeRecord("t", 0, 1),
		makeRecord("t", 2, 0),
	})
	require.NoError(t, err, "Dispatch different partitions should succeed")

	_, ok0 := registry.Get(consumer.Key{Topic: "t", Partition: 0})
	assert.True(t, ok0, "state should exist for partition 0")
	_, ok1 := registry.Get(consumer.Key{Topic: "t", Partition: 1})
	assert.True(t, ok1, "state should exist for partition 1")
	_, ok2 := registry.Get(consumer.Key{Topic: "t", Partition: 2})
	assert.True(t, ok2, "state should exist for partition 2")

	assert.Equal(t, started.Load(), 3, "startFn should be called for each new partition")
}

func TestDispatcher_Dispatch_DifferentTopics(t *testing.T) {
	router := consumer.NewRouter(nil)
	pauses := consumer.NewPauseRegistry(time.Now, &stubTopicPauser{})
	registry := consumer.NewPartitionRegistry(testlogger.NewLogger())

	require.NoError(t, router.Register(sub("a")), "should register topic a")
	require.NoError(t, router.Register(sub("b")), "should register topic b")

	var started atomic.Int32
	startFn := func(ps *consumer.PartitionState) {
		started.Add(1)
	}

	d := consumer.NewDispatcher(router, pauses, registry, nil, startFn, 64, make(chan struct{}, 1))

	err := d.Dispatch(context.Background(), context.Background(), []*kgo.Record{
		makeRecord("a", 0, 0),
		makeRecord("b", 0, 0),
	})
	require.NoError(t, err, "Dispatch different topics should succeed")

	_, okA := registry.Get(consumer.Key{Topic: "a", Partition: 0})
	assert.True(t, okA, "state should exist for topic a")
	_, okB := registry.Get(consumer.Key{Topic: "b", Partition: 0})
	assert.True(t, okB, "state should exist for topic b")

	assert.Equal(t, started.Load(), 2, "startFn should be called for each topic")
}

func TestDispatcher_Dispatch_StartFn_OnlyOnFirstCreation(t *testing.T) {
	router := consumer.NewRouter(nil)
	pauses := consumer.NewPauseRegistry(time.Now, &stubTopicPauser{})
	registry := consumer.NewPartitionRegistry(testlogger.NewLogger())

	require.NoError(t, router.Register(sub("t")), "should register topic")

	var started atomic.Int32
	startFn := func(ps *consumer.PartitionState) {
		started.Add(1)
	}

	d := consumer.NewDispatcher(router, pauses, registry, nil, startFn, 64, make(chan struct{}, 1))

	err := d.Dispatch(context.Background(), context.Background(), []*kgo.Record{
		makeRecord("t", 0, 0),
	})
	require.NoError(t, err, "first dispatch should succeed")
	assert.Equal(t, started.Load(), 1, "startFn should be called on first creation")

	err = d.Dispatch(context.Background(), context.Background(), []*kgo.Record{
		makeRecord("t", 0, 1),
	})
	require.NoError(t, err, "second dispatch should succeed")
	assert.Equal(t, started.Load(), 1, "startFn should NOT be called again")
}

func TestDispatcher_Dispatch_WaitForCapacity_ContextCancelled(t *testing.T) {
	router := consumer.NewRouter(nil)
	pauses := consumer.NewPauseRegistry(time.Now, &stubTopicPauser{})
	registry := consumer.NewPartitionRegistry(testlogger.NewLogger())

	require.NoError(t, router.Register(sub("t")), "should register topic")

	ps, created, err := registry.GetOrCreate(
		context.Background(),
		consumer.Key{Topic: "t", Partition: 0},
		sub("t"),
		1,
	)
	require.NoError(t, err, "GetOrCreate should succeed")
	require.True(t, created, "state should be newly created")

	enqueued, _ := ps.TryEnqueue([]*kgo.Record{makeRecord("t", 0, 0)})
	require.Equal(t, enqueued, 1, "should enqueue record to fill the queue")

	capacityCh := make(chan struct{})
	d := consumer.NewDispatcher(
		router,
		pauses,
		registry,
		nil,
		func(ps *consumer.PartitionState) {},
		1,
		capacityCh,
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = d.Dispatch(ctx, context.Background(), []*kgo.Record{makeRecord("t", 0, 1)})
	assert.ErrorIs(
		t,
		err,
		context.Canceled,
		"Dispatch should return context.Canceled when capacity never arrives",
	)
}

func TestDispatcher_Dispatch_BackpressurePause(t *testing.T) {
	router := consumer.NewRouter(nil)
	pauses := consumer.NewPauseRegistry(time.Now, &stubTopicPauser{})
	registry := consumer.NewPartitionRegistry(testlogger.NewLogger())

	require.NoError(t, router.Register(sub("t")), "should register topic")

	pauser := &stubFetchPauser{}

	d := consumer.NewDispatcher(
		router,
		pauses,
		registry,
		pauser,
		func(ps *consumer.PartitionState) {},
		2,
		make(chan struct{}, 1),
	)

	err := d.Dispatch(context.Background(), context.Background(), []*kgo.Record{
		makeRecord("t", 0, 0),
	})
	require.NoError(t, err, "Dispatch should succeed with backpressure")

	require.Equal(t, len(pauser.calls), 1, "PauseFetchPartitions should be called once")
	assert.Equal(t, pauser.calls[0]["t"], []int32{0}, "should pause partition 0 of topic t")

	state, ok := registry.Get(consumer.Key{Topic: "t", Partition: 0})
	require.True(t, ok, "partition state should exist")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	dequeued, okDequeue := state.Dequeue(ctx)
	assert.True(t, okDequeue, "should dequeue the record")
	assert.Equal(t, len(dequeued), 1, "should have one record")
}

func TestDispatcher_Dispatch_WaitForCapacity_Signalled(t *testing.T) {
	router := consumer.NewRouter(nil)
	pauses := consumer.NewPauseRegistry(time.Now, &stubTopicPauser{})
	registry := consumer.NewPartitionRegistry(testlogger.NewLogger())

	require.NoError(t, router.Register(sub("t")), "should register topic")

	ps, created, err := registry.GetOrCreate(
		context.Background(),
		consumer.Key{Topic: "t", Partition: 0},
		sub("t"),
		2,
	)
	require.NoError(t, err, "GetOrCreate should succeed")
	require.True(t, created, "state should be newly created")

	n, _ := ps.TryEnqueue([]*kgo.Record{makeRecord("t", 0, 0), makeRecord("t", 0, 1)})
	require.Equal(t, n, 2, "should enqueue records to fill the queue")

	pauser := &stubFetchPauser{}
	capacityCh := make(chan struct{}, 1)
	d := consumer.NewDispatcher(
		router,
		pauses,
		registry,
		pauser,
		func(ps *consumer.PartitionState) {},
		2,
		capacityCh,
	)

	errCh := make(chan error, 1)
	go func() {
		errCh <- d.Dispatch(context.Background(), context.Background(), []*kgo.Record{
			makeRecord("t", 0, 2),
		})
	}()

	waitForDispatcherToBlock(t)
	func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		ps.Dequeue(ctx)
	}()
	capacityCh <- struct{}{}

	err = <-errCh
	require.NoError(t, err, "Dispatch should succeed after capacity arrives")

	require.Equal(t, len(pauser.calls), 1, "backpressure should fire once after capacity recovery")
	assert.Equal(t, pauser.calls[0]["t"], []int32{0}, "should pause partition 0 of topic t")

	state, ok := registry.Get(consumer.Key{Topic: "t", Partition: 0})
	require.True(t, ok, "state should still exist")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	dequeued, okDequeue := state.Dequeue(ctx)
	assert.True(t, okDequeue, "should dequeue the new record")
	assert.Equal(t, len(dequeued), 1, "should have one remaining record")
	assert.Equal(t, dequeued[0].Offset, 2, "should be the third record")
}

func TestDispatcher_Dispatch_PartialEnqueue_RetriesAfterCapacity(t *testing.T) {
	router := consumer.NewRouter(nil)
	pauses := consumer.NewPauseRegistry(time.Now, &stubTopicPauser{})
	registry := consumer.NewPartitionRegistry(testlogger.NewLogger())

	require.NoError(t, router.Register(sub("t")), "should register topic")

	ps, created, err := registry.GetOrCreate(
		context.Background(),
		consumer.Key{Topic: "t", Partition: 0},
		sub("t"),
		4,
	)
	require.NoError(t, err, "GetOrCreate should succeed")
	require.True(t, created, "state should be newly created")

	n, _ := ps.TryEnqueue([]*kgo.Record{makeRecord("t", 0, 0), makeRecord("t", 0, 1)})
	require.Equal(t, n, 2, "should pre-fill 2 records")

	pauser := &stubFetchPauser{}
	capacityCh := make(chan struct{}, 1)
	d := consumer.NewDispatcher(router, pauses, registry, pauser,
		func(ps *consumer.PartitionState) {}, 4, capacityCh)

	errCh := make(chan error, 1)
	go func() {
		errCh <- d.Dispatch(context.Background(), context.Background(), []*kgo.Record{
			makeRecord("t", 0, 2),
			makeRecord("t", 0, 3),
			makeRecord("t", 0, 4),
		})
	}()

	waitForDispatcherToBlock(t)

	func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		ps.Dequeue(ctx)
		ps.Dequeue(ctx)
	}()

	capacityCh <- struct{}{}

	err = <-errCh
	require.NoError(t, err, "dispatch should succeed after partial enqueue retry")

	assert.GreaterOrEqual(t, len(pauser.calls), 1, "backpressure should fire")

	state, ok := registry.Get(consumer.Key{Topic: "t", Partition: 0})
	require.True(t, ok, "state should exist")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	dequeued, okDequeue := state.Dequeue(ctx)
	assert.True(t, okDequeue, "should dequeue remaining record")
	assert.Equal(t, len(dequeued), 1, "should have exactly one record")
	assert.Equal(t, dequeued[0].Offset, 4, "should be the last record")
}

func TestDispatcher_Dispatch_MultiPartitionBackpressure(t *testing.T) {
	router := consumer.NewRouter(nil)
	pauses := consumer.NewPauseRegistry(time.Now, &stubTopicPauser{})
	registry := consumer.NewPartitionRegistry(testlogger.NewLogger())

	require.NoError(t, router.Register(sub("t")), "should register topic")

	pauser := &stubFetchPauser{}

	var started atomic.Int32
	startFn := func(ps *consumer.PartitionState) {
		started.Add(1)
	}

	d := consumer.NewDispatcher(router, pauses, registry, pauser, startFn, 2, make(chan struct{}, 1))

	err := d.Dispatch(context.Background(), context.Background(), []*kgo.Record{
		makeRecord("t", 0, 0),
		makeRecord("t", 1, 0),
		makeRecord("t", 2, 0),
	})
	require.NoError(t, err, "dispatch should succeed")

	require.Equal(t, len(pauser.calls), 3, "PauseFetchPartitions should be called for all 3 partitions")

	paused := make(map[int32]bool)
	for _, call := range pauser.calls {
		for _, parts := range call {
			for _, p := range parts {
				paused[p] = true
			}
		}
	}
	assert.Equal(t, len(paused), 3, "should have paused 3 distinct partitions")
	assert.True(t, paused[0], "partition 0 should be paused")
	assert.True(t, paused[1], "partition 1 should be paused")
	assert.True(t, paused[2], "partition 2 should be paused")

	assert.Equal(t, started.Load(), 3, "startFn should be called for each partition")
}

func TestDispatcher_Dispatch_PartialEnqueue_NoRecordLoss(t *testing.T) {
	router := consumer.NewRouter(nil)
	pauses := consumer.NewPauseRegistry(time.Now, &stubTopicPauser{})
	registry := consumer.NewPartitionRegistry(testlogger.NewLogger())

	require.NoError(t, router.Register(sub("t")), "should register topic")

	pauser := &stubFetchPauser{}
	capacityCh := make(chan struct{}, 1)

	// queueCapacity=3: only 3 records fit per TryEnqueue call.
	// Dispatching 5 records exercises the cursor advancement loop:
	//   Pass 1: TryEnqueue enqueues 3, cursor.next=3
	//           TryEnqueue returns 0 for remaining 2
	//   Pass 2: TryEnqueue returns 0, WaitForCapacity
	d := consumer.NewDispatcher(router, pauses, registry, pauser,
		func(ps *consumer.PartitionState) {}, 3, capacityCh)

	errCh := make(chan error, 1)
	go func() {
		errCh <- d.Dispatch(context.Background(), context.Background(), []*kgo.Record{
			makeRecord("t", 0, 0),
			makeRecord("t", 0, 1),
			makeRecord("t", 0, 2),
			makeRecord("t", 0, 3),
			makeRecord("t", 0, 4),
		})
	}()

	waitForDispatcherToBlock(t)

	// Drain the first batch (3 records) and verify it.
	state, ok := registry.Get(consumer.Key{Topic: "t", Partition: 0})
	require.True(t, ok, "state should exist")
	func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		batch1, dequeueOk := state.Dequeue(ctx)
		require.True(t, dequeueOk, "should dequeue first batch")
		assert.Equal(t, len(batch1), 3, "first batch should have 3 records")
		assert.Equal(t, batch1[0].Offset, 0, "first record offset")
		assert.Equal(t, batch1[1].Offset, 1, "second record offset")
		assert.Equal(t, batch1[2].Offset, 2, "third record offset")
	}()

	capacityCh <- struct{}{}

	err := <-errCh
	require.NoError(t, err, "dispatch should succeed")

	// The remaining 2 records arrive in one batch.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	records, ok := state.Dequeue(ctx)
	require.True(t, ok, "should dequeue remaining batch")

	assert.Equal(t, len(records), 2, "remaining batch should have 2 records")
	assert.Equal(t, records[0].Offset, 3, "fourth record offset")
	assert.Equal(t, records[1].Offset, 4, "fifth record offset")

	// Queue should be empty after all 5 records are drained.
	emptyCtx, emptyCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer emptyCancel()
	_, ok = state.Dequeue(emptyCtx)
	assert.False(t, ok, "queue should be empty after all records are drained")
}
