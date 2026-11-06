package tymux

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// These tests stub package-level seams, so none run in parallel.

func overlongSocketPath() string { return "/tmp/" + strings.Repeat("d", 132) + ".sock" }

// notDirSocketPath has a regular file where a directory should be (ENOTDIR on connect).
func notDirSocketPath(t *testing.T) string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "afile")
	require.NoError(t, os.WriteFile(f, nil, 0o600))
	return filepath.Join(f, "d.sock")
}

// stubUnixDial fails only unix dials, so a real closed TCP port still reads as refused.
func stubUnixDial(t *testing.T, err error) {
	t.Helper()
	orig := dialFn
	dialFn = func(ctx context.Context, network, address string, timeout time.Duration) (net.Conn, error) {
		if network == "unix" {
			return nil, err
		}
		return orig(ctx, network, address, timeout)
	}
	t.Cleanup(func() { dialFn = orig })
}

func TestUnixSocketListening_BrokenPathsAreNotListening_ButDenialsStillVeto(t *testing.T) {
	cfg := func(path string) DaemonConfig { return DaemonConfig{Addr: closedAddr(t), SocketPath: path} }

	require.False(t, unixSocketListening(cfg(overlongSocketPath())), "an undialable sun_path cannot hold a live daemon")
	require.False(t, unixSocketListening(cfg(notDirSocketPath(t))))

	for name, errno := range map[string]syscall.Errno{"EINVAL": syscall.EINVAL, "ENOTDIR": syscall.ENOTDIR, "ENAMETOOLONG": syscall.ENAMETOOLONG, "ELOOP": syscall.ELOOP} {
		stubDial(t, dialErr(errno))
		require.False(t, unixSocketListening(cfg("/tmp/x.sock")), name)
	}
	for name, errno := range map[string]syscall.Errno{"EACCES": syscall.EACCES, "EPERM": syscall.EPERM} {
		stubDial(t, dialErr(errno))
		require.True(t, unixSocketListening(cfg("/tmp/x.sock")), name+": a live daemon we may not talk to still vetoes a kill")
	}
	stubDial(t, os.ErrDeadlineExceeded)
	require.True(t, unixSocketListening(cfg("/tmp/x.sock")), "a timeout is inconclusive")
}

func TestEnsureDaemonRunning_DeadDaemonWithBrokenSocketPath_Spawns(t *testing.T) {
	for name, path := range map[string]func() string{
		"overlong": overlongSocketPath,
		"ENOTDIR":  func() string { return notDirSocketPath(t) },
	} {
		t.Run(name, func(t *testing.T) {
			withFastRetryBounds(t, 5, time.Millisecond, 4*time.Millisecond)
			shrinkProbeTimeouts(t)
			var spawned atomic.Bool
			defer stubCheckDaemonHealthy(func(context.Context, DaemonConfig) bool { return spawned.Load() })()
			defer stubStartDaemonAttempt(func(DaemonConfig) (*os.Process, error) { spawned.Store(true); return &os.Process{}, nil })()
			defer stubStopTymuxd(func() error { return nil })()

			ready, err := EnsureDaemonRunning(context.Background(), DaemonConfig{Addr: closedAddr(t), SocketPath: path(), BinaryPath: "tymuxd"})

			require.NoError(t, err, "a dead daemon behind an unusable socket path must not be reported as 'control socket unresponsive'")
			require.True(t, ready.Spawned)
		})
	}
}

func TestEnsureDaemonRunning_SocketPermissionDenied_StillVetoesSpawn(t *testing.T) {
	for name, errno := range map[string]syscall.Errno{"EACCES": syscall.EACCES, "EPERM": syscall.EPERM} {
		t.Run(name, func(t *testing.T) {
			withFastRetryBounds(t, 2, time.Millisecond, 2*time.Millisecond)
			shrinkProbeTimeouts(t)
			stubUnixDial(t, dialErr(errno))
			defer stubCheckDaemonHealthy(func(context.Context, DaemonConfig) bool { return false })()
			stop, spawn := neverCalled(t)
			defer stubStopTymuxd(stop)()
			defer stubStartDaemonAttempt(spawn)()

			_, err := EnsureDaemonRunning(context.Background(), DaemonConfig{Addr: closedAddr(t), SocketPath: "/tmp/x.sock", BinaryPath: "tymuxd"})

			require.ErrorIs(t, err, ErrTymuxdControlSocketUnresponsive, "reported, never killed or respawned over")
		})
	}
}

