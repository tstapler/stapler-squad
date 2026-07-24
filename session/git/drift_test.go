package git

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// commitOnOrigin adds n commits to origin, simulating unrelated upstream activity
// landing on main while a work session's branch sits open.
func commitOnOrigin(t *testing.T, origin string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		fname := fmt.Sprintf("upstream-%d.txt", i)
		require.NoError(t, os.WriteFile(filepath.Join(origin, fname), []byte("upstream\n"), 0o644))
		runGit(t, origin, "add", fname)
		runGit(t, origin, "commit", "-m", fmt.Sprintf("upstream commit %d", i))
	}
}

// TestBehindOriginMain_should_ReportZero_When_BranchIsCurrent verifies the
// no-drift baseline: a freshly branched worktree with nothing new on main reports
// zero commits behind.
func TestBehindOriginMain_should_ReportZero_When_BranchIsCurrent(t *testing.T) {
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	runGit(t, work, "checkout", "-b", "feature")

	behind, err := BehindOriginMain(work, "main")
	require.NoError(t, err)
	assert.Equal(t, 0, behind)
}

// TestBehindOriginMain_should_CountFreshlyPushedUpstreamCommits_When_BranchNeverFetched
// verifies BehindOriginMain always fetches for itself — it must see commits that
// landed on origin after the worktree's clone, without requiring any prior fetch by
// the caller.
func TestBehindOriginMain_should_CountFreshlyPushedUpstreamCommits_When_BranchNeverFetched(t *testing.T) {
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	runGit(t, work, "checkout", "-b", "feature")

	commitOnOrigin(t, origin, 5)

	behind, err := BehindOriginMain(work, "main")
	require.NoError(t, err)
	assert.Equal(t, 5, behind, "must reflect origin's current state even though work never fetched before this call")
}

// TestBehindOriginMain_should_ReturnError_When_OriginUnreachable verifies a fetch
// failure surfaces as an error rather than silently reporting zero drift.
func TestBehindOriginMain_should_ReturnError_When_OriginUnreachable(t *testing.T) {
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	runGit(t, work, "checkout", "-b", "feature")
	runGit(t, work, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "does-not-exist"))

	_, err := BehindOriginMain(work, "main")
	require.Error(t, err)
}

// TestEnsureBranchSyncedWithMain_should_ReturnOkWithoutSyncing_When_UnderThreshold
// verifies the common case: a branch behind main but not past the threshold is left
// alone entirely (no merge attempted).
func TestEnsureBranchSyncedWithMain_should_ReturnOkWithoutSyncing_When_UnderThreshold(t *testing.T) {
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	runGit(t, work, "checkout", "-b", "feature")
	commitOnOrigin(t, origin, 3)
	beforeSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))

	ok, summary := EnsureBranchSyncedWithMain(work, "feature", "main", 50)
	assert.True(t, ok)
	assert.Empty(t, summary)

	afterSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))
	assert.Equal(t, beforeSHA, afterSHA, "must not touch the worktree when under the drift threshold")
}

// TestEnsureBranchSyncedWithMain_should_SyncAndReturnOk_When_DriftedButMergeable is
// the core BUG-044 happy path: a branch that has drifted past the threshold, but
// merges cleanly, is resynced and pushed automatically so review proceeds on an
// up-to-date branch instead of an inflated diff.
func TestEnsureBranchSyncedWithMain_should_SyncAndReturnOk_When_DriftedButMergeable(t *testing.T) {
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	runGit(t, work, "checkout", "-b", "feature")

	require.NoError(t, os.WriteFile(filepath.Join(work, "feature.txt"), []byte("feature work\n"), 0o644))
	runGit(t, work, "add", "feature.txt")
	runGit(t, work, "commit", "-m", "feature work")

	commitOnOrigin(t, origin, 10) // past a low test threshold, non-conflicting

	ok, summary := EnsureBranchSyncedWithMain(work, "feature", "main", 5)
	assert.True(t, ok, "a clean merge must unblock review: %s", summary)
	assert.Empty(t, summary)

	behindAfter, err := BehindOriginMain(work, "main")
	require.NoError(t, err)
	assert.Equal(t, 0, behindAfter, "worktree must now contain everything from main")

	// The push must have actually landed on origin, not just the local worktree —
	// otherwise the next reviewer/fix session sees the sync disappear.
	originFeatureTip := strings.TrimSpace(runGit(t, origin, "rev-parse", "feature"))
	workFeatureTip := strings.TrimSpace(runGit(t, work, "rev-parse", "feature"))
	assert.Equal(t, workFeatureTip, originFeatureTip, "the merge commit must be pushed to origin")
}

