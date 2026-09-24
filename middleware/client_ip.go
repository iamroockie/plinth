package middleware

import (
	"context"
	"net/http"
	"net/netip"

	"github.com/iamroockie/plinth"
)

type clientIPContextKey struct{}

// ClientIP resolves the client address with c and stores it in the request
// context, where [ClientIPFromContext] reads it. Resolution errors are reported
// with [plinth.ReportError]. If the address cannot be resolved, the request goes
// on without one; with [plinth.ErrMalformedHeader] the peer address is stored.
func ClientIP(c plinth.ClientIPResolver) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip, err := c.Resolve(r)
			if err != nil {
				plinth.ReportError(r.Context(), err)
			}
			if !ip.IsValid() {
				next.ServeHTTP(w, r)
				return
			}

			next.ServeHTTP(w, r.WithContext(ContextWithClientIP(r.Context(), ip)))
		})
	}
}

// ContextWithClientIP returns a copy of ctx that carries ip as the client
// address. [ClientIP] uses it; call it directly to test code that reads the
// address without running the middleware.
func ContextWithClientIP(ctx context.Context, ip netip.Addr) context.Context {
	return context.WithValue(ctx, clientIPContextKey{}, ip)
}

// ClientIPFromContext returns the client address stored by [ClientIP] or
// [ContextWithClientIP].
func ClientIPFromContext(ctx context.Context) (netip.Addr, bool) {
	ip, ok := ctx.Value(clientIPContextKey{}).(netip.Addr)
	return ip, ok
}
