package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/iamroockie/plinth"
	"github.com/iamroockie/plinth/middleware"
)

func TestTimeoutSetsDeadline(t *testing.T) {
	const d = time.Minute

	var (
		deadline time.Time
		ok       bool
	)
	h := middleware.Timeout(d)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		deadline, ok = r.Context().Deadline()
	}))

	start := time.Now()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	end := time.Now()

	if !ok {
		t.Fatal("request context has no deadline")
	}
	if deadline.Before(start.Add(d)) || deadline.After(end.Add(d)) {
		t.Errorf("deadline = %v, want between %v and %v", deadline, start.Add(d), end.Add(d))
	}
}

func TestTimeoutKeepsShorterParentDeadline(t *testing.T) {
	var deadline time.Time
	h := middleware.Timeout(time.Hour)(
		http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			deadline, _ = r.Context().Deadline()
		}),
	)

	parentDeadline := time.Now().Add(time.Minute)
	ctx, cancel := context.WithDeadline(t.Context(), parentDeadline)
	defer cancel()
	h.ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequestWithContext(ctx, http.MethodGet, "/", nil))

	if !deadline.Equal(parentDeadline) {
		t.Errorf("deadline = %v, want parent deadline %v", deadline, parentDeadline)
	}
}

func TestTimeoutRespondsServiceUnavailable(t *testing.T) {
	h := middleware.Timeout(10 * time.Millisecond)(plinth.RespondJSON(
		func(r *http.Request) (*plinth.Response, error) {
			<-r.Context().Done()
			return nil, r.Context().Err()
		},
	))

	rec := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(
		plinth.WithErrorReporter(t.Context()), http.MethodGet, "/", nil,
	)
	h.ServeHTTP(rec, r)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if err := plinth.FlushReportedError(r.Context()); err == nil {
		t.Error("timeout was not reported")
	}
}

func TestTimeoutPanicsOnNonPositive(t *testing.T) {
	for _, d := range []time.Duration{0, -time.Second} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("Timeout(%v) did not panic", d)
				}
			}()
			middleware.Timeout(d)
		}()
	}
}
