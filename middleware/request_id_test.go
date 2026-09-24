package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"uuid"

	"github.com/iamroockie/plinth"
	"github.com/iamroockie/plinth/middleware"
)

func serveRequestID(t *testing.T, incoming string) (uuid.UUID, string) {
	t.Helper()

	var (
		id    uuid.UUID
		found bool
	)
	h := middleware.RequestID()(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		id, found = middleware.RequestIDFromContext(r.Context())
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if incoming != "" {
		r.Header.Set(plinth.HeaderXRequestID, incoming)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)

	if !found {
		t.Fatal("RequestIDFromContext found no id")
	}
	return id, rec.Header().Get(plinth.HeaderXRequestID)
}

func TestRequestID(t *testing.T) {
	incoming := uuid.NewV4()

	tests := []struct {
		name     string
		incoming string
		want     uuid.UUID
	}{
		{name: "no header generates id"},
		{name: "valid header is kept", incoming: incoming.String(), want: incoming},
		{
			name:     "uppercase header is normalized",
			incoming: "0192F0A1-7C3B-7D2E-8F10-123456789ABC",
			want:     uuid.MustParse("0192f0a1-7c3b-7d2e-8f10-123456789abc"),
		},
		{name: "garbage header is replaced", incoming: "garbage\nfake=1"},
		{name: "nil uuid is replaced", incoming: uuid.Nil().String()},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			id, header := serveRequestID(t, tc.incoming)

			if header != id.String() {
				t.Errorf("response header = %q, want %q", header, id.String())
			}
			if tc.want != uuid.Nil() {
				if id != tc.want {
					t.Errorf("id = %v, want %v", id, tc.want)
				}
				return
			}
			if version := id[6] >> 4; version != 7 {
				t.Errorf("generated id %v has version %d, want 7", id, version)
			}
		})
	}
}

func TestRequestIDUnique(t *testing.T) {
	first, _ := serveRequestID(t, "")
	second, _ := serveRequestID(t, "")

	if first == second {
		t.Errorf("two requests got the same id %v", first)
	}
}

func TestContextWithRequestID(t *testing.T) {
	want := uuid.NewV4()

	got, ok := middleware.RequestIDFromContext(middleware.ContextWithRequestID(t.Context(), want))
	if !ok || got != want {
		t.Errorf("RequestIDFromContext = %v, %v, want %v, true", got, ok, want)
	}
}

func TestRequestIDFromContextEmpty(t *testing.T) {
	if id, ok := middleware.RequestIDFromContext(t.Context()); ok {
		t.Errorf("RequestIDFromContext = %v, true, want false", id)
	}
}
