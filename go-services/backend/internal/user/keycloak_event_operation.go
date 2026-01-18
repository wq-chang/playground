package user

type KeycloakOperation string

const (
	OperationCreate KeycloakOperation = "CREATE"
	OperationUpdate KeycloakOperation = "UPDATE"
	OperationDelete KeycloakOperation = "DELETE"
)
