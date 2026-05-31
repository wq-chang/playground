package gsync_test

import (
	"testing"

	"go-services/library/assert"
	"go-services/library/gsync"
)

func TestValue(t *testing.T) {
	t.Run("load returns zero value before first store", func(t *testing.T) {
		var value gsync.Value[int]
		assert.Equal(t, value.Load(), 0, "should return zero value")
	})

	t.Run("store and load preserves type safely", func(t *testing.T) {
		var value gsync.Value[string]
		value.Store("hello generic value")

		assert.Equal(t, value.Load(), "hello generic value", "stored value should round-trip")
	})

	t.Run("swap returns previous value and stores new value", func(t *testing.T) {
		var value gsync.Value[int]
		value.Store(10)

		previous := value.Swap(20)

		assert.Equal(t, previous, 10, "swap should return old value")
		assert.Equal(t, value.Load(), 20, "swap should store new value")
	})

	t.Run("swap returns zero value when called before first store", func(t *testing.T) {
		var value gsync.Value[int]

		previous := value.Swap(5)

		assert.Equal(t, previous, 0, "swap should return zero value before first store")
		assert.Equal(t, value.Load(), 5, "swap should store new value")
	})

	t.Run("compare and swap updates value on match", func(t *testing.T) {
		var value gsync.Value[int]
		value.Store(100)

		swapped := value.CompareAndSwap(100, 200)

		assert.True(t, swapped, "compare and swap should succeed when old value matches")
		assert.Equal(t, value.Load(), 200, "value should be updated after successful compare and swap")
	})

	t.Run("compare and swap does not update value on mismatch", func(t *testing.T) {
		var value gsync.Value[int]
		value.Store(100)

		swapped := value.CompareAndSwap(50, 200)

		assert.False(t, swapped, "compare and swap should fail when old value does not match")
		assert.Equal(t, value.Load(), 100, "value should remain unchanged after failed compare and swap")
	})
}
