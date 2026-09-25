# plinth

[![CI](https://github.com/iamroockie/plinth/actions/workflows/ci.yml/badge.svg)](https://github.com/iamroockie/plinth/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/iamroockie/plinth.svg)](https://pkg.go.dev/github.com/iamroockie/plinth)

Building blocks for JSON HTTP APIs on top of `net/http`: request parsing, JSON
responses, typed API errors, client IP resolution behind proxies and middleware
for request IDs, logging, panic recovery and timeouts.

plinth is not a framework. Handlers stay `http.Handler`, routing stays
`http.ServeMux`, logging stays `log/slog`, and the module has no dependencies
outside the standard library.

## Install

```sh
go get github.com/iamroockie/plinth
```

Requires Go 1.27 or later.

## Quick start

```go
package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/iamroockie/plinth"
	"github.com/iamroockie/plinth/middleware"
)

type createUser struct {
	Name string `json:"name"`
}

func main() {
	logger := slog.New(middleware.NewContextLogHandler(slog.NewJSONHandler(os.Stdout, nil)))

	resolver, err := plinth.NewClientIPResolver().WithTrustedProxies("10.0.0.0/8")
	if err != nil {
		logger.Error("invalid proxy", "error", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.Handle("POST /users", plinth.RespondJSON(func(r *http.Request) (*plinth.Response, error) {
		in, err := plinth.ParseRequestJSON[createUser](r)
		if err != nil {
			return nil, err // 400, 413 or 415
		}
		if in.Name == "" {
			return nil, plinth.ValidationError(plinth.ErrorDetails{"name": "required"})
		}
		return plinth.NewResponse(http.StatusCreated, in), nil
	}))
	mux.Handle("GET /users/{id}", plinth.RespondJSON(func(r *http.Request) (*plinth.Response, error) {
		id, err := plinth.PathInt(r, "id")
		if err != nil {
			return nil, err // 400
		}
		logger.InfoContext(r.Context(), "loading user", "id", id)
		return nil, plinth.NotFoundError(nil)
	}))
	mux.Handle("GET /healthz", plinth.Healthz())

	handler := middleware.Chain(
		middleware.RequestID(),
		middleware.ClientIP(resolver),
		middleware.RequestLog(logger, "/healthz"),
		middleware.ErrorLog(logger),
		middleware.Recover(),
		middleware.Timeout(10*time.Second),
	)(plinth.JSONMux(mux))

	srv := &http.Server{
		Addr:              ":8080",
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		logger.Error("server stopped", "error", err)
	}
}
```

## Errors

Every error response has the same shape. `details` is present only for
validation errors:

```json
{"error": {"code": "validation_error", "message": "Validation failed", "details": {"name": "required"}}}
```

Return an error from a `ResponseFunc`, or pass it to `plinth.WriteError`:

| Helper                          | Status | Code                       |
|---------------------------------|--------|----------------------------|
| `BadRequestError(msg, cause)`   | 400    | `bad_request`              |
| `UnauthorizedError(cause)`      | 401    | `unauthorized`             |
| `ForbiddenError(cause)`         | 403    | `forbidden`                |
| `NotFoundError(cause)`          | 404    | `not_found`                |
| `MethodNotAllowedError(cause)`  | 405    | `method_not_allowed`       |
| `ConflictError(msg, cause)`     | 409    | `conflict`                 |
| `RequestEntityTooLargeError(cause)` | 413 | `request_entity_too_large` |
| `UnsupportedMediaTypeError(cause)`  | 415 | `unsupported_media_type`   |
| `ValidationError(details)`      | 422    | `validation_error`         |
| `TooManyRequestsError(cause)`   | 429    | `too_many_requests`        |
| `InternalServerError(cause)`    | 500    | `internal_error`           |
| `ServiceUnavailableError(cause)`| 503    | `service_unavailable`      |
| `GatewayTimeoutError(cause)`    | 504    | `gateway_timeout`          |

For other cases use `plinth.NewError(status, code, message, cause)` with your own
`plinth.ErrorCode`. `plinth.ErrorStatus(err)` returns the status and code that an
error carries, for example for metrics or tests.

- The `cause` is kept for `errors.Is`, `errors.As` and logs and is never sent to
  the client.
- Errors can be wrapped with `fmt.Errorf("...: %w", err)` and keep their status.
- Any other error becomes a 500 `internal_error` without details. If the request
  deadline has passed and the error wraps `context.DeadlineExceeded`, it becomes
  a 503 `service_unavailable` instead.
- Errors with a 5xx status are reported with `plinth.ReportError` and logged by
  the `ErrorLog` middleware. Handlers can report errors that do not change the
  response in the same way.

## Middleware

The order of middleware matters: `middleware.Chain` lists them from the outermost
to the innermost, and the order in the quick start is the recommended one.

| Middleware          | What it does |
|---------------------|--------------|
| `RequestID()`       | Reuses a valid UUID from the `X-Request-Id` request header or generates a UUIDv7, and returns it in the `X-Request-Id` response header. |
| `ClientIP(resolver)`| Resolves the client address, see below. |
| `RequestLog(log, quiet...)` | Logs one record per request with method, path, status, size and duration. The level is Error for 5xx, Warn for 4xx and Info otherwise. Paths in `quiet` are not logged. |
| `ErrorLog(log)`     | Logs the errors reported during a request as one record. |
| `Recover()`         | Turns a panic into a 500 response and reports it with the stack trace. |
| `Timeout(d)`        | Sets a deadline on the request context. Handlers must respect the context; a handler that returns the context error gets a 503. |

`RequestID` and `ClientIP` come first so that logs have `request_id` and
`client_ip`. `RequestLog` and `ErrorLog` wrap `Recover` so that panics are logged
with the right status.

### Request ID and client IP in application logs

`RequestLog` and `ErrorLog` add `request_id` and `client_ip` to their records.
To get them in your own logs, wrap your handler and log with a context:

```go
logger := slog.New(middleware.NewContextLogHandler(slog.NewJSONHandler(os.Stdout, nil)))

logger.InfoContext(r.Context(), "loading user", "id", id)
// {"time":"...","level":"INFO","msg":"loading user","id":42,"request_id":"0192f0a1-...","client_ip":"203.0.113.7"}
```

The values are also available with `middleware.RequestIDFromContext` and
`middleware.ClientIPFromContext`. In tests, `middleware.ContextWithRequestID` and
`middleware.ContextWithClientIP` put them into a context without running the
middleware.

## Client IP behind proxies

`X-Forwarded-For` and similar headers can be set by anyone, so
`ClientIPResolver` uses them only when the request comes from a trusted proxy:

```go
resolver, err := plinth.NewClientIPResolver(). // X-Forwarded-For by default
	WithTrustedProxies("10.0.0.0/8", "fd00::/8")
```

The header is read from right to left. Addresses of trusted proxies are skipped,
and the first untrusted address is the client. Everything to the left of it may be
forged by the client and is ignored:

```text
X-Forwarded-For: 198.51.100.1, 203.0.113.7, 10.0.0.1   (peer 10.0.0.2)
                 forged        client       proxy
```

- Several headers can be given, such as
  `NewClientIPResolver(plinth.HeaderXRealIP, plinth.HeaderXForwardedFor)`. They are
  tried in order.
- A hop `unknown` means that a proxy did not know its client. The header is
  skipped and the next one is tried.
- A hop that is not an IP address is an error (`plinth.ErrMalformedHeader`), and
  the peer address is used.
- Behind a reverse proxy on a Unix socket, call `WithTrustedUnixSocket()`.

## JSON for unknown routes

`http.ServeMux` answers unknown routes and wrong methods with plain text.
`plinth.JSONMux(mux)` turns them into JSON `404 not_found` and
`405 method_not_allowed` errors; the 405 keeps the `Allow` header.

## Health checks

```go
mux.Handle("GET /healthz", plinth.Healthz())
mux.Handle("GET /readyz", plinth.Readyz(2*time.Second, map[string]plinth.CheckFunc{
	"postgres": pool.Ping,      // *pgxpool.Pool
	"mysql":    db.PingContext, // *sql.DB
	"redis": func(ctx context.Context) error { // *redis.Client
		return rdb.Ping(ctx).Err()
	},
}))
```

`Healthz` always answers `200 {"status":"ok"}`. `Readyz` runs all checks
concurrently and answers `200 {"status":"ready"}`, or `503 service_unavailable`
if any of them fails, panics or does not answer within the timeout. The names
show up in the error logged by the ErrorLog middleware, never in the response.

Pass the probe paths to `middleware.RequestLog` so that successful probes do not
flood the log; failed ones are still logged by ErrorLog.

## Request parameters

```go
id, err := plinth.PathInt(r, "id")             // /users/{id}
uid, err := plinth.PathUUID(r, "id")           // /orders/{id}
limit, err := plinth.QueryInt(r, "limit", 20)  // ?limit=50, 20 if missing
```

An invalid value gives a `400 bad_request` error that can be returned as is.

## Request bodies

`ParseRequestJSON` checks `Content-Type` (`application/json` or
`application/*+json`) and limits the body to 1 MiB by default:

```go
in, err := plinth.ParseRequestJSON[createUser](r,
	plinth.WithBodyLimit(10<<20),
	plinth.WithJSONOptions(json.RejectUnknownMembers(true)),
)
```

When the body is over the limit, the 413 response closes the connection, so the
server does not read the rest of it.

## Testing handlers

The `plinthtest` package removes the boilerplate from handler tests:

```go
import "github.com/iamroockie/plinth/plinthtest"

func TestCreateUser(t *testing.T) {
	// The body is encoded as JSON; a string or []byte is sent as is.
	req := plinthtest.NewRequest(t, http.MethodPost, "/users", createUser{Name: ""})

	// reported holds the errors reported with plinth.ReportError, such as 5xx errors.
	rec, reported := plinthtest.Serve(t, handler, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	if e := plinthtest.DecodeError(t, rec); e.Details["name"] != "required" {
		t.Errorf("details = %v, want name: required", e.Details)
	}
	if reported != nil {
		t.Errorf("unexpected reported error: %v", reported)
	}
}
```

- `plinthtest.DecodeJSON[T](t, rec)` decodes a successful response and fails the
  test if it is not JSON.
- If the handler under test includes the `ErrorLog` middleware, it logs the
  reported errors itself and `Serve` returns nil.
- To test a `ResponseFunc` without HTTP, check its error with
  `plinth.ErrorStatus(err)`.

## License

[MIT](LICENSE)
