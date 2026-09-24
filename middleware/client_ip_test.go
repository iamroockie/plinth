package middleware_test

import (
	"errors"
	"net/http"
	"net/netip"
	"testing"

	"github.com/iamroockie/plinth"
	"github.com/iamroockie/plinth/middleware"
	"github.com/iamroockie/plinth/plinthtest"
)

func TestClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		xff        string
		want       string
		wantErr    error
	}{
		{name: "trusted proxy", remoteAddr: "10.0.0.1:1234", xff: "1.2.3.4", want: "1.2.3.4"},
		{name: "untrusted peer", remoteAddr: "8.8.8.8:1234", xff: "1.2.3.4", want: "8.8.8.8"},
		{
			name:       "malformed header keeps peer",
			remoteAddr: "10.0.0.1:1234",
			xff:        "zzz",
			want:       "10.0.0.1",
			wantErr:    plinth.ErrMalformedHeader,
		},
		{
			name:       "invalid remote addr",
			remoteAddr: "garbage",
			xff:        "1.2.3.4",
			wantErr:    plinth.ErrInvalidRemoteAddr,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newProxiedRequest(t, "/")
			r.RemoteAddr = tc.remoteAddr
			r.Header.Set(plinth.HeaderXForwardedFor, tc.xff)

			var (
				got   netip.Addr
				found bool
			)
			h := middleware.ClientIP(newTestResolver(t))(
				http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
					got, found = middleware.ClientIPFromContext(r.Context())
				}),
			)
			_, reported := plinthtest.Serve(t, h, r)

			if wantFound := tc.want != ""; found != wantFound {
				t.Fatalf("ClientIPFromContext found = %v, want %v", found, wantFound)
			}
			if found && got != netip.MustParseAddr(tc.want) {
				t.Errorf("ClientIPFromContext = %v, want %s", got, tc.want)
			}

			if !errors.Is(reported, tc.wantErr) {
				t.Errorf("reported error = %v, want %v", reported, tc.wantErr)
			}
		})
	}
}

func TestContextWithClientIP(t *testing.T) {
	want := netip.MustParseAddr("2001:db8::1")

	got, ok := middleware.ClientIPFromContext(middleware.ContextWithClientIP(t.Context(), want))
	if !ok || got != want {
		t.Errorf("ClientIPFromContext = %v, %v, want %v, true", got, ok, want)
	}
}

func TestClientIPFromContextEmpty(t *testing.T) {
	if ip, ok := middleware.ClientIPFromContext(t.Context()); ok {
		t.Errorf("ClientIPFromContext = %v, true, want false", ip)
	}
}
