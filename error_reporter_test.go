package plinth

import (
	"errors"
	"strings"
	"sync"
	"testing"
)

func TestReportErrorCollectsUntilFlush(t *testing.T) {
	logs := captureDefaultLog(t)
	errA := errors.New("a")
	errB := errors.New("b")

	ctx := WithErrorReporter(t.Context())
	ReportError(ctx, errA)
	ReportError(ctx, nil)
	ReportError(ctx, errB)

	if logs.Len() != 0 {
		t.Errorf("default log = %q, want nothing before flush", logs.String())
	}

	err := FlushReportedError(ctx)
	if !errors.Is(err, errA) || !errors.Is(err, errB) {
		t.Fatalf("FlushReportedError = %v, want both reported errors", err)
	}

	if err := FlushReportedError(ctx); err != nil {
		t.Errorf("second FlushReportedError = %v, want nil", err)
	}
}

func TestReportErrorAfterFlushGoesToDefaultLog(t *testing.T) {
	logs := captureDefaultLog(t)

	ctx := WithErrorReporter(t.Context())
	if err := FlushReportedError(ctx); err != nil {
		t.Fatalf("FlushReportedError = %v, want nil", err)
	}

	ReportError(ctx, errors.New("late"))

	if !strings.Contains(logs.String(), "error=late") {
		t.Errorf("default log = %q, want late error", logs.String())
	}
	if err := FlushReportedError(ctx); err != nil {
		t.Errorf("FlushReportedError after late report = %v, want nil", err)
	}
}

func TestReportErrorWithoutReporter(t *testing.T) {
	logs := captureDefaultLog(t)

	ReportError(t.Context(), errors.New("orphan"))
	ReportError(t.Context(), nil)

	if got := strings.Count(logs.String(), "reported error"); got != 1 {
		t.Errorf("default log has %d records, want 1: %q", got, logs.String())
	}
	if err := FlushReportedError(t.Context()); err != nil {
		t.Errorf("FlushReportedError without reporter = %v, want nil", err)
	}
}

func TestWithErrorReporterReusesExisting(t *testing.T) {
	outer := WithErrorReporter(t.Context())
	inner := WithErrorReporter(outer)

	if inner != outer {
		t.Error("WithErrorReporter created a new context over an existing reporter")
	}

	errA := errors.New("a")
	ReportError(inner, errA)
	if err := FlushReportedError(outer); !errors.Is(err, errA) {
		t.Errorf("FlushReportedError = %v, want %v", err, errA)
	}
}

func TestReportErrorConcurrent(t *testing.T) {
	const n = 100

	ctx := WithErrorReporter(t.Context())

	var wg sync.WaitGroup
	for range n {
		wg.Go(func() { ReportError(ctx, errors.New("err")) })
	}
	wg.Wait()

	err := FlushReportedError(ctx)
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		t.Fatalf("FlushReportedError = %T, want joined error", err)
	}
	if got := len(joined.Unwrap()); got != n {
		t.Errorf("collected %d errors, want %d", got, n)
	}
}
