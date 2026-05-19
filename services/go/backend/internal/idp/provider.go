package idp

import (
	"context"

	"github.com/gofrs/uuid/v5"
)

type UserProvider interface {
	GetUserByID(ctx context.Context, userID uuid.UUID) (User, error)
}
