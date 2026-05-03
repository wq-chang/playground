package user

import (
	"github.com/gofrs/uuid/v5"
	"github.com/guregu/null/v6"
)

type Event struct {
	EventType EventType                  `json:"eventType"`
	Operation Operation                  `json:"operation"`
	Updated   null.Value[UpdatedDetails] `json:"updatedDetails"`
	UserID    uuid.UUID                  `json:"userId"`
}

type UpdatedDetails struct {
	FirstName null.String `json:"firstName"`
	LastName  null.String `json:"lastName"`
	Username  null.String `json:"username"`
	Email     null.String `json:"email"`
}
