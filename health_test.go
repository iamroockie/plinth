package plinth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func serveHealth(t *testing.T, r *http.Request, h http.Handler) (
	*httptest.ResponseRecorder, error,
) {
	t.Helper()

	r = r.WithContext(WithErrorReporter(r.Context()))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)

	return rec, FlushReportedError(r.Context())
}

func okCheck(context.Context) error { return nil }

func assertNotReady(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if code := decodeErrorBody(t, rec).Error.Code; code != CodeServiceUnavailable {
		t.Errorf("code = %q, want %q", code, CodeServiceUnavailable)
	}
}

func TestHealthz(t *testing.T) {
	rec, reported := serveHealth(t, httptest.NewRequest(http.MethodGet, "/", nil), Healthz())

	if reported != nil {
		t.Errorf("reported error = %v, want nil", reported)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != `{"status":"ok"}` {
		t.Errorf("body = %s, want %s", got, `{"status":"ok"}`)
	}
}

func TestReadyzReady(t *testing.T) {
	tests := map[string]map[string]CheckFunc{
		"nil map":     nil,
		"all succeed": {"postgres": okCheck, "redis": okCheck},
	}

	for name, checks := range tests {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			rec, reported := serveHealth(t, r, Readyz(time.Second, checks))

			if reported != nil {
				t.Errorf("reported error = %v, want nil", reported)
			}
			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if got := rec.Body.String(); got != `{"status":"ready"}` {
				t.Errorf("body = %s, want %s", got, `{"status":"ready"}`)
			}
		})
	}
}

func TestReadyzCheckFails(t *testing.T) {
	errPostgres := errors.New("connection refused")
	errRedis := errors.New("no route to host")
	h := Readyz(time.Second, map[string]CheckFunc{
		"redis":    func(context.Context) error { return errRedis },
		"postgres": func(context.Context) error { return errPostgres },
		"kafka":    okCheck,
	})

	rec, reported := serveHealth(t, httptest.NewRequest(http.MethodGet, "/", nil), h)

	assertNotReady(t, rec)
	if !errors.Is(reported, errPostgres) || !errors.Is(reported, errRedis) {
		t.Errorf("reported error = %v, want it to wrap both check errors", reported)
	}
	msg := reported.Error()
	if !strings.Contains(msg, "postgres: connection refused\nredis: no route to host") {
		t.Errorf("reported error = %q, want check errors named and sorted", msg)
	}
	if strings.Contains(rec.Body.String(), "connection refused") {
		t.Errorf("body = %s, leaks the check error", rec.Body.String())
	}
}

func TestReadyzTimeout(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	h := Readyz(20*time.Millisecond, map[string]CheckFunc{
		"fast": okCheck,
		// Ignores its context: the handler must not wait for it.
		"stuck": func(context.Context) error {
			<-release
			return nil
		},
	})

	start := time.Now()
	rec, reported := serveHealth(t, httptest.NewRequest(http.MethodGet, "/", nil), h)

	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("handler took %v, want about the timeout", elapsed)
	}
	assertNotReady(t, rec)
	if !errors.Is(reported, context.DeadlineExceeded) {
		t.Errorf("reported error = %v, want context.DeadlineExceeded", reported)
	}
	if msg := reported.Error(); !strings.Contains(msg, "stuck: ") || strings.Contains(msg, "fast") {
		t.Errorf("reported error = %q, want only the stuck check", msg)
	}
}

func TestReadyzClientCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	h := Readyz(time.Second, map[string]CheckFunc{
		"postgres": func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		},
	})
	r := httptest.NewRequestWithContext(ctx, http.MethodGet, "/", nil)
	rec, reported := serveHealth(t, r, h)

	assertNotReady(t, rec)
	if !errors.Is(reported, context.Canceled) {
		t.Errorf("reported error = %v, want context.Canceled", reported)
	}
}

func TestReadyzCheckPanics(t *testing.T) {
	var db *fakeDB // a method value of a nil pointer is not nil, it panics when called
	h := Readyz(time.Second, map[string]CheckFunc{"postgres": db.Ping})

	rec, reported := serveHealth(t, httptest.NewRequest(http.MethodGet, "/", nil), h)

	assertNotReady(t, rec)
	if msg := reported.Error(); !strings.Contains(msg, "postgres: panic: ") {
		t.Errorf("reported error = %q, want the recovered panic", msg)
	}
}

type fakeDB struct{ err error }

func (db *fakeDB) Ping(context.Context) error { return db.err }

func TestReadyzInvalidArguments(t *testing.T) {
	tests := map[string]func(){
		"zero timeout":     func() { Readyz(0, nil) },
		"negative timeout": func() { Readyz(-time.Second, nil) },
		"nil check":        func() { Readyz(time.Second, map[string]CheckFunc{"postgres": nil}) },
	}

	for name, fn := range tests {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("Readyz did not panic")
				}
			}()
			fn()
		})
	}
}
