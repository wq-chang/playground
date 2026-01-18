package user

import (
	"github.com/gofrs/uuid/v5"
	"github.com/guregu/null/v6"
)

type KeycloakEvent struct {
	EventType KeycloakEventType          `json:"eventType"`
	Operation KeycloakOperation          `json:"operation"`
	UserID    uuid.UUID                  `json:"userId"`
	Updated   null.Value[UpdatedDetails] `json:"updatedDetails"`
}

type UpdatedDetails struct {
	FirstName null.String `json:"firstName"`
	LastName  null.String `json:"lastName"`
	Username  null.String `json:"username"`
	Email     null.String `json:"email"`
}
