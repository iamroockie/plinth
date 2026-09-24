package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// Timeout sets a deadline d from now on the request context, unless the context
// already has an earlier one. It does not stop the handler: the handler and the
// calls it makes must respect the context. A handler that returns the context
// error through [plinth.RespondJSON] or [plinth.WriteError] gets a 503 response.
// Timeout panics if d is not positive.
func Timeout(d time.Duration) Middleware {
	if d <= 0 {
		panic(fmt.Sprintf("plinth: timeout must be positive, got %s", d))
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
