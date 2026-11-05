package tymux

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"syscall"
	"time"

	"connectrpc.com/connect"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
	"github.com/tstapler/tymux/clients/go/udsdialer"

	"github.com/tstapler/stapler-squad/log"
)

const (
	transportTCP  = "tcp"
	transportUnix = "unix"
)

// transportReprobeInterval is how long a fallback to the Unix socket sticks before the next call
// cheaply re-probes TCP. The re-probe runs on a caller's own RPC, never on a timer.
const transportReprobeInterval = 30 * time.Second

// transportProbeTimeout bounds each GetServerInfo probe the selector makes.
var transportProbeTimeout = 3 * time.Second

var (
	transportActiveGauge = mustInt64Gauge("tymux_transport",
		metric.WithDescription("1 for the transport (tcp|unix) squad's tymux client currently uses to reach tymuxd, 0 for the other"))
	transportTransitionCounter = mustInt64Counter("tymux_transport_transitions_total",
		metric.WithDescription("Switches of squad's tymux client between the TCP and Unix-socket transports, by from/to"))
)

func recordTransportActive(active string) {
	for _, kind := range []string{transportTCP, transportUnix} {
		v := int64(0)
		if kind == active {
			v = 1
		}
		transportActiveGauge.Record(context.Background(), v, metric.WithAttributes(attribute.String("transport", kind)))
	}
}

// selectingTransport is the single place that decides which transport reaches tymuxd right now.
// It starts on TCP. When a call is refused (nothing listening on TCP) and tymuxd's Unix control
// socket answers, it sticks to the socket and re-probes TCP every transportReprobeInterval.
// Existing Attach streams are untouched by a switch (a dead listener does not close connections
// it already accepted); only new dials, including ReconnectLoop's redial, go through the choice.
type selectingTransport struct {
	tcp, unix rpcTransport
	closers   []*http.Client
	now       func() time.Time
	addr      string
	quiet     bool // one-shot probe: no metrics, Debug logging

	mu           sync.Mutex
	useUnix      bool
	nextTCPProbe time.Time
}

func newSelectingTransport(tcp, unix rpcTransport, addr string, closers ...*http.Client) *selectingTransport {
	recordTransportActive(transportTCP)
	return &selectingTransport{tcp: tcp, unix: unix, closers: closers, now: time.Now, addr: addr}
}

// newQuietSelectingTransport is for one-shot probes: it falls back the same way but leaves the
// process-global tymux_transport gauge, transition counter and Warn log to the session transport.
func newQuietSelectingTransport(tcp, unix rpcTransport, addr string, closers ...*http.Client) *selectingTransport {
	s := &selectingTransport{tcp: tcp, unix: unix, closers: closers, now: time.Now, addr: addr}
	s.quiet = true
	return s
}

// Active reports the transport new calls use.
func (s *selectingTransport) Active() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activeLocked()
}

func (s *selectingTransport) activeLocked() string {
	if s.useUnix {
		return transportUnix
	}
	return transportTCP
}

func (s *selectingTransport) closeIdle() {
	for _, c := range s.closers {
		c.CloseIdleConnections()
	}
}

// pick returns the transport for a new call, first re-probing TCP when a Unix fallback is due.
func (s *selectingTransport) pick(ctx context.Context) (rpcTransport, string) {
	s.mu.Lock()
	if s.useUnix && !s.now().Before(s.nextTCPProbe) {
		s.nextTCPProbe = s.now().Add(transportReprobeInterval) // claim the probe: other callers skip it
		s.mu.Unlock()
		if probeTransport(ctx, s.tcp) {
			s.switchTo(transportTCP, "tcp listener answers again")
		}
		s.mu.Lock()
	}
	defer s.mu.Unlock()
	if s.useUnix {
		return s.unix, transportUnix
	}
	return s.tcp, transportTCP
}

// probeTransport is bound by the caller's ctx as well as transportProbeTimeout, so a cancelled
// RPC never waits on a probe.
func probeTransport(ctx context.Context, t rpcTransport) bool {
	ctx, cancel := context.WithTimeout(ctx, transportProbeTimeout)
	defer cancel()
	_, err := t.GetServerInfo(ctx, connect.NewRequest(&v1.GetServerInfoRequest{}))
	return err == nil
}

// switchReason is the human-readable cause logged with a transport change.
type switchReason string

func (s *selectingTransport) switchTo(to string, why switchReason) {
	s.mu.Lock()
	from := s.activeLocked()
	if from == to {
		s.mu.Unlock()
		return
	}
	s.useUnix = to == transportUnix
	if s.useUnix {
		s.nextTCPProbe = s.now().Add(transportReprobeInterval)
	}
	s.mu.Unlock()
	if s.quiet {
		log.Debug("[tymux] probe transport changed", "from", from, "to", to, "addr", s.addr)
		return
	}
	recordTransportActive(to)
	transportTransitionCounter.Add(context.Background(), 1,
		metric.WithAttributes(attribute.String("from", from), attribute.String("to", to)))
	log.Warn("[tymux] tymuxd transport changed", "from", from, "to", to, "why", string(why), "addr", s.addr)
}

// isDialRefused reports whether err proves the endpoint has no listener.
func isDialRefused(err error) bool {
	return errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ENOENT)
}

