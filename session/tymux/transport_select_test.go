package tymux

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
	"github.com/tstapler/tymux/clients/go/gen/tymux/v1/tymuxv1connect"
	"github.com/tstapler/tymux/clients/go/selecting"
)

// fakeEndpoint is a hand-driven rpcTransport standing for one listener (TCP or Unix).
type fakeEndpoint struct {
	selecting.Transport
	err     atomic.Pointer[error]
	calls   atomic.Int32
	streams atomic.Int32
	// lastInfoTimeout is the deadline budget (ns) of the latest GetServerInfo call, 0 if unbounded.
	lastInfoTimeout atomic.Int64
}

func (f *fakeEndpoint) fail(err error) { f.err.Store(&err) }
func (f *fakeEndpoint) heal()          { f.err.Store(nil) }

func (f *fakeEndpoint) result() error {
	f.calls.Add(1)
	if e := f.err.Load(); e != nil {
		return *e
	}
	return nil
}

func (f *fakeEndpoint) ListSessions(context.Context, *connect.Request[v1.ListSessionsRequest]) (*connect.Response[v1.ListSessionsResponse], error) {
	if err := f.result(); err != nil {
		return nil, err
	}
	return connect.NewResponse(&v1.ListSessionsResponse{}), nil
}

func (f *fakeEndpoint) GetServerInfo(ctx context.Context, _ *connect.Request[v1.GetServerInfoRequest]) (*connect.Response[v1.ServerInfo], error) {
	if dl, ok := ctx.Deadline(); ok {
		f.lastInfoTimeout.Store(int64(time.Until(dl)))
	}
	if err := f.result(); err != nil {
		return nil, err
	}
	return connect.NewResponse(&v1.ServerInfo{}), nil
}

func (f *fakeEndpoint) Attach(context.Context) attachStream {
	f.streams.Add(1)
	return &fakeStream{ep: f}
}

type fakeStream struct{ ep *fakeEndpoint }

func (s *fakeStream) Send(*v1.AttachRequest) error { return nil }
func (s *fakeStream) Receive() (*v1.AttachEvent, error) {
	if e := s.ep.err.Load(); e != nil {
		return nil, *e
	}
	return &v1.AttachEvent{}, nil
}
func (s *fakeStream) CloseRequest() error  { return nil }
func (s *fakeStream) CloseResponse() error { return nil }

func refused() error {
	return connect.NewError(connect.CodeUnavailable, &net.OpError{Op: "dial", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)})
}

type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *testClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

const reprobeInterval = 30 * time.Second // upstream's default

func newTestSelector() (*selecting.Client, *fakeEndpoint, *fakeEndpoint, *testClock) {
	tcp, unix := &fakeEndpoint{}, &fakeEndpoint{}
	clock := &testClock{t: time.Unix(1_000_000, 0)}
	return selecting.NewFromTransports(unix, tcp, selecting.Options{Now: clock.now}), tcp, unix, clock
}

func list(t *testing.T, s *selecting.Client) error {
	t.Helper()
	_, err := s.ListSessions(context.Background(), connect.NewRequest(&v1.ListSessionsRequest{}))
	return err
}

func syscallDialErr(errno syscall.Errno) error {
	return connect.NewError(connect.CodeUnavailable, &net.OpError{Op: "dial", Err: os.NewSyscallError("connect", errno)})
}

func enoent() error { return syscallDialErr(syscall.ENOENT) }
func eacces() error { return syscallDialErr(syscall.EACCES) }

func TestSelectingTransport_UnixHealthy_UsesUnixAndNeverTouchesTCP(t *testing.T) {
	t.Parallel()
	s, tcp, unix, _ := newTestSelector()

	require.NoError(t, list(t, s))
	require.Equal(t, selecting.Unix, s.Active())
	require.Positive(t, unix.calls.Load())
	require.Zero(t, tcp.calls.Load(), "the deprecated unauthenticated listener is not touched while the socket works")
}

