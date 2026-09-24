package plinth

import (
	"errors"
	"net/http"
)

// ErrorCode is a machine-readable error identifier sent in the "code" field of
// error responses.
type ErrorCode string

// ErrorDetails holds messages about individual fields, sent in the "details"
// field of error responses. See [ValidationError].
type ErrorDetails map[string]string

type errorEnvelope struct {
	Error apiError `json:"error"`
}

type apiError struct {
	Code    ErrorCode    `json:"code"`
	Message string       `json:"message"`
	Details ErrorDetails `json:"details,omitempty"`

	status int
	cause  error
}

// NewError returns an error that [RespondJSON] and [WriteError] turn into a JSON
// response with the given status and code.
//
// An empty message defaults to the status text. For a status without one it
// defaults to "Bad Request" for 4xx statuses and "Internal Server Error" otherwise.
// The cause is available to [errors.Is], [errors.As] and logs but is never sent to
// the client. The status must be 4xx or 5xx: an error with any other status is
// answered with 500.
func NewError(status int, code ErrorCode, message string, cause error) error {
	return newAPIError(status, code, message, cause)
}

func newAPIError(status int, code ErrorCode, message string, cause error) apiError {
	if message == "" {
		message = http.StatusText(status)
	}
	if message == "" {
		message = http.StatusText(fallbackStatus(status))
	}

	return apiError{
		Code:    code,
		Message: message,
		status:  status,
		cause:   cause,
	}
}

// InternalServerError returns a 500 error with code [CodeInternal].
func InternalServerError(cause error) error {
	return NewError(http.StatusInternalServerError, CodeInternal, "", cause)
}

// BadRequestError returns a 400 error with code [CodeBadRequest]. An empty
// message defaults to "Bad Request".
func BadRequestError(message string, cause error) error {
	return NewError(http.StatusBadRequest, CodeBadRequest, message, cause)
}

// NotFoundError returns a 404 error with code [CodeNotFound].
func NotFoundError(cause error) error {
	return NewError(http.StatusNotFound, CodeNotFound, "", cause)
}

// UnauthorizedError returns a 401 error with code [CodeUnauthorized].
func UnauthorizedError(cause error) error {
	return NewError(http.StatusUnauthorized, CodeUnauthorized, "", cause)
}

// ForbiddenError returns a 403 error with code [CodeForbidden].
func ForbiddenError(cause error) error {
	return NewError(http.StatusForbidden, CodeForbidden, "", cause)
}

// ConflictError returns a 409 error with code [CodeConflict]. An empty message
// defaults to "Conflict".
func ConflictError(message string, cause error) error {
	return NewError(http.StatusConflict, CodeConflict, message, cause)
}

// TooManyRequestsError returns a 429 error with code [CodeTooManyRequests].
func TooManyRequestsError(cause error) error {
	return NewError(http.StatusTooManyRequests, CodeTooManyRequests, "", cause)
}

// MethodNotAllowedError returns a 405 error with code [CodeMethodNotAllowed].
func MethodNotAllowedError(cause error) error {
	return NewError(http.StatusMethodNotAllowed, CodeMethodNotAllowed, "", cause)
}

// UnsupportedMediaTypeError returns a 415 error with code
// [CodeUnsupportedMediaType].
func UnsupportedMediaTypeError(cause error) error {
	return NewError(http.StatusUnsupportedMediaType, CodeUnsupportedMediaType, "", cause)
}

// ServiceUnavailableError returns a 503 error with code [CodeServiceUnavailable].
func ServiceUnavailableError(cause error) error {
	return NewError(http.StatusServiceUnavailable, CodeServiceUnavailable, "", cause)
}

// GatewayTimeoutError returns a 504 error with code [CodeGatewayTimeout].
func GatewayTimeoutError(cause error) error {
	return NewError(http.StatusGatewayTimeout, CodeGatewayTimeout, "", cause)
}

// RequestEntityTooLargeError returns a 413 error with code
// [CodeRequestEntityTooLarge].
func RequestEntityTooLargeError(cause error) error {
	return NewError(http.StatusRequestEntityTooLarge, CodeRequestEntityTooLarge, "", cause)
}

// ValidationError returns a 422 error with code [CodeValidation] and the message
// "Validation failed". The details are sent to the client, so they must not
// contain anything private.
func ValidationError(details ErrorDetails) error {
	return apiError{
		Code:    CodeValidation,
		Message: "Validation failed",
		Details: details,
		status:  http.StatusUnprocessableEntity,
		cause:   nil,
	}
}

// ErrorStatus returns the status and code carried by err or by an error it wraps,
// if it was created with [NewError] or one of its helpers. For any other error ok
// is false; [RespondJSON] and [WriteError] answer such errors with 500, or with
// 503 after the request's deadline has passed.
func ErrorStatus(err error) (status int, code ErrorCode, ok bool) {
	e, ok := errors.AsType[apiError](err)
	if !ok {
		return 0, "", false
	}
	return e.status, e.Code, true
}

func internalError(cause error) apiError {
	return apiError{
		Code:    CodeInternal,
		Message: http.StatusText(http.StatusInternalServerError),
		status:  http.StatusInternalServerError,
		cause:   cause,
	}
}

func fallbackStatus(status int) int {
	if status >= http.StatusBadRequest && status < http.StatusInternalServerError {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

func (e apiError) Error() string {
	if e.cause == nil {
		return e.Message
	}

	if e.Message == "" {
		return e.cause.Error()
	}

	return e.Message + ": " + e.cause.Error()
}

func (e apiError) Unwrap() error {
	return e.cause
}
