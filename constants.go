package plinth

// Error codes sent in the "code" field of error responses. Clients can rely on
// them to tell errors apart; the messages are meant for people.
const (
	CodeBadRequest            ErrorCode = "bad_request"
	CodeConflict              ErrorCode = "conflict"
	CodeForbidden             ErrorCode = "forbidden"
	CodeGatewayTimeout        ErrorCode = "gateway_timeout"
	CodeInternal              ErrorCode = "internal_error"
	CodeMethodNotAllowed      ErrorCode = "method_not_allowed"
	CodeNotFound              ErrorCode = "not_found"
	CodeRequestEntityTooLarge ErrorCode = "request_entity_too_large"
	CodeServiceUnavailable    ErrorCode = "service_unavailable"
	CodeTooManyRequests       ErrorCode = "too_many_requests"
	CodeUnauthorized          ErrorCode = "unauthorized"
	CodeUnsupportedMediaType  ErrorCode = "unsupported_media_type"
	CodeValidation            ErrorCode = "validation_error"
)

// Header names used by the package.
const (
	HeaderAllow         = "Allow"
	HeaderConnection    = "Connection"
	HeaderContentType   = "Content-Type"
	HeaderXForwardedFor = "X-Forwarded-For"
	HeaderXRealIP       = "X-Real-Ip"
	HeaderXRequestID    = "X-Request-Id"
)

// Media types used by the package.
const (
	MIMEApplicationJSON = "application/json"
)
