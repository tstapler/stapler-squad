package telemetry

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestLongTailLatencyViews_should_KeepBucketsAbove10s(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader), sdkmetric.WithView(longTailLatencyViews()...))
	h, err := mp.Meter("t").Float64Histogram("rpc.server.duration")
	require.NoError(t, err)
	h.Record(context.Background(), 54_000)

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))
	hd := rm.ScopeMetrics[0].Metrics[0].Data.(metricdata.Histogram[float64])
	dp := hd.DataPoints[0]
	assert.Equal(t, rpcDurationMsBuckets, dp.Bounds)
	// 54 s falls in the (30000, 60000] bucket, not the overflow bucket.
	assert.EqualValues(t, 1, dp.BucketCounts[len(rpcDurationMsBuckets)-2])
	assert.EqualValues(t, 0, dp.BucketCounts[len(rpcDurationMsBuckets)])
}
