package consumer

import (
	"context"
	"fmt"
	"sync"
)

// RunState owns the run lifecycle of a consumer: context, cancel, fatal error,
// and goroutine tracking. It rejects concurrent runs and ensures only the
// first fatal error is recorded.
//
// Use: Begin → Go(...) / Fail(...) → Stop → Wait → Reset.
type RunState struct {
	ctx     context.Context
	err     error
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	errOnce sync.Once
	mu      sync.Mutex
}

// NewRunState creates a run-lifecycle owner in the idle state.
func NewRunState() *RunState {
	return &RunState{}
}

// Begin starts a new run, creating an isolated context. Returns an error if
// a run is already active.
func (rs *RunState) Begin() (context.Context, error) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if rs.ctx != nil {
		return nil, fmt.Errorf("consumer run is already active")
	}

	ctx, cancel := context.WithCancel(context.Background())
	rs.ctx = ctx
	rs.cancel = cancel
	rs.err = nil
	rs.errOnce = sync.Once{}
	rs.wg = sync.WaitGroup{}
	return ctx, nil
}

// Context returns the current run context, or nil if no run is active.
func (rs *RunState) Context() context.Context {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.ctx
}

// Fail records the first fatal error and cancels the run context.
// Subsequent calls are silently ignored.
func (rs *RunState) Fail(err error) {
	if err == nil {
		return
	}
	rs.errOnce.Do(func() {
		rs.mu.Lock()
		rs.err = err
		if rs.cancel != nil {
			rs.cancel()
		}
		rs.mu.Unlock()
	})
}

// Err returns the stored fatal error, if any.
func (rs *RunState) Err() error {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.err
}

// Stop cancels the current run context. Idempotent.
func (rs *RunState) Stop() {
	rs.mu.Lock()
	cancel := rs.cancel
	rs.mu.Unlock()

	if cancel != nil {
		cancel()
	}
}

// Go runs a function in a new goroutine tracked by the run's wait group.
func (rs *RunState) Go(fn func()) {
	rs.wg.Add(1)
	go func() {
		defer rs.wg.Done()
		fn()
	}()
}

// Wait blocks until all goroutines started via Go have completed.
func (rs *RunState) Wait() {
	rs.wg.Wait()
}

// Reset clears the run state so a new run can begin. errOnce is reset by
// Begin, not by Reset. Must be called after Stop + Wait complete.
func (rs *RunState) Reset() {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.ctx = nil
	rs.cancel = nil
	rs.err = nil
}
