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
	t.Run("unix socket answering with a dead TCP listener is healthy: squad does not need TCP", func(t *testing.T) {
		cfg := DaemonConfig{Addr: closedAddr(t), SocketPath: answeringUnixSocket(t)}
		require.Equal(t, daemonProbeHealthy, probeDaemon(context.Background(), cfg))
		require.True(t, checkDaemonHealthy(context.Background(), cfg))
	})
	t.Run("answering unix socket with a TCP squatter is healthy", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("hi")) }))
		t.Cleanup(srv.Close)
		cfg := DaemonConfig{Addr: srv.URL, SocketPath: answeringUnixSocket(t)}
		require.Equal(t, daemonProbeHealthy, probeDaemon(context.Background(), cfg), "the socket decides; TCP is not consulted")
	})
	t.Run("missing unix socket file with TCP answering is an older daemon: healthy, not degraded", func(t *testing.T) {
		cfg := DaemonConfig{Addr: tcpAnswering(t), SocketPath: missingSocketPath(t)}
		require.Equal(t, daemonProbeHealthy, probeDaemon(context.Background(), cfg))
	})
	t.Run("missing unix socket and dead TCP is not listening", func(t *testing.T) {
		cfg := DaemonConfig{Addr: closedAddr(t), SocketPath: missingSocketPath(t)}
		require.Equal(t, daemonProbeNotListening, probeDaemon(context.Background(), cfg))
	})
	t.Run("missing unix socket with a TCP squatter stays unhealthy", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("hi")) }))
		t.Cleanup(srv.Close)
		cfg := DaemonConfig{Addr: srv.URL, SocketPath: missingSocketPath(t)}
		require.Equal(t, daemonProbeUnhealthy, probeDaemon(context.Background(), cfg))
	})
	t.Run("silent unix socket is busy even when TCP answers", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		grpcOKServer(t, ln)
		cfg := DaemonConfig{Addr: "http://" + ln.Addr().String(), SocketPath: silentUnixSocket(t)}
		require.Equal(t, daemonProbeBusy, probeDaemon(context.Background(), cfg), "inconclusive: never degraded, never killed")
	})
	t.Run("caller deadline mid-answer is unhealthy, not busy", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		require.Equal(t, daemonProbeUnhealthy, probeDaemon(ctx, DaemonConfig{Addr: silentListener(t)}))
	})
}

// answeringUnixSocket serves a healthy gRPC reply on a fresh Unix socket and returns its path.
func answeringUnixSocket(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "ssq")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "d.sock")
	ln, err := net.Listen("unix", sock)
	require.NoError(t, err)
	grpcOKServer(t, ln)
	return sock
}

// silentUnixSocket accepts connections on a Unix socket and never answers.
func silentUnixSocket(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "ssq")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	ln, err := net.Listen("unix", filepath.Join(dir, "s.sock"))
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
	return ln.Addr().String()
}

// TCP dead, Unix socket alive: degraded but usable, so the daemon is reused: no error, no stop, no spawn.
func TestEnsureDaemonRunning_TCPDeadUnixUp_ReusesDaemon(t *testing.T) {
	withFastRetryBounds(t, 2, time.Millisecond, 2*time.Millisecond)
	shrinkProbeTimeouts(t)
	stop, spawn := neverCalled(t)
	defer stubStopTymuxd(stop)()
	defer stubStartDaemonAttempt(spawn)()

	ready, err := EnsureDaemonRunning(context.Background(), DaemonConfig{Addr: closedAddr(t), SocketPath: answeringUnixSocket(t), BinaryPath: "tymuxd"})

	require.NoError(t, err)
	require.Equal(t, TymuxdReady{}, ready)
}

func dialErr(errno syscall.Errno) error {
	return &net.OpError{Op: "dial", Err: os.NewSyscallError("connect", errno)}
}

func stubDial(t *testing.T, err error) {
	t.Helper()
	orig := dialFn
	t.Cleanup(func() { dialFn = orig })
	dialFn = func(context.Context, string, string, time.Duration) (net.Conn, error) { return nil, err }
}

