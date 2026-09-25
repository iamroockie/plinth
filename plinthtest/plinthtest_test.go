package plinthtest_test

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/iamroockie/plinth"
	"github.com/iamroockie/plinth/middleware"
	"github.com/iamroockie/plinth/plinthtest"
)

type fakeTB struct {
	testing.TB

	failed  bool
	message string
}

func (f *fakeTB) Helper() {}

func (f *fakeTB) Fatalf(format string, args ...any) {
	f.failed = true
	f.message = fmt.Sprintf(format, args...)
	runtime.Goexit()
}

func expectFatal(t *testing.T, fn func(tb testing.TB)) string {
	t.Helper()

	fake := &fakeTB{TB: t}
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn(fake)
	}()
	<-done

	if !fake.failed {
		t.Fatal("helper did not call Fatalf")
	}
	return fake.message
}

func recorder(contentType, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	if contentType != "" {
		rec.Header().Set(plinth.HeaderContentType, contentType)
	}
	_, _ = rec.WriteString(body)
	return rec
}

func TestNewRequest(t *testing.T) {
	tests := []struct {
		name            string
		body            any
		wantBody        string
		wantContentType string
	}{
		{name: "no body"},
		{
			name:            "value encoded as json",
			body:            map[string]string{"name": "gopher"},
			wantBody:        `{"name":"gopher"}`,
			wantContentType: plinth.MIMEApplicationJSON,
		},
		{
			name:            "string sent as is",
			body:            `{"name":`,
			wantBody:        `{"name":`,
			wantContentType: plinth.MIMEApplicationJSON,
		},
		{
			name:            "bytes sent as is",
			body:            []byte(`[1,2]`),
			wantBody:        `[1,2]`,
			wantContentType: plinth.MIMEApplicationJSON,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := plinthtest.NewRequest(t, http.MethodPost, "/users?x=1", tc.body)

			if r.Method != http.MethodPost || r.URL.Path != "/users" || r.URL.RawQuery != "x=1" {
				t.Errorf("request = %s %s, want POST /users?x=1", r.Method, r.URL)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if string(body) != tc.wantBody {
				t.Errorf("body = %q, want %q", body, tc.wantBody)
			}
			if got := r.Header.Get(plinth.HeaderContentType); got != tc.wantContentType {
				t.Errorf("Content-Type = %q, want %q", got, tc.wantContentType)
			}
		})
	}
}

func TestNewRequestCollectsReportedErrors(t *testing.T) {
	r := plinthtest.NewRequest(t, http.MethodGet, "/", nil)
	errBoom := errors.New("boom")

	plinth.ReportError(r.Context(), errBoom)

	if err := plinth.FlushReportedError(r.Context()); !errors.Is(err, errBoom) {
		t.Errorf("reported error = %v, want %v", err, errBoom)
	}
}

func TestNewRequestFailsOnUnencodableBody(t *testing.T) {
	msg := expectFatal(t, func(tb testing.TB) {
		tb.Helper()
		plinthtest.NewRequest(tb, http.MethodPost, "/", make(chan int))
	})

	if !strings.Contains(msg, "encode request body") {
		t.Errorf("message = %q, want it to mention the request body", msg)
	}
}

func TestServe(t *testing.T) {
	errBoom := errors.New("boom")
	h := plinth.RespondJSON(func(r *http.Request) (*plinth.Response, error) {
		if r.URL.Query().Get("fail") != "" {
			return nil, errBoom
		}
		return plinth.NewResponse(http.StatusOK, map[string]int{"id": 1}), nil
	})

	tests := []struct {
		name       string
		request    *http.Request
		wantStatus int
		wantErr    error
	}{
		{
			name:       "success",
			request:    plinthtest.NewRequest(t, http.MethodGet, "/", nil),
			wantStatus: http.StatusOK,
		},
		{
			name:       "reported error",
			request:    plinthtest.NewRequest(t, http.MethodGet, "/?fail=1", nil),
			wantStatus: http.StatusInternalServerError,
			wantErr:    errBoom,
		},
		{
			name:       "request without reporter",
			request:    httptest.NewRequest(http.MethodGet, "/?fail=1", nil),
			wantStatus: http.StatusInternalServerError,
			wantErr:    errBoom,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec, reported := plinthtest.Serve(t, h, tc.request)

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if !errors.Is(reported, tc.wantErr) {
				t.Errorf("reported error = %v, want %v", reported, tc.wantErr)
			}
		})
	}
}

