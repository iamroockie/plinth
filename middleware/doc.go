// Package middleware provides net/http middleware for JSON APIs built with the
// plinth package: request IDs, client IP resolution, request and error logging,
// panic recovery and timeouts.
//
// Middleware that reads what other middleware stores must run inside it, so the
// order matters. [Chain] lists middleware from the outermost to the innermost. The
// recommended order is:
//
//	handler := middleware.Chain(
//		middleware.RequestID(),
//		middleware.ClientIP(resolver),
//		middleware.RequestLog(logger),
//		middleware.ErrorLog(logger),
//		middleware.Recover(),
//		middleware.Timeout(10*time.Second),
//	)(plinth.JSONMux(mux))
//
// RequestID and ClientIP come first, so that every log record has request_id and
// client_ip. RequestLog wraps Recover, so that it logs the 500 status written
// after a panic. ErrorLog wraps Recover and the handler, so that it logs the
// errors they report.
package middleware
