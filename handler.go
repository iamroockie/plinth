package plinth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// Response is a JSON response returned from a [ResponseFunc]: a status, a payload
// and optional headers.
type Response struct {
	status  int
	payload any
	header  []headerOp
}

type headerOp struct {
	key     string
	value   string
	replace bool
}

// NewResponse returns a response with the given status and payload. See
// [WriteJSON] for how they are written.
func NewResponse(status int, payload any) *Response {
	return &Response{status: status, payload: payload}
}

// SetHeader sets a response header, replacing its earlier values, and returns r.
func (r *Response) SetHeader(key, value string) *Response {
	r.header = append(r.header, headerOp{key: key, value: value, replace: true})
	return r
}

// AddHeader adds a value to a response header and returns r.
func (r *Response) AddHeader(key, value string) *Response {
	r.header = append(r.header, headerOp{key: key, value: value})
	return r
}

// ResponseFunc handles a request and returns a response or an error. See
// [RespondJSON].
type ResponseFunc func(r *http.Request) (*Response, error)

// RespondJSON returns a handler that calls fn and writes its result as JSON.
//
// If fn returns an error, the response is built from it:
//   - errors created with [NewError] or its helpers, also when wrapped, keep their
//     status and code;
//   - an error wrapping [context.DeadlineExceeded] after the request's deadline
//     has passed becomes a 503 with [CodeServiceUnavailable], see the Timeout
//     middleware;
//   - any other error becomes a 500 with [CodeInternal].
//
// The cause of an error is never sent to the client. Errors with a 5xx status are
// reported with [ReportError]. Returning a nil response and a nil error is treated
// as an internal error.
func RespondJSON(fn ResponseFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp, err := fn(r)
		if err != nil {
			resp = errorResponse(r.Context(), err)
		}

		if resp == nil {
			resp = errorResponse(r.Context(), errors.New("handler returned no response"))
		}

		writeJSON(w, r, resp.status, resp.payload, resp.header)
	}
}

func applyHeader(dst http.Header, ops []headerOp) {
	for _, op := range ops {
		if op.replace {
			dst.Set(op.key, op.value)
		} else {
			dst.Add(op.key, op.value)
		}
	}
}

func errorResponse(ctx context.Context, err error) *Response {
	if err == nil {
		err = errors.New("error response without error")
	}

	e, ok := errors.AsType[apiError](err)
	switch {
	case ok:
	case isRequestTimeout(ctx, err):
		e = newAPIError(http.StatusServiceUnavailable, CodeServiceUnavailable, "", err)
	default:
		e = internalError(err)
	}

	if e.status < http.StatusBadRequest || e.status > 599 {
		err = fmt.Errorf("invalid error response status %d: %w", e.status, err)
		e = internalError(err)
	}

	if e.status >= http.StatusInternalServerError {
		ReportError(ctx, err)
	}

	resp := NewResponse(e.status, errorEnvelope{e})
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		resp.SetHeader(HeaderConnection, "close")
	}

	return resp
}

func isRequestTimeout(ctx context.Context, err error) bool {
	return errors.Is(err, context.DeadlineExceeded) &&
		errors.Is(ctx.Err(), context.DeadlineExceeded)
}

// WriteError writes err as a JSON error response, following the same rules as
// [RespondJSON]. Use it in handlers that do not use RespondJSON.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	resp := errorResponse(r.Context(), err)

	writeJSON(w, r, resp.status, resp.payload, resp.header)
}
