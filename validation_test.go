package plinth

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func writeValidationError(t *testing.T, err error) (*httptest.ResponseRecorder, error) {
	t.Helper()

	r := httptest.NewRequestWithContext(WithErrorReporter(t.Context()), http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	WriteError(rec, r, err)

	return rec, FlushReportedError(r.Context())
}

func TestValidationErrorResponse(t *testing.T) {
	tests := []struct {
		name       string
		violations []FieldViolation
		want       string
	}{
		{
			name: "violations in given order",
			violations: []FieldViolation{
				{Field: "url", Code: "unsupported_scheme"},
				{Field: "interval", Code: "out_of_range", Params: Map{"max": 86400}},
				{Field: "", Code: "too_many_monitors"},
			},
			want: `{"error":{"code":"validation_error","message":"Validation failed","details":[` +
				`{"field":"url","code":"unsupported_scheme"},` +
				`{"field":"interval","code":"out_of_range","params":{"max":86400}},` +
				`{"field":"","code":"too_many_monitors"}]}}`,
		},
		{
			name:       "empty params omitted",
			violations: []FieldViolation{{Field: "name", Code: "required", Params: Map{}}},
			want: `{"error":{"code":"validation_error","message":"Validation failed","details":[` +
				`{"field":"name","code":"required"}]}}`,
		},
		{
			name: "no violations",
			want: `{"error":{"code":"validation_error","message":"Validation failed"}}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec, reported := writeValidationError(t, ValidationError(tc.violations...))

			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
			}
			if got := rec.Body.String(); got != tc.want {
				t.Errorf("body = %s\nwant   %s", got, tc.want)
			}
			if reported != nil {
				t.Errorf("reported error = %v, want nil", reported)
			}
		})
	}
}

func TestValidationErrorUnencodableParams(t *testing.T) {
	err := ValidationError(FieldViolation{Field: "name", Code: "invalid", Params: Map{
		"bad": make(chan int),
	}})

	rec, reported := writeValidationError(t, err)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	body := decodeErrorBody(t, rec)
	if body.Error.Code != CodeInternal || body.Error.Details != nil {
		t.Errorf("body = %s, want internal_error without details", rec.Body)
	}
	if _, ok := errors.AsType[*json.SemanticError](reported); !ok {
		t.Errorf("reported error = %v, want the marshal error", reported)
	}
}

func TestValidationErrorCopiesViolations(t *testing.T) {
	violations := []FieldViolation{{Field: "name", Code: "too_long", Params: Map{"max": 64}}}
	err := ValidationError(violations...)
	violations[0].Code = "changed"
	violations[0].Params["max"] = 1

	e := asAPIError(t, err)
	want := []FieldViolation{{Field: "name", Code: "too_long", Params: Map{"max": 64}}}
	if !reflect.DeepEqual(e.Details, want) {
		t.Errorf("details = %v, want %v", e.Details, want)
	}
	if e.Unwrap() != nil {
		t.Errorf("Unwrap = %v, want nil", e.Unwrap())
	}
}

var (
	errNameRequired = errors.New("name required")
	errNameTooLong  = errors.New("name too long")
	errNameCharset  = errors.New("name has invalid characters")
	errAgeRange     = errors.New("age out of range")
)

func TestMatchViolations(t *testing.T) {
	ageParams := Map{"min": 0, "max": 150}
	rules := []FieldRule{
		{Err: errNameRequired, Field: "name", Code: "required"},
		{Err: errNameTooLong, Field: "name", Code: "too_long", Params: Map{"max": 64}},
		{Err: errNameCharset, Field: "name", Code: "invalid_charset"},
		{Err: errAgeRange, Field: "age", Code: "out_of_range", Params: ageParams},
	}

	tests := []struct {
		name string
		err  error
		want []FieldViolation
	}{
		{
			name: "single error",
			err:  errAgeRange,
			want: []FieldViolation{{Field: "age", Code: "out_of_range", Params: ageParams}},
		},
		{
			name: "joined errors in rule order",
			err:  errors.Join(errAgeRange, errNameRequired),
			want: []FieldViolation{
				{Field: "name", Code: "required"},
				{Field: "age", Code: "out_of_range", Params: ageParams},
			},
		},
		{
			name: "wrapped error",
			err:  fmt.Errorf("validate user: %w", errNameRequired),
			want: []FieldViolation{{Field: "name", Code: "required"}},
		},
		{
			name: "two rules for one field",
			err:  fmt.Errorf("validate: %w", errors.Join(errNameCharset, errNameTooLong)),
			want: []FieldViolation{
				{Field: "name", Code: "too_long", Params: Map{"max": 64}},
				{Field: "name", Code: "invalid_charset"},
			},
		},
		{name: "no match", err: errors.New("other")},
		{name: "nil error"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := MatchViolations(tc.err, rules...)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("MatchViolations = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMatchViolationsCopiesParams(t *testing.T) {
	rules := []FieldRule{
		{Err: errAgeRange, Field: "age", Code: "out_of_range", Params: Map{"max": 150}},
	}

	v := MatchViolations(errAgeRange, rules...)
	v[0].Params["max"] = 1

	if want := (Map{"max": 150}); !reflect.DeepEqual(rules[0].Params, want) {
		t.Errorf("rule params = %v, want %v", rules[0].Params, want)
	}
}

func TestMatchViolationsDoesNotAllocateWithoutMatches(t *testing.T) {
	rules := []FieldRule{
		{Err: errNameRequired, Field: "name", Code: "required"},
		{Err: errAgeRange, Field: "age", Code: "out_of_range"},
	}
	unmatched := errors.Join(errNameTooLong, errNameCharset)

	for _, err := range []error{nil, unmatched} {
		allocs := testing.AllocsPerRun(100, func() {
			if v := MatchViolations(err, rules...); v != nil {
				t.Fatalf("MatchViolations(%v) = %v, want nil", err, v)
			}
		})
		if allocs != 0 {
			t.Errorf("MatchViolations(%v) allocates %v times, want 0", err, allocs)
		}
	}
}
