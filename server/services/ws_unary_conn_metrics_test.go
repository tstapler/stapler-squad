package services

import (
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func histogramCount(t *testing.T, name string) uint64 {
	t.Helper()
	m := collectMetric(t, name)
	if m == nil {
		return 0
	}
	hist, ok := m.Data.(metricdata.Histogram[float64])
	require.True(t, ok, "unexpected data type %T for %s", m.Data, name)
	var n uint64
	for _, dp := range hist.DataPoints {
		n += dp.Count
	}
	return n
}

func sumWithReason(t *testing.T, name, reason string) int64 {
	t.Helper()
	m := collectMetric(t, name)
	if m == nil {
		return 0
	}
	sum, ok := m.Data.(metricdata.Sum[int64])
	require.True(t, ok, "unexpected data type %T for %s", m.Data, name)
	var n int64
	for _, dp := range sum.DataPoints {
		if v, ok := dp.Attributes.Value("reason"); ok && v.AsString() == reason {
			n += dp.Value
		}
	}
	return n
}

func sumAll(t *testing.T, name string) int64 {
	t.Helper()
	m := collectMetric(t, name)
	if m == nil {
		return 0
	}
	sum, ok := m.Data.(metricdata.Sum[int64])
	require.True(t, ok, "unexpected data type %T for %s", m.Data, name)
	var n int64
	for _, dp := range sum.DataPoints {
		n += dp.Value
	}
	return n
}

func TestUnaryWsBridge_should_RecordCallDuration(t *testing.T) {
	before := histogramCount(t, "ws_unary_call_duration")
	h := newBridgeHarness(t, ListenerLocal)
	c := h.connect(nil)
	sendListFiles(t, c, 1, "timed")
	require.Equal(t, "ok:timed", decodeListFiles(t, readFrame(t, c)).BasePath)
	assert.Eventually(t, func() bool { return histogramCount(t, "ws_unary_call_duration") > before }, readWait, 5*time.Millisecond)
}

func TestUnaryWsBridge_should_CountConnectionOpenAndPeerClose(t *testing.T) {
	closedBefore := sumWithReason(t, "ws_unary_connections_closed_total", "peer_close")
	openBefore := sumAll(t, "ws_unary_connections_open")
	h := newBridgeHarness(t, ListenerLocal)
	c := h.connect(nil)
	assert.Eventually(t, func() bool { return sumAll(t, "ws_unary_connections_open") == openBefore+1 }, readWait, 5*time.Millisecond)
	require.NoError(t, c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bye")))
	_ = c.Close()
	assert.Eventually(t, func() bool {
		return sumWithReason(t, "ws_unary_connections_closed_total", "peer_close") == closedBefore+1 &&
			sumAll(t, "ws_unary_connections_open") == openBefore
	}, readWait, 5*time.Millisecond)
}

func TestUnaryWsBridge_should_RecordPingRTT(t *testing.T) {
	before := histogramCount(t, "ws_unary_ping_rtt")
	h := newBridgeHarness(t, ListenerLocal, WithUnaryKeepalive(5*time.Millisecond, time.Second))
	c := h.connect(nil)
	go func() { // the client's default ping handler answers with a pong while it reads
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				return
			}
		}
	}()
	assert.Eventually(t, func() bool { return histogramCount(t, "ws_unary_ping_rtt") > before }, readWait, 5*time.Millisecond)
}
