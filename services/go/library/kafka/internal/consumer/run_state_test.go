package consumer_test

import (
	"context"
	"errors"
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
