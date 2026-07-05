// services/go/library/kafka/internal/consumer/router_test.go
package consumer_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"

	"go-services/library/assert"
	"go-services/library/kafka/internal/consumer"
	"go-services/library/require"
)

func TestRouter_Register_Success(t *testing.T) {
	var called []string
	addTopics := func(topics ...string) {
		called = append(called, topics...)
	}
	r := consumer.NewRouter(addTopics)

	sub := consumer.Subscription{
		Topic:         "my-topic",
		Handler:       func(_ context.Context, _ *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: consumer.FailurePolicy{},
		AckMode:       consumer.AckModeAtLeastOnce,
	}

	err := r.Register(sub)
	require.NoError(t, err, "register should succeed")

	lookup, ok := r.Lookup("my-topic")
	assert.True(t, ok, "topic should be found")
	assert.Equal(t, lookup.Topic, "my-topic", "topic should match")
}

func TestRouter_Register_Duplicate(t *testing.T) {
	var called []string
	addTopics := func(topics ...string) {
		called = append(called, topics...)
	}
	r := consumer.NewRouter(addTopics)

	sub := consumer.Subscription{
		Topic:         "dup-topic",
		Handler:       func(_ context.Context, _ *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: consumer.FailurePolicy{},
		AckMode:       consumer.AckModeAtLeastOnce,
	}

	require.NoError(t, r.Register(sub), "first register should succeed")
	err := r.Register(sub)
	assert.ErrorContains(t, err, "already registered", "duplicate should error")
}

func TestRouter_Lookup_Missing(t *testing.T) {
	r := consumer.NewRouter(nil)

	_, ok := r.Lookup("nonexistent")
	assert.False(t, ok, "missing topic should return false")
}

func TestRouter_Snapshot_Immutable(t *testing.T) {
	var called []string
	addTopics := func(topics ...string) {
		called = append(called, topics...)
	}
	r := consumer.NewRouter(addTopics)

	sub := consumer.Subscription{
		Topic:         "snap-topic",
		Handler:       func(_ context.Context, _ *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: consumer.FailurePolicy{},
		AckMode:       consumer.AckModeAtLeastOnce,
	}
	require.NoError(t, r.Register(sub), "first register should succeed")

	snap := r.Snapshot()
	assert.Equal(t, len(snap), 1, "snapshot should have 1 entry")

	sub2 := consumer.Subscription{
		Topic:         "another",
		Handler:       func(_ context.Context, _ *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: consumer.FailurePolicy{},
		AckMode:       consumer.AckModeAtLeastOnce,
	}
	require.NoError(t, r.Register(sub2), "second register should succeed")

	assert.Equal(t, len(snap), 1, "old snapshot should still have 1 entry (immutable)")
}

func TestRouter_RegisterQuietBatch_SkipsClient(t *testing.T) {
	var called []string
	addTopics := func(topics ...string) {
		called = append(called, topics...)
	}
	r := consumer.NewRouter(addTopics)

	sub1 := consumer.Subscription{
		Topic:         "topic-a",
		Handler:       func(_ context.Context, _ *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: consumer.FailurePolicy{},
		AckMode:       consumer.AckModeAtLeastOnce,
	}
	sub2 := consumer.Subscription{
		Topic:         "topic-b",
		Handler:       func(_ context.Context, _ *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: consumer.FailurePolicy{},
		AckMode:       consumer.AckModeAtLeastOnce,
	}

	err := r.RegisterQuietBatch([]consumer.Subscription{sub1, sub2})
	require.NoError(t, err, "RegisterQuietBatch should succeed")

	// Verify both are in the router.
	got, ok := r.Lookup("topic-a")
	assert.True(t, ok, "topic-a should be found")
	assert.Equal(t, got.Topic, "topic-a", "topic should match")

	got, ok = r.Lookup("topic-b")
	assert.True(t, ok, "topic-b should be found")
	assert.Equal(t, got.Topic, "topic-b", "topic should match")

	// Verify addTopics was NOT called.
	assert.Equal(t, len(called), 0, "addTopics should not be called")
}

func TestRouter_RegisterQuietBatch_Duplicate(t *testing.T) {
	r := consumer.NewRouter(nil)

	sub1 := consumer.Subscription{
		Topic:         "topic-a",
		Handler:       func(_ context.Context, _ *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: consumer.FailurePolicy{},
		AckMode:       consumer.AckModeAtLeastOnce,
	}
	sub2 := consumer.Subscription{
		Topic:         "topic-a",
		Handler:       func(_ context.Context, _ *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: consumer.FailurePolicy{},
		AckMode:       consumer.AckModeAtLeastOnce,
	}

	err := r.RegisterQuietBatch([]consumer.Subscription{sub1, sub2})
	assert.ErrorContains(t, err, "duplicate topic", "duplicate in batch should error")
}

func TestRouter_AddConsumeTopics_Called(t *testing.T) {
	var called []string
	addTopics := func(topics ...string) {
		called = append(called, topics...)
	}
	r := consumer.NewRouter(addTopics)

	sub := consumer.Subscription{
		Topic:         "topic-a",
		Handler:       func(_ context.Context, _ *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: consumer.FailurePolicy{},
		AckMode:       consumer.AckModeAtLeastOnce,
	}

	err := r.Register(sub)
	require.NoError(t, err, "register should succeed")

	assert.Equal(t, len(called), 1, "should have called addTopics once")
	assert.Equal(t, called[0], "topic-a", "should add the correct topic")
}

func TestRouter_Concurrent_NoRace(t *testing.T) {
	r := consumer.NewRouter(nil)

	// Pre-register some topics.
	for i := range 50 {
		topic := fmt.Sprintf("pre-%d", i)
		err := r.Register(consumer.Subscription{
			Topic:         topic,
			Handler:       func(_ context.Context, _ *kgo.Record) error { return nil },
			BatchHandler:  nil,
			FailurePolicy: consumer.FailurePolicy{},
			AckMode:       consumer.AckModeAtLeastOnce,
		})
		require.NoError(t, err, "pre-register should succeed")
	}

	var wg sync.WaitGroup

	numWriters := 20
	for i := range numWriters {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			topic := fmt.Sprintf("conc-%d", id)
			if err := r.Register(consumer.Subscription{
				Topic:         topic,
				Handler:       func(_ context.Context, _ *kgo.Record) error { return nil },
				BatchHandler:  nil,
				FailurePolicy: consumer.FailurePolicy{},
				AckMode:       consumer.AckModeAtLeastOnce,
			}); err != nil {
				t.Errorf("Register failed: %v", err)
			}
		}(i)
	}

	numReaders := 20
	for i := range numReaders {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()
			for j := range 100 {
				oldTopic := fmt.Sprintf("pre-%d", j%50)
				newTopic := fmt.Sprintf("conc-%d", j%20)
				r.Lookup(oldTopic)
				r.Lookup(newTopic)
				r.Snapshot()
			}
		}(i)
	}

	wg.Wait()

	for i := range numWriters {
		topic := fmt.Sprintf("conc-%d", i)
		_, ok := r.Lookup(topic)
		assert.True(t, ok, "topic %q should have been registered", topic)
	}

	snap := r.Snapshot()
	assert.True(t, len(snap) >= (50+numWriters), "snapshot should contain all topics, got %d", len(snap))
}
