package user

import (
	"go-services/backend/internal/postgres"

	"github.com/gofrs/uuid/v5"
)

func toUpdateUserParams(userID uuid.UUID, details UpdatedDetails) postgres.UpdateUserParams {
	return postgres.UpdateUserParams{
		ID:        userID,
		Username:  postgres.ToPGText(details.Username),
		Email:     postgres.ToPGText(details.Email),
		FirstName: postgres.ToPGText(details.FirstName),
		LastName:  postgres.ToPGText(details.LastName),
	}
}
