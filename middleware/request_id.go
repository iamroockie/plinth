package middleware

import (
	"context"
	"net/http"
	"uuid"

	"github.com/iamroockie/plinth"
)

type requestIDContextKey struct{}

// RequestID assigns an ID to the request, stores it in the request context, where
// [RequestIDFromContext] reads it, and sends it in the X-Request-Id response
// header. A valid non-nil UUID in the X-Request-Id request header is reused, so an
// ID assigned by a gateway is kept across services. Otherwise a new UUIDv7 is
// generated.
func RequestID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := requestIDFromHeader(r.Header)
			w.Header().Set(plinth.HeaderXRequestID, id.String())
			next.ServeHTTP(w, r.WithContext(ContextWithRequestID(r.Context(), id)))
		})
	}
}

// ContextWithRequestID returns a copy of ctx that carries id as the request ID.
// [RequestID] uses it; call it directly to test code that reads the ID without
// running the middleware.
func ContextWithRequestID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, requestIDContextKey{}, id)
}

// RequestIDFromContext returns the request ID stored by [RequestID] or
// [ContextWithRequestID].
func RequestIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(requestIDContextKey{}).(uuid.UUID)
	return id, ok
}

func requestIDFromHeader(h http.Header) uuid.UUID {
	id, err := uuid.Parse(h.Get(plinth.HeaderXRequestID))
	if err != nil || id == uuid.Nil() {
		return uuid.NewV7()
	}
	return id
}
