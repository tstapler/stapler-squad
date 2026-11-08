package tymux

import (
	"context"
	"net/http"
	"os"
	"sync"

	"connectrpc.com/connect"
	"connectrpc.com/otelconnect"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
	"github.com/tstapler/tymux/clients/go/gen/tymux/v1/tymuxv1connect"
	"github.com/tstapler/tymux/clients/go/selecting"

	"github.com/tstapler/stapler-squad/log"
)

// rpcTransport is the narrow seam between tymuxGRPCSession and the generated
// Connect-Go client (tymuxv1connect.TymuxServiceClient). It exposes only the
// methods tymuxGRPCSession actually calls, rather than the full generated
// client, so tests can substitute a hand-driven fake and unit-test
// Start/Close/IsAlive/exit-callback-ordering without a live tymuxd daemon.
//
// tymuxGRPCSession is architecturally the only real TymuxManager implementation
// (BackendTymux always wraps one), so this interface — not TymuxManager — is the
// actual testability seam for the RPC-calling logic; see plan.md Task 2.1.2d.
type rpcTransport interface {
	CreateSession(context.Context, *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error)
	KillSession(context.Context, *connect.Request[v1.KillSessionRequest]) (*connect.Response[v1.KillSessionResponse], error)
	ListSessions(context.Context, *connect.Request[v1.ListSessionsRequest]) (*connect.Response[v1.ListSessionsResponse], error)
	ReviveSession(context.Context, *connect.Request[v1.ReviveSessionRequest]) (*connect.Response[v1.ReviveSessionResponse], error)
	CapturePane(context.Context, *connect.Request[v1.CapturePaneRequest]) (*connect.Response[v1.PaneSnapshot], error)
	GetServerInfo(context.Context, *connect.Request[v1.GetServerInfoRequest]) (*connect.Response[v1.ServerInfo], error)
	GetPaneStatus(context.Context, *connect.Request[v1.GetPaneStatusRequest]) (*connect.Response[v1.PaneStatus], error)
	// Attach opens the standing bidi stream. The returned attachStream exposes
	// only Send/Receive/CloseRequest/CloseResponse — the subset of
	// *connect.BidiStreamForClient[v1.AttachRequest, v1.AttachEvent] that
	// tymuxGRPCSession's Attach/ReconnectLoop logic (Epic 2.3/2.5) actually uses.
	Attach(context.Context) attachStream
}

// attachStream is the narrow surface of the generated bidi stream that tymuxGRPCSession uses;
// it is upstream's so *selecting.Client satisfies rpcTransport directly.
type attachStream = selecting.AttachStream

// defaultTymuxdAddr matches clients/go/examples/list-sessions/main.go's
// default and tymuxd's documented loopback-only listen address/port.
const defaultTymuxdAddr = "http://127.0.0.1:7419"

// tymuxdAddr resolves the base URL for a real tymuxd connection: the
// TYMUXD_ADDR environment variable when set, else defaultTymuxdAddr.
// Story 2.2.6's "stapler-squad does not start or supervise tymuxd itself"
// scope decision no longer holds — see
// project_plans/tymux-bundled-integration/decisions/ADR-003-supersede-story-2-2-6-no-supervision.md.
// stapler-squad now supervises tymuxd's lifecycle (start-if-not-running,
// health-check) whenever the tymux backend is actually in use. This
// resolution is still supervision-aware: TYMUXD_ADDR remains the override,
// matching DaemonConfig.Addr's resolution in
// session/tymux/daemon_config.go's ResolveDaemonConfig, which is the choke
// point supervision call sites actually use.
func tymuxdAddr() string {
	if v := os.Getenv("TYMUXD_ADDR"); v != "" {
		return v
	}
	return defaultTymuxdAddr
}

// NewRealTransport constructs an rpcTransport backed by a real Connect-Go
// gRPC client against a live tymuxd. addr is the tymuxd base URL (e.g.
// "http://127.0.0.1:7419"); pass "" to use tymuxdAddr()'s
// TYMUXD_ADDR-or-default resolution. When the daemon at addr has a known Unix
// control socket (socketPathForAddr), the result prefers that socket and falls
// back to TCP only while the socket is absent or refusing; the selector is
// shared by every caller for the same daemon. See selecting.Client.
func NewRealTransport(addr string) rpcTransport {
	t, _ := newRealTransport(addr, false)
	return t
}

