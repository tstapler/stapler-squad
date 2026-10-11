package backend

import (
	"context"

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
)

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
	return routerMetrics{
		fallback:       counter(MetricFallbackTotal, "Calls routed to the CLI instead of the cohort backend, by reason"),
		errors:         counter(MetricErrorTotal, "Operational backend errors returned to the caller"),
		shadowMismatch: counter(MetricShadowMismatchTotal, "Shadow-mode CLI/go-git disagreements by class"),
		shadowCalls:    counter(MetricShadowCallsTotal, "Shadow-mode calls (the mismatch window denominator)"),
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
