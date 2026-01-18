package user

type KeycloakEventType string

const (
	EventTypeUser  KeycloakEventType = "USER_EVENT"
	EventTypeAdmin KeycloakEventType = "ADMIN_EVENT"
)
