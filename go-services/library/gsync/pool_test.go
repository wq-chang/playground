package gsync_test

import (
	"testing"

	"go-services/library/assert"
	"go-services/library/gsync"
)

func TestPool(t *testing.T) {
	t.Run("new function should create the correct value", func(t *testing.T) {
		p := gsync.Pool[int]{
			New: func() int { return 42 },
		}

		val := p.Get()
		assert.Equal(t, val, 42, "value created by new function")
	})

	t.Run("get should get the value from put", func(t *testing.T) {
		p := gsync.Pool[string]{}
		input := "hello generic world"

		p.Put(input)
		val := p.Get()

		assert.Equal(t, val, input, "value get from pool")
	})

	t.Run("no new function should create zero value", func(t *testing.T) {
		// When New is nil and pool is empty, it should return the zero value
		p := gsync.Pool[float64]{}
		val := p.Get()

		assert.Equal(t, val, 0.0, "zero value")
	})
}
