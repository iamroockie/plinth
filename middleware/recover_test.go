package middleware_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/iamroockie/plinth"
	"github.com/iamroockie/plinth/middleware"
	"github.com/iamroockie/plinth/plinthtest"
)

func serveRecover(t *testing.T, h http.Handler) (*httptest.ResponseRecorder, error) {
	t.Helper()

	outer := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Outer", "kept")
		middleware.Recover()(h).ServeHTTP(w, r)
	})

	return plinthtest.Serve(t, outer, plinthtest.NewRequest(t, http.MethodGet, "/", nil))
}

func TestRecoverBeforeWrite(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{name: "string", value: "boom", want: "panic: boom"},
		{name: "error", value: errors.New("broken"), want: "panic: broken"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec, reported := serveRecover(t, http.HandlerFunc(
				func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("X-Inner", "dropped")
					panic(tc.value)
				},
			))

			if rec.Code != http.StatusInternalServerError {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
			}

			if code := plinthtest.DecodeError(t, rec).Code; code != plinth.CodeInternal {
				t.Errorf("code = %q, want %q", code, plinth.CodeInternal)
			}
			if strings.Contains(rec.Body.String(), "panic") {
				t.Errorf("body = %s, leaks panic details", rec.Body.String())
			}

			if got := rec.Header().Get("X-Outer"); got != "kept" {
				t.Errorf("X-Outer = %q, want kept", got)
			}
			if got := rec.Header().Get("X-Inner"); got != "" {
				t.Errorf("X-Inner = %q, want header set before panic to be dropped", got)
			}

			if reported == nil || !strings.Contains(reported.Error(), tc.want) {
				t.Fatalf("reported error = %v, want it to contain %q", reported, tc.want)
			}
			if !strings.Contains(reported.Error(), "goroutine") {
				t.Errorf("reported error has no stack trace: %v", reported)
			}
		})
	}
}

func TestRecoverAfterWrite(t *testing.T) {
	rec, reported := serveRecover(t, http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte("partial"))
			panic("boom")
		},
	))

	if rec.Code != http.StatusAccepted || rec.Body.String() != "partial" {
		t.Errorf("response = %d %q, want 202 %q", rec.Code, rec.Body.String(), "partial")
	}
	if reported == nil || !strings.Contains(reported.Error(), "panic: boom") {
		t.Errorf("reported error = %v, want panic: boom", reported)
	}
}

func TestRecoverWithoutPanic(t *testing.T) {
	rec, reported := serveRecover(t, http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		},
	))

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if reported != nil {
		t.Errorf("reported error = %v, want nil", reported)
	}
}

func TestRecoverRepanicsAbortHandler(t *testing.T) {
	h := middleware.Recover()(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))

	defer func() {
		err, _ := recover().(error)
		if !errors.Is(err, http.ErrAbortHandler) {
			t.Errorf("recovered %v, want %v", err, http.ErrAbortHandler)
		}
	}()

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	t.Error("ServeHTTP returned without re-panicking")
}
