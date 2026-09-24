package plinth

import (
	"encoding/json/v2"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type testPayload struct {
	Name string `json:"name"`
}

func newJSONRequest(t *testing.T, contentType, body string) *http.Request {
	t.Helper()

	r := httptest.NewRequestWithContext(
		WithErrorReporter(t.Context()), http.MethodPost, "/", strings.NewReader(body),
	)
	if contentType != "" {
		r.Header.Set(HeaderContentType, contentType)
	}
	return r
}

func TestParseRequestJSON(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		chunked     bool
		opts        []ParseOption
		want        testPayload
		wantStatus  int
	}{
		{
			name:        "valid",
			contentType: MIMEApplicationJSON,
			body:        `{"name":"gopher"}`,
			want:        testPayload{Name: "gopher"},
		},
		{
			name:        "content type with charset",
			contentType: "application/json; charset=utf-8",
			body:        `{"name":"gopher"}`,
			want:        testPayload{Name: "gopher"},
		},
		{
			name:        "json suffix media type",
			contentType: "application/merge-patch+json",
			body:        `{"name":"gopher"}`,
			want:        testPayload{Name: "gopher"},
		},
		{
			name:        "unknown fields ignored by default",
			contentType: MIMEApplicationJSON,
			body:        `{"name":"gopher","age":3}`,
			want:        testPayload{Name: "gopher"},
		},
		{
			name:        "json options applied",
			contentType: MIMEApplicationJSON,
			body:        `{"name":"gopher","age":3}`,
			opts:        []ParseOption{WithJSONOptions(json.RejectUnknownMembers(true))},
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:       "missing content type",
			body:       `{"name":"gopher"}`,
			wantStatus: http.StatusUnsupportedMediaType,
		},
		{
			name:        "wrong content type",
			contentType: "text/plain",
			body:        `{"name":"gopher"}`,
			wantStatus:  http.StatusUnsupportedMediaType,
		},
		{
			name:        "malformed content type",
			contentType: "application/json; charset",
			body:        `{"name":"gopher"}`,
			wantStatus:  http.StatusUnsupportedMediaType,
		},
		{
			name:        "invalid json",
			contentType: MIMEApplicationJSON,
			body:        `{"name":`,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "empty body",
			contentType: MIMEApplicationJSON,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "content length over limit",
			contentType: MIMEApplicationJSON,
			body:        `{"name":"gopher"}`,
			opts:        []ParseOption{WithBodyLimit(8)},
			wantStatus:  http.StatusRequestEntityTooLarge,
		},
		{
			name:        "chunked body over limit",
			contentType: MIMEApplicationJSON,
			body:        `{"name":"gopher"}`,
			chunked:     true,
			opts:        []ParseOption{WithBodyLimit(8)},
			wantStatus:  http.StatusRequestEntityTooLarge,
		},
		{
			name:        "body exactly at limit",
			contentType: MIMEApplicationJSON,
			body:        `{"name":"gopher"}`,
			chunked:     true,
			opts:        []ParseOption{WithBodyLimit(int64(len(`{"name":"gopher"}`)))},
			want:        testPayload{Name: "gopher"},
		},
		{
			name:        "default limit",
			contentType: MIMEApplicationJSON,
			body:        `{"name":"` + strings.Repeat("x", int(DefaultBodyLimit)) + `"}`,
			wantStatus:  http.StatusRequestEntityTooLarge,
		},
		{
			name:        "without body limit",
			contentType: MIMEApplicationJSON,
			body:        `{"name":"` + strings.Repeat("x", int(DefaultBodyLimit)) + `"}`,
			chunked:     true,
			opts:        []ParseOption{WithBodyLimit(8), WithoutBodyLimit()},
			want:        testPayload{Name: strings.Repeat("x", int(DefaultBodyLimit))},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newJSONRequest(t, tc.contentType, tc.body)
			if tc.chunked {
				r.ContentLength = -1
			}

			got, err := ParseRequestJSON[testPayload](r, tc.opts...)

			if tc.wantStatus == 0 {
				if err != nil {
					t.Fatalf("ParseRequestJSON: %v", err)
				}
				if got != tc.want {
					t.Errorf("payload = %+v, want %+v", got, tc.want)
				}
				return
			}

			e := asAPIError(t, err)
			if e.status != tc.wantStatus {
				t.Errorf("status = %d, want %d (err: %v)", e.status, tc.wantStatus, err)
			}
			if got != (testPayload{}) {
				t.Errorf("payload = %+v, want zero value on error", got)
			}
			_, isMaxBytes := errors.AsType[*http.MaxBytesError](err)
			wantMaxBytes := tc.wantStatus == http.StatusRequestEntityTooLarge
			if isMaxBytes != wantMaxBytes {
				t.Errorf("wraps *http.MaxBytesError = %v, want %v", isMaxBytes, wantMaxBytes)
			}
		})
	}
}

