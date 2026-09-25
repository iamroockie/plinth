// Package plinth provides building blocks for JSON HTTP APIs on top of net/http.
//
// It covers what handlers of a JSON API repeat:
//   - [ParseRequestJSON] decodes a request body, checking its content type and size.
//   - [RespondJSON] turns a function that returns a [Response] or an error into a
//     handler. [WriteJSON] and [WriteError] write responses directly.
//   - [NewError] and helpers such as [NotFoundError] create errors that carry a
//     status and an [ErrorCode]. Any other error becomes a 500 response that does
//     not reveal its details.
//   - [ReportError] collects server-side errors of a request, so that they are
//     logged once when the request ends.
//   - [ClientIPResolver] finds the client address behind trusted proxies.
//   - [JSONMux] makes [http.ServeMux] answer unknown routes and methods with JSON.
//   - [PathInt], [PathUUID] and [QueryInt] parse request parameters.
//   - [Healthz] and [Readyz] answer liveness and readiness probes; [Readyz]
//     runs a [CheckFunc] for every dependency.
//
// Error responses have this form, with details present only for
// [ValidationError]:
//
//	{"error": {"code": "validation_error", "message": "Validation failed",
//	  "details": [{"field": "name", "code": "required"}]}}
//
// [MatchViolations] turns validation errors into the violations in details.
//
// The middleware package provides request IDs, client IP resolution, logging,
// panic recovery and timeouts that work with this package. The plinthtest
// package helps to test handlers built with it.
package plinth