// Only a refusal proves nothing is listening; every other dial error is inconclusive and must
// never lead to killing the daemon.
func TestDialErrors_OnlyRefusalMeansDown(t *testing.T) {
	timeout := &net.OpError{Op: "dial", Err: os.ErrDeadlineExceeded}
	for _, tc := range []struct {
		name     string
		network  string
		err      error
		wantDown bool
	}{
		{"tcp refused", "tcp", dialErr(syscall.ECONNREFUSED), true},
		{"unix refused", "unix", dialErr(syscall.ECONNREFUSED), true},
		{"unix missing socket", "unix", dialErr(syscall.ENOENT), true},
		{"tcp ENOENT is not a refusal", "tcp", dialErr(syscall.ENOENT), false},
		{"timeout", "tcp", timeout, false},
		{"unix timeout", "unix", timeout, false},
		{"EMFILE", "tcp", dialErr(syscall.EMFILE), false},
		{"ENFILE", "tcp", dialErr(syscall.ENFILE), false},
		{"ENOBUFS", "tcp", dialErr(syscall.ENOBUFS), false},
		{"ENOMEM", "unix", dialErr(syscall.ENOMEM), false},
		{"EADDRNOTAVAIL", "tcp", dialErr(syscall.EADDRNOTAVAIL), false},
		{"ENETUNREACH", "tcp", dialErr(syscall.ENETUNREACH), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stubDial(t, tc.err)
			wantProbe := daemonProbeBusy
			if tc.wantDown {
				wantProbe = daemonProbeNotListening
			}
			if tc.name == "unix missing socket" {
				wantProbe = daemonProbeNoSocketFile // still "down" for the kill veto below, but told apart for the supervisor
			}
			require.Equal(t, wantProbe, probeEndpoint(context.Background(), tc.network, "addr", nil))
			if tc.network == "tcp" {
				require.Equal(t, !tc.wantDown, portListening(DaemonConfig{Addr: "http://127.0.0.1:1"}))
			} else {
				require.Equal(t, !tc.wantDown, unixSocketListening(DaemonConfig{Addr: "http://127.0.0.1:1", SocketPath: "/tmp/x.sock"}))
			}
		})
	}
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

			if tc.tcp {
				require.ErrorIs(t, err, ErrTymuxdPortSquatted)
			} else {
				require.ErrorIs(t, err, ErrTymuxdControlSocketUnresponsive)
				require.NotErrorIs(t, err, ErrTymuxdPortSquatted, "nothing listens on TCP, so nothing is squatting the port")
				require.NotContains(t, err.Error(), "something is listening")
			}
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

			err := StopTymuxd(StopReason{Kind: StopManual})

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
		require.NoError(t, terminateProcess(4242, StopReason{Kind: StopManual}))
		return sent
	}

	graceful := run(true)
	require.Equal(t, os.Signal(syscall.SIGTERM), graceful[0])
	require.NotContains(t, graceful, os.Signal(syscall.SIGKILL), "a daemon that exits on SIGTERM is never SIGKILLed")

	stubborn := run(false)
	require.Equal(t, os.Signal(syscall.SIGTERM), stubborn[0])
	require.Equal(t, os.Signal(syscall.SIGKILL), stubborn[len(stubborn)-1])
}

// A daemon this process spawned must be reaped when it exits, or terminateProcess sees a zombie
// that still answers signal 0 and waits out the whole grace period.
func TestTerminateProcess_SpawnedDaemonIsReapedPromptly(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	origGrace := daemonStopGracePeriod
	t.Cleanup(func() { daemonStopGracePeriod = origGrace })
	daemonStopGracePeriod = 5 * time.Second

	script := filepath.Join(t.TempDir(), "fake-tymuxd")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nexec sleep 300\n"), 0o700))
	proc, err := startDaemonAttempt(DaemonConfig{Addr: "http://127.0.0.1:1", BinaryPath: script})
	require.NoError(t, err)
	t.Cleanup(func() { _ = syscall.Kill(proc.Pid, syscall.SIGKILL) })

	start := time.Now()
	require.NoError(t, terminateProcess(proc.Pid, StopReason{Kind: StopManual}))
	require.Less(t, time.Since(start), 2*time.Second, "exited child must read as gone, not wait for SIGKILL")
}

// missingSocketPath is a socket path nothing listens on (ENOENT).
func missingSocketPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "ssq")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir + "/none.sock"
}
