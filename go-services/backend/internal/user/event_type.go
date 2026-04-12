package user

type EventType string

const (
	EventTypeUser  EventType = "USER_EVENT"
	EventTypeAdmin EventType = "ADMIN_EVENT"
)
