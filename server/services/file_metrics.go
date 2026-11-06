package services

import (
	"context"
	"time"

	"connectrpc.com/connect"
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

var (
	filesWriteTotal = mustInt64Counter(telemetry.GetMeter(), "files.write.total",
		metric.WithDescription("WriteFile calls by outcome (ok, conflict_rejected, denied, disabled, rejected, error)"))

	filesWriteDurationMS = mustFloat64Histogram(telemetry.GetMeter(), "files.write.duration_ms",
		metric.WithDescription("Wall-clock duration of a WriteFile call, in milliseconds"),
		metric.WithUnit("ms"))
)

func recordWriteMetrics(ctx context.Context, outcome WriteOutcome, total time.Duration) {
	attrs := metric.WithAttributes(attribute.String("outcome", string(outcome)))
	filesWriteTotal.Add(ctx, 1, attrs)
	filesWriteDurationMS.Record(ctx, float64(total.Milliseconds()), attrs)
}

func recordSearchMetrics(ctx context.Context, outcome SearchOutcome, total time.Duration, firstResult time.Duration) {
	attrs := metric.WithAttributes(attribute.String("outcome", string(outcome)))
	filesSearchTotal.Add(ctx, 1, attrs)
	filesSearchDurationMS.Record(ctx, float64(total.Milliseconds()), attrs)
	if firstResult > 0 {
		filesSearchFirstResultMS.Record(ctx, float64(firstResult.Milliseconds()))
	}
}

var (
	filesRangeTotal = mustInt64Counter(telemetry.GetMeter(), "files.range.total",
		metric.WithDescription("ReadFileRange calls by outcome (ok, torn, busy, denied, invalid, disabled, index_pending, too_large, refused, error)"))

	filesRangeDurationMS = mustFloat64Histogram(telemetry.GetMeter(), "files.range.duration_ms",
		metric.WithDescription("Wall-clock duration of a ReadFileRange call, in milliseconds"),
		metric.WithUnit("ms"))
)

func recordRangeMetrics(ctx context.Context, outcome string, total time.Duration) {
	attrs := metric.WithAttributes(attribute.String("outcome", outcome))
	filesRangeTotal.Add(ctx, 1, attrs)
	filesRangeDurationMS.Record(ctx, float64(total.Milliseconds()), attrs)
}

var (
	filesStatTotal = mustInt64Counter(telemetry.GetMeter(), "files.stat.total",
		metric.WithDescription("StatFile calls by outcome (ok, invalid, denied, refused, error)"))

	filesStatDurationMS = mustFloat64Histogram(telemetry.GetMeter(), "files.stat.duration_ms",
		metric.WithDescription("Wall-clock duration of a StatFile call, in milliseconds"),
		metric.WithUnit("ms"))
)

func statOutcome(err error) string {
	if err == nil {
		return "ok"
	}
	switch connect.CodeOf(err) {
	case connect.CodeInvalidArgument, connect.CodeNotFound:
		return "invalid"
	case connect.CodePermissionDenied:
		return "denied"
	case connect.CodeFailedPrecondition:
		return "refused"
	default:
		return "error"
	}
}

func recordStatMetrics(ctx context.Context, err error, total time.Duration) {
	attrs := metric.WithAttributes(attribute.String("outcome", statOutcome(err)))
	filesStatTotal.Add(ctx, 1, attrs)
	filesStatDurationMS.Record(ctx, float64(total)/float64(time.Millisecond), attrs)
}
