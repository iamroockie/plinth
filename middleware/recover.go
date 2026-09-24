package middleware

import (
	"errors"
	"fmt"
	"maps"
	"net/http"
	"runtime/debug"

	"github.com/iamroockie/plinth"
)

// Recover turns a panic in the handler into a 500 JSON error and reports the
// panic value with its stack trace with [plinth.ReportError]. Headers set by the
// handler before the panic are discarded; headers set by outer middleware are
// kept. If the response has already started, the panic is only reported. A panic
// with [http.ErrAbortHandler] is passed on, so that net/http aborts the response.
func Recover() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rw := plinth.NewResponseWriter(w)
			header := w.Header().Clone()

			defer func() {
				rec := recover()
				if rec == nil {
					return
				}

				if err, ok := rec.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					panic(rec)
				}

				err := fmt.Errorf("panic: %v\n\n%s", rec, debug.Stack())
				if rw.WroteHeader() {
					plinth.ReportError(r.Context(), err)
					return
				}

				restoreHeader(rw.Header(), header)

				plinth.WriteError(rw, r, plinth.InternalServerError(err))
			}()

			next.ServeHTTP(rw, r)
		})
	}
}

func restoreHeader(dst, src http.Header) {
	clear(dst)
	maps.Copy(dst, src)
}
