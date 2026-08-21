package log_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	applog "github.com/tstapler/stapler-squad/log"
	"go.opentelemetry.io/otel/trace"
)

func TestTraceIDHandler_NoSpan(t *testing.T) {
	var buf bytes.Buffer
	base := slog.NewJSONHandler(&buf, nil)
	h := applog.NewTraceIDHandler(base)
	logger := slog.New(h)

	logger.InfoContext(context.Background(), "no span")

	if strings.Contains(buf.String(), "trace_id") {
		t.Error("expected no trace_id when no span is active")
	}
}

func TestTraceIDHandler_WithSpan(t *testing.T) {
	// Use a no-op span to verify the handler doesn't panic on a non-recording span.
	var buf bytes.Buffer
	base := slog.NewJSONHandler(&buf, nil)
	h := applog.NewTraceIDHandler(base)
	logger := slog.New(h)

	ctx := trace.ContextWithSpan(context.Background(), trace.SpanFromContext(context.Background()))
	logger.InfoContext(ctx, "with noop span")
	// Non-recording span — trace_id should not be injected
	if strings.Contains(buf.String(), "trace_id") {
		t.Error("expected no trace_id for non-recording span")
	}
}
