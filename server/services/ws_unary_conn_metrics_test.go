package services

import (
	"sync/atomic"
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

func TestUnaryWsBridge_should_IgnoreForgedPongPayload(t *testing.T) {
	before := histogramCount(t, "ws_unary_ping_rtt")
	h := newBridgeHarness(t, ListenerLocal, WithUnaryKeepalive(time.Hour, time.Hour)) // the server never pings
	c := h.connect(nil)
	for _, payload := range []string{"1", "-5", "99999999999999999999", "not-a-number"} {
		require.NoError(t, c.WriteControl(websocket.PongMessage, []byte(payload), time.Now().Add(time.Second)))
	}
	sendListFiles(t, c, 1, "after-forgery") // a round trip proves the forged pongs were already processed
	require.Equal(t, "ok:after-forgery", decodeListFiles(t, readFrame(t, c)).BasePath)
	assert.Equal(t, before, histogramCount(t, "ws_unary_ping_rtt"), "a pong that matches no outstanding ping records nothing")
}

func TestUnaryWsBridge_should_LabelProtocolClose(t *testing.T) {
	before := sumWithReason(t, "ws_unary_connections_closed_total", "protocol")
	h := newBridgeHarness(t, ListenerLocal)
	c := h.connect(nil)
	require.NoError(t, c.WriteMessage(websocket.TextMessage, []byte("hi")))
	require.NoError(t, c.SetReadDeadline(time.Now().Add(readWait)))
	_, _, err := c.ReadMessage()
	require.True(t, websocket.IsCloseError(err, websocket.CloseUnsupportedData), "got %v", err)
	assert.Eventually(t, func() bool {
		return sumWithReason(t, "ws_unary_connections_closed_total", "protocol") == before+1
	}, readWait, 5*time.Millisecond)
}

func TestUnaryWsBridge_should_LabelReadErrorClose(t *testing.T) {
	before := sumWithReason(t, "ws_unary_connections_closed_total", "read_error")
	h := newBridgeHarness(t, ListenerLocal)
	c := h.connect(nil)
	_ = c.UnderlyingConn().Close() // abrupt TCP close: no close frame
	assert.Eventually(t, func() bool {
		return sumWithReason(t, "ws_unary_connections_closed_total", "read_error") == before+1
	}, readWait, 5*time.Millisecond)
}

func TestUnaryWsBridge_should_LabelPongTimeoutAsReadError(t *testing.T) {
	before := sumWithReason(t, "ws_unary_connections_closed_total", "read_error")
	h := newBridgeHarness(t, ListenerLocal, WithUnaryKeepalive(5*time.Millisecond, 40*time.Millisecond))
	c := h.connect(nil)
	c.SetPingHandler(func(string) error { return nil }) // never answer pings
	go func() {
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				return
			}
		}
	}()
	assert.Eventually(t, func() bool {
		return sumWithReason(t, "ws_unary_connections_closed_total", "read_error") == before+1
	}, readWait, 5*time.Millisecond)
}

func TestUnaryWsBridge_should_LabelServerContextEndAsShutdown(t *testing.T) {
	before := sumWithReason(t, "ws_unary_connections_closed_total", "shutdown")
	h := newBridgeHarness(t, ListenerLocal)
	_ = h.connect(nil)
	h.cancel()
	assert.Eventually(t, func() bool {
		return sumWithReason(t, "ws_unary_connections_closed_total", "shutdown") == before+1
	}, readWait, 5*time.Millisecond)
}

func TestUnaryWsBridge_should_RecordEachPingOnlyOnce_When_PongIsReplayed(t *testing.T) {
	before := histogramCount(t, "ws_unary_ping_rtt")
	h := newBridgeHarness(t, ListenerLocal, WithUnaryKeepalive(100*time.Millisecond, time.Hour))
	c := h.connect(nil)
	var answered atomic.Bool
	c.SetPingHandler(func(payload string) error {
		if answered.CompareAndSwap(false, true) { // answer the first ping twice, ignore the rest
			for i := 0; i < 2; i++ {
				if err := c.WriteControl(websocket.PongMessage, []byte(payload), time.Now().Add(time.Second)); err != nil {
					return err
				}
			}
		}
		return nil
	})
	go func() {
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				return
			}
		}
	}()
	require.Eventually(t, func() bool { return histogramCount(t, "ws_unary_ping_rtt") > before }, readWait, 5*time.Millisecond)
	assert.Never(t, func() bool { return histogramCount(t, "ws_unary_ping_rtt") > before+1 }, 300*time.Millisecond, 20*time.Millisecond,
		"the replayed pong must not record a second sample")
}
