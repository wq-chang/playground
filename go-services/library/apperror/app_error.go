package apperror

import (
	"errors"
	"fmt"
	"net/http"
)

type AppError struct {
	Msg        string
	Code       ErrorCode
	StatusCode int
	Err        error
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%v", e.Err)
	}
	return e.Msg
}

func (e *AppError) Unwrap() error {
	return e.Err
}

func New(msg string, code ErrorCode, statusCode int, err error) error {
	return &AppError{
		Msg:        msg,
		Code:       code,
		StatusCode: statusCode,
		Err:        err,
	}
}

func Wrap(msg string, code ErrorCode, statusCode int, err error) error {
	return New(msg, code, statusCode, fmt.Errorf("%s: %w", msg, err))
}

func NewBadRequest(msg string, code ErrorCode) error {
	return New(msg, code, http.StatusBadRequest, errors.New(msg))
}

func WrapBadRequest(msg string, code ErrorCode, err error) error {
	return Wrap(msg, code, http.StatusBadRequest, err)
}

func NewNotFound(msg string, code ErrorCode) error {
	return New(msg, code, http.StatusNotFound, errors.New(msg))
}

func WrapNotFound(msg string, code ErrorCode, err error) error {
	return Wrap(msg, code, http.StatusNotFound, err)
}

func NewUnauthorized(msg string, code ErrorCode) error {
	return New(msg, code, http.StatusUnauthorized, errors.New(msg))
}

func WrapUnauthorized(msg string, code ErrorCode, err error) error {
	return Wrap(msg, code, http.StatusUnauthorized, err)
}

func NewInternal(msg string, code ErrorCode) error {
	return New(msg, code, http.StatusInternalServerError, errors.New(msg))
}

func WrapInternal(msg string, code ErrorCode, err error) error {
	return Wrap(msg, code, http.StatusInternalServerError, err)
}

func NewBadGateway(msg string, code ErrorCode) error {
	return New(msg, code, http.StatusBadGateway, errors.New(msg))
}

func WrapBadGateway(msg string, code ErrorCode, err error) error {
	return Wrap(msg, code, http.StatusBadGateway, err)
}

func NewUnavailable(msg string, code ErrorCode) error {
	return New(msg, code, http.StatusServiceUnavailable, errors.New(msg))
}

func WrapUnavailable(msg string, code ErrorCode, err error) error {
	return Wrap(msg, code, http.StatusServiceUnavailable, err)
}
