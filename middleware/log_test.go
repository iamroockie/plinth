package middleware_test

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/iamroockie/plinth"
	"github.com/iamroockie/plinth/middleware"
)

func statusHandler(status int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if status != 0 {
			w.WriteHeader(status)
		}
	})
}

func TestRequestLog(t *testing.T) {
	log, buf := newJSONLogger(t)
	h := middleware.RequestLog(log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("hello"))
	}))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/items?x=1", nil))

	rec := singleLogRecord(t, buf)
	want := map[string]any{
		"level":  "INFO",
		"msg":    "http request",
		"method": http.MethodPost,
		"path":   "/items",
		"status": float64(http.StatusCreated),
		"size":   float64(5),
	}
	for key, value := range want {
		if rec[key] != value {
			t.Errorf("%s = %v, want %v", key, rec[key], value)
		}
	}
	if _, ok := rec["duration_ms"].(float64); !ok {
		t.Errorf("duration_ms = %v, want a number", rec["duration_ms"])
	}
	for _, key := range []string{"request_id", "client_ip"} {
		if _, ok := rec[key]; ok {
			t.Errorf("record has %s without the middleware that sets it", key)
		}
	}
}

func TestRequestLogLevel(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   string
	}{
		{name: "nothing written", want: "INFO"},
		{name: "ok", status: http.StatusOK, want: "INFO"},
		{name: "redirect", status: http.StatusFound, want: "INFO"},
		{name: "client error", status: http.StatusNotFound, want: "WARN"},
		{name: "server error", status: http.StatusBadGateway, want: "ERROR"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			log, buf := newJSONLogger(t)
			h := middleware.RequestLog(log)(statusHandler(tc.status))

			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

			if got := singleLogRecord(t, buf)["level"]; got != tc.want {
				t.Errorf("level = %v, want %s", got, tc.want)
			}
		})
	}
}

func TestRequestLogQuietPaths(t *testing.T) {
	log, buf := newJSONLogger(t)
	h := middleware.RequestLog(log, "/health", "/ready")(statusHandler(http.StatusOK))

	for _, target := range []string{"/health", "/ready?full=1", "/items"} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, target, nil))
	}

	if got := singleLogRecord(t, buf)["path"]; got != "/items" {
		t.Errorf("logged path = %v, want /items", got)
	}
}

func TestRequestLogNilLoggerUsesDefault(t *testing.T) {
	buf := captureDefaultLog(t)
	h := middleware.RequestLog(nil)(statusHandler(http.StatusOK))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if got := singleLogRecord(t, buf)["msg"]; got != "http request" {
		t.Errorf("msg = %v, want %q", got, "http request")
	}
}

func TestRequestLogContextAttrs(t *testing.T) {
	tests := []struct {
		name string
		wrap bool
	}{
		{name: "plain logger"},
		{name: "logger already wrapped", wrap: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			log, buf := newJSONLogger(t)
			if tc.wrap {
				log = slog.New(middleware.NewContextLogHandler(log.Handler())).With("k", "v")
			}

			var requestID string
			h := middleware.Chain(
				middleware.RequestID(),
				middleware.ClientIP(newTestResolver(t)),
				middleware.RequestLog(log),
			)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				id, _ := middleware.RequestIDFromContext(r.Context())
				requestID = id.String()
			}))

			h.ServeHTTP(httptest.NewRecorder(), newProxiedRequest(t, "/"))

			line := buf.String()
			rec := singleLogRecord(t, buf)
			if rec["request_id"] != requestID || rec["client_ip"] != "1.2.3.4" {
				t.Errorf("record = %v, want request_id %s and client_ip 1.2.3.4", rec, requestID)
			}
			for _, key := range []string{`"request_id"`, `"client_ip"`} {
				if n := strings.Count(line, key); n != 1 {
					t.Errorf("%s appears %d times in %s, want once", key, n, line)
				}
			}
		})
	}
}

func TestErrorLog(t *testing.T) {
	errA := errors.New("first failure")
	errB := errors.New("second failure")

	log, buf := newJSONLogger(t)
	h := middleware.Chain(
		middleware.RequestID(),
		middleware.ErrorLog(log),
	)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		plinth.ReportError(r.Context(), errA)
		plinth.ReportError(r.Context(), errB)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	record := singleLogRecord(t, buf)
	if record["level"] != "ERROR" || record["msg"] != "request error" {
		t.Errorf("record = %v, want ERROR request error", record)
	}
	errText, _ := record["error"].(string)
	if !strings.Contains(errText, errA.Error()) || !strings.Contains(errText, errB.Error()) {
		t.Errorf("error = %q, want both reported errors", errText)
	}
	if record["request_id"] != rec.Header().Get(plinth.HeaderXRequestID) {
		t.Errorf("request_id = %v, want %s", record["request_id"],
			rec.Header().Get(plinth.HeaderXRequestID))
	}
}

func TestErrorLogWithoutErrors(t *testing.T) {
	log, buf := newJSONLogger(t)
	h := middleware.ErrorLog(log)(statusHandler(http.StatusOK))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if buf.Len() != 0 {
		t.Errorf("log = %s, want nothing", buf.String())
	}
}

func TestErrorLogNilLoggerUsesDefault(t *testing.T) {
	buf := captureDefaultLog(t)
	h := middleware.ErrorLog(nil)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		plinth.ReportError(r.Context(), errors.New("boom"))
	}))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if got := singleLogRecord(t, buf)["msg"]; got != "request error" {
		t.Errorf("msg = %v, want %q", got, "request error")
	}
}

func TestErrorLogLateErrorGoesToDefault(t *testing.T) {
	defaultBuf := captureDefaultLog(t)
	log, buf := newJSONLogger(t)

	var reqCtx context.Context
	h := middleware.ErrorLog(log)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		reqCtx = r.Context()
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	plinth.ReportError(reqCtx, errors.New("late"))

	if buf.Len() != 0 {
		t.Errorf("middleware log = %s, want nothing", buf.String())
	}
	if got := singleLogRecord(t, defaultBuf)["error"]; got != "late" {
		t.Errorf("default log error = %v, want late", got)
	}
}