// failover reacts to a failed call over `from`. It reports whether the active transport changed
// (or already had), so the caller should retry once.
func (s *selectingTransport) failover(ctx context.Context, from string, err error) bool {
	if err == nil || !isDialRefused(err) {
		return false
	}
	if cur := s.Active(); cur != from {
		return true // another caller already switched
	}
	switch from {
	case transportTCP:
		if s.unix == nil || !probeTransport(ctx, s.unix) {
			return false
		}
		s.switchTo(transportUnix, "tcp listener refused connections while the control socket answers")
	default:
		s.switchTo(transportTCP, "control socket refused connections")
	}
	return true
}

func invoke[Resp any](ctx context.Context, s *selectingTransport, call func(rpcTransport) (*connect.Response[Resp], error)) (*connect.Response[Resp], error) {
	t, kind := s.pick(ctx)
	resp, err := call(t)
	if err != nil && s.failover(ctx, kind, err) {
		t, _ = s.pick(ctx)
		return call(t)
	}
	return resp, err
}

func (s *selectingTransport) CreateSession(ctx context.Context, r *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
	return invoke(ctx, s, func(t rpcTransport) (*connect.Response[v1.Session], error) { return t.CreateSession(ctx, r) })
}

func (s *selectingTransport) KillSession(ctx context.Context, r *connect.Request[v1.KillSessionRequest]) (*connect.Response[v1.KillSessionResponse], error) {
	return invoke(ctx, s, func(t rpcTransport) (*connect.Response[v1.KillSessionResponse], error) { return t.KillSession(ctx, r) })
}

func (s *selectingTransport) ListSessions(ctx context.Context, r *connect.Request[v1.ListSessionsRequest]) (*connect.Response[v1.ListSessionsResponse], error) {
	return invoke(ctx, s, func(t rpcTransport) (*connect.Response[v1.ListSessionsResponse], error) {
		return t.ListSessions(ctx, r)
	})
}

func (s *selectingTransport) ReviveSession(ctx context.Context, r *connect.Request[v1.ReviveSessionRequest]) (*connect.Response[v1.ReviveSessionResponse], error) {
	return invoke(ctx, s, func(t rpcTransport) (*connect.Response[v1.ReviveSessionResponse], error) {
		return t.ReviveSession(ctx, r)
	})
}

func (s *selectingTransport) CapturePane(ctx context.Context, r *connect.Request[v1.CapturePaneRequest]) (*connect.Response[v1.PaneSnapshot], error) {
	return invoke(ctx, s, func(t rpcTransport) (*connect.Response[v1.PaneSnapshot], error) { return t.CapturePane(ctx, r) })
}

func (s *selectingTransport) GetServerInfo(ctx context.Context, r *connect.Request[v1.GetServerInfoRequest]) (*connect.Response[v1.ServerInfo], error) {
	return invoke(ctx, s, func(t rpcTransport) (*connect.Response[v1.ServerInfo], error) { return t.GetServerInfo(ctx, r) })
}

func (s *selectingTransport) GetPaneStatus(ctx context.Context, r *connect.Request[v1.GetPaneStatusRequest]) (*connect.Response[v1.PaneStatus], error) {
	return invoke(ctx, s, func(t rpcTransport) (*connect.Response[v1.PaneStatus], error) { return t.GetPaneStatus(ctx, r) })
}

// Attach opens the stream on the current transport. The dial happens lazily on the first
// Send/Receive, so the stream reports a refused dial back to the selector, and the next Attach
// (ReconnectLoop's redial) lands on the fallback.
func (s *selectingTransport) Attach(ctx context.Context) attachStream {
	t, kind := s.pick(ctx)
	return &failoverStream{inner: t.Attach(ctx), sel: s, kind: kind, ctx: ctx}
}

type failoverStream struct {
	inner attachStream
	sel   *selectingTransport
	kind  string
	ctx   context.Context //nolint:containedctx // the Attach ctx bounds the failover probe
}

func (f *failoverStream) observe(err error) error {
	if err != nil {
		f.sel.failover(f.ctx, f.kind, err)
	}
	return err
}

// Send surfaces the real dial error: connect-go reports only io.EOF from Send when the request
// failed, with the cause on Receive, and a bare EOF would hide a refused dial from the selector.
// The failover probe runs on the stream's own goroutine but is bounded by the Attach ctx and
// transportProbeTimeout.
func (f *failoverStream) Send(r *v1.AttachRequest) error {
	err := f.inner.Send(r)
	if errors.Is(err, io.EOF) {
		if _, rxErr := f.inner.Receive(); rxErr != nil {
			err = rxErr
		}
	}
	return f.observe(err)
}

func (f *failoverStream) Receive() (*v1.AttachEvent, error) {
	ev, err := f.inner.Receive()
	return ev, f.observe(err)
}

func (f *failoverStream) CloseRequest() error  { return f.inner.CloseRequest() }
func (f *failoverStream) CloseResponse() error { return f.inner.CloseResponse() }

var _ rpcTransport = (*selectingTransport)(nil)

// socketPathForAddr is the Unix control socket that belongs to the daemon at addr, or "" when it
// cannot be known. It only ever returns the socket of the daemon this process itself resolves
// (ResolveDaemonConfig: STAPLER_SQUAD_INSTANCE-scoped) or the shared default daemon's; an addr
// that is neither has no socket, so another instance's daemon is never used.
func socketPathForAddr(addr string) string {
	if rc := ResolveDaemonConfig(); rc.Addr == addr {
		return unixSocketPath(rc)
	}
	return unixSocketPath(DaemonConfig{Addr: addr})
}

func newUnixTransport(socketPath string) (rpcTransport, *http.Client) {
	hc := udsdialer.DialUnixHTTPClient(socketPath)
	return &grpcTransport{client: newTymuxClient(hc, "http://unix"), http: hc}, hc
}
