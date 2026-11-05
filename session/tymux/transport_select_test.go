package tymux

import (
	"context"
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

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
	"github.com/tstapler/tymux/clients/go/gen/tymux/v1/tymuxv1connect"
)

// fakeEndpoint is a hand-driven rpcTransport standing for one listener (TCP or Unix).
type fakeEndpoint struct {
	rpcTransport
	err     atomic.Pointer[error]
	calls   atomic.Int32
	streams atomic.Int32
	rxErr   atomic.Pointer[error]
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

func (f *fakeEndpoint) GetServerInfo(context.Context, *connect.Request[v1.GetServerInfoRequest]) (*connect.Response[v1.ServerInfo], error) {
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

func newTestSelector() (*selectingTransport, *fakeEndpoint, *fakeEndpoint, *testClock) {
	tcp, unix := &fakeEndpoint{}, &fakeEndpoint{}
	clock := &testClock{t: time.Unix(1_000_000, 0)}
	s := newSelectingTransport(tcp, unix, "http://127.0.0.1:1")
	s.now = clock.now
	return s, tcp, unix, clock
}

func list(t *testing.T, s *selectingTransport) error {
	t.Helper()
	_, err := s.ListSessions(context.Background(), connect.NewRequest(&v1.ListSessionsRequest{}))
	return err
}

func TestSelectingTransport_TCPRefusedUnixHealthy_UsesUnix(t *testing.T) {
	t.Parallel()
	s, tcp, unix, _ := newTestSelector()
	tcp.fail(refused())

	require.NoError(t, list(t, s))
	require.Equal(t, transportUnix, s.Active())

	tcpCalls := tcp.calls.Load()
	require.NoError(t, list(t, s))
	require.Equal(t, tcpCalls, tcp.calls.Load(), "sticky: no TCP attempt while the fallback is fresh")
	require.Positive(t, unix.calls.Load())
}

func TestSelectingTransport_TCPRecovers_ReturnsAfterReprobeInterval(t *testing.T) {
	t.Parallel()
	s, tcp, _, clock := newTestSelector()
	tcp.fail(refused())
	require.NoError(t, list(t, s))
	require.Equal(t, transportUnix, s.Active())

	tcp.heal()
	clock.advance(transportReprobeInterval - time.Second)
	require.NoError(t, list(t, s))
	require.Equal(t, transportUnix, s.Active(), "not yet due")

	clock.advance(2 * time.Second)
	require.NoError(t, list(t, s))
	require.Equal(t, transportTCP, s.Active())
}

func TestSelectingTransport_ReprobeStillDown_StaysOnUnixAndWaitsAnotherInterval(t *testing.T) {
	t.Parallel()
	s, tcp, _, clock := newTestSelector()
	tcp.fail(refused())
	require.NoError(t, list(t, s))

	clock.advance(transportReprobeInterval)
	before := tcp.calls.Load()
	require.NoError(t, list(t, s))
	require.Equal(t, before+1, tcp.calls.Load(), "exactly one cheap probe")
	require.Equal(t, transportUnix, s.Active())

	require.NoError(t, list(t, s))
	require.Equal(t, before+1, tcp.calls.Load(), "no hot loop: next probe is a full interval away")
}

func TestSelectingTransport_BothDown_ReturnsOriginalUnreachableError(t *testing.T) {
	t.Parallel()
	s, tcp, unix, _ := newTestSelector()
	tcp.fail(refused())
	unix.fail(refused())

	err := list(t, s)

	require.Error(t, err)
	require.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))
	require.True(t, isDialRefused(err))
	require.Equal(t, transportTCP, s.Active(), "no switch to a dead socket")
	require.ErrorIs(t, classifyRPCError("ListSessions", err), ErrTymuxdUnreachable)
}

func TestSelectingTransport_OtherErrorsNeverSwitch(t *testing.T) {
	t.Parallel()
	s, tcp, unix, _ := newTestSelector()
	tcp.fail(connect.NewError(connect.CodeDeadlineExceeded, context.DeadlineExceeded))

	require.Error(t, list(t, s))
	require.Equal(t, transportTCP, s.Active())
	require.Zero(t, unix.calls.Load(), "a slow daemon is not a dead listener")
}

func TestSelectingTransport_UnixRefusedFallsBackToTCP(t *testing.T) {
	t.Parallel()
	s, tcp, unix, _ := newTestSelector()
	tcp.fail(refused())
	require.NoError(t, list(t, s))
	require.Equal(t, transportUnix, s.Active())

	tcp.heal()
	unix.fail(refused())
	require.NoError(t, list(t, s))
	require.Equal(t, transportTCP, s.Active())
}

func TestSelectingTransport_AttachRedialGoesThroughSelector(t *testing.T) {
	t.Parallel()
	s, tcp, unix, _ := newTestSelector()
	tcp.fail(refused())

	// First dial: opened on TCP, refused when the stream is first used.
	stream := s.Attach(context.Background())
	require.NoError(t, stream.Send(&v1.AttachRequest{}))
	_, err := stream.Receive()
	require.Error(t, err)
	require.Equal(t, transportUnix, s.Active(), "the refused dial told the selector to fall back")

	// ReconnectLoop's redial: a fresh Attach lands on the Unix socket.
	redial := s.Attach(context.Background())
	_, err = redial.Receive()
	require.NoError(t, err)
	require.EqualValues(t, 1, unix.streams.Load())
	require.EqualValues(t, 1, tcp.streams.Load())
}

func TestSelectingTransport_ExistingStreamUnaffectedBySwitch(t *testing.T) {
	t.Parallel()
	s, tcp, _, _ := newTestSelector()
	stream := s.Attach(context.Background()) // opened while TCP was healthy
	tcp.fail(refused())
	require.NoError(t, list(t, s)) // another caller flips to unix

	_, err := stream.Receive()
	require.Error(t, err, "the standing stream is not migrated; its own redial will")
	require.Equal(t, transportUnix, s.Active())
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
	sel, ok := tr.(*selectingTransport)
	require.True(t, ok, "a daemon with a known socket gets the selecting transport")

	info, err := tr.GetServerInfo(context.Background(), connect.NewRequest(&v1.GetServerInfoRequest{}))
	require.NoError(t, err)
	require.Equal(t, "uds-only", info.Msg.GetVersion())
	require.Equal(t, transportUnix, sel.Active())

	list, err := tr.ListSessions(context.Background(), connect.NewRequest(&v1.ListSessionsRequest{}))
	require.NoError(t, err)
	require.Len(t, list.Msg.GetSessions(), 1)

	_, err = tr.CapturePane(context.Background(), connect.NewRequest(&v1.CapturePaneRequest{}))
	require.NoError(t, err)

	// The supervisor treats the same daemon as degraded, reusing it.
	cfg := DaemonConfig{Addr: addr, SocketPath: sock, BinaryPath: "tymuxd"}
	require.Equal(t, daemonProbeDegraded, probeDaemon(context.Background(), cfg))
	n, err := countLiveSessions(context.Background(), cfg)
	require.NoError(t, err)
	require.Equal(t, 1, n)
}
