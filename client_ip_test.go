package plinth

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"path/filepath"
	"testing"
	"time"
)

func mustResolver(t *testing.T, c ClientIPResolver, proxies ...string) ClientIPResolver {
	t.Helper()

	c, err := c.WithTrustedProxies(proxies...)
	if err != nil {
		t.Fatalf("WithTrustedProxies(%q): %v", proxies, err)
	}
	return c
}

func TestParseIP(t *testing.T) {
	tests := []struct {
		value  string
		want   string
		wantOK bool
	}{
		{value: "1.2.3.4", want: "1.2.3.4", wantOK: true},
		{value: " 1.2.3.4 ", want: "1.2.3.4", wantOK: true},
		{value: "1.2.3.4:8080", want: "1.2.3.4", wantOK: true},
		{value: "::1", want: "::1", wantOK: true},
		{value: "[::1]", want: "::1", wantOK: true},
		{value: "[::1]:8080", want: "::1", wantOK: true},
		{value: "::ffff:1.2.3.4", want: "1.2.3.4", wantOK: true},
		{value: "[::ffff:1.2.3.4]:80", want: "1.2.3.4", wantOK: true},
		{value: "fe80::1%eth0", wantOK: false},
		{value: "[fe80::1%eth0]:80", wantOK: false},
		{value: "", wantOK: false},
		{value: "@", wantOK: false},
		{value: "unknown", wantOK: false},
		{value: "1.2.3.4:port", wantOK: false},
		{value: "[1.2.3.4", wantOK: false},
		{value: "1.2.3.256", wantOK: false},
	}

	for _, tc := range tests {
		t.Run(tc.value, func(t *testing.T) {
			got, ok := parseIP(tc.value)
			if ok != tc.wantOK {
				t.Fatalf("parseIP(%q) ok = %v, want %v", tc.value, ok, tc.wantOK)
			}
			if !ok {
				if got.IsValid() {
					t.Errorf("parseIP(%q) = %v, want invalid address", tc.value, got)
				}
				return
			}
			if got != netip.MustParseAddr(tc.want) {
				t.Errorf("parseIP(%q) = %v, want %s", tc.value, got, tc.want)
			}
		})
	}
}

func TestParseProxies(t *testing.T) {
	tests := []struct {
		proxy   string
		want    string
		wantErr bool
	}{
		{proxy: "10.0.0.0/8", want: "10.0.0.0/8"},
		{proxy: " 10.0.0.0/8 ", want: "10.0.0.0/8"},
		{proxy: "10.1.2.3/8", want: "10.0.0.0/8"},
		{proxy: "1.2.3.4", want: "1.2.3.4/32"},
		{proxy: "::1", want: "::1/128"},
		{proxy: "fd00::/8", want: "fd00::/8"},
		{proxy: "::ffff:1.2.3.4", want: "1.2.3.4/32"},
		{proxy: "::ffff:10.0.0.0/104", want: "10.0.0.0/8"},
		{proxy: "::ffff:0.0.0.0/96", want: "0.0.0.0/0"},
		{proxy: "::ffff:0.0.0.0/95", wantErr: true},
		{proxy: "fe80::1%eth0", wantErr: true},
		{proxy: "10.0.0.0/33", wantErr: true},
		{proxy: "proxy.local", wantErr: true},
		{proxy: "", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.proxy, func(t *testing.T) {
			got, err := parseProxies([]string{tc.proxy})
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseProxies(%q) = %v, want error", tc.proxy, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseProxies(%q): %v", tc.proxy, err)
			}
			if len(got) != 1 || got[0] != netip.MustParsePrefix(tc.want) {
				t.Errorf("parseProxies(%q) = %v, want [%s]", tc.proxy, got, tc.want)
			}
		})
	}
}

func TestParseProxiesStopsOnFirstError(t *testing.T) {
	got, err := parseProxies([]string{"10.0.0.0/8", "bad", "::1"})
	if err == nil {
		t.Fatalf("parseProxies = %v, want error", got)
	}
	if got != nil {
		t.Errorf("parseProxies returned %v with error, want nil", got)
	}
}

func TestWithTrustedProxiesError(t *testing.T) {
	_, err := NewClientIPResolver().WithTrustedProxies("10.0.0.0/8", "bad")
	if err == nil {
		t.Fatal("WithTrustedProxies with invalid proxy returned nil error")
	}
}

