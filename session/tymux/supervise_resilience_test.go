package tymux

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/envtest"
)

// These tests shrink package-level timeouts and stub seams, so none run in parallel.

func shrinkProbeTimeouts(t *testing.T) {
	t.Helper()
	origProbe, origDial := daemonProbeTimeout, daemonProbeDialTimeout
	daemonProbeTimeout, daemonProbeDialTimeout = 50*time.Millisecond, 50*time.Millisecond
	t.Cleanup(func() { daemonProbeTimeout, daemonProbeDialTimeout = origProbe, origDial })
}

func stubUnixListening(fn func(DaemonConfig) bool) func() {
	orig := unixListeningFn
	unixListeningFn = fn
	return func() { unixListeningFn = orig }
}

// silentListener accepts TCP connections and never answers: a daemon too loaded to reply.
func silentListener(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { _ = c.Close() })
		}
	}()
	return "http://" + ln.Addr().String()
}

func closedAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	return "http://" + addr
}

// grpcOKServer answers every call with an empty, successful gRPC reply (a healthy tymuxd for probing).
func grpcOKServer(t *testing.T, ln net.Listener) {
	t.Helper()
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("Trailer", "Grpc-Status")
		_, _ = w.Write([]byte{0, 0, 0, 0, 0})
		w.Header().Set("Grpc-Status", "0")
	}))
	srv.Config.Protocols = new(http.Protocols)
	srv.Config.Protocols.SetUnencryptedHTTP2(true)
	if ln != nil {
		srv.Listener = ln
	}
	srv.Start()
	t.Cleanup(srv.Close)
}

func TestProbeDaemon_ClassifiesListeners(t *testing.T) {
	shrinkProbeTimeouts(t)

	t.Run("nothing listening", func(t *testing.T) {
		require.Equal(t, daemonProbeNotListening, probeDaemon(context.Background(), DaemonConfig{Addr: closedAddr(t)}))
	})
	t.Run("accepts but never answers is busy, not down", func(t *testing.T) {
		cfg := DaemonConfig{Addr: silentListener(t)}
		require.Equal(t, daemonProbeBusy, probeDaemon(context.Background(), cfg))
		require.True(t, checkDaemonHealthy(context.Background(), cfg), "a busy daemon must not be reported unhealthy")
	})
	t.Run("non-gRPC listener is unhealthy", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("hi")) }))
		t.Cleanup(srv.Close)
		cfg := DaemonConfig{Addr: srv.URL}
		require.Equal(t, daemonProbeUnhealthy, probeDaemon(context.Background(), cfg))
		require.False(t, checkDaemonHealthy(context.Background(), cfg))
	})
	t.Run("answering daemon is healthy", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		grpcOKServer(t, ln)
		require.Equal(t, daemonProbeHealthy, probeDaemon(context.Background(), DaemonConfig{Addr: "http://" + ln.Addr().String()}))
	})
	t.Run("unix socket answers while TCP is down", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "ssq")
		require.NoError(t, err)
		t.Cleanup(func() { _ = os.RemoveAll(dir) })
		sock := filepath.Join(dir, "d.sock")
		ln, err := net.Listen("unix", sock)
		require.NoError(t, err)
		grpcOKServer(t, ln)

		cfg := DaemonConfig{Addr: closedAddr(t), SocketPath: sock}
		require.Equal(t, daemonProbeHealthy, probeDaemon(context.Background(), cfg))
		require.True(t, checkDaemonHealthy(context.Background(), cfg))
	})
}

func neverCalled(t *testing.T) (stop func() error, spawn func(DaemonConfig) (*os.Process, error)) {
	t.Helper()
	return func() error { t.Error("daemon must not be stopped"); return nil },
		func(DaemonConfig) (*os.Process, error) {
			t.Error("daemon must not be spawned")
			return &os.Process{}, nil
		}
}

