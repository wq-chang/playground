package apperror_test

import (
	"errors"
	"fmt"
	"testing"

	"go-services/library/apperror"
	"go-services/library/assert"
)

func TestWrap(t *testing.T) {
	codeA := apperror.CodeNotImplemented
	codeB := apperror.CodeTooManyRequests

	otherMsg := "not app error"
	otherErr := errors.New(otherMsg)

	innerMsg := "db error"
	err1 := apperror.Wrap(codeA, otherErr, "error1: %s", innerMsg)

	wrapMsg := "second msg"
	wrappedErr := apperror.Wrap(codeB, err1, "%s", wrapMsg)

	expectedStr := fmt.Sprintf("%s: %v", wrapMsg, err1)
	assert.Equal(t, wrappedErr.Error(), expectedStr, "wrapped error message")

	appErr, ok := apperror.As(wrappedErr)
	assert.True(t, ok, "wrapped error should be an *AppError")
	assert.Equal(t, appErr.Code, codeB, "Wrap should overwrite the error code with the new one")
	assert.True(t, errors.Is(wrappedErr, err1), "wrapped error should contain the inner error in its chain")
}

func TestWrap_Nil(t *testing.T) {
	err := apperror.Wrap(apperror.CodeAccountLocked, nil, "msg")
	assert.Nil(t, err, "expected nil when wrapping nil error, got %v", err)
}
