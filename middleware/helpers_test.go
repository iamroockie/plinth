package middleware_test

import (
	"bytes"
	"encoding/json/v2"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/iamroockie/plinth"
	"github.com/iamroockie/plinth/plinthtest"
)

func newJSONLogger(t *testing.T) (*slog.Logger, *bytes.Buffer) {
	t.Helper()

	var buf bytes.Buffer
	return slog.New(slog.NewJSONHandler(&buf, nil)), &buf
}

func captureDefaultLog(t *testing.T) *bytes.Buffer {
	t.Helper()

	log, buf := newJSONLogger(t)
	prev := slog.Default()
	slog.SetDefault(log)
	t.Cleanup(func() { slog.SetDefault(prev) })

	return buf
}

func logRecords(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()

	var records []map[string]any
	for line := range strings.Lines(buf.String()) {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("decode log line %q: %v", line, err)
		}
		records = append(records, rec)
	}
	return records
}

func singleLogRecord(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()

	records := logRecords(t, buf)
	if len(records) != 1 {
		t.Fatalf("got %d log records, want 1: %s", len(records), buf.String())
	}
	return records[0]
}

func newTestResolver(t *testing.T) plinth.ClientIPResolver {
	t.Helper()

	c, err := plinth.NewClientIPResolver().WithTrustedProxies("10.0.0.0/8")
	if err != nil {
		t.Fatalf("WithTrustedProxies: %v", err)
	}
	return c
}

func newProxiedRequest(t *testing.T, target string) *http.Request {
	t.Helper()

	r := plinthtest.NewRequest(t, http.MethodGet, target, nil)
	r.RemoteAddr = "10.0.0.1:1234"
	r.Header.Set(plinth.HeaderXForwardedFor, "1.2.3.4")
	return r
}
