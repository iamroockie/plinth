package middleware_test

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	"github.com/iamroockie/plinth"
	"github.com/iamroockie/plinth/middleware"
)

//nolint:lll // log records in the expected output do not fit the line limit
func ExampleChain() {
	// Drop the time and the duration so that the output is stable.
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey || a.Key == "duration_ms" {
				return slog.Attr{}
			}
			return a
		},
	}))
	// Application logs get request_id and client_ip as well.
	logger = slog.New(middleware.NewContextLogHandler(logger.Handler()))

	resolver, err := plinth.NewClientIPResolver().WithTrustedProxies("10.0.0.0/8")
	if err != nil {
		panic(err)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /users/{id}", plinth.RespondJSON(
		func(r *http.Request) (*plinth.Response, error) {
			logger.InfoContext(r.Context(), "loading user")
			return nil, errors.New("connection refused")
		},
	))

	handler := middleware.Chain(
		middleware.RequestID(),
		middleware.ClientIP(resolver),
		middleware.RequestLog(logger),
		middleware.ErrorLog(logger),
		middleware.Recover(),
		middleware.Timeout(10*time.Second),
	)(plinth.JSONMux(mux))

	r := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	r.RemoteAddr = "10.0.0.2:51234"
	r.Header.Set(plinth.HeaderXForwardedFor, "203.0.113.7")
	r.Header.Set(plinth.HeaderXRequestID, "0192f0a1-7c3b-7d2e-8f10-123456789abc")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, r)
	fmt.Println(rec.Code, rec.Body.String())

	// Output:
	// level=INFO msg="loading user" request_id=0192f0a1-7c3b-7d2e-8f10-123456789abc client_ip=203.0.113.7
	// level=ERROR msg="request error" error="connection refused" request_id=0192f0a1-7c3b-7d2e-8f10-123456789abc client_ip=203.0.113.7
	// level=ERROR msg="http request" method=GET path=/users/42 status=500 size=69 request_id=0192f0a1-7c3b-7d2e-8f10-123456789abc client_ip=203.0.113.7
	// 500 {"error":{"code":"internal_error","message":"Internal Server Error"}}
}