func TestProbeEndpoint_BrokenSocketPathErrorsMeanNoSocket_NotBusy(t *testing.T) {
	for name, errno := range map[string]syscall.Errno{"EINVAL": syscall.EINVAL, "ENOTDIR": syscall.ENOTDIR, "ENAMETOOLONG": syscall.ENAMETOOLONG, "ELOOP": syscall.ELOOP} {
		stubDial(t, dialErr(errno))
		require.Equal(t, daemonProbeNoSocketFile, probeEndpoint(context.Background(), "unix", "/tmp/x.sock", nil), name)
	}
	stubDial(t, dialErr(syscall.EINVAL))
	require.Equal(t, daemonProbeBusy, probeEndpoint(context.Background(), "tcp", "127.0.0.1:1", nil), "only a unix path can be 'broken'")
}

func TestProbeDaemon_BrokenSocketPath_ConsultsTCP(t *testing.T) {
	shrinkProbeTimeouts(t)
	for name, path := range map[string]func() string{
		"ENOTDIR": func() string { return notDirSocketPath(t) },
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, daemonProbeHealthy, probeDaemon(context.Background(), DaemonConfig{Addr: tcpAnswering(t), SocketPath: path()}),
				"a broken socket path is no reason to ignore a healthy TCP listener")
		})
	}
}

// The dial layer turns an undialable socket path into a refusal, which is what makes upstream's
// selector fall back to TCP (end-to-end ENOTDIR case: TestSelectingTransport_BrokenSocketPath_FallsBackToTCP).
func TestRefusedIfSocketPathBroken_BrokenPathReadsAsRefusal(t *testing.T) {
	t.Parallel()
	for _, errno := range []syscall.Errno{syscall.EINVAL, syscall.ENOTDIR, syscall.ELOOP, syscall.ENAMETOOLONG} {
		require.ErrorIs(t, refusedIfSocketPathBroken("unix", syscallDialErr(errno)), syscall.ENOENT, errno.Error())
	}
	require.NotErrorIs(t, refusedIfSocketPathBroken("tcp", syscallDialErr(syscall.EINVAL)), syscall.ENOENT, "only a unix path can be broken")
	require.NotErrorIs(t, refusedIfSocketPathBroken("unix", syscallDialErr(syscall.EACCES)), syscall.ENOENT)
}

// --- TCP-liveness signal: only a definite refusal is "down" ---

func TestObserveTCP_OnlyDefiniteRefusalIsDown(t *testing.T) {
	for name, tc := range map[string]struct {
		tcp      daemonProbeResult
		wantGaug int64
		wantWarn bool
	}{
		"healthy":       {daemonProbeHealthy, 1, false},
		"not listening": {daemonProbeNotListening, 0, true},
		"busy":          {daemonProbeBusy, 1, false},
		"access denied": {daemonProbeAccessDenied, 1, false},
		"unhealthy":     {daemonProbeUnhealthy, 1, false},
	} {
		t.Run(name, func(t *testing.T) {
			logs := captureTymuxLogs(t)
			recordTCPListenerUp(context.Background(), true) // the last published value
			m := daemonMonitor{tcpProbe: func(context.Context, DaemonConfig) (daemonProbeResult, bool) { return tc.tcp, true }}
			var tr tcpListenerTracker

			m.observeTCP(context.Background(), DaemonConfig{Addr: "http://x"}, daemonProbeHealthy, &tr)

			require.Equal(t, tc.wantGaug, tcpUpGauge(t))
			require.Equal(t, tc.wantWarn, strings.Contains(logs.String(), "TCP-only clients"))
		})
	}
}

func TestObserveTCP_ReusesProbeDaemonsTCPResult_AndBoundsTheExtraProbe(t *testing.T) {
	var extra atomic.Int32
	m := daemonMonitor{tcpProbe: func(context.Context, DaemonConfig) (daemonProbeResult, bool) {
		extra.Add(1)
		return daemonProbeHealthy, true
	}}
	var tr tcpListenerTracker

	m.observeTCP(context.Background(), DaemonConfig{}, daemonProbeDegraded, &tr)
	require.Zero(t, extra.Load(), "Degraded already proves TCP answers: no second probe")

	// The default monitor stashes probeDaemon's own TCP result and consumes it instead of re-probing.
	dm := newDaemonMonitor(nil, nil)
	cfg := DaemonConfig{Addr: tcpAnswering(t), SocketPath: missingSocketPath(t)}
	require.Equal(t, daemonProbeHealthy, dm.probe(context.Background(), cfg))
	res, ok := dm.tcpProbe(context.Background(), cfg)
	require.True(t, ok)
	require.Equal(t, daemonProbeHealthy, res)

	require.Equal(t, 2*time.Second, tcpLivenessProbeTimeout, "a hung daemon adds at most this to a tick")
}