func TestWithBodyLimitPanicsOnNonPositive(t *testing.T) {
	for _, n := range []int64{0, -1} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("WithBodyLimit(%d) did not panic", n)
				}
			}()
			WithBodyLimit(n)
		}()
	}
}

func TestWriteJSON(t *testing.T) {
	r := httptest.NewRequestWithContext(WithErrorReporter(t.Context()), http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	WriteJSON(rec, r, http.StatusCreated, map[string]string{"name": "gopher"})

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if got := rec.Header().Get(HeaderContentType); got != MIMEApplicationJSON {
		t.Errorf("Content-Type = %q, want %q", got, MIMEApplicationJSON)
	}
	if got := rec.Body.String(); got != `{"name":"gopher"}` {
		t.Errorf("body = %s, want %s", got, `{"name":"gopher"}`)
	}
	if err := FlushReportedError(r.Context()); err != nil {
		t.Errorf("reported error = %v, want nil", err)
	}
}

func TestWriteJSONKeepsContentType(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	rec.Header().Set(HeaderContentType, "application/problem+json")

	WriteJSON(rec, r, http.StatusOK, nil)

	if got := rec.Header().Get(HeaderContentType); got != "application/problem+json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/problem+json")
	}
	if got := rec.Body.String(); got != "null" {
		t.Errorf("body = %q, want %q", got, "null")
	}
}

func TestWriteJSONBodylessStatus(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		payload    any
		wantReport bool
	}{
		{name: "204 without payload", status: http.StatusNoContent},
		{name: "205 without payload", status: http.StatusResetContent},
		{name: "304 without payload", status: http.StatusNotModified},
		{
			name:       "204 with payload",
			status:     http.StatusNoContent,
			payload:    map[string]string{"a": "b"},
			wantReport: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequestWithContext(
				WithErrorReporter(t.Context()), http.MethodGet, "/", nil,
			)
			rec := httptest.NewRecorder()

			WriteJSON(rec, r, tc.status, tc.payload)

			if rec.Code != tc.status {
				t.Errorf("status = %d, want %d", rec.Code, tc.status)
			}
			if rec.Body.Len() != 0 {
				t.Errorf("body = %q, want empty", rec.Body.String())
			}
			if got := rec.Header().Get(HeaderContentType); got != "" {
				t.Errorf("Content-Type = %q, want empty", got)
			}
			if err := FlushReportedError(r.Context()); (err != nil) != tc.wantReport {
				t.Errorf("reported error = %v, want reported: %v", err, tc.wantReport)
			}
		})
	}
}

func TestWriteJSONFallsBackToInternalError(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		payload any
	}{
		{name: "informational status", status: http.StatusContinue, payload: "x"},
		{name: "status above 599", status: 600, payload: "x"},
		{name: "unmarshalable payload", status: http.StatusOK, payload: make(chan int)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequestWithContext(
				WithErrorReporter(t.Context()), http.MethodGet, "/", nil,
			)
			rec := httptest.NewRecorder()

			WriteJSON(rec, r, tc.status, tc.payload)

			if rec.Code != http.StatusInternalServerError {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
			}
			body := decodeErrorBody(t, rec)
			if body.Error.Code != CodeInternal {
				t.Errorf("code = %q, want %q", body.Error.Code, CodeInternal)
			}
			if err := FlushReportedError(r.Context()); err == nil {
				t.Error("no error reported")
			}
		})
	}
}

type failingWriter struct {
	*httptest.ResponseRecorder
}

var errWriteFailed = errors.New("write failed")

func (failingWriter) Write([]byte) (int, error) {
	return 0, errWriteFailed
}

func TestWriteJSONReportsWriteError(t *testing.T) {
	r := httptest.NewRequestWithContext(WithErrorReporter(t.Context()), http.MethodGet, "/", nil)

	WriteJSON(failingWriter{httptest.NewRecorder()}, r, http.StatusOK, "x")

	if err := FlushReportedError(r.Context()); !errors.Is(err, errWriteFailed) {
		t.Errorf("reported error = %v, want %v", err, errWriteFailed)
	}
}

func TestIsJSONMediaType(t *testing.T) {
	tests := []struct {
		mediaType string
		want      bool
	}{
		{mediaType: "application/json", want: true},
		{mediaType: "application/problem+json", want: true},
		{mediaType: "application/vnd.api+json", want: true},
		{mediaType: "application/jsonx", want: false},
		{mediaType: "application/json+xml", want: false},
		{mediaType: "text/json", want: false},
		{mediaType: "text/plain", want: false},
		{mediaType: "application/+json", want: true},
		{mediaType: "", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.mediaType, func(t *testing.T) {
			if got := isJSONMediaType(tc.mediaType); got != tc.want {
				t.Errorf("isJSONMediaType(%q) = %v, want %v", tc.mediaType, got, tc.want)
			}
		})
	}
}
