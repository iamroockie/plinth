package middleware_test

import (
	"log/slog"
	"net/netip"
	"strings"
	"testing"
	"uuid"

	"github.com/iamroockie/plinth/middleware"
)

func TestContextLogHandler(t *testing.T) {
	id := uuid.NewV7()
	ctx := middleware.ContextWithRequestID(t.Context(), id)
	ctx = middleware.ContextWithClientIP(ctx, netip.MustParseAddr("1.2.3.4"))

	t.Run("adds request attrs", func(t *testing.T) {
		base, buf := newJSONLogger(t)
		log := slog.New(middleware.NewContextLogHandler(base.Handler()))

		log.InfoContext(ctx, "app")

		rec := singleLogRecord(t, buf)
		if rec["request_id"] != id.String() || rec["client_ip"] != "1.2.3.4" {
			t.Errorf("record = %v, want request_id %s and client_ip 1.2.3.4", rec, id)
		}
	})

	t.Run("no request attrs without context values", func(t *testing.T) {
		base, buf := newJSONLogger(t)
		log := slog.New(middleware.NewContextLogHandler(base.Handler()))

		log.InfoContext(t.Context(), "app")

		rec := singleLogRecord(t, buf)
		for _, key := range []string{"request_id", "client_ip"} {
			if _, ok := rec[key]; ok {
				t.Errorf("record has %s, want none", key)
			}
		}
	})

	t.Run("wrapping twice does not duplicate attrs", func(t *testing.T) {
		base, buf := newJSONLogger(t)
		h := middleware.NewContextLogHandler(middleware.NewContextLogHandler(base.Handler()))

		slog.New(h).InfoContext(ctx, "app")

		if n := strings.Count(buf.String(), `"request_id"`); n != 1 {
			t.Errorf("request_id appears %d times in %s, want once", n, buf.String())
		}
	})

	t.Run("With keeps request attrs", func(t *testing.T) {
		base, buf := newJSONLogger(t)
		log := slog.New(middleware.NewContextLogHandler(base.Handler())).With("k", "v")

		log.InfoContext(ctx, "app")

		rec := singleLogRecord(t, buf)
		if rec["k"] != "v" || rec["request_id"] != id.String() {
			t.Errorf("record = %v, want k=v and request_id %s", rec, id)
		}
	})

	t.Run("WithGroup keeps request attrs", func(t *testing.T) {
		base, buf := newJSONLogger(t)
		log := slog.New(middleware.NewContextLogHandler(base.Handler())).WithGroup("g")

		log.InfoContext(ctx, "app")

		if !strings.Contains(buf.String(), id.String()) {
			t.Errorf("log = %s, want request_id %s", buf.String(), id)
		}
	})

	t.Run("respects level", func(t *testing.T) {
		base, buf := newJSONLogger(t)
		log := slog.New(middleware.NewContextLogHandler(base.Handler()))

		log.DebugContext(ctx, "hidden")

		if buf.Len() != 0 {
			t.Errorf("log = %s, want nothing below the handler level", buf.String())
		}
	})
}
