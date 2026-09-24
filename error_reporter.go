package plinth

import (
	"context"
	"errors"
	"log/slog"
	"sync"
)

type errorReporterContextKey struct{}

type errorReporter struct {
	mu     sync.Mutex
	errs   []error
	closed bool
}

// WithErrorReporter returns a context that collects errors passed to
// [ReportError] until [FlushReportedError] is called. If ctx already has a
// reporter, WithErrorReporter returns ctx, so nested calls share one reporter.
//
// Most code does not need it: the ErrorLog middleware from the middleware package
// sets up a reporter for every request and logs what it collects.
func WithErrorReporter(ctx context.Context) context.Context {
	if getErrorReporter(ctx) != nil {
		return ctx
	}
	return context.WithValue(ctx, errorReporterContextKey{}, &errorReporter{})
}

// ReportError records a server-side error of the request that ctx belongs to.
// Nil errors are ignored.
//
// If ctx has a reporter (see [WithErrorReporter]), the error is kept until
// [FlushReportedError]. Without a reporter, or after the flush, it is logged right
// away with [slog.Default].
//
// The package itself reports 5xx errors returned to [RespondJSON] and
// [WriteError] and problems with writing responses. Handlers can use it for errors
// that do not change the response, such as a failed cache update.
func ReportError(ctx context.Context, err error) {
	if err == nil {
		return
	}

	if rep := getErrorReporter(ctx); rep != nil && rep.append(err) {
		return
	}

	slog.Default().ErrorContext(ctx, "reported error", "error", err)
}

// FlushReportedError returns the errors reported to ctx, joined with
// [errors.Join], or nil if there were none. It closes the reporter: errors
// reported later are logged right away with [slog.Default]. Without a reporter in
// ctx it returns nil.
func FlushReportedError(ctx context.Context) error {
	rep := getErrorReporter(ctx)
	if rep == nil {
		return nil
	}

	rep.mu.Lock()
	defer rep.mu.Unlock()

	rep.closed = true
	err := errors.Join(rep.errs...)
	rep.errs = nil

	return err
}

func (rep *errorReporter) append(err error) bool {
	rep.mu.Lock()
	defer rep.mu.Unlock()

	if rep.closed {
		return false
	}
	rep.errs = append(rep.errs, err)

	return true
}

func getErrorReporter(ctx context.Context) *errorReporter {
	rep, ok := ctx.Value(errorReporterContextKey{}).(*errorReporter)
	if !ok {
		return nil
	}
	return rep
}
