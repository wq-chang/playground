package apperror

type ErrorCode string

const (
	// 4xx Client Errors - Validation
	CodeInvalidInput  ErrorCode = "INVALID_INPUT"
	CodeMissingField  ErrorCode = "MISSING_FIELD"
	CodeInvalidFormat ErrorCode = "INVALID_FORMAT"
	CodeTooLarge      ErrorCode = "TOO_LARGE"
	CodeOutOfRange    ErrorCode = "OUT_OF_RANGE"

	// 4xx Client Errors - Auth / Access
	CodeUnauthorized  ErrorCode = "UNAUTHORIZED"
	CodeForbidden     ErrorCode = "FORBIDDEN"
	CodeTokenExpired  ErrorCode = "TOKEN_EXPIRED"
	CodeAccountLocked ErrorCode = "ACCOUNT_LOCKED"

	// 4xx Client Errors - Resource
	CodeNotFound             ErrorCode = "NOT_FOUND"
	CodeDuplicateRecord      ErrorCode = "DUPLICATE_RECORD"
	CodeConflict             ErrorCode = "CONFLICT"
	CodeTooManyRequests      ErrorCode = "TOO_MANY_REQUESTS"
	CodeUnsupportedMediaType ErrorCode = "UNSUPPORTED_MEDIA_TYPE"
	CodeNotAcceptable        ErrorCode = "NOT_ACCEPTABLE"

	// 5xx Server Errors - General
	CodeInternalError      ErrorCode = "INTERNAL_ERROR"
	CodeServiceUnavailable ErrorCode = "SERVICE_UNAVAILABLE"
	CodeGatewayTimeout     ErrorCode = "GATEWAY_TIMEOUT"
	CodeDependencyFailed   ErrorCode = "DEPENDENCY_FAILED"
	CodeSerializationError ErrorCode = "SERIALIZATION_ERROR"

	// 5xx Server Errors - Database
	CodeDBConnection   ErrorCode = "DB_CONNECTION_FAILED"
	CodeDBTimeout      ErrorCode = "DB_TIMEOUT"
	CodeDBTransaction  ErrorCode = "DB_TRANSACTION_FAILED"
	CodeRecordLocked   ErrorCode = "RECORD_LOCKED"
	CodeDataCorruption ErrorCode = "DATA_CORRUPTION"

	// 5xx Server Errors - Network / External
	CodeConnectionFailed ErrorCode = "CONNECTION_FAILED"
	CodeExternalService  ErrorCode = "EXTERNAL_SERVICE_ERROR"
	CodeDNSResolution    ErrorCode = "DNS_RESOLUTION_FAILED"
	CodeRequestTimeout   ErrorCode = "REQUEST_TIMEOUT"
)