func TestSelectingTransport_UnixAbsentOrRefusing_FallsBackToTCP(t *testing.T) {
	t.Parallel()
	for name, mk := range map[string]func() error{"ENOENT": enoent, "ECONNREFUSED": refused} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s, tcp, unix, _ := newTestSelector()
			unix.fail(mk())

			require.NoError(t, list(t, s))
			require.Equal(t, selecting.TCP, s.Active())

			unixCalls := unix.calls.Load()
			require.NoError(t, list(t, s))
			require.Equal(t, unixCalls, unix.calls.Load(), "sticky: no socket attempt while the fallback is fresh")
			require.Positive(t, tcp.calls.Load())
		})
	}
}

func TestSelectingTransport_UnixEACCES_IsHardErrorAndNeverFallsBack(t *testing.T) {
	t.Parallel()
	s, tcp, unix, _ := newTestSelector()
	unix.fail(eacces())

	err := list(t, s)

	require.ErrorIs(t, err, ErrTymuxdSocketAccessDenied)
	require.ErrorIs(t, err, syscall.EACCES, "the cause stays matchable")
	require.Zero(t, tcp.calls.Load(), "never fall back to the unauthenticated listener on EACCES")
	require.Equal(t, selecting.Unix, s.Active())
}

func TestSelectingTransport_UnixEACCES_StreamSurfacesHardError(t *testing.T) {
	t.Parallel()
	s, tcp, unix, _ := newTestSelector()
	unix.fail(eacces())

	_, err := s.Attach(context.Background()).Receive()

	require.ErrorIs(t, err, ErrTymuxdSocketAccessDenied)
	require.Zero(t, tcp.streams.Load()+tcp.calls.Load())
	require.Equal(t, selecting.Unix, s.Active())
}

func TestSelectingTransport_UnixRecovers_ReturnsAfterReprobeInterval(t *testing.T) {
	t.Parallel()
	s, _, unix, clock := newTestSelector()
	unix.fail(enoent())
	require.NoError(t, list(t, s))
	require.Equal(t, selecting.TCP, s.Active())

	unix.heal()
	clock.advance(reprobeInterval - time.Second)
	require.NoError(t, list(t, s))
	require.Equal(t, selecting.TCP, s.Active(), "not yet due")

	clock.advance(2 * time.Second)
	require.NoError(t, list(t, s))
	require.Equal(t, selecting.Unix, s.Active())
}

func TestSelectingTransport_ReprobeStillDown_StaysOnTCPAndWaitsAnotherInterval(t *testing.T) {
	t.Parallel()
	s, _, unix, clock := newTestSelector()
	unix.fail(enoent())
	require.NoError(t, list(t, s))

	clock.advance(reprobeInterval)
	before := unix.calls.Load()
	require.NoError(t, list(t, s))
	require.Equal(t, before+1, unix.calls.Load(), "exactly one cheap probe")
	require.Equal(t, selecting.TCP, s.Active())

	require.NoError(t, list(t, s))
	require.Equal(t, before+1, unix.calls.Load(), "no hot loop: next probe is a full interval away")
}

func TestSelectingTransport_ReprobeSeesEACCES_SurfacesHardErrorInsteadOfStayingOnTCP(t *testing.T) {
	t.Parallel()
	s, tcp, unix, clock := newTestSelector()
	unix.fail(enoent())
	require.NoError(t, list(t, s))
	require.Equal(t, selecting.TCP, s.Active())

	unix.fail(eacces()) // a socket appeared that denies us
	clock.advance(reprobeInterval)
	tcpCalls := tcp.calls.Load()

	err := list(t, s)

	require.ErrorIs(t, err, ErrTymuxdSocketAccessDenied)
	require.Equal(t, selecting.Unix, s.Active())
	require.Equal(t, tcpCalls, tcp.calls.Load(), "the call did not go to TCP")
}

func TestSelectingTransport_ConcurrentCallersShareOneReprobe(t *testing.T) {
	t.Parallel()
	s, _, unix, clock := newTestSelector()
	unix.fail(enoent())
	require.NoError(t, list(t, s))
	clock.advance(reprobeInterval)
	before := unix.calls.Load()

	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() { defer wg.Done(); _ = list(t, s) }()
	}
	wg.Wait()

	require.EqualValues(t, 1, unix.calls.Load()-before, "one prober claims the interval; the other callers use TCP")
}

