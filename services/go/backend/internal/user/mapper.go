package user

import (
	"github.com/gofrs/uuid/v5"
	"github.com/guregu/null/v6"

	"go-services/backend/internal/idp"
	"go-services/backend/internal/pgutil"
	"go-services/backend/internal/user/internal/db"
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

func toCreateUserParams(identityUser idp.User) db.CreateUserParams {
	return db.CreateUserParams{
		ID:        identityUser.ID,
		Username:  identityUser.Username,
		Email:     identityUser.Email,
		FirstName: identityUser.FirstName,
		LastName:  identityUser.LastName,
	}
}

func toUpdateUserParamsFromIDP(identityUser idp.User) db.UpdateUserParams {
	return toUpdateUserParams(identityUser.ID, UpdatedDetails{
		FirstName: null.StringFrom(identityUser.FirstName),
		LastName:  null.StringFrom(identityUser.LastName),
		Username:  null.StringFrom(identityUser.Username),
		Email:     null.StringFrom(identityUser.Email),
	})
}
