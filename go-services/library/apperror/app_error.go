package apperror

import (
	"errors"
	"fmt"
)

type AppError struct {
	Msg  string
	Code ErrorCode
	Err  error
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%v: %v", e.Msg, e.Err)
	}
	return e.Msg
}

func (e *AppError) Unwrap() error {
	return e.Err
}

func New(msg string, code ErrorCode, err error) error {
	return &AppError{
		Msg:  msg,
		Code: code,
		Err:  err,
	}
}

func As(err error) (*AppError, bool) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr, true
	}

	return nil, false
}

// ToHTTPStatus converts a domain ErrorCode into a standard HTTP status code.
func (e *AppError) ToHTTPStatus() int {
	// TODO: cover all error code
	switch e.Code {
	case CodeInvalidInput, CodeInvalidFormat:
		return 400 // Bad Request
	case CodeUnauthorized:
		return 401 // Unauthorized
	case CodeForbidden:
		return 403 // Forbidden
	case CodeNotFound:
		return 404 // Not Found
	case CodeConflict:
		return 409 // Conflict
	case CodeTooManyRequests:
		return 429
	case CodeInternalError:
		return 500 // Internal Server Error
	case CodeServiceUnavailable:
		return 503
	default:
		return 500 // Default fallback
	}
}
