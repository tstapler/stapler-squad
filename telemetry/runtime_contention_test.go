package telemetry

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestRegisterRuntimeContentionMetrics_should_ExportMutexWaitSeconds(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	require.NoError(t, RegisterRuntimeContentionMetrics(mp.Meter("test")))

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))

	var found bool
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "go.runtime.mutex_wait" {
				continue
			}
			found = true
			sum, ok := m.Data.(metricdata.Sum[float64])
			require.True(t, ok, "want a float64 cumulative sum, got %T", m.Data)
			require.True(t, sum.IsMonotonic)
			require.Len(t, sum.DataPoints, 1)
			require.GreaterOrEqual(t, sum.DataPoints[0].Value, 0.0)
		}
	}
	require.True(t, found, "go.runtime.mutex_wait not exported")
}

func TestReadMutexWaitSeconds_should_BeMonotonic(t *testing.T) {
	a := readMutexWaitSeconds()
	b := readMutexWaitSeconds()
	require.GreaterOrEqual(t, b, a)
}
