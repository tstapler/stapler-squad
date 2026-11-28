package vcsstatus

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/trace"

	"github.com/tstapler/stapler-squad/session/vc"
)

// OTel's global meter delegates to the first provider set in a process, so one reader serves
// every run of this binary and assertions are on deltas.
var testMetricReader = sdkmetric.NewManualReader()

func init() {
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(testMetricReader)))
}

func requestCount(t *testing.T, outcome string) int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, testMetricReader.Collect(context.Background(), &rm))
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "vcs.status.requests" {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok)
			for _, p := range sum.DataPoints {
				if v, ok := p.Attributes.Value(attribute.Key("outcome")); ok && v.AsString() == outcome {
					return p.Value
				}
			}
		}
	}
	return 0
}

// Not parallel: reads a process-wide counter by delta.
func TestServe_ShouldCountACacheHit_When_EntryIsFresh(t *testing.T) {
	var c Cache
	c.StoreForTest("/w", &vc.VCSStatus{Branch: "cached"}, time.Now())
	before := requestCount(t, "cache_hit")

	resp, err := c.Serve(context.Background(), Request{WorkDir: "/w"})

	require.NoError(t, err)
	assert.Equal(t, "cached", resp.Msg.VcsStatus.Branch)
	assert.Equal(t, int64(1), requestCount(t, "cache_hit")-before)
}

// The literal age pins the stale window: an entry past a minute is a miss that waits for git.
func TestServe_ShouldNotServeAnEntryOlderThanTheStaleWindow(t *testing.T) {
	t.Parallel()
	var c Cache
	t.Cleanup(c.flights.wait)
	const dir = "/ssq-no-vcs/stale" // computing it fails, which is how a miss shows
	c.StoreForTest(dir, &vc.VCSStatus{Branch: "old"}, time.Now().Add(-61*time.Second))

	resp, err := c.Serve(context.Background(), Request{WorkDir: dir, BaseSHA: func() string { return "" }})

	require.NoError(t, err)
	assert.Nil(t, resp.Msg.VcsStatus, "the 61 s old entry must not be served")
	assert.NotEmpty(t, resp.Msg.Error)
}

func TestFailure_ShouldMapTheCallerAndFlightContextErrors(t *testing.T) {
	t.Parallel()
	expired, cancelExpired := context.WithDeadline(context.Background(), time.Unix(0, 0))
	t.Cleanup(cancelExpired)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	tests := []struct {
		name      string
		ctx       context.Context
		flightErr error
		want      connect.Code
	}{
		{"caller deadline", expired, context.DeadlineExceeded, connect.CodeDeadlineExceeded},
		{"caller canceled", canceled, context.Canceled, connect.CodeCanceled},
		{"shared computation timed out", context.Background(), context.DeadlineExceeded, connect.CodeUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var c Cache

			_, err := c.failure(tt.ctx, trace.SpanFromContext(tt.ctx), tt.flightErr)

			assert.Equal(t, tt.want, connect.CodeOf(err))
		})
	}
}
