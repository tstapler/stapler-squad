package tymux

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
	"github.com/tstapler/tymux/clients/go/gen/tymux/v1/tymuxv1connect"

	"github.com/tstapler/stapler-squad/log"
)

// These tests swap package-global log/metric state or shrink probe timeouts: none run in parallel.

type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// captureTymuxLogs routes squad's logger into a buffer for the test's duration. It also starts a
// fresh warn-once episode: onceLogged is process-global and keyed by address, so a recycled
// ephemeral port (or an earlier test) would otherwise already hold the key and silence the warning.
func captureTymuxLogs(t *testing.T) *lockedBuf {
	t.Helper()
	resetOnceLogged := func() { onceLogged.Range(func(k, _ any) bool { onceLogged.Delete(k); return true }) }
	resetOnceLogged()
	t.Cleanup(resetOnceLogged)
	buf := &lockedBuf{}
	prev := log.SetSlogDefaultForTest(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { log.SetSlogDefaultForTest(prev) })
	return buf
}

// codeService answers GetServerInfo with a fixed connect error code.
type codeService struct {
	tymuxv1connect.UnimplementedTymuxServiceHandler
	code connect.Code
}

func (c codeService) GetServerInfo(context.Context, *connect.Request[v1.GetServerInfoRequest]) (*connect.Response[v1.ServerInfo], error) {
	return nil, connect.NewError(c.code, os.ErrPermission)
}

func serveConnect(t *testing.T, ln net.Listener, svc tymuxv1connect.TymuxServiceHandler) {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle(tymuxv1connect.NewTymuxServiceHandler(svc))
	srv := &http.Server{Handler: mux, Protocols: new(http.Protocols)}
	srv.Protocols.SetUnencryptedHTTP2(true)
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
}

// deniedSocket is a live socket whose file mode makes connect() fail with EACCES.
func deniedSocket(t *testing.T) string {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root bypasses socket file permissions")
	}
	sock := answeringUnixSocket(t)
	require.NoError(t, os.Chmod(sock, 0))
	return sock
}

// staleSocketFile is a socket file nobody listens on (ECONNREFUSED), as a crashed daemon leaves.
func staleSocketFile(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "ssq")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "stale.sock")
	ln, err := net.Listen("unix", sock)
	require.NoError(t, err)
	ln.(*net.UnixListener).SetUnlinkOnClose(false)
	require.NoError(t, ln.Close())
	_, err = os.Stat(sock)
	require.NoError(t, err, "the stale file must remain")
	return sock
}

func tcpAnswering(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	grpcOKServer(t, ln)
	return "http://" + ln.Addr().String()
}

func TestProbeDaemon_SocketAccessDenied_IsItsOwnAliveResult_AndWarnsWithThePath(t *testing.T) {
	shrinkProbeTimeouts(t)
	logs := captureTymuxLogs(t)
	cfg := DaemonConfig{Addr: tcpAnswering(t), SocketPath: deniedSocket(t)}

	require.Equal(t, daemonProbeAccessDenied, probeDaemon(context.Background(), cfg), "TCP answering must not turn a permission problem into Degraded")
	require.True(t, checkDaemonHealthy(context.Background(), cfg), "alive: never killed, never respawned")
	require.Contains(t, logs.String(), cfg.SocketPath)
	require.Contains(t, logs.String(), "permission denied connecting")
}

func TestProbeDaemon_AccessDeniedWarnsOncePerEpisode(t *testing.T) {
	shrinkProbeTimeouts(t)
	logs := captureTymuxLogs(t)
	cfg := DaemonConfig{Addr: closedAddr(t), SocketPath: deniedSocket(t)}

	probeDaemon(context.Background(), cfg)
	probeDaemon(context.Background(), cfg)

	require.Equal(t, 1, bytes.Count([]byte(logs.String()), []byte("permission denied connecting")), "one diagnosis, not one per 5s tick")
}

func TestProbeEndpoint_UnauthenticatedOrPermissionDeniedReply_IsAccessDenied(t *testing.T) {
	shrinkProbeTimeouts(t)
	for name, code := range map[string]connect.Code{"unauthenticated": connect.CodeUnauthenticated, "permission denied": connect.CodePermissionDenied} {
		t.Run(name, func(t *testing.T) {
			logs := captureTymuxLogs(t)
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			serveConnect(t, ln, codeService{code: code})
			cfg := DaemonConfig{Addr: "http://" + ln.Addr().String()}

			require.Equal(t, daemonProbeAccessDenied, probeDaemon(context.Background(), cfg), "a credentials problem is not 'unhealthy'")
			require.True(t, checkDaemonHealthy(context.Background(), cfg))
			require.Contains(t, logs.String(), "credential")
		})
	}
}

func TestWedgeTracker_AccessDeniedIsNeverReportedAsWedged(t *testing.T) {
	var w wedgeTracker
	t0 := time.Unix(3_000_000, 0)
	for i := range 5 {
		require.False(t, w.observe(context.Background(), DaemonConfig{}, daemonProbeAccessDenied, t0.Add(time.Duration(i)*daemonWedgedAfter)))
	}
}

