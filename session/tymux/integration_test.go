//go:build integration

package tymux

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTymuxGRPCSession_LiveTymuxd_StartSendKeysCaptureClose is Task
// 2.2.1d's reserved live-daemon tier, exercised through Epic 2.3's real
// transport (tymux.NewRealTransport) and standing Attach stream — the
// first point in this whole implementation where stapler-squad's Go code
// can prove it actually talks to a live tymuxd, not just a fake
// rpcTransport. Every other test in this package runs against
// fakeTransport/fakeAttachStream and proves nothing about the real wire
// protocol.
//
// Requires a real tymuxd already listening (default 127.0.0.1:7419,
// overridable via TYMUXD_ADDR — see transport.go's tymuxdAddr()). Run
// with: go test -tags integration ./session/tymux/...
func TestTymuxGRPCSession_LiveTymuxd_StartSendKeysCaptureClose(t *testing.T) {
	dir := t.TempDir()
	sess := NewTymuxGRPCSession(NewRealTransport(""))

	require.NoError(t, sess.Start(dir), "Start against a live tymuxd — is tymuxd running (see TYMUXD_ADDR)?")
	defer sess.Close()

	assert.NotEmpty(t, sess.GetSessionIdentifier())
	assert.True(t, sess.IsAlive())

	marker := "tymux-epic-2-3-integration-test"
	n, err := sess.SendKeys("echo " + marker + "\r")
	require.NoError(t, err)
	assert.Greater(t, n, 0)

	var content string
	require.Eventually(t, func() bool {
		content, err = sess.CapturePaneContentRaw()
		return err == nil && strings.Contains(content, marker)
	}, 5*time.Second, 100*time.Millisecond, "expected the echoed marker in captured pane content; last content: %q", content)

	require.NoError(t, sess.Close())
	assert.False(t, sess.IsAlive())
}

// --- Task 2.5.3c: live daemon-restart drill ---
//
// Unlike TestTymuxGRPCSession_LiveTymuxd_StartSendKeysCaptureClose above
// (which assumes an out-of-band, already-running tymuxd per Story 2.2.6's
// documented scope decision), Story 2.5.3's daemon-restart contract can
// only be exercised by actually killing and restarting tymuxd mid-test —
// something no already-running, possibly shared daemon can safely be
// subjected to. So this test spawns and tears down its own tymuxd
// subprocess(es) directly, entirely self-contained (isolated port,
// isolated XDG_STATE_HOME), rather than reusing tymuxdAddr()'s
// TYMUXD_ADDR-or-default resolution.

// findTymuxdBinary locates a built tymuxd binary: TYMUXD_BIN if set, else
// the sibling tymux checkout's target/debug/tymuxd (the go.mod replace
// directive's own convention: `../tymux`), else whatever's on PATH. Skips
// the test (not a failure) if none is found, since a from-scratch
// checkout of stapler-squad alone has no reason to have built tymuxd yet.
func findTymuxdBinary(t *testing.T) string {
	t.Helper()
	if v := os.Getenv("TYMUXD_BIN"); v != "" {
		return v
	}
	wd, err := os.Getwd()
	require.NoError(t, err)
	candidate := filepath.Join(wd, "..", "..", "..", "tymux", "target", "debug", "tymuxd")
	if abs, err := filepath.Abs(candidate); err == nil {
		if _, statErr := os.Stat(abs); statErr == nil {
			return abs
		}
	}
	if p, err := exec.LookPath("tymuxd"); err == nil {
		return p
	}
	t.Skip("no tymuxd binary found (set TYMUXD_BIN, or `cargo build` the sibling tymux repo) — skipping Task 2.5.3c live daemon-restart drill")
	return ""
}

// liveTymuxd manages one spawned tymuxd subprocess, isolated from any
// real daemon on the machine via its own port and XDG_STATE_HOME.
type liveTymuxd struct {
	cmd  *exec.Cmd
	addr string
}

// reserveLoopbackAddr picks a free loopback port once, up front, so both
// tymuxd instances in a restart drill can be started on the SAME address
// — the test's single *tymuxGRPCSession is constructed with one fixed
// address baked into its transport at NewRealTransport() time, so a
// restart that lands on a different port would be unreachable from
// ReconnectLoop's redials, not merely "the daemon restarted."
func reserveLoopbackAddr(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := listener.Addr().String()
	require.NoError(t, listener.Close()) // release the port for tymuxd's own bind — a small TOCTOU window, acceptable for a test
	return addr
}

// startLiveTymuxd spawns tymuxd against xdgStateHome (shared across
// restarts within one test, so the second instance loads the first's
// persisted records — exactly the daemon-restart contract Story 2.5.3
// documents) and addr (also shared across restarts — see
// reserveLoopbackAddr).
func startLiveTymuxd(t *testing.T, bin, xdgStateHome, addr string) *liveTymuxd {
	t.Helper()
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(),
		"TYMUXD_ADDR="+addr,
		"XDG_STATE_HOME="+xdgStateHome,
		"RUST_LOG=warn",
	)
	require.NoError(t, cmd.Start(), "failed to start tymuxd at %s", bin)

	lt := &liveTymuxd{cmd: cmd, addr: addr}
	require.Eventually(t, func() bool {
		conn, dialErr := net.DialTimeout("tcp", lt.addr, 200*time.Millisecond)
		if dialErr != nil {
			return false
		}
		_ = conn.Close()
		return true
	}, 5*time.Second, 50*time.Millisecond, "tymuxd never started listening on %s", lt.addr)
	return lt
}