func TestServeWithErrorLogReturnsNil(t *testing.T) {
	var logs strings.Builder
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	h := middleware.ErrorLog(logger)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		plinth.ReportError(r.Context(), errors.New("logged by ErrorLog"))
	}))

	_, reported := plinthtest.Serve(t, h, plinthtest.NewRequest(t, http.MethodGet, "/", nil))

	if reported != nil {
		t.Errorf("reported error = %v, want nil", reported)
	}
	if !strings.Contains(logs.String(), "logged by ErrorLog") {
		t.Errorf("log = %q, want the error logged by ErrorLog", logs.String())
	}
}

func TestDecodeJSON(t *testing.T) {
	type user struct {
		ID int `json:"id"`
	}

	tests := []struct {
		name        string
		contentType string
	}{
		{name: "json", contentType: plinth.MIMEApplicationJSON},
		{name: "json with charset", contentType: "application/json; charset=utf-8"},
		{name: "json suffix", contentType: "application/problem+json"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := plinthtest.DecodeJSON[user](t, recorder(tc.contentType, `{"id":7}`))
			if got.ID != 7 {
				t.Errorf("DecodeJSON = %+v, want ID 7", got)
			}
		})
	}
}

func TestDecodeJSONFails(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		want        string
	}{
		{name: "no content type", body: `{}`, want: "is not JSON"},
		{name: "text", contentType: "text/plain", body: `{}`, want: "is not JSON"},
		{
			name:        "malformed content type",
			contentType: "application/json; charset",
			body:        `{}`,
			want:        "is not JSON",
		},
		{
			name:        "invalid body",
			contentType: plinth.MIMEApplicationJSON,
			body:        `{"id":`,
			want:        "decode",
		},
		{
			name:        "wrong type",
			contentType: plinth.MIMEApplicationJSON,
			body:        `{"id":"seven"}`,
			want:        "decode",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg := expectFatal(t, func(tb testing.TB) {
				tb.Helper()
				plinthtest.DecodeJSON[struct {
					ID int `json:"id"`
				}](tb, recorder(tc.contentType, tc.body))
			})

			if !strings.Contains(msg, tc.want) {
				t.Errorf("message = %q, want it to contain %q", msg, tc.want)
			}
		})
	}
}

func TestDecodeError(t *testing.T) {
	rec := httptest.NewRecorder()
	plinth.WriteError(rec, httptest.NewRequest(http.MethodPost, "/", nil),
		plinth.ValidationError(
			plinth.FieldViolation{
				Field: "age", Code: "out_of_range", Params: plinth.Map{"max": 150},
			},
			plinth.FieldViolation{Field: "name", Code: "required"},
		))

	got := plinthtest.DecodeError(t, rec)

	want := []plinth.FieldViolation{
		{Field: "age", Code: "out_of_range", Params: plinth.Map{"max": float64(150)}},
		{Field: "name", Code: "required"},
	}
	if got.Code != plinth.CodeValidation || got.Message != "Validation failed" ||
		!reflect.DeepEqual(got.Details, want) {
		t.Errorf("DecodeError = %+v, want the validation error", got)
	}
}

func TestDecodeErrorFails(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		want        string
	}{
		{
			name:        "success response",
			contentType: plinth.MIMEApplicationJSON,
			body:        `{"id":1}`,
			want:        "no error object",
		},
		{name: "not json", contentType: "text/plain", body: "oops", want: "is not JSON"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg := expectFatal(t, func(tb testing.TB) {
				tb.Helper()
				plinthtest.DecodeError(tb, recorder(tc.contentType, tc.body))
			})

			if !strings.Contains(msg, tc.want) {
				t.Errorf("message = %q, want it to contain %q", msg, tc.want)
			}
		})
	}
}
