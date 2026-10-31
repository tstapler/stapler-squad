package telemetry

import sdkmetric "go.opentelemetry.io/otel/sdk/metric"

// The SDK's default histogram buckets stop at 10 s (10000 ms), so any slower
// request lands in +Inf and histogram_quantile clamps its p95 to exactly 10 s,
// hiding whether a stall lasted 11 s or 54 s. These extend the tail.
var (
	rpcDurationMsBuckets = []float64{0, 5, 10, 25, 50, 75, 100, 250, 500, 750, 1000, 2500, 5000, 7500, 10000, 15000, 30000, 60000, 120000}
	httpDurationSBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1, 2.5, 5, 7.5, 10, 15, 30, 60, 120}
)

func longTailLatencyViews() []sdkmetric.View {
	hist := func(name string, bounds []float64) sdkmetric.View {
		return sdkmetric.NewView(
			sdkmetric.Instrument{Name: name},
			sdkmetric.Stream{Aggregation: sdkmetric.AggregationExplicitBucketHistogram{Boundaries: bounds}},
		)
	}
	return []sdkmetric.View{
		hist("rpc.server.duration", rpcDurationMsBuckets),
		hist("http.server.request.duration", httpDurationSBuckets),
	}
}
