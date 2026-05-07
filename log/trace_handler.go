package log

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// TraceIDHandler is a slog.Handler middleware that injects OTel trace_id and
// span_id into every log record when a span is active in the context.
// It must be the outermost handler in the chain so trace IDs are extracted at
// call time, before the record enters the async buffer.
type TraceIDHandler struct {
	next slog.Handler
}

// NewTraceIDHandler wraps next, injecting trace context into every Handle call.
func NewTraceIDHandler(next slog.Handler) *TraceIDHandler {
	return &TraceIDHandler{next: next}
}

func (h *TraceIDHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *TraceIDHandler) Handle(ctx context.Context, r slog.Record) error {
	if span := trace.SpanFromContext(ctx); span.IsRecording() {
		sc := span.SpanContext()
		r.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}
	return h.next.Handle(ctx, r)
}

func (h *TraceIDHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &TraceIDHandler{next: h.next.WithAttrs(attrs)}
}

func (h *TraceIDHandler) WithGroup(name string) slog.Handler {
	return &TraceIDHandler{next: h.next.WithGroup(name)}
}
