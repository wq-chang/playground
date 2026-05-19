package idp

import "github.com/gofrs/uuid/v5"

type User struct {
	FirstName string
	LastName  string
	Email     string
	Username  string
	ID        uuid.UUID
}