func TestSelectingTransport_BothDown_ReturnsOriginalUnreachableError(t *testing.T) {
	t.Parallel()
	s, tcp, unix, _ := newTestSelector()
	tcp.fail(refused())
	unix.fail(enoent())

	err := list(t, s)

	require.Error(t, err)
	require.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))
	require.True(t, dialRefused("unix", err))
	require.Equal(t, selecting.Unix, s.Active(), "no switch to a dead TCP listener")
	require.ErrorIs(t, classifyRPCError("ListSessions", err), ErrTymuxdUnreachable)
}

func TestSelectingTransport_OtherErrorsNeverSwitch(t *testing.T) {
	t.Parallel()
	s, tcp, unix, _ := newTestSelector()
	unix.fail(connect.NewError(connect.CodeDeadlineExceeded, context.DeadlineExceeded))

	require.Error(t, list(t, s))
	require.Equal(t, selecting.Unix, s.Active())
	require.Zero(t, tcp.calls.Load(), "a slow daemon is not a dead listener")
}

func TestSelectingTransport_TCPRefusedFallsBackToUnix(t *testing.T) {
	t.Parallel()
	s, tcp, unix, _ := newTestSelector()
	unix.fail(enoent())
	require.NoError(t, list(t, s))
	require.Equal(t, selecting.TCP, s.Active())

	unix.heal()
	tcp.fail(refused())
	require.NoError(t, list(t, s))
	require.Equal(t, selecting.Unix, s.Active())
}

func TestSelectingTransport_AttachRedialGoesThroughSelector(t *testing.T) {
	t.Parallel()
	s, tcp, unix, _ := newTestSelector()
	unix.fail(enoent())

	// First dial: opened on the socket, refused when the stream is first used.
	stream := s.Attach(context.Background())
	require.NoError(t, stream.Send(&v1.AttachRequest{}))
	_, err := stream.Receive()
	require.Error(t, err)
	require.Equal(t, selecting.TCP, s.Active(), "the refused dial told the selector to fall back")

	// ReconnectLoop's redial: a fresh Attach lands on TCP.
	redial := s.Attach(context.Background())
	_, err = redial.Receive()
	require.NoError(t, err)
	require.EqualValues(t, 1, tcp.streams.Load())
	require.EqualValues(t, 1, unix.streams.Load())
}

func TestSelectingTransport_ExistingStreamUnaffectedBySwitch(t *testing.T) {
	t.Parallel()
	s, _, unix, _ := newTestSelector()
	stream := s.Attach(context.Background()) // opened while the socket was healthy
	unix.fail(enoent())
	require.NoError(t, list(t, s)) // another caller flips to TCP

	_, err := stream.Receive()
	require.Error(t, err, "the standing stream is not migrated; its own redial will")
	require.Equal(t, selecting.TCP, s.Active())
}

func TestSharedTransportFor_OneSelectorPerDaemon(t *testing.T) {
	t.Parallel()
	var built atomic.Int32
	prefix := fmt.Sprintf("shared-test-%p-", &built) // unique per run: the registry is process-global, so -count=N would reuse keys
	mk := func() *selecting.Client {
		built.Add(1)
		s, _, _, _ := newTestSelector()
		return s
	}
	a := sharedTransportFor(prefix+"a", mk)
	b := sharedTransportFor(prefix+"a", mk)
	c := sharedTransportFor(prefix+"c", mk)

	require.Same(t, a, b, "every session of one daemon shares one selector")
	require.NotSame(t, a, c)
	require.EqualValues(t, 2, built.Load())
}

func TestNewRealTransport_SessionsOfOneDaemonShareOneSelector(t *testing.T) {
	addr := closedAddr(t)
	t.Setenv("STAPLER_SQUAD_INSTANCE", "")
	t.Setenv("TYMUXD_ADDR", addr)
	t.Setenv("TYMUXD_SOCKET_PATH", missingSocketPath(t))

	require.Same(t, NewRealTransport(addr), NewRealTransport(addr), "N sessions must not each probe and log the same transition")
	probe, release := newProbeTransport(addr)
	defer release()
	require.NotSame(t, NewRealTransport(addr), probe, "one-shot probes stay private and quiet")
}

