//go:build integration

package tymux

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/envtest"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

// This file exercises EnsureDaemonRunning/startDaemonAttempt/StopTymuxd
// against a REAL spawned tymuxd binary — the supervision layer BUG-106
// (shared Unix-socket lock collides across instances) and BUG-110 (a
// process that stays alive after its TCP listener dies can never be
// evicted) both lived in. Every test in supervise_test.go injects fakes for
// startDaemonAttemptFn/checkDaemonHealthyFn/portListeningFn by design — fast
// and deterministic, but structurally incapable of catching a bug that only
// exists because two real OS processes fight over a real lock file, which
// is exactly what both bugs were. integration_test.go's two existing
// real-tymuxd tests don't cover this either: one requires an
// already-running external daemon and talks to it via TymuxGRPCSession
// directly, the other spawns its own tymuxd via a raw exec.Command that
// bypasses EnsureDaemonRunning/startDaemonAttempt entirely.
//
// Each test isolates config.GetConfigDir() via envtest.NewIsolatedStateDir
// (STAPLER_SQUAD_TEST_DIR) so writeTymuxdPIDFile/StopTymuxd/
// openTymuxdOutputLog never touch a real PID file or another test's. Tests
// that need two independent "instances" (BUG-106's shape) call it again
// mid-test between EnsureDaemonRunning calls, exactly mirroring how two
// separate real processes would each resolve their own config dir.

// decoyTymuxdOpts configures spawnDecoyTymuxd. A struct instead of positional
// string parameters per the `primitive-obsession-checklist` skill — four
// same-typed strings in a row is exactly the swappable-at-the-call-site
// shape it warns about.
type decoyTymuxdOpts struct {
	addr       string // TYMUXD_ADDR to bind
	socketPath string // TYMUXD_SOCKET_PATH; empty means tymuxd's own default
	waitAddr   string // poll this TCP address until it accepts a connection; empty skips polling
}

// decoyTymuxd wraps a manually-spawned tymuxd subprocess with a single
// background Wait(), so a test can confirm it actually exited (not just
// that a signal was delivered) without racing exec.Cmd's "Wait must be
// called at most once" rule against StopTymuxd's own independent
// os.FindProcess(pid).Kill() (which reaps nothing itself -- only this
// process, as the real OS-level parent, can do that).
type decoyTymuxd struct {
	cmd  *exec.Cmd
	exit chan struct{} // closed once cmd.Wait() returns, i.e. the process is truly gone (not just signaled)
}

// exitedWithin reports whether the process has actually exited (reaped, not
// merely signaled/zombie) within timeout. A killed-but-unreaped process
// still answers Signal(0) successfully on Linux, which is why this waits
// for Wait() to return instead of polling liveness by signal.
func (d *decoyTymuxd) exitedWithin(timeout time.Duration) bool {
	select {
	case <-d.exit:
		return true
	case <-time.After(timeout):
		return false
	}
}

// spawnDecoyTymuxd starts bin directly via exec.Command, bypassing
// startDaemonAttempt entirely — used to seed a daemon this test controls
// directly (a stuck predecessor, a competing instance, a port squatter),
// never the one under test via EnsureDaemonRunning.
func spawnDecoyTymuxd(t *testing.T, bin string, opts decoyTymuxdOpts) *decoyTymuxd {
	t.Helper()
	cmd := exec.Command(bin)
	env := append(os.Environ(), "TYMUXD_ADDR="+opts.addr, "RUST_LOG=warn")
	if opts.socketPath != "" {
		env = append(env, "TYMUXD_SOCKET_PATH="+opts.socketPath)
	}
	cmd.Env = env
	require.NoError(t, cmd.Start(), "failed to start decoy tymuxd (%s)", bin)

	d := &decoyTymuxd{cmd: cmd, exit: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		close(d.exit)
	}()
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		d.exitedWithin(5 * time.Second)
	})

	if opts.waitAddr != "" {
		wait.RequireEventually(t, func() bool {
			conn, dialErr := net.DialTimeout("tcp", opts.waitAddr, 200*time.Millisecond)
			if dialErr != nil {
				return false
			}
			_ = conn.Close()
			return true
		}, 5*time.Second, 50*time.Millisecond, "decoy tymuxd never started listening on %s", opts.waitAddr)
	} else {
		// No TCP address to poll -- give it a moment to finish its own
		// startup (bind the Unix socket, restore persisted state) before
		// the test proceeds to rely on that socket being held.
		time.Sleep(300 * time.Millisecond)
	}
	return d
}

