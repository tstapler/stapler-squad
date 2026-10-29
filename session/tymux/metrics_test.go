package tymux

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"

	"github.com/tstapler/stapler-squad/testutil/wait"
)

// counterByLabel sums the Int64 Sum data points of name whose label
// labelKey equals labelValue (0 when the metric has no data yet). Tests
// assert on before/after deltas because the reader lives for the whole
// test binary.
func counterByLabel(t *testing.T, name, labelKey, labelValue string) int64 {
	t.Helper()
	m := collectMetric(t, name)
	if m == nil {
		return 0
	}
	sum, ok := m.Data.(metricdata.Sum[int64])
	require.True(t, ok, "unexpected data type %T for %s", m.Data, name)
	var total int64
	for _, dp := range sum.DataPoints {
		if v, ok := dp.Attributes.Value(attribute.Key(labelKey)); ok && v.AsString() == labelValue {
			total += dp.Value
		}
	}
	return total
}

func TestOutputGapEvent_RecordsReconnectAndAttachEventAndSnapshotRedraw(t *testing.T) {
	sess, _, transport := startedSessionWithStream(t)
	transport.capturePaneFn = func(context.Context, *connect.Request[v1.CapturePaneRequest]) (*connect.Response[v1.PaneSnapshot], error) {
		return connect.NewResponse(&v1.PaneSnapshot{Grid: []*v1.Row{row(cell("x", 0, 0, 0))}}), nil
	}
	concrete := sess.(*tymuxGRPCSession)

	reconnectsBefore := counterByLabel(t, "tymux_attach_stream_reconnects_total", "cause", "output_gap")
	gapsBefore := counterByLabel(t, "tymux_attach_events_total", "kind", "output_gap")
	redrawsBefore := counterByLabel(t, "tymux_attach_events_total", "kind", "snapshot_redraw")

	concrete.handleAttachEvent(&v1.AttachEvent{Payload: &v1.AttachEvent_OutputGap{OutputGap: true}})

	assert.Equal(t, reconnectsBefore+1, counterByLabel(t, "tymux_attach_stream_reconnects_total", "cause", "output_gap"))
	assert.Equal(t, gapsBefore+1, counterByLabel(t, "tymux_attach_events_total", "kind", "output_gap"))
	assert.Equal(t, redrawsBefore+1, counterByLabel(t, "tymux_attach_events_total", "kind", "snapshot_redraw"),
		"the CapturePane resync that follows an OutputGap redraws the snapshot")
}

func TestGapExceededEvent_RecordsAttachEvent(t *testing.T) {
	sess, _, _ := startedSessionWithStream(t)
	concrete := sess.(*tymuxGRPCSession)
	before := counterByLabel(t, "tymux_attach_events_total", "kind", "gap_exceeded")

	concrete.handleAttachEvent(&v1.AttachEvent{Payload: &v1.AttachEvent_GapExceeded{GapExceeded: &v1.GapExceeded{}}})

	assert.Equal(t, before+1, counterByLabel(t, "tymux_attach_events_total", "kind", "gap_exceeded"))
}

func float64GaugeValue(t *testing.T, name string) (float64, metricdata.Metrics) {
	t.Helper()
	m := collectMetric(t, name)
	require.NotNil(t, m, "%s not exported", name)
	g, ok := m.Data.(metricdata.Gauge[float64])
	require.True(t, ok, "unexpected data type %T for %s", m.Data, name)
	require.Len(t, g.DataPoints, 1, "%s must be a single series (no per-session label)", name)
	require.Equal(t, 0, g.DataPoints[0].Attributes.Len())
	return g.DataPoints[0].Value, *m
}

func TestHeartbeatAgeGauge_ReportsStalestLiveStream_AndDropsClosedOnes(t *testing.T) {
	sess, _, _ := startedSessionWithStream(t)
	concrete := sess.(*tymuxGRPCSession)
	_, live := liveStreams.Load(concrete)
	require.True(t, live, "an open standing stream registers itself")

	// Backdate this stream's last event by 3h: no concurrently running test
	// stream can be older, so the max across streams is this one.
	concrete.lastEventUnixNano.Store(time.Now().Add(-3 * time.Hour).UnixNano())
	age, m := float64GaugeValue(t, "tymux_attach_heartbeat_age_seconds")
	assert.GreaterOrEqual(t, age, (3 * time.Hour).Seconds())
	assert.Less(t, age, (3*time.Hour + time.Minute).Seconds())
	assert.Equal(t, "s", m.Unit)

	require.NoError(t, concrete.Close())
	wait.RequireEventually(t, func() bool {
		_, still := liveStreams.Load(concrete)
		return !still
	}, time.Second, time.Millisecond, "a closed stream must leave the live set")
}

func TestHeartbeatAgeGauge_AnyEventResetsAge(t *testing.T) {
	sess, stream, _ := startedSessionWithStream(t)
	concrete := sess.(*tymuxGRPCSession)
	concrete.lastEventUnixNano.Store(time.Now().Add(-3 * time.Hour).UnixNano())

	stream.push(&v1.AttachEvent{Payload: &v1.AttachEvent_Heartbeat{}})

	wait.RequireEventually(t, func() bool {
		return time.Since(time.Unix(0, concrete.lastEventUnixNano.Load())) < time.Minute
	}, time.Second, time.Millisecond, "a received event must refresh the stream's last-event time")
}

func TestReconnectLoop_RecordsReconnectCounterWithCause(t *testing.T) {
	sess, _, _ := startedSessionWithStream(t)
	concrete := sess.(*tymuxGRPCSession)
	concrete.beginClosing() // ReconnectLoop records, then exits at its first closing check
	before := counterByLabel(t, "tymux_attach_stream_reconnects_total", "cause", "error")

	_, _, _, _ = concrete.ReconnectLoop(context.Background(), "pane-1", "error")

	assert.Equal(t, before+1, counterByLabel(t, "tymux_attach_stream_reconnects_total", "cause", "error"))
}

func TestLiveStreams_LateExitOfOldGeneration_DoesNotUnregisterReplacementStream(t *testing.T) {
	s := &tymuxGRPCSession{}
	oldGen, newGen := make(chan struct{}), make(chan struct{})
	registerLiveStream(s, oldGen)
	registerLiveStream(s, newGen) // reopened before the old reader goroutine exited
	t.Cleanup(func() { unregisterLiveStream(s, newGen) })

	unregisterLiveStream(s, oldGen) // the abandoned reader's deferred exit

	_, live := liveStreams.Load(s)
	require.True(t, live, "the replacement stream must stay registered")
}