// Alive-but-slow (probe times out, port open): reuse it, never stop it.
func TestEnsureDaemonRunning_BusyDaemonIsReusedNotStopped(t *testing.T) {
	shrinkProbeTimeouts(t)
	stop, spawn := neverCalled(t)
	defer stubStopTymuxd(stop)()
	defer stubStartDaemonAttempt(spawn)()

	ready, err := EnsureDaemonRunning(context.Background(), DaemonConfig{Addr: silentListener(t), BinaryPath: "tymuxd"})

	require.NoError(t, err)
	require.Equal(t, TymuxdReady{}, ready)
}

// One failed probe against a daemon whose listener is up is retried; the daemon (same pid) is kept.
func TestEnsureDaemonRunning_FirstProbeFailsOnce_KeepsDaemon(t *testing.T) {
	withFastRetryBounds(t, 4, time.Millisecond, 2*time.Millisecond)
	var probes int32
	defer stubCheckDaemonHealthy(func(context.Context, DaemonConfig) bool { return atomic.AddInt32(&probes, 1) > 1 })()
	defer stubPortListening(func(DaemonConfig) bool { return true })()
	stop, spawn := neverCalled(t)
	defer stubStopTymuxd(stop)()
	defer stubStartDaemonAttempt(spawn)()

	ready, err := EnsureDaemonRunning(context.Background(), DaemonConfig{Addr: "http://127.0.0.1:19981", BinaryPath: "tymuxd"})

	require.NoError(t, err)
	require.Equal(t, TymuxdReady{}, ready, "reused, not respawned")
	require.EqualValues(t, 2, atomic.LoadInt32(&probes))
}

// A listener that stays up but never answers is reported, not killed, whichever listener it is.
func TestEnsureDaemonRunning_ListenerUpButUnhealthy_IsNotStopped(t *testing.T) {
	for name, tc := range map[string]struct{ tcp, unix bool }{"tcp": {true, false}, "unix": {false, true}} {
		t.Run(name, func(t *testing.T) {
			withFastRetryBounds(t, 3, time.Millisecond, 2*time.Millisecond)
			defer stubCheckDaemonHealthy(func(context.Context, DaemonConfig) bool { return false })()
			defer stubPortListening(func(DaemonConfig) bool { return tc.tcp })()
			defer stubUnixListening(func(DaemonConfig) bool { return tc.unix })()
			stop, spawn := neverCalled(t)
			defer stubStopTymuxd(stop)()
			defer stubStartDaemonAttempt(spawn)()

			_, err := EnsureDaemonRunning(context.Background(), DaemonConfig{Addr: "http://127.0.0.1:19982", BinaryPath: "tymuxd"})

			require.ErrorIs(t, err, ErrTymuxdPortSquatted)
		})
	}
}

// Both listeners definitively dead: stop the (stuck) predecessor and respawn.
func TestEnsureDaemonRunning_BothListenersDead_Respawns(t *testing.T) {
	withFastRetryBounds(t, 3, time.Millisecond, 2*time.Millisecond)
	var spawned atomic.Bool
	defer stubCheckDaemonHealthy(func(context.Context, DaemonConfig) bool { return spawned.Load() })()
	defer stubPortListening(func(DaemonConfig) bool { return false })()
	defer stubUnixListening(func(DaemonConfig) bool { return false })()
	var stops int32
	defer stubStopTymuxd(func() error { atomic.AddInt32(&stops, 1); return nil })()
	defer stubStartDaemonAttempt(func(DaemonConfig) (*os.Process, error) { spawned.Store(true); return &os.Process{}, nil })()

	ready, err := EnsureDaemonRunning(context.Background(), DaemonConfig{Addr: closedAddr(t), BinaryPath: "tymuxd"})

	require.NoError(t, err)
	require.Equal(t, TymuxdReady{Spawned: true}, ready)
	require.EqualValues(t, 1, atomic.LoadInt32(&stops))
}