// readTymuxdPID reads the current tymuxd.pid file the same way StopTymuxd
// does internally, so tests can assert on which PID is recorded without a
// second, drifting implementation of that parsing.
func readTymuxdPID(t *testing.T) int {
	t.Helper()
	configDir, err := config.GetConfigDir()
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(configDir, tymuxdPIDFileName)) //nolint:gosec // configDir + a hardcoded constant, not user input
	require.NoError(t, err)
	pid, err := strconv.Atoi(string(data))
	require.NoError(t, err)
	return pid
}

// shortSocketPath returns a short, test-unique Unix socket path under
// os.TempDir() -- never derived from t.TempDir() directly, which embeds the
// full (often long) test name and reliably exceeds Unix domain sockets'
// kernel-enforced sun_path length limit (~108 bytes on Linux) once a
// filename is appended. Mirrors resolveSocketPath's own short-hash-under-
// os.TempDir() approach (daemon_config.go), keyed on t.Name()+label instead
// of a config dir since these tests construct DaemonConfig.SocketPath
// directly rather than through ResolveDaemonConfig.
func shortSocketPath(t *testing.T, label string) string {
	t.Helper()
	hash := sha256.Sum256([]byte(t.Name() + label))
	dir := filepath.Join(os.TempDir(), "ssq-inttest-"+hex.EncodeToString(hash[:8]))
	require.NoError(t, os.MkdirAll(dir, 0700))
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "tymuxd.sock")
}

func TestIntegration_EnsureDaemonRunning_ColdStart_SpawnsRealTymuxdAndBecomesHealthy(t *testing.T) {
	bin := findTymuxdBinary(t)
	envtest.NewIsolatedStateDir(t)
	withFastRetryBounds(t, 8, 50*time.Millisecond, 300*time.Millisecond)
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir()) // no SocketPath below: isolate tymuxd's own default socket from any real daemon already on this machine
	addr := "http://" + reserveLoopbackAddr(t)

	cfg := DaemonConfig{Addr: addr, BinaryPath: bin}
	ready, err := EnsureDaemonRunning(context.Background(), cfg)
	t.Cleanup(func() { _ = stopTymuxdFn() })

	require.NoError(t, err)
	assert.True(t, ready.Spawned, "a genuine cold start must report Spawned true")
	assert.True(t, checkDaemonHealthy(context.Background(), cfg), "the daemon EnsureDaemonRunning reports ready must actually answer ListSessions")
}

func TestIntegration_EnsureDaemonRunning_ReuseCase_DoesNotRespawnHealthyRealTymuxd(t *testing.T) {
	bin := findTymuxdBinary(t)
	envtest.NewIsolatedStateDir(t)
	withFastRetryBounds(t, 8, 50*time.Millisecond, 300*time.Millisecond)
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir()) // no SocketPath below: isolate tymuxd's own default socket from any real daemon already on this machine
	addr := "http://" + reserveLoopbackAddr(t)
	cfg := DaemonConfig{Addr: addr, BinaryPath: bin}

	_, err := EnsureDaemonRunning(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = stopTymuxdFn() })
	firstPID := readTymuxdPID(t)

	ready, err := EnsureDaemonRunning(context.Background(), cfg)

	require.NoError(t, err)
	assert.False(t, ready.Spawned, "an already-healthy real daemon must be reused, not respawned")
	assert.Equal(t, firstPID, readTymuxdPID(t), "the reuse case must not touch the recorded PID at all")
}

