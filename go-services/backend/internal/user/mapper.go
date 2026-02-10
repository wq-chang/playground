package user

import (
	"go-services/backend/internal/pgutil"
	"go-services/backend/internal/user/internal/db"

	"github.com/gofrs/uuid/v5"
)

func toUpdateUserParams(userID uuid.UUID, details UpdatedDetails) db.UpdateUserParams {
	return db.UpdateUserParams{
		ID:        userID,
		Username:  pgutil.ToPGText(details.Username),
		Email:     pgutil.ToPGText(details.Email),
		FirstName: pgutil.ToPGText(details.FirstName),
		LastName:  pgutil.ToPGText(details.LastName),
	}
}
