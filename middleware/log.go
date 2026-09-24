package middleware

import (
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/iamroockie/plinth"
)

// ErrorLog collects the errors reported with [plinth.ReportError] while the
// request is handled and logs them as one "request error" record at the Error
// level after the handler returns. A nil log means [slog.Default]. Records include
// request_id and client_ip, see [NewContextLogHandler].
//
// Errors reported after the handler has returned, for example from a goroutine,
// are logged right away with [slog.Default].
func ErrorLog(log *slog.Logger) Middleware {
	log = withContextAttrs(log)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := plinth.WithErrorReporter(r.Context())
			defer func() {
				if err := plinth.FlushReportedError(ctx); err != nil {
					log.ErrorContext(ctx, "request error", "error", err)
				}
			}()

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequestLog logs an "http request" record after the handler returns, with the
// method, path, status, response size in bytes and duration in milliseconds. The
// level is Error for 5xx statuses, Warn for 4xx and Info for the rest. Requests
// whose URL path equals one of quiet, such as a health check, are not logged. A
// nil log means [slog.Default]. Records include request_id and client_ip, see
// [NewContextLogHandler].
func RequestLog(log *slog.Logger, quiet ...string) Middleware {
	log = withContextAttrs(log)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := plinth.NewResponseWriter(w)
			start := time.Now()

			defer func() {
				if slices.Contains(quiet, r.URL.Path) {
					return
				}

				elapsed := time.Since(start)
				status := ww.Status()

				log.Log(r.Context(), statusLevel(status), "http request",
					"method", r.Method,
					"path", r.URL.Path,
					"status", status,
					"size", ww.Size(),
					"duration_ms", float64(elapsed.Microseconds())/1000,
				)
			}()

			next.ServeHTTP(ww, r)
		})
	}
}

func statusLevel(status int) slog.Level {
	switch {
	case status >= http.StatusInternalServerError:
		return slog.LevelError
	case status >= http.StatusBadRequest:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}