// TestIntegration_EnsureDaemonRunning_RecoversFromStuckAliveDaemon is
// BUG-110's integration-level regression test, reproducing the exact
// production incident this session root-caused: a tymuxd process survives
// while its TCP listener has died (simulated here as a real tymuxd bound to
// the WRONG TYMUXD_ADDR but holding the RIGHT TYMUXD_SOCKET_PATH — from
// EnsureDaemonRunning's caller's perspective, checkDaemonHealthyFn against
// the real cfg.Addr fails exactly the same way a dead listener would).
// Before BUG-110's fix, EnsureDaemonRunning's own spawn attempt would
// collide with this process's held socket lock, die within milliseconds,
// and leave the actual stuck process untouched forever. After the fix, it
// must be killed before the replacement is spawned.
func TestIntegration_EnsureDaemonRunning_RecoversFromStuckAliveDaemon(t *testing.T) {
	bin := findTymuxdBinary(t)
	envtest.NewIsolatedStateDir(t)
	withFastRetryBounds(t, 8, 50*time.Millisecond, 300*time.Millisecond)

	socketPath := shortSocketPath(t, "stuck")
	wrongAddr := reserveLoopbackAddr(t)
	stuck := spawnDecoyTymuxd(t, bin, decoyTymuxdOpts{addr: wrongAddr, socketPath: socketPath, waitAddr: wrongAddr})
	require.NoError(t, writeTymuxdPIDFile(stuck.cmd.Process.Pid), "seed the PID file as if startDaemonAttempt had spawned this process")

	rightAddr := "http://" + reserveLoopbackAddr(t)
	cfg := DaemonConfig{Addr: rightAddr, BinaryPath: bin, SocketPath: socketPath}

	ready, err := EnsureDaemonRunning(context.Background(), cfg)
	t.Cleanup(func() { _ = stopTymuxdFn() })

	require.NoError(t, err, "must recover by killing the stuck predecessor and spawning a healthy replacement")
	assert.True(t, ready.Spawned)
	assert.True(t, checkDaemonHealthy(context.Background(), cfg))
	assert.True(t, stuck.exitedWithin(5*time.Second), "the stuck predecessor must have actually exited (reaped), not just been signaled")
}

// TestIntegration_EnsureDaemonRunning_PortSquatted_FailsLoudly reproduces
// Task 2.1.2d's contract against a REAL non-gRPC listener (a plain TCP
// accept-and-hang), not the injected portListeningFn=true fake
// TestEnsureDaemonRunning_PortSquattedFailsLoudly in supervise_test.go
// uses. Confirms checkDaemonHealthy's real gRPC ListSessions call actually
// fails against a non-tymuxd listener, and portListening's real raw TCP
// dial actually succeeds against it.
func TestIntegration_EnsureDaemonRunning_PortSquatted_FailsLoudly(t *testing.T) {
	bin := findTymuxdBinary(t)
	envtest.NewIsolatedStateDir(t)
	withFastRetryBounds(t, 3, 20*time.Millisecond, 50*time.Millisecond)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			_ = conn // accept and hold open; never speaks gRPC
		}
	}()

	cfg := DaemonConfig{Addr: "http://" + listener.Addr().String(), BinaryPath: bin}

	_, err = EnsureDaemonRunning(context.Background(), cfg)
	t.Cleanup(func() { _ = stopTymuxdFn() })

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTymuxdPortSquatted)
}

// TestIntegration_EnsureDaemonRunning_MissingBinary_FailsPlainly needs no
// real tymuxd at all (a broken/missing binary is the point), so it skips
// findTymuxdBinary's gate.
func TestIntegration_EnsureDaemonRunning_MissingBinary_FailsPlainly(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	withFastRetryBounds(t, 2, 10*time.Millisecond, 20*time.Millisecond)

	cfg := DaemonConfig{Addr: "http://127.0.0.1:1", BinaryPath: "/nonexistent/tymuxd-binary-does-not-exist"}

	_, err := EnsureDaemonRunning(context.Background(), cfg)
	t.Cleanup(func() { _ = stopTymuxdFn() })

	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrTymuxdPortSquatted, "a missing binary is a plain spawn failure, not a port-squat")
}

// TestIntegration_EnsureDaemonRunning_DistinctSocketPaths_AllowConcurrentInstances
// is BUG-106's positive regression test: two independent "instances" (two
// separate config dirs, two separate SocketPaths, two separate ports —
// exactly how two real STAPLER_SQUAD_INSTANCEs differ) must both reach a
// healthy real tymuxd without either colliding on the other's lock.
func TestIntegration_EnsureDaemonRunning_DistinctSocketPaths_AllowConcurrentInstances(t *testing.T) {
	bin := findTymuxdBinary(t)
	withFastRetryBounds(t, 8, 50*time.Millisecond, 300*time.Millisecond)

	envtest.NewIsolatedStateDir(t)
	cfgA := DaemonConfig{Addr: "http://" + reserveLoopbackAddr(t), BinaryPath: bin, SocketPath: shortSocketPath(t, "a")}
	readyA, errA := EnsureDaemonRunning(context.Background(), cfgA)
	require.NoError(t, errA)
	t.Cleanup(func() { _ = stopTymuxdFn() }) // stops whatever instance A's config dir currently names

	envtest.NewIsolatedStateDir(t) // re-sets STAPLER_SQUAD_TEST_DIR — a second, independent config dir
	cfgB := DaemonConfig{Addr: "http://" + reserveLoopbackAddr(t), BinaryPath: bin, SocketPath: shortSocketPath(t, "b")}
	readyB, errB := EnsureDaemonRunning(context.Background(), cfgB)
	require.NoError(t, errB)
	t.Cleanup(func() { _ = stopTymuxdFn() }) // stops whatever instance B's config dir currently names

	assert.True(t, readyA.Spawned)
	assert.True(t, readyB.Spawned)
	assert.True(t, checkDaemonHealthy(context.Background(), cfgA), "instance A must still be healthy after instance B started")
	assert.True(t, checkDaemonHealthy(context.Background(), cfgB))
}

