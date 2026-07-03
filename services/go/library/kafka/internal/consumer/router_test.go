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
	"go-services/library/kafka/ktype"
	"go-services/library/require"
)

// stubRegisterClient implements consumer.RegisterClient for testing.
type stubRegisterClient struct {
	topics []string
	mu     sync.Mutex
}

func (s *stubRegisterClient) AddConsumeTopics(topics ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.topics = append(s.topics, topics...)
}

func TestRouter_Register_Success(t *testing.T) {
	client := &stubRegisterClient{}
	r := consumer.NewRouter(client)

	sub := ktype.Subscription{
		Topic:         "my-topic",
		Handler:       func(_ context.Context, _ *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: ktype.FailurePolicy{},
		AckMode:       ktype.AckModeAtLeastOnce,
	}

	err := r.Register(sub)
	require.NoError(t, err, "register should succeed")

	lookup, ok := r.Lookup("my-topic")
	assert.True(t, ok, "topic should be found")
	assert.Equal(t, lookup.Topic, "my-topic", "topic should match")
}

func TestRouter_Register_Duplicate(t *testing.T) {
	client := &stubRegisterClient{}
	r := consumer.NewRouter(client)

	sub := ktype.Subscription{
		Topic:         "dup-topic",
		Handler:       func(_ context.Context, _ *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: ktype.FailurePolicy{},
		AckMode:       ktype.AckModeAtLeastOnce,
	}

	require.NoError(t, r.Register(sub), "first register should succeed")
	err := r.Register(sub)
	assert.ErrorContains(t, err, "already registered", "duplicate should error")
}

func TestRouter_Lookup_Missing(t *testing.T) {
	r := consumer.NewRouter(&stubRegisterClient{})

	_, ok := r.Lookup("nonexistent")
	assert.False(t, ok, "missing topic should return false")
}

func TestRouter_Snapshot_Immutable(t *testing.T) {
	client := &stubRegisterClient{}
	r := consumer.NewRouter(client)

	sub := ktype.Subscription{
		Topic:         "snap-topic",
		Handler:       func(_ context.Context, _ *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: ktype.FailurePolicy{},
		AckMode:       ktype.AckModeAtLeastOnce,
	}
	require.NoError(t, r.Register(sub), "first register should succeed")

	snap := r.Snapshot()
	assert.Equal(t, len(snap), 1, "snapshot should have 1 entry")

	sub2 := ktype.Subscription{
		Topic:         "another",
		Handler:       func(_ context.Context, _ *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: ktype.FailurePolicy{},
		AckMode:       ktype.AckModeAtLeastOnce,
	}
	require.NoError(t, r.Register(sub2), "second register should succeed")

	assert.Equal(t, len(snap), 1, "old snapshot should still have 1 entry (immutable)")
}

func TestRouter_RegisterQuietBatch_SkipsClient(t *testing.T) {
	client := &stubRegisterClient{}
	r := consumer.NewRouter(client)

	sub1 := ktype.Subscription{
		Topic:         "topic-a",
		Handler:       func(_ context.Context, _ *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: ktype.FailurePolicy{},
		AckMode:       ktype.AckModeAtLeastOnce,
	}
	sub2 := ktype.Subscription{
		Topic:         "topic-b",
		Handler:       func(_ context.Context, _ *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: ktype.FailurePolicy{},
		AckMode:       ktype.AckModeAtLeastOnce,
	}

	err := r.RegisterQuietBatch([]ktype.Subscription{sub1, sub2})
	require.NoError(t, err, "RegisterQuietBatch should succeed")

	// Verify both are in the router.
	got, ok := r.Lookup("topic-a")
	assert.True(t, ok, "topic-a should be found")
	assert.Equal(t, got.Topic, "topic-a", "topic should match")

	got, ok = r.Lookup("topic-b")
	assert.True(t, ok, "topic-b should be found")
	assert.Equal(t, got.Topic, "topic-b", "topic should match")

	// Verify AddConsumeTopics was NOT called.
	client.mu.Lock()
	assert.Equal(t, len(client.topics), 0, "AddConsumeTopics should not be called")
	client.mu.Unlock()
}

func TestRouter_RegisterQuietBatch_Duplicate(t *testing.T) {
	client := &stubRegisterClient{}
	r := consumer.NewRouter(client)

	sub1 := ktype.Subscription{
		Topic:         "topic-a",
		Handler:       func(_ context.Context, _ *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: ktype.FailurePolicy{},
		AckMode:       ktype.AckModeAtLeastOnce,
	}
	sub2 := ktype.Subscription{
		Topic:         "topic-a",
		Handler:       func(_ context.Context, _ *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: ktype.FailurePolicy{},
		AckMode:       ktype.AckModeAtLeastOnce,
	}

	err := r.RegisterQuietBatch([]ktype.Subscription{sub1, sub2})
	assert.ErrorContains(t, err, "duplicate topic", "duplicate in batch should error")
}

func TestRouter_AddConsumeTopics_Called(t *testing.T) {
	client := &stubRegisterClient{}
	r := consumer.NewRouter(client)

	sub := ktype.Subscription{
		Topic:         "topic-a",
		Handler:       func(_ context.Context, _ *kgo.Record) error { return nil },
		BatchHandler:  nil,
		FailurePolicy: ktype.FailurePolicy{},
		AckMode:       ktype.AckModeAtLeastOnce,
	}

	err := r.Register(sub)
	require.NoError(t, err, "register should succeed")

	client.mu.Lock()
	assert.Equal(t, len(client.topics), 1, "should have called AddConsumeTopics once")
	assert.Equal(t, client.topics[0], "topic-a", "should add the correct topic")
	client.mu.Unlock()
}

func TestRouter_Concurrent_NoRace(t *testing.T) {
	r := consumer.NewRouter(nil)

	// Pre-register some topics.
	for i := range 50 {
		topic := fmt.Sprintf("pre-%d", i)
		err := r.Register(ktype.Subscription{
			Topic:         topic,
			Handler:       func(_ context.Context, _ *kgo.Record) error { return nil },
			BatchHandler:  nil,
			FailurePolicy: ktype.FailurePolicy{},
			AckMode:       ktype.AckModeAtLeastOnce,
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
			if err := r.Register(ktype.Subscription{
				Topic:         topic,
				Handler:       func(_ context.Context, _ *kgo.Record) error { return nil },
				BatchHandler:  nil,
				FailurePolicy: ktype.FailurePolicy{},
				AckMode:       ktype.AckModeAtLeastOnce,
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
