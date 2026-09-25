// Package plinthtest provides helpers for testing handlers built with the plinth
// package: building JSON requests, serving them and decoding JSON responses and
// errors.
//
// A typical test:
//
//	func TestCreateUser(t *testing.T) {
//		req := plinthtest.NewRequest(t, http.MethodPost, "/users", createUser{Name: ""})
//
//		rec, reported := plinthtest.Serve(t, handler, req)
//
//		if rec.Code != http.StatusUnprocessableEntity {
//			t.Fatalf("status = %d, want 422", rec.Code)
//		}
//		want := []plinth.FieldViolation{{Field: "name", Code: "required"}}
//		if e := plinthtest.DecodeError(t, rec); !reflect.DeepEqual(e.Details, want) {
//			t.Errorf("details = %v, want %v", e.Details, want)
//		}
//		if reported != nil {
//			t.Errorf("unexpected reported error: %v", reported)
//		}
//	}
//
// To test code that reads the request ID or the client address, put them into
// the request context with middleware.ContextWithRequestID and
// middleware.ContextWithClientIP. To check the status of an error returned by a
// [plinth.ResponseFunc] without serving it, use [plinth.ErrorStatus].
package plinthtest

import (
	"bytes"
	"encoding/json/v2"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/iamroockie/plinth"
)

// Error is the decoded "error" object of an error response. Details is nil
// unless the response has a "details" field, as a [plinth.ValidationError] does.
// Numbers in Params are decoded as float64.
type Error struct {
	Code    plinth.ErrorCode        `json:"code"`
	Message string                  `json:"message"`
	Details []plinth.FieldViolation `json:"details"`
}

// NewRequest returns a request for testing a handler, like
// [httptest.NewRequest], with a context that collects errors reported with
// [plinth.ReportError], see [Serve].
//
// A string or []byte body is sent as is, which is useful for testing invalid
// JSON. Any other non-nil body is encoded as JSON. With a non-nil body the
// Content-Type header is application/json. It calls tb.Fatal if the body cannot
// be encoded.
func NewRequest(tb testing.TB, method, target string, body any) *http.Request {
	tb.Helper()

	var reader io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		reader = strings.NewReader(b)
	case []byte:
		reader = bytes.NewReader(b)
	default:
		data, err := json.Marshal(body)
		if err != nil {
			tb.Fatalf("plinthtest: encode request body: %v", err)
		}
		reader = bytes.NewReader(data)
	}

	ctx := plinth.WithErrorReporter(tb.Context())
	r := httptest.NewRequestWithContext(ctx, method, target, reader)
	if body != nil {
		r.Header.Set(plinth.HeaderContentType, plinth.MIMEApplicationJSON)
	}
	return r
}

// Serve serves r with h and returns the recorded response and the errors
// reported with [plinth.ReportError] while h handled r, joined, or nil if there
// were none.
//
// When h includes the ErrorLog middleware, the middleware logs the reported
// errors itself and Serve returns nil.
func Serve(tb testing.TB, h http.Handler, r *http.Request) (*httptest.ResponseRecorder, error) {
	tb.Helper()

	r = r.WithContext(plinth.WithErrorReporter(r.Context()))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)

	return rec, plinth.FlushReportedError(r.Context())
}

// DecodeJSON decodes the body of a recorded JSON response into a value of type T.
// It calls tb.Fatal if Content-Type is not a JSON media type or the body cannot
// be decoded.
func DecodeJSON[T any](tb testing.TB, rec *httptest.ResponseRecorder) T {
	tb.Helper()

	var v T

	contentType := rec.Header().Get(plinth.HeaderContentType)
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || !isJSON(mediaType) {
		tb.Fatalf("plinthtest: Content-Type %q is not JSON, body: %s", contentType, rec.Body)
		return v
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		tb.Fatalf("plinthtest: decode %T: %v, body: %s", v, err, rec.Body)
	}
	return v
}

// DecodeError decodes the error object of a recorded error response. It calls
// tb.Fatal if the response is not JSON or has no error object.
func DecodeError(tb testing.TB, rec *httptest.ResponseRecorder) Error {
	tb.Helper()

	body := DecodeJSON[struct {
		Error *Error `json:"error"`
	}](tb, rec)
	if body.Error == nil {
		tb.Fatalf("plinthtest: response has no error object, status %d, body: %s",
			rec.Code, rec.Body)
		return Error{}
	}
	return *body.Error
}

func isJSON(mediaType string) bool {
	if mediaType == plinth.MIMEApplicationJSON {
		return true
	}
	subtype, ok := strings.CutPrefix(mediaType, "application/")
	return ok && strings.HasSuffix(subtype, "+json")
}
