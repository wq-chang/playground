package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"go-services/library/apperror"
)

// SendJSON writes a success JSON response.
func SendJSON(w http.ResponseWriter, status int, data any) error {
	w.Header().Set("Content-Type", ContentTypeJSON)
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(SuccessResponse{Success: true, Data: data})
}

// SendError writes a structured JSON error response.
func SendError(w http.ResponseWriter, status int, code apperror.ErrorCode, message string) error {
	w.Header().Set("Content-Type", ContentTypeJSON)
	w.WriteHeader(status)

	return json.NewEncoder(w).Encode(ErrorResponse{
		Success: false,
		Error: ErrorResponseBody{
			Code:    code,
			Message: message,
		},
	})
}

func SendErrorLog(
	ctx context.Context,
	log *slog.Logger,
	w http.ResponseWriter,
	status int,
	code apperror.ErrorCode,
	message string,
) {
	err := SendError(w, status, code, message)
	if err != nil {
		log.ErrorContext(ctx, "failed to encode response", "err", err)
	}
}