func TestEnsureDaemonRunning_SocketAccessDenied_ReusesWithoutSpawningOrStopping(t *testing.T) {
	shrinkProbeTimeouts(t)
	cfg := DaemonConfig{Addr: closedAddr(t), SocketPath: deniedSocket(t)}
	stop, spawn := neverCalled(t)
	defer stubStopTymuxd(stop)()
	defer stubStartDaemonAttempt(spawn)()

	ready, err := EnsureDaemonRunning(context.Background(), cfg)

	require.NoError(t, err)
	require.False(t, ready.Spawned)
}

func TestProbeDaemon_NoSocketFile_TCPAnswering_IsHealthyNotDegraded_OlderDaemon(t *testing.T) {
	shrinkProbeTimeouts(t)
	logs := captureTymuxLogs(t)
	cfg := DaemonConfig{Addr: tcpAnswering(t), SocketPath: missingSocketPath(t)}

	require.Equal(t, daemonProbeHealthy, probeDaemon(context.Background(), cfg), "a daemon that never had a socket is not degraded")
	probeDaemon(context.Background(), cfg)
	require.Contains(t, logs.String(), "no Unix control socket")
	require.Equal(t, 1, bytes.Count([]byte(logs.String()), []byte("no Unix control socket")), "one Info, not one per tick")
	require.NotContains(t, logs.String(), "level=WARN")
}

func TestProbeDaemon_StaleSocketFile_TCPAnswering_IsDegraded(t *testing.T) {
	shrinkProbeTimeouts(t)
	cfg := DaemonConfig{Addr: tcpAnswering(t), SocketPath: staleSocketFile(t)}

	require.Equal(t, daemonProbeDegraded, probeDaemon(context.Background(), cfg), "ECONNREFUSED on an existing socket file: the control socket died")
}

func TestProbeDaemon_NoSocketFile_TCPDead_IsNotListening(t *testing.T) {
	shrinkProbeTimeouts(t)
	cfg := DaemonConfig{Addr: closedAddr(t), SocketPath: missingSocketPath(t)}

	require.Equal(t, daemonProbeNotListening, probeDaemon(context.Background(), cfg))
}

func tcpUpGauge(t *testing.T) int64 {
	t.Helper()
	m := collectMetric(t, "tymux_tcp_listener_up")
	require.NotNil(t, m)
	g := m.Data.(metricdata.Gauge[int64])
	require.Len(t, g.DataPoints, 1)
	return g.DataPoints[0].Value
}

func TestTCPListenerTracker_GaugeFollowsTCP_WarnsRateLimitedOnlyWhileDaemonAlive(t *testing.T) {
	logs := captureTymuxLogs(t)
	var tr tcpListenerTracker
	ctx := context.Background()
	t0 := time.Unix(4_000_000, 0)

	require.True(t, tr.observe(ctx, DaemonConfig{Addr: "http://x"}, false, t0), "first down tick warns")
	require.EqualValues(t, 0, tcpUpGauge(t))
	require.Contains(t, logs.String(), "TCP-only clients")
	require.False(t, tr.observe(ctx, DaemonConfig{}, false, t0.Add(daemonDegradedWarnEvery-time.Second)), "rate-limited")
	require.True(t, tr.observe(ctx, DaemonConfig{}, false, t0.Add(daemonDegradedWarnEvery)))

	require.False(t, tr.observe(ctx, DaemonConfig{}, true, t0.Add(time.Hour)))
	require.EqualValues(t, 1, tcpUpGauge(t))
	require.True(t, tr.observe(ctx, DaemonConfig{}, false, t0.Add(time.Hour+time.Second)), "a new outage warns immediately")
}

func TestMonitor_SocketHealthyTCPDead_StaysHealthy_ButPublishesTCPDown(t *testing.T) {
	var probes atomic.Int32
	m := daemonMonitor{
		interval: time.Millisecond, maxBackoff: time.Millisecond, threshold: 3,
		probe:    func(context.Context, DaemonConfig) daemonProbeResult { probes.Add(1); return daemonProbeHealthy },
		tcpProbe: func(context.Context, DaemonConfig) (daemonProbeResult, bool) { return daemonProbeNotListening, true },
		ensure: func(context.Context, DaemonConfig) (TymuxdReady, error) {
			t.Error("must not recover a healthy daemon")
			return TymuxdReady{}, nil
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); m.run(ctx, DaemonConfig{Addr: "http://x"}) }()
	require.Eventually(t, func() bool { return probes.Load() >= 3 }, 5*time.Second, time.Millisecond)
	cancel()
	<-done

	require.EqualValues(t, 0, tcpUpGauge(t))
}

func TestDegradedTracker_WarnWordingNamesTheSocketAndTheTCPFallback(t *testing.T) {
	logs := captureTymuxLogs(t)
	var d degradedTracker

	require.True(t, d.observe(context.Background(), DaemonConfig{Addr: "http://x"}, true, time.Unix(5_000_000, 0)))

	require.Contains(t, logs.String(), "Unix control socket is not answering")
	require.Contains(t, logs.String(), "TCP")
	require.NotContains(t, logs.String(), "TCP listener is down")
}
