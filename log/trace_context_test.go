package log_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	applog "github.com/tstapler/stapler-squad/log"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// TestInfoContext_InjectsTraceID_WhenSpanRecording guards the exact bug this
// fix closes: logAt used to hardcode context.Background() internally, so
// TraceIDHandler (which is wired into the real handler chain in
// initializeWithConfig) could never see a live span, for any call site --
// there was no way to reach it. InfoContext threads a real ctx through.
func TestInfoContext_InjectsTraceID_WhenSpanRecording(t *testing.T) {
	var buf bytes.Buffer
	base := slog.NewJSONHandler(&buf, nil)
	origDefault := applog.SetSlogDefaultForTest(slog.New(applog.NewTraceIDHandler(base)))
	t.Cleanup(func() { applog.SetSlogDefaultForTest(origDefault) })

	tp := sdktrace.NewTracerProvider() // default sampler is AlwaysSample
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	ctx, span := tp.Tracer("test").Start(context.Background(), "op")
	defer span.End()

	applog.InfoContext(ctx, "traced message")

	if !strings.Contains(buf.String(), "trace_id") {
		t.Errorf("expected trace_id to be injected via InfoContext, got: %s", buf.String())
	}
}

// TestInfo_NeverInjectsTraceID documents the current, deliberate scope of the
// fix: the ctx-less Info/Warn/Error/Debug convenience functions still pass
// context.Background() internally, so they never carry trace correlation --
// only call sites that switch to InfoContext/WarnContext/ErrorContext/
// DebugContext get it. This is intentional (see those functions' doc
// comments), not a remaining gap.
func TestInfo_NeverInjectsTraceID(t *testing.T) {
	var buf bytes.Buffer
	base := slog.NewJSONHandler(&buf, nil)
	origDefault := applog.SetSlogDefaultForTest(slog.New(applog.NewTraceIDHandler(base)))
	t.Cleanup(func() { applog.SetSlogDefaultForTest(origDefault) })

	applog.Info("untraced message")

	if strings.Contains(buf.String(), "trace_id") {
		t.Errorf("Info() should never carry trace correlation, got: %s", buf.String())
	}
}
