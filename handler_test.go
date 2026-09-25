package plinth

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func serveRespondJSON(t *testing.T, r *http.Request, fn ResponseFunc) (
	*httptest.ResponseRecorder, error,
) {
	t.Helper()

	r = r.WithContext(WithErrorReporter(r.Context()))
	rec := httptest.NewRecorder()
	RespondJSON(fn).ServeHTTP(rec, r)

	return rec, FlushReportedError(r.Context())
}

func TestRespondJSONSuccess(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	rec, reported := serveRespondJSON(t, r, func(*http.Request) (*Response, error) {
		resp := NewResponse(http.StatusCreated, map[string]int{"id": 1})
		return resp.
			SetHeader("X-Single", "a").
			SetHeader("X-Single", "b").
			AddHeader("X-Multi", "1").
			AddHeader("X-Multi", "2"), nil
	})

	if reported != nil {
		t.Errorf("reported error = %v, want nil", reported)
	}
	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if got := rec.Body.String(); got != `{"id":1}` {
		t.Errorf("body = %s, want %s", got, `{"id":1}`)
	}
	if got := rec.Header().Values("X-Single"); !slices.Equal(got, []string{"b"}) {
		t.Errorf("X-Single = %q, want [b]", got)
	}
	if got := rec.Header().Values("X-Multi"); !slices.Equal(got, []string{"1", "2"}) {
		t.Errorf("X-Multi = %q, want [1 2]", got)
	}
	if got := rec.Header().Get(HeaderContentType); got != MIMEApplicationJSON {
		t.Errorf("Content-Type = %q, want %q", got, MIMEApplicationJSON)
	}
}

func TestRespondJSONContentTypeOverride(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	rec, _ := serveRespondJSON(t, r, func(*http.Request) (*Response, error) {
		resp := NewResponse(http.StatusOK, "x")
		return resp.SetHeader(HeaderContentType, "application/problem+json"), nil
	})

	if got := rec.Header().Get(HeaderContentType); got != "application/problem+json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/problem+json")
	}
}

