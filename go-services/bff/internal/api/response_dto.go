package api

import "go-services/library/apperror"

type SuccessResponse struct {
	Success bool `json:"success"`
	Data    any  `json:"data,omitempty"`
}

type ErrorResponse struct {
	Success bool              `json:"success"`
	Error   ErrorResponseBody `json:"error"`
}

type ErrorResponseBody struct {
	Code    apperror.ErrorCode `json:"code"`
	Message string             `json:"message"`
}
