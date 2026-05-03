package user

type Operation string

const (
	OperationCreate Operation = "CREATE"
	OperationUpdate Operation = "UPDATE"
	OperationDelete Operation = "DELETE"
)