// TestEnsureBranchSyncedWithMain_should_BlockWithConflictDetails_When_DriftedAndConflicting
// is the regression guard for the misleading-diagnosis half of BUG-044: when the
// branch has drifted past the threshold AND a real conflict blocks the auto-sync,
// the caller must not be told "ok" and sent on to review an inflated diff — it must
// be told exactly why, and handed the conflicted file list.
func TestEnsureBranchSyncedWithMain_should_BlockWithConflictDetails_When_DriftedAndConflicting(t *testing.T) {
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	runGit(t, work, "checkout", "-b", "feature")

	require.NoError(t, os.WriteFile(filepath.Join(work, "README.md"), []byte("# Feature Edit\n"), 0o644))
	runGit(t, work, "add", "README.md")
	runGit(t, work, "commit", "-m", "feature edits README")

	require.NoError(t, os.WriteFile(filepath.Join(origin, "README.md"), []byte("# Main Edit\n"), 0o644))
	runGit(t, origin, "add", "README.md")
	runGit(t, origin, "commit", "-m", "main edits README")
	commitOnOrigin(t, origin, 5) // push behind past threshold too

	beforeSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))

	ok, summary := EnsureBranchSyncedWithMain(work, "feature", "main", 3)
	assert.False(t, ok, "a real conflict must block review rather than letting it proceed on stale/half-synced content")
	assert.Contains(t, summary, "README.md", "the conflicted file must be named so a fix session knows exactly what to resolve")
	assert.Contains(t, summary, "feature", "the branch name must be named for an operator reading the message out of context")
	assert.Contains(t, summary, "BUG-044")

	// The aborted merge must leave the worktree exactly as it was — no half-merged
	// state left behind for the next thing that touches it.
	afterSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))
	assert.Equal(t, beforeSHA, afterSHA)
	status := runGit(t, work, "status", "--porcelain")
	assert.Empty(t, strings.TrimSpace(status))
}

// TestEnsureBranchSyncedWithMain_should_ReturnOk_When_WorktreePathEmpty verifies the
// nil-worktree guard (directory-mode sessions, or a worktree row that's been cleaned
// up) fails open rather than erroring.
func TestEnsureBranchSyncedWithMain_should_ReturnOk_When_WorktreePathEmpty(t *testing.T) {
	ok, summary := EnsureBranchSyncedWithMain("", "feature", "main", 50)
	assert.True(t, ok)
	assert.Empty(t, summary)
}

// TestEnsureBranchSyncedWithMain_should_ReturnOk_When_DriftDetectionFails verifies
// the fail-open contract: if BehindOriginMain itself cannot determine drift (here:
// unreachable origin), EnsureBranchSyncedWithMain must not block review on a broken
// detector.
func TestEnsureBranchSyncedWithMain_should_ReturnOk_When_DriftDetectionFails(t *testing.T) {
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	runGit(t, work, "checkout", "-b", "feature")
	runGit(t, work, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "does-not-exist"))

	ok, summary := EnsureBranchSyncedWithMain(work, "feature", "main", 50)
	assert.True(t, ok, "a broken drift detector must fail open, never block review")
	assert.Empty(t, summary)
}
