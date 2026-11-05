package tymux

import (
	"context"
	"errors"
	"fmt"
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

// transportReprobeInterval is how long a fallback to TCP sticks before the next call cheaply
// re-probes the Unix socket. The re-probe runs on a caller's own RPC, never on a timer.
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

// ErrTymuxdSocketAccessDenied means the kernel refused connect() on tymuxd's Unix control socket
// (EACCES): a daemon is listening and does not authorize this user. It is a hard error, never a
// reason to fall back to the unauthenticated TCP listener (same rule as upstream udsdialer).
var ErrTymuxdSocketAccessDenied = errors.New("tymux: not authorized to use tymuxd's Unix control socket")

// selectingTransport is the single place that decides which transport reaches tymuxd right now.
// The Unix control socket is primary: tymuxd authenticates its peers (SO_PEERCRED) while the TCP
// listener is deprecated and unauthenticated. A call refused on the socket (ENOENT/ECONNREFUSED)
// falls back to TCP if that answers, sticks there, and re-probes the socket every
// transportReprobeInterval. EACCES never falls back. Existing Attach streams are untouched by a
// switch; only new dials, including ReconnectLoop's redial, go through the choice.
// One instance is shared by every session of a daemon (see sharedTransportFor).
type selectingTransport struct {
	tcp, unix rpcTransport
	closers   []*http.Client
	now       func() time.Time
	addr      string
	quiet     bool // one-shot probe: no metrics, Debug logging

	mu            sync.Mutex
	useTCP        bool
	nextUnixProbe time.Time
}

func newSelectingTransport(tcp, unix rpcTransport, addr string, closers ...*http.Client) *selectingTransport {
	recordTransportActive(transportUnix)
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
	if s.useTCP {
		return transportTCP
	}
	return transportUnix
}

func (s *selectingTransport) closeIdle() {
	for _, c := range s.closers {
		c.CloseIdleConnections()
	}
}

// pick returns the transport for a new call, first re-probing the Unix socket when a TCP fallback
// is due.
func (s *selectingTransport) pick(ctx context.Context) (rpcTransport, string) {
	s.mu.Lock()
	if s.useTCP && !s.now().Before(s.nextUnixProbe) {
		prev := s.nextUnixProbe
		s.nextUnixProbe = s.now().Add(transportReprobeInterval) // claim the probe: other callers skip it
		s.mu.Unlock()
		err := probeTransport(ctx, s.unix)
		switch {
		case err == nil:
			s.switchTo(transportUnix, "control socket answers again")
		case errors.Is(err, syscall.EACCES):
			// A socket now exists and denies us: surface that, do not keep using TCP.
			s.switchTo(transportUnix, "control socket denies access")
		case ctx.Err() != nil:
			s.mu.Lock() // our own cancellation says nothing about the socket: give the probe back
			s.nextUnixProbe = prev
			s.mu.Unlock()
		}
		s.mu.Lock()
	}
	defer s.mu.Unlock()
	if s.useTCP {
		return s.tcp, transportTCP
	}
	return s.unix, transportUnix
}

// probeTransport is bound by the caller's ctx as well as transportProbeTimeout, so a cancelled
// RPC never waits on a probe.
func probeTransport(ctx context.Context, t rpcTransport) error {
	ctx, cancel := context.WithTimeout(ctx, transportProbeTimeout)
	defer cancel()
	_, err := t.GetServerInfo(ctx, connect.NewRequest(&v1.GetServerInfoRequest{}))
	return err
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
	s.useTCP = to == transportTCP
	if s.useTCP {
		s.nextUnixProbe = s.now().Add(transportReprobeInterval)
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

// annotate makes an EACCES from the control socket a clear, matchable hard error.
func annotate(kind string, err error) error {
	if kind == transportUnix && errors.Is(err, syscall.EACCES) && !errors.Is(err, ErrTymuxdSocketAccessDenied) {
		return fmt.Errorf("%w: %w", ErrTymuxdSocketAccessDenied, err)
	}
	return err
}

// failover reacts to a failed call over `from`. It reports whether the active transport changed
// (or already had), so the caller should retry once. Only a refused dial qualifies: EACCES and
// every other error are returned as they are.
func (s *selectingTransport) failover(ctx context.Context, from string, err error) bool {
	if err == nil || !isDialRefused(err) {
		return false
	}
	if cur := s.Active(); cur != from {
		return true // another caller already switched
	}
	switch from {
	case transportUnix:
		if s.tcp == nil || probeTransport(ctx, s.tcp) != nil {
			return false
		}
		s.switchTo(transportTCP, "control socket refused connections while the tcp listener answers")
	default:
		s.switchTo(transportUnix, "tcp listener refused connections")
	}
	return true
}

func invoke[Resp any](ctx context.Context, s *selectingTransport, call func(rpcTransport) (*connect.Response[Resp], error)) (*connect.Response[Resp], error) {
	t, kind := s.pick(ctx)
	resp, err := call(t)
	if err != nil && s.failover(ctx, kind, err) {
		t, kind = s.pick(ctx)
		resp, err = call(t)
	}
	return resp, annotate(kind, err)
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

	mu        sync.Mutex
	rxStarted bool          // a Receive has begun: the stream has a reader, so Send must never Receive
	drained   bool          // the one-time drain already ran
	drainDone chan struct{} // non-nil while a drain's Receive is in flight
	stash     *v1.AttachEvent
}

func (f *failoverStream) observe(err error) error {
	if err != nil {
		f.sel.failover(f.ctx, f.kind, err)
	}
	return annotate(f.kind, err)
}

// Send surfaces the real dial error: connect-go reports only io.EOF from Send when the request
// failed, with the cause on Receive, and a bare EOF would hide a refused dial from the selector.
// It drains at most once and only before any Receive has begun: Receive is not safe to call
// concurrently, and on a live stream the reader goroutine owns it. An event the drain consumes is
// stashed for the next Receive.
func (f *failoverStream) Send(r *v1.AttachRequest) error {
	err := f.inner.Send(r)
	if errors.Is(err, io.EOF) {
		err = f.drainOnce(err)
	}
	return f.observe(err)
}

func (f *failoverStream) drainOnce(eof error) error {
	f.mu.Lock()
	if f.rxStarted || f.drained {
		f.mu.Unlock()
		return eof
	}
	f.drained = true
	done := make(chan struct{})
	f.drainDone = done
	f.mu.Unlock()

	ev, rxErr := f.inner.Receive()

	f.mu.Lock()
	f.drainDone = nil
	if rxErr == nil {
		f.stash = ev
	}
	f.mu.Unlock()
	close(done)
	if rxErr != nil {
		return rxErr
	}
	return eof // the send still failed; the drained event is kept for Receive
}

func (f *failoverStream) Receive() (*v1.AttachEvent, error) {
	f.mu.Lock()
	f.rxStarted = true
	done := f.drainDone
	f.mu.Unlock()
	if done != nil {
		<-done
	}
	f.mu.Lock()
	if ev := f.stash; ev != nil {
		f.stash = nil
		f.mu.Unlock()
		return ev, nil
	}
	f.mu.Unlock()
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
