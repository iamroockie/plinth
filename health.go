package plinth

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"time"
)

// CheckFunc checks that a dependency, such as a database, is available, and
// returns an error if it is not. Methods such as (*pgxpool.Pool).Ping and
// (*sql.DB).PingContext can be used as is.
type CheckFunc func(ctx context.Context) error

// Healthz returns a handler for liveness probes that always answers 200
// {"status":"ok"}.
func Healthz() http.Handler {
	return RespondJSON(func(_ *http.Request) (*Response, error) {
		return NewResponse(http.StatusOK, Map{"status": "ok"}), nil
	})
}

// Readyz returns a handler for readiness probes. It runs all checks concurrently
// and answers 200 {"status":"ready"} if all of them succeed within timeout, or 503
// with [CodeServiceUnavailable] otherwise. The keys of checks name the
// dependencies in the reported error, which is never sent to the client.
//
// The handler answers within timeout even if a check ignores its context, and a
// panicking check fails the probe instead of crashing the process. Readyz panics
// if timeout is not positive or a check is nil.
func Readyz(timeout time.Duration, checks map[string]CheckFunc) http.Handler {
	if timeout <= 0 {
		panic("plinth: Readyz timeout must be positive")
	}

	named := make([]namedCheck, 0, len(checks))
	for _, name := range slices.Sorted(maps.Keys(checks)) {
		fn := checks[name]
		if fn == nil {
			panic(fmt.Sprintf("plinth: Readyz check %q is nil", name))
		}
		named = append(named, namedCheck{name: name, fn: fn})
	}

	return RespondJSON(func(r *http.Request) (*Response, error) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()

		if err := runChecks(ctx, named); err != nil {
			return nil, ServiceUnavailableError(err)
		}
		return NewResponse(http.StatusOK, Map{"status": "ready"}), nil
	})
}

type namedCheck struct {
	name string
	fn   CheckFunc
}

type checkResult struct {
	index int
	err   error
}

// runChecks runs all checks concurrently and returns their errors joined in the
// order of checks. It returns when all checks are done or ctx is done, whichever
// comes first.
func runChecks(ctx context.Context, checks []namedCheck) error {
	// Buffered so that checks finishing after ctx is done do not block forever.
	results := make(chan checkResult, len(checks))
	for i, c := range checks {
		go func() {
			results <- checkResult{index: i, err: runCheck(ctx, c.fn)}
		}()
	}

	errs := make([]error, len(checks))
	done := make([]bool, len(checks))
	for range checks {
		select {
		case res := <-results:
			done[res.index] = true
			if res.err != nil {
				errs[res.index] = fmt.Errorf("%s: %w", checks[res.index].name, res.err)
			}
		case <-ctx.Done():
			for i, c := range checks {
				if !done[i] {
					errs[i] = fmt.Errorf("%s: %w", c.name, context.Cause(ctx))
				}
			}
			return errors.Join(errs...)
		}
	}

	return errors.Join(errs...)
}

// runCheck calls fn and turns a panic into an error, so that a broken check
// fails the probe instead of crashing the process.
func runCheck(ctx context.Context, fn CheckFunc) (err error) {
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("panic: %v", v)
		}
	}()

	return fn(ctx)
}
