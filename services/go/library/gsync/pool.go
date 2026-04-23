package gsync

import (
	"fmt"
	"sync"
)

// Pool is a type-safe wrapper around sync.Pool.
type Pool[T any] struct {
	// internal pool to avoid exposing the non-type-safe Get/Put methods directly.
	internal sync.Pool

	// New optionally specifies a function to generate
	// a value when Get would otherwise return nil.
	New func() T
}

// Put adds x to the pool.
func (p *Pool[T]) Put(x T) {
	p.internal.Put(x)
}

// Get selects an arbitrary item from the Pool, removes it from the
// Pool, and returns it to the caller.
func (p *Pool[T]) Get() T {
	vRaw := p.internal.Get()
	if vRaw == nil {
		if p.New != nil {
			return p.New()
		}
		var zero T
		return zero
	}
	v, ok := vRaw.(T)
	if !ok {
		panic(fmt.Sprintf("gsync.Pool: type mismatch; pool contains %T, but expected %T", vRaw, *new(T)))
	}

	return v
}
