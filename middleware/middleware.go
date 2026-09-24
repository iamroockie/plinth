package middleware

import (
	"net/http"
	"slices"
)

// Middleware wraps an [http.Handler] with extra behavior.
type Middleware func(next http.Handler) http.Handler

// Chain combines middleware into one. The first middleware is the outermost: it
// sees the request first and the response last.
func Chain(mws ...Middleware) Middleware {
	return func(next http.Handler) http.Handler {
		for _, mw := range slices.Backward(mws) {
			next = mw(next)
		}
		return next
	}
}
