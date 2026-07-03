// services/go/library/kafka/internal/consumer/router.go
package consumer

import (
	"fmt"
	"maps"
	"sync"

	"go-services/library/gsync"
)

// RegisterClient is the narrow interface Router needs to coordinate
// with the parent consumer for runtime topic subscription.
type RegisterClient interface {
	// AddConsumeTopics subscribes to one or more Kafka topics at runtime.
	AddConsumeTopics(topics ...string)
}

// Router owns subscription registration, lookup, and immutable snapshots.
// Writes clone into a gsync.Value for lock-free reads.
type Router struct {
	snapshot      gsync.Value[map[string]Subscription]
	client        RegisterClient
	subscriptions map[string]Subscription
	mu            sync.Mutex
}

// NewRouter creates an empty subscription registry with the given client adapter.
func NewRouter(client RegisterClient) *Router {
	r := &Router{
		subscriptions: make(map[string]Subscription),
		client:        client,
		mu:            sync.Mutex{},
		snapshot:      gsync.Value[map[string]Subscription]{},
	}
	r.snapshot.Store(make(map[string]Subscription))
	return r
}

// Register stores a normalized subscription after validating preconditions.
// Returns an error if the topic is already registered.
func (r *Router) Register(sub Subscription) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.subscriptions[sub.Topic]; ok {
		return fmt.Errorf("topic handler already registered for %q", sub.Topic)
	}

	r.subscriptions[sub.Topic] = sub
	r.snapshot.Store(maps.Clone(r.subscriptions))
	r.client.AddConsumeTopics(sub.Topic)
	return nil
}

// Lookup returns the normalized subscription for a topic, if registered.
// Uses the atomic snapshot — lock-free, may be slightly stale.
func (r *Router) Lookup(topic string) (Subscription, bool) {
	snap := r.snapshot.Load()
	sub, ok := snap[topic]
	return sub, ok
}

// RegisterQuietBatch stores multiple subscriptions without client-side interaction.
// Subscriptions must already be normalized (use Register for runtime additions
// that need client-side effects). Stores all subscriptions under one lock
// and clones the map once.
//
// Two-pass validation with a seen-set prevents partial state corruption:
// all subscriptions are checked for duplicates (against both existing entries
// and other entries in the same batch) before any are stored.
func (r *Router) RegisterQuietBatch(subs []Subscription) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	seen := make(map[string]struct{}, len(subs))
	for _, sub := range subs {
		if _, ok := r.subscriptions[sub.Topic]; ok {
			return fmt.Errorf("topic handler already registered for %q", sub.Topic)
		}
		if _, ok := seen[sub.Topic]; ok {
			return fmt.Errorf("duplicate topic %q in batch", sub.Topic)
		}
		seen[sub.Topic] = struct{}{}
	}
	for _, sub := range subs {
		r.subscriptions[sub.Topic] = sub
	}
	r.snapshot.Store(maps.Clone(r.subscriptions))
	return nil
}

// Snapshot returns an immutable view of all registered subscriptions.
// Atomic and lock-free — matches v1's gsync.Value pattern.
func (r *Router) Snapshot() map[string]Subscription {
	return r.snapshot.Load()
}