// newProbeTransport is NewRealTransport for one-shot calls: it is quiet (the session transport
// owns the process-global metrics) and the returned release func must be deferred, or each probe
// leaks a connection to tymuxd.
func newProbeTransport(addr string) (rpcTransport, func()) {
	return newRealTransport(addr, true)
}

func newRealTransport(addr string, quiet bool) (rpcTransport, func()) {
	if addr == "" {
		addr = tymuxdAddr()
	}
	if sharedDaemonBlockedInTest(addr) {
		addr = deadLoopbackAddr
	}
	sock := socketPathForAddr(addr)
	if sock == "" {
		t, hc := newTCPTransport(addr)
		return t, hc.CloseIdleConnections
	}
	if quiet {
		return newSelector(addr, sock, true)
	}
	key := addr + "|" + sock
	control := sharedTransportFor(key, func() *selecting.Client {
		s, _ := newSelector(addr, sock, false)
		recordTransportActive(addr, selecting.Unix)
		return s
	})
	// Quiet: the control selector already owns the transition metrics and Warn lines.
	stream := sharedTransportFor(key+"|attach", func() *selecting.Client {
		s, _ := newSelector(addr, sock, true)
		return s
	})
	// The wrapper is thin and its selectors are shared, so a lost LoadOrStore race is harmless.
	split, _ := sharedSplits.LoadOrStore(key, &splitTransport{Client: control, stream: stream})
	return split.(*splitTransport), func() {}
}

// sharedSplits memoizes one splitTransport per daemon so every session gets the same value.
var sharedSplits sync.Map

// splitTransport sends the standing Attach streams over their own connection, so a pane streaming
// a burst cannot delay CapturePane/resize/lifecycle RPCs: over one shared HTTP/2 connection they
// queue behind the streams' frames and flow-control window.
type splitTransport struct {
	*selecting.Client // control RPCs
	stream            *selecting.Client
}

func (s *splitTransport) Attach(ctx context.Context) attachStream { return s.stream.Attach(ctx) }

// sharedSelectors holds one long-lived selector per daemon (addr+socket) so every session's
// transport shares one fallback state, one probe claim and one Warn per genuine transition,
// instead of N sessions each rediscovering and logging the same switch.
var (
	sharedSelectorsMu sync.Mutex
	sharedSelectors   = map[string]*selecting.Client{}
)

// sharedTransportFor returns the selector for key, building it under the lock so concurrent first
// callers neither build (and leak http clients for) a loser nor race its side effects.
func sharedTransportFor(key string, mk func() *selecting.Client) *selecting.Client {
	sharedSelectorsMu.Lock()
	defer sharedSelectorsMu.Unlock()
	if s, ok := sharedSelectors[key]; ok {
		return s
	}
	s := mk()
	sharedSelectors[key] = s
	return s
}

// newTymuxClient builds the generated client with gRPC framing (tymuxd is a
// strict tonic server) and, so a stapler-squad trace continues into tymuxd
// as a W3C traceparent header, an otelconnect client interceptor. Traces
// only: otelconnect's own metrics and per-message span events are off, since
// the Attach stream would emit one event per output chunk.
func newTymuxClient(httpClient *http.Client, addr string) tymuxv1connect.TymuxServiceClient {
	opts := []connect.ClientOption{connect.WithGRPC()}
	interceptor, err := otelconnect.NewInterceptor(otelconnect.WithoutMetrics(), otelconnect.WithoutTraceEvents())
	if err != nil {
		log.Warn("tymux: otelconnect client interceptor unavailable, RPCs will not propagate trace context", "err", err)
	} else {
		opts = append(opts, connect.WithInterceptors(interceptor))
	}
	return tymuxv1connect.NewTymuxServiceClient(httpClient, addr, opts...)
}
