package backend

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"

	"github.com/tstapler/stapler-squad/log"
)

// Metric names (plan Observability Plan). Labels are closed enums or typed names, never free
// text, URLs or paths.
const (
	MetricFallbackTotal       = "git_backend_fallback_total"        // {operation,cohort,reason}
	MetricErrorTotal          = "git_backend_error_total"           // {operation,implementation}
	MetricShadowMismatchTotal = "git_backend_shadow_mismatch_total" // {operation,class}
	MetricShadowCallsTotal    = "git_backend_shadow_calls_total"    // {operation}
	// MetricCapabilityDetectTotal counts capability preflights, one per call that consulted the
	// probe: {outcome} is "ok", a capability kind, or "detect_error" (with sub_reason set), so
	// detect_error over the sum is the ratio the soak ceiling watches (plan Story 1.3.4 h).
	MetricCapabilityDetectTotal = "git_capability_detect_total"
	// MetricShadowDurationMS records how long each side of a shadow comparison took, so the soak
	// can check go-git p50 against the CLI's: {operation,implementation}. One sample per side per
	// shadowed call (the first read only, not the classification re-reads).
	MetricShadowDurationMS = "git_backend_shadow_duration_ms"
)

// ShadowDurationBucketsMS are the explicit histogram bounds for MetricShadowDurationMS. Fixed so
// that a persisted soak ledger can be merged across restarts.
var ShadowDurationBucketsMS = []float64{0.1, 0.25, 0.5, 1, 2, 5, 10, 20, 50, 100, 250, 500, 1000, 5000}

// Implementation labels the backend that ran, matching withOperationSpan's label.
type Implementation string

const (
	ImplCLI   Implementation = "cli"
	ImplGoGit Implementation = "gogit"
)

type routerMetrics struct {
	fallback       metric.Int64Counter
	errors         metric.Int64Counter
	shadowMismatch metric.Int64Counter
	shadowCalls    metric.Int64Counter
	capability     metric.Int64Counter
	shadowDuration metric.Float64Histogram
}

func newRouterMetrics(meter metric.Meter) routerMetrics {
	counter := func(name, desc string) metric.Int64Counter {
		c, err := meter.Int64Counter(name, metric.WithDescription(desc))
		if err != nil {
			log.Warn("git backend metric unavailable, counting disabled", "metric", name, "err", err)
			return noop.Int64Counter{}
		}
		return c
	}
	duration, err := meter.Float64Histogram(MetricShadowDurationMS,
		metric.WithDescription("Shadow-mode wall time per implementation, in milliseconds"),
		metric.WithUnit("ms"),
		metric.WithExplicitBucketBoundaries(ShadowDurationBucketsMS...))
	if err != nil {
		log.Warn("git backend metric unavailable, counting disabled", "metric", MetricShadowDurationMS, "err", err)
		duration = noop.Float64Histogram{}
	}
	return routerMetrics{
		shadowDuration: duration,
		fallback:       counter(MetricFallbackTotal, "Calls routed to the CLI instead of the cohort backend, by reason"),
		errors:         counter(MetricErrorTotal, "Operational backend errors returned to the caller"),
		shadowMismatch: counter(MetricShadowMismatchTotal, "Shadow-mode CLI/go-git disagreements by class"),
		shadowCalls:    counter(MetricShadowCallsTotal, "Shadow-mode calls (the mismatch window denominator)"),
		capability:     counter(MetricCapabilityDetectTotal, "Capability preflights by outcome; detect_error carries a closed sub_reason"),
	}
}

func (m routerMetrics) countFallback(op OperationName, c Cohort, reason FallbackReason) {
	m.fallback.Add(context.Background(), 1, metric.WithAttributes(
		attribute.String("operation", string(op)),
		attribute.String("cohort", c.String()),
		attribute.String("reason", string(reason))))
}

func (m routerMetrics) countError(op OperationName, impl Implementation) {
	m.errors.Add(context.Background(), 1, metric.WithAttributes(
		attribute.String("operation", string(op)),
		attribute.String("implementation", string(impl))))
}

func (m routerMetrics) countShadowCall(op OperationName) {
	m.shadowCalls.Add(context.Background(), 1, metric.WithAttributes(attribute.String("operation", string(op))))
}

func (m routerMetrics) countMismatch(op OperationName, class MismatchClass) {
	m.shadowMismatch.Add(context.Background(), 1, metric.WithAttributes(
		attribute.String("operation", string(op)),
		attribute.String("class", string(class))))
}

func (m routerMetrics) countCapability(outcome string, sub DetectSub) {
	m.capability.Add(context.Background(), 1, metric.WithAttributes(
		attribute.String("outcome", outcome),
		attribute.String("sub_reason", string(sub))))
}

func (m routerMetrics) recordShadowDuration(op OperationName, impl Implementation, d time.Duration) {
	m.shadowDuration.Record(context.Background(), float64(d)/float64(time.Millisecond), metric.WithAttributes(
		attribute.String("operation", string(op)),
		attribute.String("implementation", string(impl))))
}
