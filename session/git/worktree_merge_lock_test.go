package git

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNativeMerge_And_NativeSetup_SerializeThroughSameLock proves a native merge and a
// native Setup() against the same repoPath serialize through the same repoWorktreeLock
// registry entry — mirroring TestSetupRemove_MixedImplementations_SerializeThroughSameLock's
// proof strategy (worktree_ops_test.go).
//
// Proof strategy: the test acquires the shared repoWorktreeLock's intra-process mutex
// itself, before launching either call, then confirms both are still blocked (neither
// returned) after a generous wait — not a timing heuristic, since mu is a genuine
// sync.Mutex (see the mirrored test's doc comment for the full reasoning).
func TestNativeMerge_And_NativeSetup_SerializeThroughSameLock(t *testing.T) {
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	runGit(t, work, "checkout", "-b", "feature")

	require.NoError(t, os.WriteFile(filepath.Join(origin, "main-fix.txt"), []byte("fix on main\n"), 0o644))
	runGit(t, origin, "add", "main-fix.txt")
	runGit(t, origin, "commit", "-m", "fix landed on main")

	wt, _, err := NewGitWorktreeWithBranch(work, "sess-merge-lock", "sess-merge-lock-branch")
	require.NoError(t, err)

	lockInstance, err := lockForRepo(work)
	require.NoError(t, err)

	lockInstance.mu.Lock()

	errs := make([]error, 2)
	done := make(chan int, 2)
	go func() {
		errs[0] = wt.Setup()
		done <- 0
	}()
	go func() {
		_, errs[1] = MergeMainIntoWorktree(work, "main")
		done <- 1
	}()

	select {
	case which := <-done:
		t.Fatalf("call %d completed while the test held repoWorktreeLock.mu externally -- native merge/setup dispatch is bypassing WithRepoWorktreeLock", which)
	case <-time.After(150 * time.Millisecond):
		// Expected: both goroutines are blocked on mu.Lock() inside WithRepoWorktreeLock.
	}

	lockInstance.mu.Unlock()

	<-done
	<-done
	require.NoError(t, errs[0], "native Setup() must not fail")
	require.NoError(t, errs[1], "native merge must not fail")
	defer func() { _ = wt.Cleanup() }()

	lockAfter, err := lockForRepo(work)
	require.NoError(t, err)
	assert.Same(t, lockInstance, lockAfter, "both calls must resolve to the same repoWorktreeLock singleton for repoPath")
}