// TestIntegration_EnsureDaemonRunning_SharedDefaultSocket_CollisionSurfacesLoudly
// proves BUG-106's precondition is real (not a hypothetical) and that
// EnsureDaemonRunning never masks it: two configs with NO SocketPath set
// (tymuxd's own default, keyed only by $XDG_RUNTIME_DIR — overridden here to
// an isolated temp dir, never the real machine-wide default) inevitably
// collide on tymuxd's shared lock, and the second EnsureDaemonRunning call
// must fail loudly rather than silently report a fake success.
func TestIntegration_EnsureDaemonRunning_SharedDefaultSocket_CollisionSurfacesLoudly(t *testing.T) {
	bin := findTymuxdBinary(t)
	envtest.NewIsolatedStateDir(t)
	// Generous enough for cfgA's real cold start to succeed (a tight budget
	// starves it before tymuxd finishes its own startup work, e.g. restoring
	// persisted sessions from disk); cfgB is still expected to fail within
	// this same budget since nothing will ever free the shared lock.
	withFastRetryBounds(t, 8, 50*time.Millisecond, 300*time.Millisecond)
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir()) // isolates tymuxd's own default socket location from the real machine-wide one

	cfgA := DaemonConfig{Addr: "http://" + reserveLoopbackAddr(t), BinaryPath: bin} // no SocketPath: tymuxd's own default
	readyA, errA := EnsureDaemonRunning(context.Background(), cfgA)
	require.NoError(t, errA)
	assert.True(t, readyA.Spawned)
	t.Cleanup(func() { _ = stopTymuxdFn() })

	// A separate config dir for B (matching how two real, separate
	// instances/processes would each have their own PID file even while
	// sharing tymuxd's default socket) -- WITHOUT this, B's own PID file is
	// A's, and BUG-110's fix would proactively kill A's perfectly healthy
	// daemon before B's cold-start attempt (since checkDaemonHealthyFn
	// against B's distinct, never-used address naturally fails first),
	// masking the actual collision this test exists to prove.
	envtest.NewIsolatedStateDir(t)
	cfgB := DaemonConfig{Addr: "http://" + reserveLoopbackAddr(t), BinaryPath: bin} // same default socket as cfgA (XDG_RUNTIME_DIR unchanged), different everything else
	_, errB := EnsureDaemonRunning(context.Background(), cfgB)
	t.Cleanup(func() { _ = stopTymuxdFn() }) // stops whatever B's own PID file names, if anything

	require.Error(t, errB, "a second instance sharing tymuxd's default socket must fail, not silently pretend to start a distinct healthy daemon")
	assert.True(t, checkDaemonHealthy(context.Background(), cfgA), "instance A's real daemon must be unaffected by B's failed attempt")
}

// TestIntegration_StartDaemonAttempt_CapturesRealStdoutStderrToOutputLog
// closes the observability gap BUG-106 first flagged and BUG-110 proved
// consequential: tymuxd's own stdout/stderr must land somewhere readable,
// not be discarded, so a future incident doesn't require /proc forensics
// to root-cause.
func TestIntegration_StartDaemonAttempt_CapturesRealStdoutStderrToOutputLog(t *testing.T) {
	bin := findTymuxdBinary(t)
	configDir := envtest.NewIsolatedStateDir(t)
	cfg := DaemonConfig{Addr: "http://" + reserveLoopbackAddr(t), BinaryPath: bin}

	proc, err := startDaemonAttempt(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = proc.Kill() })

	logPath := filepath.Join(configDir, tymuxdOutputLogName)
	wait.RequireEventually(t, func() bool {
		data, readErr := os.ReadFile(logPath)
		return readErr == nil && len(data) > 0
	}, 3*time.Second, 50*time.Millisecond, "expected real tymuxd output to appear at %s", logPath)

	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "tymuxd", "captured output should contain tymuxd's own log lines, not be empty boilerplate")
}
