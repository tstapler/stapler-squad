package server

// http_connection_metrics.go instruments the main HTTP listener's raw
// connection count via (*http.Server).ConnState, wired in newServerBase
// (server.go). This exists alongside watch_stream_metrics.go's per-RPC
// server-streaming gauge to catch the class of bug root-caused in 2026-09:
// this server speaks plain HTTP/1.1, so a browser's 6-connections-per-origin
// cap can be exhausted by long-lived Watch*/terminal streams, silently
// queuing unrelated unary RPCs client-side until they blow their deadline —
// with zero server-side trace evidence, since the request never arrived.
// http.server.connections_open answers "how close is the main listener to
// that ceiling" directly. See docs/how-to/enable-opentelemetry.md's
// "Connection concurrency metrics" section.
//
// Follows the same package-level-atomics-plus-Observable-instrument-callback
// pattern as telemetry/cgroup_linux.go and session/streamhub/observability.go:
// registered once via sync.Once/init() against telemetry.GetMeter().

import (
	"context"
	"net"
	"net/http"
	"sync"
	"sync/atomic"

	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/telemetry"
)

// httpConnOpenCount is the current count of connections on the main listener
// that net/http is still tracking as plain HTTP/1.1 (new/active/idle) — it
// excludes any connection that has been hijacked (StateHijacked), since Go
// stops delivering ConnState transitions for those. In this server, the only
// code that hijacks a connection off the main listener is
// server/services/connectrpc_websocket.go's gorilla/websocket upgrade path
// (the terminal stream, and any Watch* RPC relayed through
// server/services/ws_stream_bridge.go's StreamingWSBridge), so
// httpConnHijackedTotal below is in practice a WebSocket-upgrade counter.
var (
	httpConnOpenCount     atomic.Int64
	httpConnHijackedTotal atomic.Int64
)

var (
	httpConnMetricsRegisterOnce sync.Once
	httpConnMetricsRegisterErr  error
)

func init() {
	httpConnMetricsRegisterOnce.Do(func() {
		httpConnMetricsRegisterErr = registerHTTPConnectionMetrics()
		if httpConnMetricsRegisterErr != nil {
			log.Error("server: failed to register HTTP connection OTel metrics", "error", httpConnMetricsRegisterErr)
		}
	})
}

func registerHTTPConnectionMetrics() error {
	meter := telemetry.GetMeter()

	openGauge, err := meter.Int64ObservableGauge("http.server.connections_open",
		metric.WithDescription("Current count of connections on the main HTTP listener still tracked by net/http as plain HTTP/1.1 keep-alive (new/active/idle) — excludes connections hijacked away (see http.server.connections_hijacked_total), e.g. for a WebSocket upgrade"))
	if err != nil {
		return err
	}

	hijackedCounter, err := meter.Int64ObservableCounter("http.server.connections_hijacked_total",
		metric.WithDescription("Cumulative count of connections hijacked off the main HTTP listener's net/http tracking — in this server, exclusively WebSocket upgrades (terminal stream + Watch* stream bridge)"))
	if err != nil {
		return err
	}

	_, err = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		o.ObserveInt64(openGauge, httpConnOpenCount.Load())
		o.ObserveInt64(hijackedCounter, httpConnHijackedTotal.Load())
		return nil
	}, openGauge, hijackedCounter)
	return err
}

// trackHTTPConnState is installed as the main listener's
// (*http.Server).ConnState hook in newServerBase. StateNew/StateClosed
// bracket every connection's lifetime in net/http's own bookkeeping;
// StateHijacked is the terminal state for a connection net/http hands off
// (no further ConnState calls follow), so it decrements the open count and
// increments the hijacked total in the same step.
func trackHTTPConnState(_ net.Conn, state http.ConnState) {
	switch state {
	case http.StateNew:
		httpConnOpenCount.Add(1)
	case http.StateHijacked:
		httpConnOpenCount.Add(-1)
		httpConnHijackedTotal.Add(1)
	case http.StateClosed:
		httpConnOpenCount.Add(-1)
	}
}
