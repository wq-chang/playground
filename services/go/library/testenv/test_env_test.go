package testenv_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"go-services/library/assert"
	"go-services/library/require"
	"go-services/library/testenv"
)

func TestSetupServiceReusesCachedInstance(t *testing.T) {
	te := testenv.New("pkg")

	var cleanupCalls atomic.Int32
	var createCalls atomic.Int32
	instance := &struct{}{}

	create := func(context.Context) (any, func(), error) {
		createCalls.Add(1)
		return instance, func() {
			cleanupCalls.Add(1)
		}, nil
	}

	first, err := te.SetupServiceForTest("fake", "key", create)
	require.NoError(t, err, "first setup failed")

	second, err := te.SetupServiceForTest("fake", "key", create)
	require.NoError(t, err, "second setup failed")

	assert.True(t, first == second, "expected cached service instance to be reused")
	assert.Equal(t, createCalls.Load(), int32(1), "expected one create call")

	te.Cleanup()
	te.Cleanup()

	assert.Equal(t, cleanupCalls.Load(), int32(1), "expected cleanup to run once")
}

func TestSetupServiceRejectsDifferentOptions(t *testing.T) {
	te := testenv.New("pkg")

	_, err := te.SetupServiceForTest("fake", "first", func(context.Context) (any, func(), error) {
		return &struct{}{}, nil, nil
	})
	require.NoError(t, err, "initial setup failed")

	_, err = te.SetupServiceForTest("fake", "second", func(context.Context) (any, func(), error) {
		require.True(t, false, "setup should not run when options mismatch")
		return nil, nil, nil
	})
	require.ErrorContains(t, err, "different options", "expected mismatch error")
}

func TestSetupServiceAllowsRetryAfterFailure(t *testing.T) {
	te := testenv.New("pkg")

	var createCalls atomic.Int32
	expected := errors.New("boom")

	create := func(context.Context) (any, func(), error) {
		if createCalls.Add(1) == 1 {
			return nil, nil, expected
		}
		return &struct{}{}, nil, nil
	}

	_, err := te.SetupServiceForTest("fake", "key", create)
	require.ErrorIs(t, err, expected, "expected first setup error")

	service, err := te.SetupServiceForTest("fake", "key", create)
	require.NoError(t, err, "expected retry to succeed")
	assert.NotNil(t, service, "expected retry to return a service")
	assert.Equal(t, createCalls.Load(), int32(2), "expected two create attempts")
}

func TestSetupServiceCreatesOnceConcurrently(t *testing.T) {
	te := testenv.New("pkg")

	var createCalls atomic.Int32
	start := make(chan struct{})
	instance := &struct{}{}

	create := func(context.Context) (any, func(), error) {
		createCalls.Add(1)
		<-start
		return instance, nil, nil
	}

	const workers = 8
	var wg sync.WaitGroup
	results := make([]any, workers)
	errs := make([]error, workers)

	for i := range workers {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = te.SetupServiceForTest("fake", "key", create)
		}(i)
	}

	close(start)
	wg.Wait()

	assert.Equal(t, createCalls.Load(), int32(1), "expected one create call")
	for i, err := range errs {
		require.NoError(t, err, "worker %d returned error", i)
		assert.True(t, results[i] == instance, "worker %d did not receive cached instance", i)
	}
}
