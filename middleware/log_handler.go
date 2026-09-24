package middleware

import (
	"context"
	"log/slog"
)

type contextLogHandler struct {
	slog.Handler
}

// NewContextLogHandler returns a handler that adds request_id and client_ip,
// stored by [RequestID] and [ClientIP] or by [ContextWithRequestID] and
// [ContextWithClientIP], to records logged with a context that has them, such as
// with [slog.InfoContext]. Wrapping a handler twice has no further effect.
//
// The attributes are added to each record, so after WithGroup they are placed
// inside the group.
func NewContextLogHandler(h slog.Handler) slog.Handler {
	if _, ok := h.(contextLogHandler); ok {
		return h
	}
	return contextLogHandler{Handler: h}
}

func (h contextLogHandler) Handle(ctx context.Context, rec slog.Record) error {
	if id, ok := RequestIDFromContext(ctx); ok {
		rec.AddAttrs(slog.Any("request_id", id))
	}
	if ip, ok := ClientIPFromContext(ctx); ok {
		rec.AddAttrs(slog.Any("client_ip", ip))
	}
	return h.Handler.Handle(ctx, rec)
}

func (h contextLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return contextLogHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h contextLogHandler) WithGroup(name string) slog.Handler {
	return contextLogHandler{Handler: h.Handler.WithGroup(name)}
}

func withContextAttrs(log *slog.Logger) *slog.Logger {
	if log == nil {
		log = slog.Default()
	}
	if _, ok := log.Handler().(contextLogHandler); ok {
		return log
	}
	return slog.New(NewContextLogHandler(log.Handler()))
}