// stop kills the subprocess and waits for it to exit — abruptly (SIGKILL,
// not a graceful shutdown), matching the "tymuxd just died" scenario
// Story 2.5.3 is about, not a clean restart.
func (lt *liveTymuxd) stop(t *testing.T) {
	t.Helper()
	if lt.cmd.Process == nil {
		return
	}
	_ = lt.cmd.Process.Kill()
	_ = lt.cmd.Wait()
}

var pidMarkerRe = regexp.MustCompile(`marker-(\d+)`)

// extractPID pulls the numeric suffix `echo marker-$$` printed into the
// pane's captured content, so the test can compare PIDs without
// GetPanePID() — deliberately unsupported on this backend
// (ErrNotSupportedOnTymuxBackend, Story 2.2.5).
func extractPID(t *testing.T, content string) string {
	t.Helper()
	m := pidMarkerRe.FindStringSubmatch(content)
	require.NotEmptyf(t, m, "expected a marker-<pid> line in captured content, got: %q", content)
	return m[1]
}

// TestBackendTymux_DaemonRestart_RevivesWithFreshProcess_SurfacedDistinctly
// is Task 2.5.3c: start a session, kill and restart tymuxd against the
// same persisted state, let ReconnectLoop observe the drop and reconnect,
// and assert (a) the pane's underlying process is a different PID than
// before the restart — confirming ReviveSession's actual
// respawn-not-reattach behavior (Story 2.5.3's stated contract, verified
// against crates/tymux-core/src/engine.rs's revive_session, which
// unconditionally calls Pane::spawn_with_id) — and (b) BackendRestarted()
// surfaces Task 2.5.3b's distinct state rather than the session silently
// reporting plain IsAlive() == true with no memory of the restart.
func TestBackendTymux_DaemonRestart_RevivesWithFreshProcess_SurfacedDistinctly(t *testing.T) {
	bin := findTymuxdBinary(t)
	xdgStateHome := t.TempDir()
	addr := reserveLoopbackAddr(t)

	daemon1 := startLiveTymuxd(t, bin, xdgStateHome, addr)
	defer daemon1.stop(t) // no-op if already stopped below

	sess := NewTymuxGRPCSession(NewRealTransport("http://" + daemon1.addr))
	dir := t.TempDir()
	require.NoError(t, sess.Start(dir))
	defer sess.Close()

	concrete := sess.(*tymuxGRPCSession)
	concrete.mu.Lock()
	concrete.reconnectBaseDelay = 100 * time.Millisecond
	concrete.reconnectMaxDelay = 500 * time.Millisecond
	concrete.reconnectMaxAttempts = 30
	concrete.mu.Unlock()

	_, err := sess.SendKeys("echo marker-$$\r")
	require.NoError(t, err)
	var before string
	require.Eventually(t, func() bool {
		before, err = sess.CapturePaneContentRaw()
		return err == nil && pidMarkerRe.MatchString(before)
	}, 5*time.Second, 100*time.Millisecond, "expected the pre-restart marker; last content: %q", before)
	pidBefore := extractPID(t, before)

	// Kill tymuxd abruptly and restart it against the SAME persisted
	// state dir — the daemon-restart contract: PersistedPaneRecord
	// survives on disk, the live OS process does not.
	daemon1.stop(t)
	daemon2 := startLiveTymuxd(t, bin, xdgStateHome, addr)
	defer daemon2.stop(t)

	// ReconnectLoop is already running in the background (started by
	// Start() above); it detects the drop on its own. Give it enough
	// time to dial the dead pane (FailedPrecondition), call
	// ReviveSession, and reattach against daemon2.
	done, err := sess.Attach()
	require.NoError(t, err)
	select {
	case <-done:
		t.Fatal("standing stream's channel closed — a daemon-restart drop should reconnect transparently, not give up")
	case <-time.After(6 * time.Second):
	}

	require.Eventually(t, func() bool {
		restarted, _ := concrete.BackendRestarted()
		return restarted
	}, 5*time.Second, 100*time.Millisecond, "BackendRestarted() must report true after the live daemon-restart drill")

	_, err = sess.SendKeys("echo marker-$$\r")
	require.NoError(t, err)
	var after string
	require.Eventually(t, func() bool {
		after, err = sess.CapturePaneContentRaw()
		return err == nil && pidMarkerRe.MatchString(after) && !strings.Contains(after, "marker-"+pidBefore)
	}, 8*time.Second, 100*time.Millisecond, "expected a fresh post-restart marker distinct from the pre-restart one; last content: %q", after)
	pidAfter := extractPID(t, after)

	assert.NotEqual(t, pidBefore, pidAfter, "the revived pane must be a fresh process (different PID), never the original")
}
