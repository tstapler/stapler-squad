package session

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// newCapturePaneCountingTmuxSession builds a real *tmux.TmuxSession backed by a mock
// executor whose Output() returns "content-N" for the Nth invocation, so a test can
// distinguish "served from cache" (call count doesn't advance) from "re-fetched"
// (call count advances) without needing a real tmux server.
func newCapturePaneCountingTmuxSession(t *testing.T, name string) (*tmux.TmuxSession, *int) {
	t.Helper()
	calls := 0
	// sessionExists is filled in after s is constructed below, so the
	// CombinedOutputFunc closure (built first) reads it by reference rather
	// than needing to duplicate tmux's own name-sanitization logic.
	var sessionExists string
	cmdExec := tmux.MockCmdExec{
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			calls++
			return []byte("content-" + string(rune('0'+calls))), nil
		},
		RunFunc: func(cmd *exec.Cmd) error { return nil },
		// Reports the session as existing to tmux's list-sessions, so
		// CapturePaneContentContext's DoesSessionExist() guard (which the
		// underlying capture call now checks before forking) doesn't
		// short-circuit before this test's mocked capture-pane call runs.
		CombinedOutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte(sessionExists), nil
		},
	}
	s := tmux.NewTmuxSessionWithDeps(name, "echo", tmux.MakePtyFactory(), cmdExec)
	sessionExists = s.GetSanitizedName()
	return s, &calls
}

// TestTmuxProcessManager_CapturePaneContentContext_ServesFromCacheWithinTTL proves the
// capturePaneCacheTTL cache (item 7 of PR #548's review: zero coverage for
// CapturePaneContentContext's branching) actually short-circuits the underlying
// subprocess call on a cache hit, rather than merely documenting that it should.
func TestTmuxProcessManager_CapturePaneContentContext_ServesFromCacheWithinTTL(t *testing.T) {
	session, calls := newCapturePaneCountingTmuxSession(t, "cache-hit-test")
	tm := &TmuxProcessManager{}
	tm.SetSession(session)

	first, err := tm.CapturePaneContentContext(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, *calls, "first call should be a cache miss and hit the subprocess exactly once")

	second, err := tm.CapturePaneContentContext(context.Background())
	require.NoError(t, err)
	require.Equal(t, first, second, "a call within capturePaneCacheTTL must return the cached content unchanged")
	require.Equal(t, 1, *calls, "a cache hit must not invoke the subprocess again")
}

// TestTmuxProcessManager_CapturePaneContentContext_RefetchesAfterTTLExpires proves the
// cache is not permanent: once capturePaneCacheTTL elapses, the next call must go back
// to the subprocess and observe fresh content instead of serving stale cached output
// forever.
func TestTmuxProcessManager_CapturePaneContentContext_RefetchesAfterTTLExpires(t *testing.T) {
	session, calls := newCapturePaneCountingTmuxSession(t, "cache-miss-test")
	tm := &TmuxProcessManager{}
	tm.SetSession(session)

	first, err := tm.CapturePaneContentContext(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, *calls)

	// Backdate the cache timestamp instead of sleeping capturePaneCacheTTL (1s) — same
	// effect, deterministic, and doesn't slow the suite down.
	tm.mu.Lock()
	tm.captureContentAt = time.Now().Add(-2 * capturePaneCacheTTL)
	tm.mu.Unlock()

	second, err := tm.CapturePaneContentContext(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, *calls, "an expired cache entry must trigger a fresh subprocess call")
	require.NotEqual(t, first, second, "content after cache expiry should reflect the fresh subprocess call")
}

// fakeTPMPaneSettleChecker is a scripted tpmPaneSettleChecker: each call to
// HasUpdated pops the next value off updates (repeating the last one once
// exhausted), so waitForPaneSettleTPM can be tested without a real tmux
// session (AC3: TmuxProcessManager.SendPromptWithEnter's fixed 100ms sleep
// replaced with event-driven settle detection).
type fakeTPMPaneSettleChecker struct {
	updates []bool
	calls   int
}

func (f *fakeTPMPaneSettleChecker) HasUpdated() (bool, bool, string) {
	idx := f.calls
	f.calls++
	if idx >= len(f.updates) {
		idx = len(f.updates) - 1
	}
	return f.updates[idx], false, ""
}

// TestWaitForPaneSettleTPM_ReturnsAsSoonAsSettled proves the replacement for
// the fixed 100ms sleep returns as soon as two consecutive "no change" polls
// are observed, rather than waiting out the full maxWait window every time.
func TestWaitForPaneSettleTPM_ReturnsAsSoonAsSettled(t *testing.T) {
	checker := &fakeTPMPaneSettleChecker{updates: []bool{true, true, false, false, false}}

	start := time.Now()
	waitForPaneSettleTPM(checker, time.Millisecond, time.Second)
	elapsed := time.Since(start)

	if elapsed >= time.Second {
		t.Errorf("waitForPaneSettleTPM took %v — should have returned once settled, not waited out the full window", elapsed)
	}
	if checker.calls < 4 {
		t.Errorf("HasUpdated called %d times, want at least 4 (2 changing + 2 settled)", checker.calls)
	}
}

// TestWaitForPaneSettleTPM_GivesUpAfterMaxWait_WhenPaneNeverSettles proves the
// best-effort fallback: a pane that never stops changing doesn't hang forever.
func TestWaitForPaneSettleTPM_GivesUpAfterMaxWait_WhenPaneNeverSettles(t *testing.T) {
	checker := &fakeTPMPaneSettleChecker{updates: []bool{true}}

	start := time.Now()
	waitForPaneSettleTPM(checker, time.Millisecond, 20*time.Millisecond)
	elapsed := time.Since(start)

	if elapsed > 200*time.Millisecond {
		t.Errorf("waitForPaneSettleTPM took %v, want it to give up close to the 20ms maxWait", elapsed)
	}
}

// primePanePID must cache the pane PID up front so the orphan guard still has
// a PID to check after the tmux server dies (criterion: uncached-PID case).
func TestTmuxProcessManager_PrimePanePID_CachesWhenUnset(t *testing.T) {
	var sessionExists string
	cmdExec := tmux.MockCmdExec{
		OutputFunc:         func(cmd *exec.Cmd) ([]byte, error) { return []byte("4242\n"), nil },
		RunFunc:            func(cmd *exec.Cmd) error { return nil },
		CombinedOutputFunc: func(cmd *exec.Cmd) ([]byte, error) { return []byte(sessionExists), nil },
	}
	s := tmux.NewTmuxSessionWithDeps("prime-pid-test", "echo", tmux.MakePtyFactory(), cmdExec)
	sessionExists = s.GetSanitizedName()
	tm := &TmuxProcessManager{}
	tm.SetSession(s)
	require.False(t, tm.panePIDSet.Load())

	tm.primePanePID()

	require.True(t, tm.panePIDSet.Load(), "primePanePID must populate the pane PID cache when unset")
	require.Equal(t, int32(4242), tm.panePIDCached.Load())
}