func TestRespondJSONErrors(t *testing.T) {
	expired, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	t.Cleanup(cancel)
	canceled, cancel := context.WithCancel(t.Context())
	cancel()

	tests := []struct {
		name       string
		ctx        context.Context
		resp       *Response
		err        error
		wantStatus int
		wantCode   ErrorCode
		wantReport bool
	}{
		{
			name:       "client error",
			err:        NotFoundError(nil),
			wantStatus: http.StatusNotFound,
			wantCode:   CodeNotFound,
		},
		{
			name:       "wrapped client error",
			err:        fmt.Errorf("load user: %w", ForbiddenError(nil)),
			wantStatus: http.StatusForbidden,
			wantCode:   CodeForbidden,
		},
		{
			name:       "server error is reported",
			err:        ServiceUnavailableError(errors.New("db down")),
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   CodeServiceUnavailable,
			wantReport: true,
		},
		{
			name:       "plain error",
			err:        errors.New("boom"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   CodeInternal,
			wantReport: true,
		},
		{
			name:       "error wins over response",
			resp:       NewResponse(http.StatusOK, "x"),
			err:        ConflictError("", nil),
			wantStatus: http.StatusConflict,
			wantCode:   CodeConflict,
		},
		{
			name:       "no response and no error",
			wantStatus: http.StatusInternalServerError,
			wantCode:   CodeInternal,
			wantReport: true,
		},
		{
			name:       "error with redirect status",
			err:        NewError(http.StatusFound, "moved", "", nil),
			wantStatus: http.StatusInternalServerError,
			wantCode:   CodeInternal,
			wantReport: true,
		},
		{
			name:       "error with status above 599",
			err:        NewError(600, "weird", "", nil),
			wantStatus: http.StatusInternalServerError,
			wantCode:   CodeInternal,
			wantReport: true,
		},
		{
			name:       "request deadline exceeded",
			ctx:        expired,
			err:        fmt.Errorf("query: %w", context.DeadlineExceeded),
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   CodeServiceUnavailable,
			wantReport: true,
		},
		{
			name:       "deadline exceeded without request deadline",
			err:        fmt.Errorf("query: %w", context.DeadlineExceeded),
			wantStatus: http.StatusInternalServerError,
			wantCode:   CodeInternal,
			wantReport: true,
		},
		{
			name:       "other error after request deadline",
			ctx:        expired,
			err:        errors.New("boom"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   CodeInternal,
			wantReport: true,
		},
		{
			name:       "canceled request",
			ctx:        canceled,
			err:        context.Canceled,
			wantStatus: http.StatusInternalServerError,
			wantCode:   CodeInternal,
			wantReport: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := tc.ctx
			if ctx == nil {
				ctx = t.Context()
			}
			r := httptest.NewRequestWithContext(ctx, http.MethodGet, "/", nil)

			rec, reported := serveRespondJSON(t, r, func(*http.Request) (*Response, error) {
				return tc.resp, tc.err
			})

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if body := decodeErrorBody(t, rec); body.Error.Code != tc.wantCode {
				t.Errorf("code = %q, want %q", body.Error.Code, tc.wantCode)
			}
			if (reported != nil) != tc.wantReport {
				t.Errorf("reported error = %v, want reported: %v", reported, tc.wantReport)
			}
			if tc.err != nil && reported != nil &&
				!strings.Contains(reported.Error(), tc.err.Error()) {
				t.Errorf("reported error = %v, want it to contain %v", reported, tc.err)
			}
		})
	}
}

func TestRespondJSONDoesNotLeakCause(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	rec, _ := serveRespondJSON(t, r, func(*http.Request) (*Response, error) {
		return nil, BadRequestError("invalid input", errors.New("secret detail"))
	})

	if strings.Contains(rec.Body.String(), "secret detail") {
		t.Errorf("body = %s, leaks error cause", rec.Body.String())
	}
	if body := decodeErrorBody(t, rec); body.Error.Message != "invalid input" {
		t.Errorf("message = %q, want %q", body.Error.Message, "invalid input")
	}
}

func TestRespondJSONValidationDetails(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	rec, _ := serveRespondJSON(t, r, func(*http.Request) (*Response, error) {
		return nil, ValidationError(FieldViolation{Field: "name", Code: "required"})
	})

	body := decodeErrorBody(t, rec)
	want := []FieldViolation{{Field: "name", Code: "required"}}
	if rec.Code != http.StatusUnprocessableEntity || !reflect.DeepEqual(body.Error.Details, want) {
		t.Errorf("response = %d %s, want 422 with details", rec.Code, rec.Body.String())
	}
}

func TestWriteError(t *testing.T) {
	r := httptest.NewRequestWithContext(WithErrorReporter(t.Context()), http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	WriteError(rec, r, NotFoundError(nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	body := decodeErrorBody(t, rec)
	if body.Error.Code != CodeNotFound || body.Error.Message != "Not Found" {
		t.Errorf("body = %s, want not_found error", rec.Body.String())
	}
}

func TestErrorResponseClosesConnectionOnBodyLimit(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "body limit",
			err:  RequestEntityTooLargeError(&http.MaxBytesError{Limit: 1}),
			want: "close",
		},
		{name: "other error", err: BadRequestError("", nil)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", nil)
			rec := httptest.NewRecorder()

			WriteError(rec, r, tc.err)

			if got := rec.Header().Get(HeaderConnection); got != tc.want {
				t.Errorf("Connection = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBodyLimitClosesConnection(t *testing.T) {
	srv := httptest.NewServer(RespondJSON(func(r *http.Request) (*Response, error) {
		if _, err := ParseRequestJSON[testPayload](r, WithBodyLimit(32)); err != nil {
			return nil, err
		}
		return NewResponse(http.StatusOK, nil), nil
	}))
	t.Cleanup(srv.Close)

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantClosed bool
	}{
		{name: "within limit", body: `{"name":"gopher"}`, wantStatus: http.StatusOK},
		{
			name:       "over limit",
			body:       `{"name":"` + strings.Repeat("x", 100) + `"}`,
			wantStatus: http.StatusRequestEntityTooLarge,
			wantClosed: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			conn, err := net.Dial("tcp", srv.Listener.Addr().String())
			if err != nil {
				t.Fatalf("dial: %v", err)
			}
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

			br := bufio.NewReader(conn)
			if got := postChunked(t, conn, br, tc.body); got != tc.wantStatus {
				t.Fatalf("status = %d, want %d", got, tc.wantStatus)
			}

			if tc.wantClosed {
				if _, err := br.ReadByte(); !errors.Is(err, io.EOF) {
					t.Errorf("read after response = %v, want %v", err, io.EOF)
				}
				return
			}
			if got := postChunked(t, conn, br, tc.body); got != tc.wantStatus {
				t.Errorf("second request on the same connection: status = %d, want %d",
					got, tc.wantStatus)
			}
		})
	}
}

func postChunked(t *testing.T, conn net.Conn, br *bufio.Reader, body string) int {
	t.Helper()

	_, err := fmt.Fprintf(conn, "POST / HTTP/1.1\r\nHost: test\r\n"+
		"Content-Type: application/json\r\nTransfer-Encoding: chunked\r\n\r\n"+
		"%x\r\n%s\r\n0\r\n\r\n", len(body), body)
	if err != nil {
		t.Fatalf("write request: %v", err)
	}

	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	return resp.StatusCode
}

func TestWriteErrorNil(t *testing.T) {
	r := httptest.NewRequestWithContext(WithErrorReporter(t.Context()), http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	WriteError(rec, r, nil)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if err := FlushReportedError(r.Context()); err == nil {
		t.Error("nil error was not reported")
	}
}