func TestSocketPathForAddr_NeverUsesAnotherInstancesSocket(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_INSTANCE", "uds-fallback-a")
	t.Setenv("TYMUXD_ADDR", "")
	t.Setenv("TYMUXD_SOCKET_PATH", "")
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	own := ResolveDaemonConfig()
	require.NotEmpty(t, own.SocketPath)

	require.Equal(t, own.SocketPath, socketPathForAddr(own.Addr), "own instance daemon")

	t.Setenv("STAPLER_SQUAD_INSTANCE", "uds-fallback-b")
	require.NotEqual(t, own.Addr, ResolveDaemonConfig().Addr)
	require.Empty(t, socketPathForAddr(own.Addr), "addr of instance A, resolved as instance B: A's socket is never used")
	require.Empty(t, socketPathForAddr("http://127.0.0.1:9"), "an unknown addr has no known socket")
}

func TestSocketPathForAddr_CustomAddrWithoutSocketOverrideHasNone(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_INSTANCE", "")
	t.Setenv("TYMUXD_ADDR", "http://127.0.0.1:19999")
	t.Setenv("TYMUXD_SOCKET_PATH", "")
	require.Empty(t, socketPathForAddr("http://127.0.0.1:19999"), "could share a socket with another daemon")

	t.Setenv("TYMUXD_SOCKET_PATH", "/tmp/custom-d.sock")
	require.Equal(t, "/tmp/custom-d.sock", socketPathForAddr("http://127.0.0.1:19999"))
	require.Empty(t, socketPathForAddr(defaultTymuxdAddr), "a custom socket override is not the shared default daemon's socket")
}

// unixOnlyService is the minimal TymuxService squad's client needs, served on a Unix socket with
// no TCP listener anywhere.
type unixOnlyService struct {
	tymuxv1connect.UnimplementedTymuxServiceHandler
}

func (unixOnlyService) GetServerInfo(context.Context, *connect.Request[v1.GetServerInfoRequest]) (*connect.Response[v1.ServerInfo], error) {
	return connect.NewResponse(&v1.ServerInfo{Version: "uds-only"}), nil
}

func (unixOnlyService) ListSessions(context.Context, *connect.Request[v1.ListSessionsRequest]) (*connect.Response[v1.ListSessionsResponse], error) {
	return connect.NewResponse(&v1.ListSessionsResponse{Sessions: []*v1.Session{{Id: "s1", Liveness: v1.Liveness_LIVENESS_LIVE}}}), nil
}

func (unixOnlyService) CapturePane(context.Context, *connect.Request[v1.CapturePaneRequest]) (*connect.Response[v1.PaneSnapshot], error) {
	return connect.NewResponse(&v1.PaneSnapshot{}), nil
}

func TestRealTransport_UnixOnlyDaemon_RPCsSucceedOverSocket(t *testing.T) {
	sock := func() string {
		dir, err := os.MkdirTemp("", "ssq")
		require.NoError(t, err)
		t.Cleanup(func() { _ = os.RemoveAll(dir) })
		return dir + "/d.sock"
	}()
	ln, err := net.Listen("unix", sock)
	require.NoError(t, err)
	mux := http.NewServeMux()
	mux.Handle(tymuxv1connect.NewTymuxServiceHandler(unixOnlyService{}))
	srv := &http.Server{Handler: mux, Protocols: new(http.Protocols)}
	srv.Protocols.SetUnencryptedHTTP2(true)
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	addr := closedAddr(t)
	t.Setenv("STAPLER_SQUAD_INSTANCE", "")
	t.Setenv("TYMUXD_ADDR", addr)
	t.Setenv("TYMUXD_SOCKET_PATH", sock)

	tr := NewRealTransport(addr)
	sel, ok := tr.(*splitTransport)
	require.True(t, ok, "a daemon with a known socket gets the selecting transport")

	info, err := tr.GetServerInfo(context.Background(), connect.NewRequest(&v1.GetServerInfoRequest{}))
	require.NoError(t, err)
	require.Equal(t, "uds-only", info.Msg.GetVersion())
	require.Equal(t, selecting.Unix, sel.Active())

	list, err := tr.ListSessions(context.Background(), connect.NewRequest(&v1.ListSessionsRequest{}))
	require.NoError(t, err)
	require.Len(t, list.Msg.GetSessions(), 1)

	_, err = tr.CapturePane(context.Background(), connect.NewRequest(&v1.CapturePaneRequest{}))
	require.NoError(t, err)

	// The supervisor sees the same daemon as healthy: squad does not need TCP.
	cfg := DaemonConfig{Addr: addr, SocketPath: sock, BinaryPath: "tymuxd"}
	require.Equal(t, daemonProbeHealthy, probeDaemon(context.Background(), cfg))
	n, err := countLiveSessions(context.Background(), cfg)
	require.NoError(t, err)
	require.Equal(t, 1, n)
}

