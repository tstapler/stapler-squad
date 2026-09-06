package lifecycle

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/telemetry"
)

var (
	registerOnce sync.Once
	registerErr  error

	endCounter             metric.Int64Counter
	activeGenerationsGauge metric.Int64UpDownCounter
)

func init() {
	if err := RegisterMetrics(); err != nil {
		log.Error("lifecycle: failed to register OTel metrics", "error", err)
	}
}

// RegisterMetrics registers the session_lifecycle_* instruments against
// telemetry.GetMeter(). Idempotent via sync.Once — package init already
// calls this once; exported so a test can call it again safely.
func RegisterMetrics() error {
	registerOnce.Do(func() {
		registerErr = registerMetricsOnce()
	})
	return registerErr
}

func registerMetricsOnce() error {
	meter := telemetry.GetMeter()

	var err error
	if endCounter, err = meter.Int64Counter("session_lifecycle_ends_total",
		metric.WithDescription("Count of goroutine-lifecycle-generation endings, by subsystem and reason")); err != nil {
		return err
	}
	if activeGenerationsGauge, err = meter.Int64UpDownCounter("session_lifecycle_active_generations",
		metric.WithDescription("Count of currently-active (started, not yet ended) goroutine-lifecycle generations, by subsystem — stays elevated if a generation is abandoned rather than returning")); err != nil {
		return err
	}
	return nil
}

// StartGeneration opens one observability span for one goroutine-
// generation's lifetime and increments session_lifecycle_active_generations
// for subsystem. Pair with EndGeneration.
func StartGeneration(ctx context.Context, subsystem string) (context.Context, trace.Span) {
	genCtx, span := telemetry.StartLinkedBackgroundSpan(ctx, "session.lifecycle."+subsystem)
	span.SetAttributes(attribute.String("lifecycle.subsystem", subsystem))
	if activeGenerationsGauge != nil {
		activeGenerationsGauge.Add(genCtx, 1, metric.WithAttributes(attribute.String("subsystem", subsystem)))
	}
	return genCtx, span
}

// EndGeneration tags span with reason, ends it, increments
// session_lifecycle_ends_total, and decrements session_lifecycle_active_generations.
// Pairs with StartGeneration.
func EndGeneration(span trace.Span, subsystem string, reason Reason) {
	span.SetAttributes(attribute.String("lifecycle.reason", reason.String()))
	span.End()

	ctx := context.Background()
	if endCounter != nil {
		endCounter.Add(ctx, 1, metric.WithAttributes(
			attribute.String("subsystem", subsystem),
			attribute.String("reason", reason.String()),
		))
	}
	if activeGenerationsGauge != nil {
		activeGenerationsGauge.Add(ctx, -1, metric.WithAttributes(attribute.String("subsystem", subsystem)))
	}
}

// RecordEnd is a lighter-weight alternative to EndGeneration for a
// lifecycle-ending decision that does not own a dedicated generation span
// (e.g. a reconnect loop's own give-up branches, running mid-generation on
// an already-open span). Adds a span event on ctx's current span, if any —
// a no-op otherwise — plus the same counter increment as EndGeneration. It
// does not touch the active-generations gauge: callers of RecordEnd never
// called StartGeneration, so there is nothing to decrement.
func RecordEnd(ctx context.Context, subsystem string, reason Reason) {
	trace.SpanFromContext(ctx).AddEvent("session.lifecycle.end", trace.WithAttributes(
		attribute.String("lifecycle.subsystem", subsystem),
		attribute.String("lifecycle.reason", reason.String()),
	))
	if endCounter != nil {
		endCounter.Add(ctx, 1, metric.WithAttributes(
			attribute.String("subsystem", subsystem),
			attribute.String("reason", reason.String()),
		))
	}
}
