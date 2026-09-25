package plinth

import (
	"errors"
	"maps"
	"net/http"
	"slices"
)

// FieldViolation describes one reason why a request failed validation. It is
// sent as an element of the "details" array of a [ValidationError] response.
//
// Field names the invalid field as it appears in the request body. Nested fields
// are written with dots and indexes, such as "items[0].url". A violation that is
// not about a single field, such as a dependency between fields or the whole
// object, has an empty Field. Code is a machine-readable reason, such as
// "required" or "out_of_range"; its vocabulary is up to the API. Params holds
// values that a client needs to build a message, such as "min" and "max", and is
// omitted from the response when empty.
type FieldViolation struct {
	Field  string `json:"field"`
	Code   string `json:"code"`
	Params Map    `json:"params,omitempty"`
}

// FieldRule maps an error to a [FieldViolation] for [MatchViolations]. Field,
// Code and a copy of Params are set on the violation, so changing the violation
// does not change the rule. An empty Field makes it a violation of the whole
// object rather than of one field.
type FieldRule struct {
	Err    error
	Field  string
	Code   string
	Params Map
}

// ValidationError returns a 422 error with code [CodeValidation], the message
// "Validation failed" and the violations in the "details" field, in the given
// order:
//
//	{"error": {"code": "validation_error", "message": "Validation failed",
//	  "details": [{"field": "age", "code": "out_of_range", "params": {"min": 0, "max": 150}}]}}
//
// Without violations the response has no "details" field. The violations and
// their Params are copied, so changing them afterwards does not change the error.
// Field, Code and Params are sent to the client as is, so they must not contain
// anything private. If Params cannot be encoded as JSON, the response is a 500
// error, see [WriteJSON].
func ValidationError(violations ...FieldViolation) error {
	details := slices.Clone(violations)
	for i := range details {
		details[i].Params = maps.Clone(details[i].Params)
	}

	return apiError{
		Code:    CodeValidation,
		Message: "Validation failed",
		Details: details,
		status:  http.StatusUnprocessableEntity,
		cause:   nil,
	}
}

// MatchViolations returns a violation for every rule whose Err matches err
// according to [errors.Is], in the order of the rules. It works with errors
// joined with [errors.Join] and wrapped with fmt.Errorf and %w. Several rules may
// match and may name the same field; all of them are returned, so a validator
// that does not want a violation to follow from another must not return both
// errors.
//
// Errors that no rule matches are ignored, even when joined with matching ones.
// A validator must not join errors that are not about the request, such as a
// failed database lookup, with validation errors: they would be lost in a 422
// response instead of becoming a 500.
//
// If err is nil or no rule matches, MatchViolations returns nil without
// allocating.
func MatchViolations(err error, rules ...FieldRule) []FieldViolation {
	if err == nil {
		return nil
	}

	var violations []FieldViolation
	for _, rule := range rules {
		if errors.Is(err, rule.Err) {
			violations = append(violations, FieldViolation{
				Field:  rule.Field,
				Code:   rule.Code,
				Params: maps.Clone(rule.Params),
			})
		}
	}
	return violations
}
