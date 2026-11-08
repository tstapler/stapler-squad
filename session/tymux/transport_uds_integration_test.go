package tymux

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
	"github.com/tstapler/tymux/clients/go/gen/tymux/v1/tymuxv1connect"
	"github.com/tstapler/tymux/clients/go/selecting"
)

// paneService answers GetServerInfo and a CapturePane carrying a rows x cols grid, so the same
// in-process server backs the integration tests and the transport benchmark.
type paneService struct {
	tymuxv1connect.UnimplementedTymuxServiceHandler
	grid *v1.PaneSnapshot
}

func newPaneService(rows, cols int) paneService {
	snap := &v1.PaneSnapshot{PaneId: "p1", Rows: uint32(rows), Cols: uint32(cols)}
	for range rows {
		row := &v1.Row{}
		for range cols {
			row.Cells = append(row.Cells, &v1.Cell{Text: "x", Fg: 7})
		}
		snap.Grid = append(snap.Grid, row)
	}
	return paneService{grid: snap}
}

func (paneService) GetServerInfo(context.Context, *connect.Request[v1.GetServerInfoRequest]) (*connect.Response[v1.ServerInfo], error) {
	return connect.NewResponse(&v1.ServerInfo{Version: "test"}), nil
}

func (p paneService) CapturePane(context.Context, *connect.Request[v1.CapturePaneRequest]) (*connect.Response[v1.PaneSnapshot], error) {
	return connect.NewResponse(p.grid), nil
}

// testDaemon is one in-process connect server on both a Unix socket and loopback TCP, each with its
// own request counter and its own stop func.
type testDaemon struct {
	addr, sock        string
	unixHits, tcpHits atomic.Int64
	stopUnix, stopTCP func()
}

func startTestDaemon(tb testing.TB, svc paneService) *testDaemon {
	tb.Helper()
	d := &testDaemon{}
	dir, err := os.MkdirTemp("", "ssq")
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = os.RemoveAll(dir) })
	d.sock = filepath.Join(dir, "d.sock")

	serve := func(ln net.Listener, hits *atomic.Int64) func() {
		mux := http.NewServeMux()
		mux.Handle(tymuxv1connect.NewTymuxServiceHandler(svc))
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			mux.ServeHTTP(w, r)
		})
		srv := &http.Server{Handler: h, Protocols: new(http.Protocols)}
		srv.Protocols.SetUnencryptedHTTP2(true)
		go func() { _ = srv.Serve(ln) }()
		// Close the listener ourselves: srv.Close only closes listeners Serve has already
		// registered, so a stop that races the goroutine's start would leave a live backlog.
		stop := func() { _ = ln.Close(); _ = srv.Close() }
		tb.Cleanup(stop)
		return stop
	}
	uln, err := net.Listen("unix", d.sock)
	require.NoError(tb, err)
	tln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(tb, err)
	d.addr = "http://" + tln.Addr().String()
	d.stopUnix = serve(uln, &d.unixHits)
	d.stopTCP = serve(tln, &d.tcpHits)
	return d
}

func useDaemonEnv(t *testing.T, d *testDaemon) {
	t.Helper()
	t.Setenv("STAPLER_SQUAD_INSTANCE", "")
	t.Setenv("TYMUXD_ADDR", d.addr)
	t.Setenv("TYMUXD_SOCKET_PATH", d.sock)
}

func TestUDSDefault_BothListenersUp_TrafficUsesOnlyTheSocket(t *testing.T) {
	d := startTestDaemon(t, newPaneService(2, 2))
	useDaemonEnv(t, d)
	tr := NewRealTransport(d.addr)

	_, err := tr.GetServerInfo(context.Background(), connect.NewRequest(&v1.GetServerInfoRequest{}))
	require.NoError(t, err)
	snap, err := tr.CapturePane(context.Background(), connect.NewRequest(&v1.CapturePaneRequest{}))
	require.NoError(t, err)
	require.Len(t, snap.Msg.GetGrid(), 2)

	require.Equal(t, selecting.Unix, tr.(*splitTransport).Active())
	require.Positive(t, d.unixHits.Load())
	require.Zero(t, d.tcpHits.Load(), "the unauthenticated TCP listener saw no request")
	require.Equal(t, daemonProbeHealthy, probeDaemon(context.Background(), ResolveDaemonConfig()))
}

