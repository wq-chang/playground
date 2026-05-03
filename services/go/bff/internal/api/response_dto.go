package api

import "go-services/library/apperror"

type SuccessResponse struct {
	Data    any  `json:"data,omitempty"`
	Success bool `json:"success"`
}

type ErrorResponse struct {
	Error   ErrorResponseBody `json:"error"`
	Success bool              `json:"success"`
}

type ErrorResponseBody struct {
	Code    apperror.ErrorCode `json:"code"`
	Message string             `json:"message"`
}
