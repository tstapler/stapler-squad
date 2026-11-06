package session

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/telemetry"
)

// scrollForwardBlockedReasonLabelNA is blocked_reason's label value for
// every ScrollForwardOutcome other than BLOCKED -- plan.md's Observability
// Plan calls for "a bounded fourth value, not an open string".
const scrollForwardBlockedReasonLabelNA = "n/a"

// scrollForwardAttemptsTotal and scrollForwardCaptureDurationMS are the two
// instruments named in plan.md's Observability Plan for Epic 1.5, registered
// once package-level via telemetry.GetMeter() -- the same idiom
// session/backend_observability.go's backendOperationDurationMS and
// server/services/session_creation_metrics.go's sessionCreationOutcome
// already use (safe to call before telemetry.Initialize: a no-op-safe
// delegating meter until a real MeterProvider is installed).
var (
	scrollForwardAttemptsTotal = mustInt64CounterScrollForward(telemetry.GetMeter(), "scroll_forward_attempts_total",
		metric.WithDescription("Count of Instance.ForwardScroll calls, labeled by adapter, outcome, and (outcome=BLOCKED only) blocked_reason"))

	scrollForwardCaptureDurationMS = mustFloat64HistogramScrollForward(telemetry.GetMeter(), "scroll_forward_capture_duration_ms",
		metric.WithDescription("Wall-clock duration of ForwardScroll's RedrawQuiescence capture wait, in milliseconds"),
		metric.WithUnit("ms"))
)

func mustInt64CounterScrollForward(meter metric.Meter, name string, opts ...metric.Int64CounterOption) metric.Int64Counter {
	counter, err := meter.Int64Counter(name, opts...)
	if err != nil {
		// Only returned for a malformed instrument name/config, a
		// build-time-constant programmer error -- panicking at package init
		// surfaces it immediately in tests rather than silently dropping the
		// metric forever.
		panic(err)
	}
	return counter
}

func mustFloat64HistogramScrollForward(meter metric.Meter, name string, opts ...metric.Float64HistogramOption) metric.Float64Histogram {
	histogram, err := meter.Float64Histogram(name, opts...)
	if err != nil {
		panic(err)
	}
	return histogram
}

// scrollForwardAdapterLabel returns the resolved ScrollAdapter's Name() for
// program, or "unknown" when no adapter matches -- the "adapter" label value
// shared by both instruments below.
func scrollForwardAdapterLabel(program string) string {
	if adapter := resolveScrollAdapter(program); adapter != nil {
		return adapter.Name()
	}
	return "unknown"
}

// scrollForwardOutcomeLabel maps a ScrollForwardOutcome to its metric label.
func scrollForwardOutcomeLabel(outcome sessionv1.ScrollForwardOutcome) string {
	switch outcome {
	case sessionv1.ScrollForwardOutcome_DELIVERED:
		return "delivered"
	case sessionv1.ScrollForwardOutcome_AT_TOP:
		return "at_top"
	case sessionv1.ScrollForwardOutcome_NO_CAPABILITY:
		return "no_capability"
	case sessionv1.ScrollForwardOutcome_BLOCKED:
		return "blocked"
	default:
		return "unspecified"
	}
}

// scrollForwardBlockedReasonLabel maps a ScrollBlockedReason to its metric
// label, but only when outcome is BLOCKED -- scrollForwardBlockedReasonLabelNA
// for every other outcome, matching plan.md's Observability Plan.
func scrollForwardBlockedReasonLabel(outcome sessionv1.ScrollForwardOutcome, reason sessionv1.ScrollBlockedReason) string {
	if outcome != sessionv1.ScrollForwardOutcome_BLOCKED {
		return scrollForwardBlockedReasonLabelNA
	}
	switch reason {
	case sessionv1.ScrollBlockedReason_MULTIPLE_VIEWERS:
		return "multiple_viewers"
	case sessionv1.ScrollBlockedReason_LEASE_CONTENTION:
		return "lease_contention"
	case sessionv1.ScrollBlockedReason_UNSUPPORTED_STREAMING_PATH:
		return "unsupported_streaming_path"
	default:
		return "unspecified"
	}
}

// recordScrollForwardAttempt records one scroll_forward_attempts_total
// increment for one Instance.ForwardScroll call.
func recordScrollForwardAttempt(adapter string, outcome sessionv1.ScrollForwardOutcome, blockedReason sessionv1.ScrollBlockedReason) {
	scrollForwardAttemptsTotal.Add(context.Background(), 1, metric.WithAttributes(
		attribute.String("adapter", adapter),
		attribute.String("outcome", scrollForwardOutcomeLabel(outcome)),
		attribute.String("blocked_reason", scrollForwardBlockedReasonLabel(outcome, blockedReason)),
	))
}

// recordScrollForwardCaptureDuration records one
// scroll_forward_capture_duration_ms observation for the RedrawQuiescence
// wait inside captureViaRedrawQuiescence.
func recordScrollForwardCaptureDuration(adapter string, duration time.Duration) {
	scrollForwardCaptureDurationMS.Record(context.Background(), float64(duration.Milliseconds()), metric.WithAttributes(
		attribute.String("adapter", adapter),
	))
}
