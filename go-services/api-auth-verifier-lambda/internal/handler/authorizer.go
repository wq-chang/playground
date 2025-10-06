package handler

import (
	"context"
	"log/slog"
	"strings"

	"go-services/api-auth-verifier-lambda/internal/config"

	"github.com/aws/aws-lambda-go/events"
)

// tokenValidator defines the interface for validating JWT tokens.
// Any implementation should provide the Validate method, returning
// true if the token is valid, or false with an error if validation fails.
type tokenValidator interface {
	Validate(string) (bool, error)
}

// HandlerRequest processes an API Gateway request by validating the
// Authorization header using the provided tokenValidator and logger.
//
// It performs the following steps:
//
//  1. Extracts the "Authorization" header (case-insensitive).
//
//  2. Checks that it starts with "Bearer ".
//
//  3. Validates the token using the validator.
//
//  4. Logs any validation errors using the provided slog.Logger.
//
//  5. Returns an appropriate APIGatewayProxyResponse:
//
//     - 200 if the token is valid.
//     - 401 if header is missing or format is invalid.
//     - 401 if the token is invalid.
//     - 500 if validator returns an error.
func HandlerRequest(
	ctx context.Context,
	log *slog.Logger,
	cfg *config.Config,
	request events.APIGatewayProxyRequest,
	validator tokenValidator,
) (events.APIGatewayProxyResponse, error) {
	authHeader := ""
	for k, v := range request.Headers {
		if strings.ToLower(k) == "authorization" {
			authHeader = v
			break
		}
	}
	if authHeader == "" {
		return events.APIGatewayProxyResponse{
			Body:       "missing Authorization header",
			StatusCode: 401,
		}, nil
	}

	if !strings.HasPrefix(authHeader, "Bearer ") {
		return events.APIGatewayProxyResponse{
			Body:       "invalid Authorization header format",
			StatusCode: 401,
		}, nil
	}
	token := strings.TrimPrefix(authHeader, "Bearer ")

	valid, err := validator.Validate(token)
	if err != nil {
		log.Error("token validation failed", "error", err)
		return events.APIGatewayProxyResponse{
			Body:       "failed to validate token",
			StatusCode: 500,
		}, nil
	}

	if valid {
		return events.APIGatewayProxyResponse{Body: "success", StatusCode: 200}, nil
	}

	return events.APIGatewayProxyResponse{Body: "invalid token", StatusCode: 401}, nil
}
