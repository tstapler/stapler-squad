package tymux

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
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

func TestReconnectLoop_RecordsReconnectCounterWithCause(t *testing.T) {
	sess, _, _ := startedSessionWithStream(t)
	concrete := sess.(*tymuxGRPCSession)
	concrete.beginClosing() // ReconnectLoop records, then exits at its first closing check
	before := counterByLabel(t, "tymux_attach_stream_reconnects_total", "cause", "error")

	_, _, _, _ = concrete.ReconnectLoop(context.Background(), "pane-1", "error")

	assert.Equal(t, before+1, counterByLabel(t, "tymux_attach_stream_reconnects_total", "cause", "error"))
}
