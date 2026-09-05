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
	ctx      context.Context
	err      error
	cancel   context.CancelFunc
	doneCh   chan struct{}
	n        int
	errOnce  sync.Once
	doneOnce sync.Once
	mu       sync.Mutex
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
	rs.n = 0
	rs.doneCh = make(chan struct{})
	rs.doneOnce = sync.Once{}
	return ctx, nil
}

// Done returns a channel that is closed when all goroutines started via Go
// have completed. Returns nil before Begin. The channel is per-run: Begin
// allocates a fresh one.
func (rs *RunState) Done() <-chan struct{} {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.doneCh
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

// Go runs a function in a new goroutine tracked by the run. Like
// sync.WaitGroup, it must not be called after the run's wait has begun.
func (rs *RunState) Go(fn func()) {
	rs.mu.Lock()
	rs.n++
	rs.mu.Unlock()

	go func() {
		defer rs.doneGoroutine()
		fn()
	}()
}

// doneGoroutine decrements the active count and closes Done when the count
// reaches zero. The close happens under the mutex so it cannot race Begin's
// per-run doneOnce reset.
func (rs *RunState) doneGoroutine() {
	rs.mu.Lock()
	rs.n--
	zero := rs.n == 0
	if zero && rs.doneCh != nil {
		rs.doneOnce.Do(func() { close(rs.doneCh) })
	}
	rs.mu.Unlock()
}

// Wait blocks until all goroutines started via Go have completed. Returns
// immediately if none are active.
func (rs *RunState) Wait() {
	rs.mu.Lock()
	ch := rs.doneCh
	if rs.n == 0 && ch != nil {
		// No goroutines to wait for — close the signal synchronously so a
		// zero-work run still completes its Wait (mirrors wg.Wait).
		rs.doneOnce.Do(func() { close(ch) })
	}
	rs.mu.Unlock()
	if ch != nil {
		<-ch
	}
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