func transportGauge(t *testing.T, daemon, kind string) int64 {
	t.Helper()
	m := collectMetric(t, "tymux_transport")
	require.NotNil(t, m)
	g, ok := m.Data.(metricdata.Gauge[int64])
	require.True(t, ok)
	for _, dp := range g.DataPoints {
		d, _ := dp.Attributes.Value("daemon")
		if v, ok := dp.Attributes.Value("transport"); ok && v.AsString() == kind && d.AsString() == daemon {
			return dp.Value
		}
	}
	return -1
}

func transitionTotal(t *testing.T) int64 {
	t.Helper()
	m := collectMetric(t, "tymux_transport_transitions_total")
	if m == nil {
		return 0
	}
	var total int64
	for _, dp := range m.Data.(metricdata.Sum[int64]).DataPoints {
		total += dp.Value
	}
	return total
}

// Only the long-lived session transport owns tymux_transport and the transition counter; a
// one-shot probe (version checks, live-session counts) must not touch them. Not parallel: it
// reads process-global instruments.
func TestProbeTransport_DoesNotTouchSessionTransportMetrics(t *testing.T) {
	addr := closedAddr(t)
	sock := answeringUnixSocket(t)
	t.Setenv("STAPLER_SQUAD_INSTANCE", "")
	t.Setenv("TYMUXD_ADDR", addr)
	t.Setenv("TYMUXD_SOCKET_PATH", sock)

	recordTransportActive(addr, selecting.Unix) // the session transport's state
	before := transitionTotal(t)

	probe, release := newProbeTransport(addr)
	defer release()
	_, err := probe.GetServerInfo(context.Background(), connect.NewRequest(&v1.GetServerInfoRequest{}))
	require.NoError(t, err, "the probe still falls back to the socket")

	require.EqualValues(t, 1, transportGauge(t, addr, string(selecting.Unix)), "probe construction must not reset the gauge to tcp")
	require.EqualValues(t, 0, transportGauge(t, addr, string(selecting.TCP)))
	require.Equal(t, before, transitionTotal(t), "probe fallback must not count as a session transition")
}

func TestSocketPathForAddr_AddrAndSocketMustComeFromOneResolution(t *testing.T) {
	const custom = "http://127.0.0.1:19999"
	for _, tc := range []struct {
		name                  string
		instance, addr, sock  string
		queryAddr             string // "" = the resolved addr
		wantSocket, wantEmpty bool
	}{
		{name: "instance-derived pair", instance: "uds-c1", wantSocket: true},
		{name: "default addr forced under a named instance", instance: "uds-c2", addr: defaultTymuxdAddr, wantEmpty: true},
		{name: "custom addr under a named instance gets no instance socket", instance: "uds-c3", addr: custom, wantEmpty: true},
		{name: "explicit addr and socket together", addr: custom, sock: "/tmp/pair.sock", wantSocket: true},
		{name: "custom socket with the default addr", sock: "/tmp/other.sock", wantEmpty: true},
		{name: "custom socket queried for a different addr", addr: custom, sock: "/tmp/pair.sock", queryAddr: "http://127.0.0.1:9", wantEmpty: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("STAPLER_SQUAD_INSTANCE", tc.instance)
			t.Setenv("TYMUXD_ADDR", tc.addr)
			t.Setenv("TYMUXD_SOCKET_PATH", tc.sock)
			t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
			rc := ResolveDaemonConfig()
			q := tc.queryAddr
			if q == "" {
				q = rc.Addr
			}
			client := socketPathForAddr(q)
			if tc.wantEmpty {
				require.Empty(t, client)
			} else {
				require.NotEmpty(t, client)
			}
			if tc.queryAddr == "" {
				require.Equal(t, unixSocketPath(rc), client, "supervisor probe and client resolve the socket identically")
			}
		})
	}
}

