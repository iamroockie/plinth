package plinth

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestErrorConstructors(t *testing.T) {
	cause := errors.New("cause")

	tests := []struct {
		name    string
		err     error
		status  int
		code    ErrorCode
		message string
	}{
		{
			name:    "internal",
			err:     InternalServerError(cause),
			status:  http.StatusInternalServerError,
			code:    CodeInternal,
			message: "Internal Server Error",
		},
		{
			name:    "bad request default message",
			err:     BadRequestError("", cause),
			status:  http.StatusBadRequest,
			code:    CodeBadRequest,
			message: "Bad Request",
		},
		{
			name:    "bad request custom message",
			err:     BadRequestError("missing name", cause),
			status:  http.StatusBadRequest,
			code:    CodeBadRequest,
			message: "missing name",
		},
		{
			name:    "not found",
			err:     NotFoundError(cause),
			status:  http.StatusNotFound,
			code:    CodeNotFound,
			message: "Not Found",
		},
		{
			name:    "unauthorized",
			err:     UnauthorizedError(cause),
			status:  http.StatusUnauthorized,
			code:    CodeUnauthorized,
			message: "Unauthorized",
		},
		{
			name:    "forbidden",
			err:     ForbiddenError(cause),
			status:  http.StatusForbidden,
			code:    CodeForbidden,
			message: "Forbidden",
		},
		{
			name:    "conflict",
			err:     ConflictError("already exists", cause),
			status:  http.StatusConflict,
			code:    CodeConflict,
			message: "already exists",
		},
		{
			name:    "too many requests",
			err:     TooManyRequestsError(cause),
			status:  http.StatusTooManyRequests,
			code:    CodeTooManyRequests,
			message: "Too Many Requests",
		},
		{
			name:    "method not allowed",
			err:     MethodNotAllowedError(cause),
			status:  http.StatusMethodNotAllowed,
			code:    CodeMethodNotAllowed,
			message: "Method Not Allowed",
		},
		{
			name:    "unsupported media type",
			err:     UnsupportedMediaTypeError(cause),
			status:  http.StatusUnsupportedMediaType,
			code:    CodeUnsupportedMediaType,
			message: "Unsupported Media Type",
		},
		{
			name:    "service unavailable",
			err:     ServiceUnavailableError(cause),
			status:  http.StatusServiceUnavailable,
			code:    CodeServiceUnavailable,
			message: "Service Unavailable",
		},
		{
			name:    "gateway timeout",
			err:     GatewayTimeoutError(cause),
			status:  http.StatusGatewayTimeout,
			code:    CodeGatewayTimeout,
			message: "Gateway Timeout",
		},
		{
			name:    "request entity too large",
			err:     RequestEntityTooLargeError(cause),
			status:  http.StatusRequestEntityTooLarge,
			code:    CodeRequestEntityTooLarge,
			message: "Request Entity Too Large",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := asAPIError(t, tc.err)
			if e.status != tc.status || e.Code != tc.code || e.Message != tc.message {
				t.Errorf(
					"got (%d, %q, %q), want (%d, %q, %q)",
					e.status, e.Code, e.Message, tc.status, tc.code, tc.message,
				)
			}
			if !errors.Is(tc.err, cause) {
				t.Errorf("errors.Is(%v, cause) = false, want true", tc.err)
			}
		})
	}
}

func TestNewErrorMessageFallback(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		message string
		want    string
	}{
		{name: "custom message", status: http.StatusTeapot, message: "brew", want: "brew"},
		{name: "known status", status: http.StatusTeapot, want: "I'm a teapot"},
		{name: "unknown 4xx", status: 499, want: "Bad Request"},
		{name: "unknown 5xx", status: 599, want: "Internal Server Error"},
		{name: "unknown out of range", status: 700, want: "Internal Server Error"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := asAPIError(t, NewError(tc.status, "code", tc.message, nil))
			if e.Message != tc.want {
				t.Errorf("message = %q, want %q", e.Message, tc.want)
			}
			if e.status != tc.status {
				t.Errorf("status = %d, want %d", e.status, tc.status)
			}
		})
	}
}

func TestAPIErrorString(t *testing.T) {
	cause := errors.New("db down")

	tests := []struct {
		name string
		err  apiError
		want string
	}{
		{name: "message only", err: apiError{Message: "Not Found"}, want: "Not Found"},
		{
			name: "message and cause",
			err:  apiError{Message: "Not Found", cause: cause},
			want: "Not Found: db down",
		},
		{name: "cause only", err: apiError{cause: cause}, want: "db down"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.err.Error(); got != tc.want {
				t.Errorf("Error() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestErrorStatus(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   ErrorCode
		wantOK     bool
	}{
		{
			name:       "api error",
			err:        NotFoundError(nil),
			wantStatus: http.StatusNotFound,
			wantCode:   CodeNotFound,
			wantOK:     true,
		},
		{
			name:       "wrapped api error",
			err:        fmt.Errorf("load user: %w", ConflictError("taken", nil)),
			wantStatus: http.StatusConflict,
			wantCode:   CodeConflict,
			wantOK:     true,
		},
		{
			name:       "custom api error",
			err:        NewError(http.StatusTeapot, "teapot", "", nil),
			wantStatus: http.StatusTeapot,
			wantCode:   "teapot",
			wantOK:     true,
		},
		{name: "plain error", err: errors.New("boom")},
		{name: "nil error"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status, code, ok := ErrorStatus(tc.err)
			if status != tc.wantStatus || code != tc.wantCode || ok != tc.wantOK {
				t.Errorf("ErrorStatus = (%d, %q, %v), want (%d, %q, %v)",
					status, code, ok, tc.wantStatus, tc.wantCode, tc.wantOK)
			}
		})
	}
}
