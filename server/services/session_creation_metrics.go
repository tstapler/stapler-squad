package services

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/telemetry"
)

// Session-creation outcome values used as the "outcome" attribute on both
// instruments below. Kept as a closed set of constants so call sites (the
// Epic 2.2/2.3 terminal-write path) can't typo a free-form string.
const (
	SessionCreationOutcomeSuccess   = "success"
	SessionCreationOutcomeFailed    = "failed"
	SessionCreationOutcomeStale     = "stale"
	SessionCreationOutcomeCancelled = "cancelled"
)

// sessionCreationOutcome and sessionCreationDurationMS are registered once,
// package-level, following the telemetry.GetMeter() idiom already used for
// executor/safeexec's safeexec.sigkill_escalations counter
// (executor/safeexec/safeexec_metrics.go) — telemetry.GetMeter() is safe to
// call before telemetry.Initialize (returns a no-op-safe delegating meter
// until a real MeterProvider is installed via otel.SetMeterProvider).
var (
	sessionCreationOutcome = mustInt64Counter(telemetry.GetMeter(), "session.creation.outcome",
		metric.WithDescription("Count of session-creation pipeline runs by terminal outcome (success, failed, stale, cancelled)"))

	sessionCreationDurationMS = mustFloat64Histogram(telemetry.GetMeter(), "session.creation.duration_ms",
		metric.WithDescription("Wall-clock duration from session-creation start to the terminal write, in milliseconds"),
		metric.WithUnit("ms"))
)

func mustInt64Counter(meter metric.Meter, name string, opts ...metric.Int64CounterOption) metric.Int64Counter {
	counter, err := meter.Int64Counter(name, opts...)
	if err != nil {
		// Only returned for a malformed instrument name/config, a
		// build-time-constant programmer error — panicking at package init
		// surfaces it immediately in tests rather than silently dropping the
		// metric forever.
		panic(err)
	}
	return counter
}

func mustFloat64Histogram(meter metric.Meter, name string, opts ...metric.Float64HistogramOption) metric.Float64Histogram {
	histogram, err := meter.Float64Histogram(name, opts...)
	if err != nil {
		panic(err)
	}
	return histogram
}

// RecordSessionCreationMetrics increments session.creation.outcome and
// records session.creation.duration_ms, both tagged with the same "outcome"
// attribute, for one completed (or terminally-failed/stale/cancelled)
// session-creation pipeline run. Intended to be called exactly once per
// pipeline run, at the terminal write — see Epic 2.2/2.3's terminal-write
// call site (server/services/session_service.go, not yet implemented as of
// this Epic 1.3 change).
func RecordSessionCreationMetrics(ctx context.Context, outcome string, duration time.Duration) {
	attrs := metric.WithAttributes(attribute.String("outcome", outcome))
	sessionCreationOutcome.Add(ctx, 1, attrs)
	sessionCreationDurationMS.Record(ctx, float64(duration.Milliseconds()), attrs)
}