// A real connect client against a missing socket: Send may only report io.EOF, with the real
// error on Receive. The selector must still see the refusal when only Send is called.
func TestSelectingTransport_RealClientMissingSocket_SendAloneTriggersFailover(t *testing.T) {
	t.Parallel()
	unix, _ := newUnixTransport(missingSocketPath(t))
	sel := selecting.NewFromTransports(unix, &fakeEndpoint{}, selecting.Options{})

	stream := sel.Attach(context.Background())
	err := stream.Send(&v1.AttachRequest{})

	require.Error(t, err)
	require.False(t, errors.Is(err, io.EOF), "the real dial error, not a bare io.EOF, reaches the caller: %v", err)
	require.Equal(t, selecting.TCP, sel.Active())
}

// One failed attempt plus one redial must fit ReconnectLoop's budget.
func TestReconnect_MissingSocketThenTCP_SecondDialSucceeds(t *testing.T) {
	t.Parallel()
	unix, _ := newUnixTransport(missingSocketPath(t))
	sel := selecting.NewFromTransports(unix, &fakeEndpoint{}, selecting.Options{})
	s := NewTymuxGRPCSession(sel).(*tymuxGRPCSession)

	_, _, err := s.dialAttach(context.Background(), "p1")
	require.ErrorIs(t, err, ErrTymuxdUnreachable)
	require.GreaterOrEqual(t, s.reconnectMaxAttempts, 2)

	stream, first, err := s.dialAttach(context.Background(), "p1")
	require.NoError(t, err)
	require.NotNil(t, stream)
	require.NotNil(t, first)
}

func TestEnsureDaemonRunning_NamedInstanceCustomAddr_TCPDeadSocketLive_NeverStops(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_INSTANCE", "uds-veto-1")
	t.Setenv("TYMUXD_ADDR", closedAddr(t))
	t.Setenv("TYMUXD_SOCKET_PATH", "")
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	cfg := ResolveDaemonConfig()
	require.NotEmpty(t, cfg.SocketPath)
	require.Empty(t, unixSocketPath(cfg), "the client fallback does not pair this addr with the instance socket")
	ln, err := net.Listen("unix", cfg.SocketPath)
	require.NoError(t, err)
	grpcOKServer(t, ln)
	withFastRetryBounds(t, 2, time.Millisecond, 2*time.Millisecond)
	shrinkProbeTimeouts(t)
	stop, spawn := neverCalled(t)
	defer stubStopTymuxd(stop)()
	defer stubStartDaemonAttempt(spawn)()

	_, err = EnsureDaemonRunning(context.Background(), cfg)

	require.ErrorIs(t, err, ErrTymuxdControlSocketUnresponsive, "reported, never killed")
}

func TestUnixSocketListening_UnpairedSocketStillVetoesKill(t *testing.T) {
	sock := answeringUnixSocket(t)
	t.Setenv("STAPLER_SQUAD_INSTANCE", "")
	t.Setenv("TYMUXD_ADDR", "")
	t.Setenv("TYMUXD_SOCKET_PATH", sock)
	t.Setenv(allowSharedDaemonInTestEnv, "1") // only the Unix socket is dialed here, never the shared TCP port
	cfg := ResolveDaemonConfig()
	require.Equal(t, sock, cfg.SocketPath)
	require.Empty(t, unixSocketPath(cfg), "unpaired: not used for the client fallback")

	require.True(t, unixSocketListening(cfg), "but spawn exports this socket to the daemon, so a live answer vetoes killing it")
}