func TestEnsureDaemonRunning_UnverifiablePID(t *testing.T) {
	for name, tc := range map[string]struct {
		stopErr   error
		wantSpawn bool
	}{
		"unverified pid file is dropped and a daemon spawned": {fmt.Errorf("%w: gone", errTymuxdPIDUnverified), true},
		"any other stop failure aborts":                       {errors.New("permission denied"), false},
	} {
		t.Run(name, func(t *testing.T) {
			withFastRetryBounds(t, 2, time.Millisecond, 2*time.Millisecond)
			var spawned atomic.Bool
			defer stubCheckDaemonHealthy(func(context.Context, DaemonConfig) bool { return spawned.Load() })()
			defer stubPortListening(func(DaemonConfig) bool { return false })()
			defer stubUnixListening(func(DaemonConfig) bool { return false })()
			stops := 0
			defer stubStopTymuxd(func() error {
				stops++
				if stops == 1 {
					return tc.stopErr
				}
				return nil
			})()
			defer stubStartDaemonAttempt(func(DaemonConfig) (*os.Process, error) { spawned.Store(true); return &os.Process{}, nil })()

			_, err := EnsureDaemonRunning(context.Background(), DaemonConfig{Addr: "http://127.0.0.1:19983", BinaryPath: "tymuxd"})

			require.Equal(t, tc.wantSpawn, spawned.Load())
			require.Equal(t, !tc.wantSpawn, err != nil)
		})
	}
}

// A pid file naming a process that is not demonstrably tymuxd is dropped, and nothing is signalled.
func TestStopTymuxd_UnverifiablePID_DropsPIDFileWithoutKilling(t *testing.T) {
	for name, exe := range map[string]func(context.Context, int) (string, error){
		"unresolvable executable": func(context.Context, int) (string, error) { return "", errors.New("no such file") },
		"different executable":    func(context.Context, int) (string, error) { return "/usr/bin/unrelated", nil },
	} {
		t.Run(name, func(t *testing.T) {
			dir := envtest.NewIsolatedStateDir(t)
			require.NoError(t, writeTymuxdPIDFile(os.Getpid()))
			origExe, origSig := processExeFn, signalProcessFn
			t.Cleanup(func() { processExeFn, signalProcessFn = origExe, origSig })
			processExeFn = exe
			signalProcessFn = func(pid int, sig os.Signal) error {
				t.Errorf("must not signal an unverified pid, got %v", sig)
				return nil
			}

			err := StopTymuxd()

			require.ErrorIs(t, err, errTymuxdPIDUnverified)
			require.NoFileExists(t, filepath.Join(dir, tymuxdPIDFileName))
		})
	}
}

func TestTerminateProcess_SigtermFirstThenKillAfterGrace(t *testing.T) {
	origSig, origGrace := signalProcessFn, daemonStopGracePeriod
	t.Cleanup(func() { signalProcessFn, daemonStopGracePeriod = origSig, origGrace })
	daemonStopGracePeriod = 30 * time.Millisecond

	run := func(exitsOnTerm bool) []os.Signal {
		var sent []os.Signal
		dead := false
		signalProcessFn = func(_ int, sig os.Signal) error {
			sent = append(sent, sig)
			if dead {
				return syscall.ESRCH
			}
			if sig == syscall.SIGTERM && exitsOnTerm || sig == syscall.SIGKILL {
				dead = true
			}
			return nil
		}
		require.NoError(t, terminateProcess(4242))
		return sent
	}

	graceful := run(true)
	require.Equal(t, os.Signal(syscall.SIGTERM), graceful[0])
	require.NotContains(t, graceful, os.Signal(syscall.SIGKILL), "a daemon that exits on SIGTERM is never SIGKILLed")

	stubborn := run(false)
	require.Equal(t, os.Signal(syscall.SIGTERM), stubborn[0])
	require.Equal(t, os.Signal(syscall.SIGKILL), stubborn[len(stubborn)-1])
}
