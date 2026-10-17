package services

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/telemetry"
)

// Files RPC instruments, registered once like session_creation_metrics.go. Attributes
// are closed enums only: never a path, query or error text.
var (
	filesSearchTotal = mustInt64Counter(telemetry.GetMeter(), "files.search.total",
		metric.WithDescription("SearchContent calls by outcome (ok, truncated, cancelled, invalid, error, disabled, busy)"))

	filesSearchDurationMS = mustFloat64Histogram(telemetry.GetMeter(), "files.search.duration_ms",
		metric.WithDescription("Wall-clock duration of a SearchContent call, in milliseconds"),
		metric.WithUnit("ms"))

	filesSearchFirstResultMS = mustFloat64Histogram(telemetry.GetMeter(), "files.search.first_result_ms",
		metric.WithDescription("Latency from SearchContent start to the first batch, in milliseconds"),
		metric.WithUnit("ms"))
)

func recordSearchMetrics(ctx context.Context, outcome SearchOutcome, total time.Duration, firstResult time.Duration) {
	attrs := metric.WithAttributes(attribute.String("outcome", string(outcome)))
	filesSearchTotal.Add(ctx, 1, attrs)
	filesSearchDurationMS.Record(ctx, float64(total.Milliseconds()), attrs)
	if firstResult > 0 {
		filesSearchFirstResultMS.Record(ctx, float64(firstResult.Milliseconds()))
	}
}