func TestUDSDefault_SocketGone_FallsBackToTCP_AndSupervisorTreatsItAsNoSocket(t *testing.T) {
	d := startTestDaemon(t, newPaneService(1, 1))
	useDaemonEnv(t, d)
	tr := NewRealTransport(d.addr)
	d.stopUnix() // also unlinks the socket file: ENOENT

	_, err := tr.GetServerInfo(context.Background(), connect.NewRequest(&v1.GetServerInfoRequest{}))

	require.NoError(t, err)
	require.Equal(t, selecting.TCP, tr.(*splitTransport).Active())
	require.Positive(t, d.tcpHits.Load())
	require.Equal(t, daemonProbeHealthy, probeDaemon(context.Background(), ResolveDaemonConfig()), "no socket file: an older daemon, not degraded")
}

func TestUDSDefault_SocketDeniesAccess_HardErrorNoTCP(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses socket file permissions")
	}
	d := startTestDaemon(t, newPaneService(1, 1))
	useDaemonEnv(t, d)
	require.NoError(t, os.Chmod(d.sock, 0))
	tr := NewRealTransport(d.addr)

	_, err := tr.GetServerInfo(context.Background(), connect.NewRequest(&v1.GetServerInfoRequest{}))

	require.ErrorIs(t, err, ErrTymuxdSocketAccessDenied)
	require.ErrorIs(t, err, syscall.EACCES)
	require.Zero(t, d.tcpHits.Load(), "EACCES must never reach the unauthenticated listener")
	require.Equal(t, selecting.Unix, tr.(*splitTransport).Active())
}

// The fixture's stop must leave nothing listening the instant it returns, even when it races the
// serve goroutine's start: otherwise a client connects to the half-closed backlog and gets
// ECONNRESET (seen on Linux) instead of the ENOENT/ECONNREFUSED the selector reads as "no listener".
func TestStartTestDaemon_StopLeavesNothingListeningImmediately(t *testing.T) {
	t.Parallel()
	for range 300 {
		d := startTestDaemon(t, newPaneService(1, 1))
		d.stopUnix()
		d.stopTCP()

		_, err := net.Dial("unix", d.sock)
		require.Error(t, err)
		require.True(t, dialRefused("unix", err), "stopped Unix listener must refuse, got %v", err)
		_, err = net.Dial("tcp", strings.TrimPrefix(d.addr, "http://"))
		require.Error(t, err)
		require.True(t, dialRefused("tcp", err), "stopped TCP listener must refuse, got %v", err)
	}
}

// BenchmarkTransportRoundTrip compares one unary round trip over the Unix socket and loopback TCP
// to an in-process connect server; run in the background per docs/reference/benchmarks.md:
//
//	go test -run '^$' -bench BenchmarkTransportRoundTrip -benchmem -count=5 ./session/tymux &
//
// Caveats: the server is an in-process Go connect handler, not the Rust tymuxd; the client is
// sequential on one connection per transport; both transports are warmed first and the order of
// the sub-benchmarks alternates per RPC so run order does not favor either transport.
func BenchmarkTransportRoundTrip(b *testing.B) {
	d := startTestDaemon(b, newPaneService(50, 200)) // a full 200x50 screen, ~10k cells
	tcp, _ := newTCPTransport(d.addr)
	unix, _ := newUnixTransport(d.sock)
	ctx := context.Background()
	info := func(tr rpcTransport) error {
		_, err := tr.GetServerInfo(ctx, connect.NewRequest(&v1.GetServerInfoRequest{}))
		return err
	}
	capture := func(tr rpcTransport) error {
		_, err := tr.CapturePane(ctx, connect.NewRequest(&v1.CapturePaneRequest{}))
		return err
	}
	for _, tr := range []rpcTransport{tcp, unix} {
		for range 200 {
			if err := errors.Join(info(tr), capture(tr)); err != nil {
				b.Fatal(err)
			}
		}
	}
	type variant struct {
		name string
		tr   rpcTransport
	}
	for _, rpc := range []struct {
		name  string
		call  func(rpcTransport) error
		order []variant
	}{
		{"GetServerInfo", info, []variant{{"tcp", tcp}, {"unix", unix}}},
		{"CapturePane", capture, []variant{{"unix", unix}, {"tcp", tcp}}},
	} {
		for _, v := range rpc.order {
			b.Run(rpc.name+"/"+v.name, func(b *testing.B) {
				for range b.N {
					if err := rpc.call(v.tr); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
