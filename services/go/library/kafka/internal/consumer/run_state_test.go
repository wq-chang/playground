package consumer_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go-services/library/assert"
	"go-services/library/kafka/internal/consumer"
	"go-services/library/require"
)

func TestRunState_Begin_Success(t *testing.T) {
	rs := consumer.NewRunState()
	ctx, err := rs.Begin()
	require.NoError(t, err, "Begin should succeed")
	assert.NotNil(t, ctx, "Begin should return a context")
}

func TestRunState_Begin_RejectsConcurrentRuns(t *testing.T) {
	rs := consumer.NewRunState()
	_, err := rs.Begin()
	require.NoError(t, err, "first Begin should succeed")

	_, err = rs.Begin()
	assert.ErrorContains(t, err, "run is already active", "concurrent Begin should error")
}

func TestRunState_Context_ReturnsCurrentCtx(t *testing.T) {
	rs := consumer.NewRunState()
	ctx, err := rs.Begin()
	require.NoError(t, err, "Begin should succeed")

	got := rs.Context()
	assert.NotNil(t, got, "Context() should return non-nil after Begin")

	// Verify it's the same context by checking cancellation propagates.
	rs.Stop()
	assert.ErrorIs(t, ctx.Err(), context.Canceled, "original ctx should cancel")
	assert.ErrorIs(t, got.Err(), context.Canceled, "Context() result should also cancel")
}

func TestRunState_Context_BeforeBegin_ReturnsNil(t *testing.T) {
	rs := consumer.NewRunState()
	assert.Nil(t, rs.Context(), "Context() before Begin should return nil")
}

func TestRunState_Fail_StoresErrorAndCancelsCtx(t *testing.T) {
	rs := consumer.NewRunState()
	ctx, err := rs.Begin()
	require.NoError(t, err, "Begin should succeed")

	sentinel := errors.New("fatal error")
	rs.Fail(sentinel)

	assert.ErrorIs(t, rs.Err(), sentinel, "Err() should return the fatal error")
	assert.ErrorIs(t, ctx.Err(), context.Canceled, "run context should be canceled")
}

func TestRunState_Fail_StoresOnlyFirstError(t *testing.T) {
	rs := consumer.NewRunState()
	_, err := rs.Begin()
	require.NoError(t, err, "Begin should succeed")

	rs.Fail(errors.New("first error"))
	rs.Fail(errors.New("second error"))

	assert.ErrorContains(t, rs.Err(), "first error", "should keep the first error")
}

func TestRunState_Stop_CancelsContext(t *testing.T) {
	rs := consumer.NewRunState()
	ctx, err := rs.Begin()
	require.NoError(t, err, "Begin should succeed")

	rs.Stop()
	assert.ErrorIs(t, ctx.Err(), context.Canceled, "Stop should cancel the run context")
}

func TestRunState_Go_Wait_TracksGoroutines(t *testing.T) {
	rs := consumer.NewRunState()
	_, err := rs.Begin()
	require.NoError(t, err, "Begin should succeed")

	var counter atomic.Int32
	rs.Go(func() {
		time.Sleep(10 * time.Millisecond)
		counter.Add(1)
	})
	rs.Go(func() {
		time.Sleep(10 * time.Millisecond)
		counter.Add(2)
	})

	rs.Wait()
	assert.Equal(t, counter.Load(), 3, "both goroutines should have completed")
}

func TestRunState_Reset_ClearsState(t *testing.T) {
	rs := consumer.NewRunState()
	_, err := rs.Begin()
	require.NoError(t, err, "Begin should succeed")

	rs.Stop()
	rs.Wait()
	rs.Reset()

	assert.Nil(t, rs.Context(), "context should be nil after Reset")
	assert.NoError(t, rs.Err(), "error should be nil after Reset")
}

func TestRunState_Begin_AfterReset_AllowsNewRun(t *testing.T) {
	rs := consumer.NewRunState()

	ctx1, err := rs.Begin()
	require.NoError(t, err, "first Begin should succeed")
	rs.Stop()
	rs.Wait()
	rs.Reset()

	ctx2, err := rs.Begin()
	require.NoError(t, err, "Begin after Reset should succeed")
	assert.NotNil(t, ctx2, "should get a new context")
	assert.True(t, ctx1 != ctx2, "should be a different context")
}

func TestRunState_Fail_NoopOnNilError(t *testing.T) {
	rs := consumer.NewRunState()
	ctx, err := rs.Begin()
	require.NoError(t, err, "Begin should succeed")

	rs.Fail(nil)

	assert.NoError(t, rs.Err(), "Err should be nil when Fail(nil) was called")
	assert.NoError(t, ctx.Err(), "context should not be cancelled")
}

func TestRunState_Stop_Idempotent(t *testing.T) {
	rs := consumer.NewRunState()
	ctx, err := rs.Begin()
	require.NoError(t, err, "Begin should succeed")

	rs.Stop()
	rs.Stop() // must not panic

	assert.ErrorIs(t, ctx.Err(), context.Canceled, "context should be cancelled")
}

func TestRunState_Err_ReturnsNilByDefault(t *testing.T) {
	rs := consumer.NewRunState()
	assert.NoError(t, rs.Err(), "Err before Begin should be nil")

	_, err := rs.Begin()
	require.NoError(t, err, "Begin should succeed")
	assert.NoError(t, rs.Err(), "Err after Begin (before Fail) should be nil")

	rs.Stop()
	rs.Wait()
	rs.Reset()
	assert.NoError(t, rs.Err(), "Err after Reset should be nil")
}

func TestRunState_ErrOnceFresh_AfterResetAndBegin(t *testing.T) {
	rs := consumer.NewRunState()

	// First run: record an error.
	_, err := rs.Begin()
	require.NoError(t, err, "first Begin should succeed")
	rs.Fail(errors.New("first error"))
	rs.Stop()
	rs.Wait()
	rs.Reset()

	// Second run: errOnce must be fresh so Fail records this error.
	_, err = rs.Begin()
	require.NoError(t, err, "second Begin should succeed")
	rs.Fail(errors.New("second error"))

	assert.ErrorContains(t, rs.Err(), "second error", "Err should return error from second run, not first")
}

func TestRunState_Stop_BeforeBegin_Noop(t *testing.T) {
	rs := consumer.NewRunState()
	rs.Stop() // must not panic
}

func TestRunState_Concurrent_FailAndStop(t *testing.T) {
	rs := consumer.NewRunState()
	ctx, err := rs.Begin()
	require.NoError(t, err, "Begin should succeed")

	sentinel := errors.New("concurrent error")
	var wg sync.WaitGroup

	// Launch several goroutines that call Fail and Stop concurrently.
	for i := 0; i < 10; i++ {
		wg.Go(func() {
			rs.Fail(sentinel)
			rs.Stop()
		})
	}

	wg.Wait()

	// State must be consistent: error recorded and context cancelled.
	assert.ErrorIs(t, rs.Err(), sentinel, "Err should contain the sentinel error")
	assert.ErrorIs(t, ctx.Err(), context.Canceled, "context should be cancelled")
}
