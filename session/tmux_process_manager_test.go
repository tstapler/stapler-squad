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
	cmdExec := tmux.MockCmdExec{
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			calls++
			return []byte("content-" + string(rune('0'+calls))), nil
		},
		RunFunc:            func(cmd *exec.Cmd) error { return nil },
		CombinedOutputFunc: func(cmd *exec.Cmd) ([]byte, error) { return []byte(""), nil },
	}
	s := tmux.NewTmuxSessionWithDeps(name, "echo", tmux.MakePtyFactory(), cmdExec)
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
