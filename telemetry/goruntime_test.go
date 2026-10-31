package telemetry

import (
	"context"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"math"
	"runtime/metrics"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeltaQuantile_should_IgnoreHistoryBeforeThePreviousCollection(t *testing.T) {
	t.Parallel()
	buckets := []float64{0, 0.001, 0.01, 0.1, 1}
	prev := &metrics.Float64Histogram{Buckets: buckets, Counts: []uint64{1000, 0, 0, 0}}
	cur := &metrics.Float64Histogram{Buckets: buckets, Counts: []uint64{1000, 0, 0, 10}}

	got, ok := deltaQuantile(prev, cur, 0.99)
	assert.True(t, ok)
	assert.Equal(t, 1.0, got, "10 new samples all in the slowest bucket")

	_, ok = deltaQuantile(cur, cur, 0.99)
	assert.False(t, ok, "an empty window reports nothing rather than a stale value")
}

func TestDeltaQuantile_should_ReportLowerBoundForOverflowBucket(t *testing.T) {
	t.Parallel()
	h := &metrics.Float64Histogram{Buckets: []float64{0, 1, posInf()}, Counts: []uint64{0, 5}}
	got, ok := deltaQuantile(nil, h, 0.5)
	assert.True(t, ok)
	assert.Equal(t, 1.0, got)
}

func posInf() float64 { return math.Inf(1) }

func TestRegisterGoRuntimeMetrics_should_ExportRuntimeGauges(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	require.NoError(t, registerGoRuntimeMetrics(mp.Meter("t")))

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))
	got := map[string]bool{}
	for _, m := range rm.ScopeMetrics[0].Metrics {
		got[m.Name] = true
	}
	for _, name := range []string{"go.runtime.goroutines", "go.runtime.gomaxprocs", "go.runtime.memory_bytes"} {
		assert.True(t, got[name], name)
	}
}
