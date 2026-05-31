package gsync

import (
	"fmt"
	"sync/atomic"
)

// Value is a type-safe wrapper around atomic.Value.
type Value[T any] struct {
	internal atomic.Value
}

func coerceValue[T any](raw any) T {
	if raw == nil {
		var zero T
		return zero
	}
	value, ok := raw.(T)
	if !ok {
		panic(fmt.Sprintf("gsync.Value: type mismatch; value contains %T, but expected %T", raw, *new(T)))
	}
	return value
}

// Store updates the value atomically.
func (v *Value[T]) Store(value T) {
	v.internal.Store(value)
}

// Load retrieves the current value atomically.
//
// If no value has been stored yet, it returns the zero value of T.
func (v *Value[T]) Load() T {
	return coerceValue[T](v.internal.Load())
}

// Swap stores new and returns the previous value atomically.
//
// If no value has been stored yet, it returns the zero value of T.
func (v *Value[T]) Swap(new T) T {
	return coerceValue[T](v.internal.Swap(new))
}

// CompareAndSwap executes the compare-and-swap operation for the value atomically.
func (v *Value[T]) CompareAndSwap(old, new T) bool {
	return v.internal.CompareAndSwap(old, new)
}
