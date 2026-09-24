package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/iamroockie/plinth/middleware"
)

func TestChainOrder(t *testing.T) {
	var calls []string
	mark := func(name string) middleware.Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls = append(calls, name+" in")
				next.ServeHTTP(w, r)
				calls = append(calls, name+" out")
			})
		}
	}

	h := middleware.Chain(mark("a"), mark("b"), mark("c"))(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			calls = append(calls, "handler")
		}),
	)
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	want := []string{"a in", "b in", "c in", "handler", "c out", "b out", "a out"}
	if !slices.Equal(calls, want) {
		t.Errorf("calls = %q, want %q", calls, want)
	}
}

func TestChainEmpty(t *testing.T) {
	h := middleware.Chain()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusTeapot {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusTeapot)
	}
}