func TestNewClientIPResolverCopiesHeaders(t *testing.T) {
	headers := []string{HeaderXRealIP}
	c := mustResolver(t, NewClientIPResolver(headers...), "10.0.0.0/8")
	headers[0] = HeaderXForwardedFor

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.1:1234"
	r.Header.Set(HeaderXRealIP, "1.2.3.4")

	got, err := c.Resolve(r)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if want := netip.MustParseAddr("1.2.3.4"); got != want {
		t.Errorf("Resolve = %v, want %v", got, want)
	}
}

func TestClientIPResolverResolve(t *testing.T) {
	xff := mustResolver(t, NewClientIPResolver(), "10.0.0.0/8", "::1")
	both := mustResolver(t, NewClientIPResolver(HeaderXForwardedFor, HeaderXRealIP), "10.0.0.0/8")
	realFirst := mustResolver(
		t, NewClientIPResolver(HeaderXRealIP, HeaderXForwardedFor), "10.0.0.0/8",
	)
	realOnly := mustResolver(t, NewClientIPResolver(HeaderXRealIP), "10.0.0.0/8")
	unix := xff.WithTrustedUnixSocket()

	tests := []struct {
		name       string
		resolver   ClientIPResolver
		remoteAddr string
		unixSocket bool
		header     http.Header
		want       string
		wantErr    error
	}{
		{
			name:       "untrusted peer ignores header",
			resolver:   xff,
			remoteAddr: "8.8.8.8:1234",
			header:     http.Header{HeaderXForwardedFor: {"1.2.3.4"}},
			want:       "8.8.8.8",
		},
		{
			name:       "no trusted proxies",
			resolver:   NewClientIPResolver(),
			remoteAddr: "10.0.0.1:1234",
			header:     http.Header{HeaderXForwardedFor: {"1.2.3.4"}},
			want:       "10.0.0.1",
		},
		{
			name:       "trusted peer without header",
			resolver:   xff,
			remoteAddr: "10.0.0.1:1234",
			want:       "10.0.0.1",
		},
		{
			name:       "trusted peer single hop",
			resolver:   xff,
			remoteAddr: "10.0.0.1:1234",
			header:     http.Header{HeaderXForwardedFor: {"1.2.3.4"}},
			want:       "1.2.3.4",
		},
		{
			name:       "rightmost untrusted hop wins",
			resolver:   xff,
			remoteAddr: "10.0.0.1:1234",
			header:     http.Header{HeaderXForwardedFor: {"9.9.9.9, 1.2.3.4, 10.0.0.2"}},
			want:       "1.2.3.4",
		},
		{
			name:       "multiple header lines",
			resolver:   xff,
			remoteAddr: "10.0.0.1:1234",
			header:     http.Header{HeaderXForwardedFor: {"9.9.9.9, 1.2.3.4", "10.0.0.2"}},
			want:       "1.2.3.4",
		},
		{
			name:       "all hops trusted returns leftmost",
			resolver:   xff,
			remoteAddr: "10.0.0.1:1234",
			header:     http.Header{HeaderXForwardedFor: {"10.0.0.3, 10.0.0.2"}},
			want:       "10.0.0.3",
		},
		{
			name:       "empty hops skipped",
			resolver:   xff,
			remoteAddr: "10.0.0.1:1234",
			header:     http.Header{HeaderXForwardedFor: {" , 1.2.3.4 ,, "}},
			want:       "1.2.3.4",
		},
		{
			name:       "hop with port",
			resolver:   xff,
			remoteAddr: "10.0.0.1:1234",
			header:     http.Header{HeaderXForwardedFor: {"[2001:db8::1]:443"}},
			want:       "2001:db8::1",
		},
		{
			name:       "empty header value",
			resolver:   xff,
			remoteAddr: "10.0.0.1:1234",
			header:     http.Header{HeaderXForwardedFor: {""}},
			want:       "10.0.0.1",
		},
		{
			name:       "malformed hop returns error",
			resolver:   both,
			remoteAddr: "10.0.0.1:1234",
			header: http.Header{
				HeaderXForwardedFor: {"1.2.3.4, zzz"},
				HeaderXRealIP:       {"5.6.7.8"},
			},
			want:    "10.0.0.1",
			wantErr: ErrMalformedHeader,
		},
		{
			name:       "malformed hop left of client is not reached",
			resolver:   xff,
			remoteAddr: "10.0.0.1:1234",
			header:     http.Header{HeaderXForwardedFor: {"zzz, 1.2.3.4"}},
			want:       "1.2.3.4",
		},
		{
			name:       "unknown hop falls back to peer",
			resolver:   xff,
			remoteAddr: "10.0.0.1:1234",
			header:     http.Header{HeaderXForwardedFor: {"1.2.3.4, unknown"}},
			want:       "10.0.0.1",
		},
		{
			name:       "unknown hop tries next header",
			resolver:   both,
			remoteAddr: "10.0.0.1:1234",
			header: http.Header{
				HeaderXForwardedFor: {"1.2.3.4, UNKNOWN"},
				HeaderXRealIP:       {"5.6.7.8"},
			},
			want: "5.6.7.8",
		},
		{
			name:       "header order is respected",
			resolver:   realFirst,
			remoteAddr: "10.0.0.1:1234",
			header: http.Header{
				HeaderXForwardedFor: {"1.2.3.4"},
				HeaderXRealIP:       {"5.6.7.8"},
			},
			want: "5.6.7.8",
		},
		{
			name:       "missing first header uses next",
			resolver:   realFirst,
			remoteAddr: "10.0.0.1:1234",
			header:     http.Header{HeaderXForwardedFor: {"1.2.3.4"}},
			want:       "1.2.3.4",
		},
		{
			name:       "headers not configured are ignored",
			resolver:   realOnly,
			remoteAddr: "10.0.0.1:1234",
			header:     http.Header{HeaderXForwardedFor: {"1.2.3.4"}},
			want:       "10.0.0.1",
		},
		{
			name:       "ipv6 peer",
			resolver:   xff,
			remoteAddr: "[::1]:1234",
			header:     http.Header{HeaderXForwardedFor: {"1.2.3.4"}},
			want:       "1.2.3.4",
		},
		{
			name:       "ipv4-mapped peer matches ipv4 proxy",
			resolver:   xff,
			remoteAddr: "[::ffff:10.0.0.1]:1234",
			header:     http.Header{HeaderXForwardedFor: {"1.2.3.4"}},
			want:       "1.2.3.4",
		},
		{
			name:       "invalid remote addr",
			resolver:   xff,
			remoteAddr: "garbage",
			header:     http.Header{HeaderXForwardedFor: {"1.2.3.4"}},
			wantErr:    ErrInvalidRemoteAddr,
		},
		{
			name:       "unix socket trusted",
			resolver:   unix,
			remoteAddr: "@",
			unixSocket: true,
			header:     http.Header{HeaderXForwardedFor: {"1.2.3.4, 10.0.0.2"}},
			want:       "1.2.3.4",
		},
		{
			name:       "unix socket trusted with bound client path",
			resolver:   unix,
			remoteAddr: "/tmp/client.sock",
			unixSocket: true,
			header:     http.Header{HeaderXForwardedFor: {"1.2.3.4"}},
			want:       "1.2.3.4",
		},
		{
			name:       "unix socket trusted without header",
			resolver:   unix,
			remoteAddr: "@",
			unixSocket: true,
		},
		{
			name:       "unix socket trusted with malformed header",
			resolver:   unix,
			remoteAddr: "@",
			unixSocket: true,
			header:     http.Header{HeaderXForwardedFor: {"zzz"}},
			wantErr:    ErrMalformedHeader,
		},
		{
			name:       "unix socket not trusted",
			resolver:   xff,
			remoteAddr: "@",
			unixSocket: true,
			header:     http.Header{HeaderXForwardedFor: {"1.2.3.4"}},
			wantErr:    ErrInvalidRemoteAddr,
		},
		{
			name:       "unix socket option does not trust tcp garbage",
			resolver:   unix,
			remoteAddr: "garbage",
			header:     http.Header{HeaderXForwardedFor: {"1.2.3.4"}},
			wantErr:    ErrInvalidRemoteAddr,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tc.remoteAddr
			r.Header = tc.header
			if tc.unixSocket {
				local := &net.UnixAddr{Name: "/tmp/server.sock", Net: "unix"}
				r = r.WithContext(context.WithValue(r.Context(), http.LocalAddrContextKey, local))
			}

			got, err := tc.resolver.Resolve(r)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Resolve error = %v, want %v", err, tc.wantErr)
			}

			var want netip.Addr
			if tc.want != "" {
				want = netip.MustParseAddr(tc.want)
			}
			if got != want {
				t.Errorf("Resolve = %v, want %v", got, want)
			}
		})
	}
}

func TestClientIPResolverUnixSocketServer(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "s.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	resolver := NewClientIPResolver().WithTrustedUnixSocket()
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip, err := resolver.Resolve(r)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_, _ = io.WriteString(w, ip.String())
		}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	client := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", sock)
		},
	}}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://unix/", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set(HeaderXForwardedFor, "1.2.3.4")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != http.StatusOK || string(body) != "1.2.3.4" {
		t.Errorf("response = %d %q, want 200 %q", resp.StatusCode, body, "1.2.3.4")
	}
}
