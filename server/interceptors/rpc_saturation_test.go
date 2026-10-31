package interceptors

import (
	"context"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestMethodName(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "GetVCSStatus", methodName("/session.v1.SessionService/GetVCSStatus"))
	assert.Equal(t, "x", methodName("x"))
}

type fakeAnyRequest struct {
	connect.AnyRequest
	procedure string
}

func (f fakeAnyRequest) Spec() connect.Spec { return connect.Spec{Procedure: f.procedure} }

func collect(t *testing.T, r *sdkmetric.ManualReader) metricdata.ResourceMetrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, r.Collect(context.Background(), &rm))
	return rm
}

func sumOf(rm metricdata.ResourceMetrics, name string) (total int64, found bool) {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			if s, ok := m.Data.(metricdata.Sum[int64]); ok {
				for _, dp := range s.DataPoints {
					total += dp.Value
				}
				return total, true
			}
		}
	}
	return 0, false
}

// Not parallel: swaps the global meter provider.
func TestRPCSaturation_should_CountClientAbortAndReleaseInflight(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	prev := otel.GetMeterProvider()
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))
	t.Cleanup(func() { otel.SetMeterProvider(prev) })
	rpcSatOnce = sync.Once{}
	rpcSat = nil

	sat := NewRPCSaturationInterceptor()
	require.NotNil(t, sat.inflight)

	ctx, cancel := context.WithCancel(context.Background())
	inHandler := make(chan struct{})
	h := sat.WrapUnary(func(ctx context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
		close(inHandler)
		cancel() // client disconnects mid-call
		return nil, ctx.Err()
	})
	_, err := h(ctx, fakeAnyRequest{procedure: "/s.v1.S/GetVCSStatus"})
	require.Error(t, err)
	<-inHandler

	rm := collect(t, reader)
	aborted, ok := sumOf(rm, "rpc.server.client_aborted_total")
	require.True(t, ok)
	assert.Equal(t, int64(1), aborted)
	inflight, ok := sumOf(rm, "rpc.server.inflight")
	require.True(t, ok)
	assert.Equal(t, int64(0), inflight, "inflight returns to zero after the handler exits")
}
