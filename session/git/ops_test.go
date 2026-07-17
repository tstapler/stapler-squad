package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// runGit runs a git command in dir and fails the test on error.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := safeexec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %s failed: %s", strings.Join(args, " "), out)
	return string(out)
}

// cloneTestRepo clones originDir into a fresh temp directory, giving the clone an
// "origin" remote that MergeMainIntoWorktree can fetch from. The clone starts on
// "main" (setupTestRepo's default branch), matching a real backlog work session's
// worktree, which always starts from the shared repo's checked-out branch.
func cloneTestRepo(t *testing.T, originDir string) string {
	t.Helper()
	workDir := t.TempDir()
	// t.TempDir() already creates the directory; `git clone` requires an empty or
	// missing target, so clone into a subdirectory instead.
	cloneDir := filepath.Join(workDir, "clone")
	runGit(t, workDir, "clone", originDir, cloneDir)
	runGit(t, cloneDir, "config", "user.email", "test@example.com")
	runGit(t, cloneDir, "config", "user.name", "Test User")
	return cloneDir
}

// TestMergeMainIntoWorktree_should_ReportUpToDate_When_BranchAlreadyHasLatestMain
// verifies the no-op case (Task 2.1.6d): a branch that already contains everything on
// main must not be reported as merged, and must not create a spurious merge commit.
func TestMergeMainIntoWorktree_should_ReportUpToDate_When_BranchAlreadyHasLatestMain(t *testing.T) {
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	runGit(t, work, "checkout", "-b", "feature")

	beforeSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))

	result, err := MergeMainIntoWorktree(work, "main")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.UpToDate, "branch already contains main's tip; must be reported up to date")
	assert.False(t, result.Merged)
	assert.False(t, result.Conflicted)

	afterSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))
	assert.Equal(t, beforeSHA, afterSHA, "an up-to-date merge must not create a commit")
}

// TestMergeMainIntoWorktree_should_MergeCleanly_When_MainHasNewNonConflictingCommits
// verifies the preventive-sync case (Task 2.1.6d): new commits landed on main after the
// branch was created must be pulled in via a clean merge when they don't conflict.
func TestMergeMainIntoWorktree_should_MergeCleanly_When_MainHasNewNonConflictingCommits(t *testing.T) {
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	runGit(t, work, "checkout", "-b", "feature")

	// The feature branch makes its own unrelated change.
	require.NoError(t, os.WriteFile(filepath.Join(work, "feature.txt"), []byte("feature work\n"), 0o644))
	runGit(t, work, "add", "feature.txt")
	runGit(t, work, "commit", "-m", "feature work")

	// Main moves forward with an unrelated fix, simulating drift while the fix
	// session's branch sat open as a PR.
	require.NoError(t, os.WriteFile(filepath.Join(origin, "main-fix.txt"), []byte("fix on main\n"), 0o644))
	runGit(t, origin, "add", "main-fix.txt")
	runGit(t, origin, "commit", "-m", "fix landed on main")

	result, err := MergeMainIntoWorktree(work, "main")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Merged, "new non-conflicting commits on main must be merged in")
	assert.False(t, result.UpToDate)
	assert.False(t, result.Conflicted)

	// Both files must now be present in the worktree.
	assert.FileExists(t, filepath.Join(work, "feature.txt"))
	assert.FileExists(t, filepath.Join(work, "main-fix.txt"))

	status := runGit(t, work, "status", "--porcelain")
	assert.Empty(t, strings.TrimSpace(status), "worktree must be clean after a successful merge")
}

// TestMergeMainIntoWorktree_should_ReportConflictedAndAbort_When_MainAndBranchTouchSameLines
// verifies the conflict path (Task 2.1.6d): when the branch and main touched the same
// lines, the merge must be aborted (leaving the worktree clean) and the conflicting
// file paths reported so the caller can fold them into the fix context.
func TestMergeMainIntoWorktree_should_ReportConflictedAndAbort_When_MainAndBranchTouchSameLines(t *testing.T) {
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	runGit(t, work, "checkout", "-b", "feature")

	// The feature branch edits README.md.
	require.NoError(t, os.WriteFile(filepath.Join(work, "README.md"), []byte("# Feature Edit\n"), 0o644))
	runGit(t, work, "add", "README.md")
	runGit(t, work, "commit", "-m", "feature edits README")

	// Main edits the same line differently.
	require.NoError(t, os.WriteFile(filepath.Join(origin, "README.md"), []byte("# Main Edit\n"), 0o644))
	runGit(t, origin, "add", "README.md")
	runGit(t, origin, "commit", "-m", "main edits README")

	beforeSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))

	result, err := MergeMainIntoWorktree(work, "main")
	require.NoError(t, err, "a real conflict must be reported via the result, not returned as an error")
	require.NotNil(t, result)
	assert.True(t, result.Conflicted)
	assert.False(t, result.UpToDate)
	assert.False(t, result.Merged)
	assert.Equal(t, []string{"README.md"}, result.ConflictedFiles)

	// The merge must have been aborted: HEAD unchanged, no lingering MERGE_HEAD, clean tree.
	afterSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))
	assert.Equal(t, beforeSHA, afterSHA, "a conflicted merge must be aborted, leaving HEAD untouched")
	assert.NoFileExists(t, filepath.Join(work, ".git", "MERGE_HEAD"))
	status := runGit(t, work, "status", "--porcelain")
	assert.Empty(t, strings.TrimSpace(status), "worktree must be restored to clean after aborting a conflicted merge")
}

// TestMergeMainIntoWorktree_should_ReturnError_When_FetchFails verifies that a fetch
// failure (bad/unreachable "origin" remote) is propagated as an error rather than
// silently reported as any MergeMainResult state — callers (syncPRBranchWithMain) rely
// on a non-nil error to distinguish "nothing to tell the caller" from "something broke".
func TestMergeMainIntoWorktree_should_ReturnError_When_FetchFails(t *testing.T) {
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	runGit(t, work, "checkout", "-b", "feature")

	// Point "origin" at a path that isn't a git repo at all, so `git fetch origin main`
	// fails outright rather than just finding nothing new.
	runGit(t, work, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "does-not-exist"))

	result, err := MergeMainIntoWorktree(work, "main")
	require.Error(t, err, "an unreachable origin must surface as an error, not a result")
	assert.Nil(t, result)
}

// TestMergeMainIntoWorktree_should_ReturnError_When_MergeFailsForNonConflictReason
// verifies that a merge failure NOT caused by a content conflict (here: uncommitted
// local changes that the merge would overwrite) is propagated as an error and does not
// get misreported as MergeMainResult.Conflicted — aborting a merge that never started
// would mask the real problem.
func TestMergeMainIntoWorktree_should_ReturnError_When_MergeFailsForNonConflictReason(t *testing.T) {
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	runGit(t, work, "checkout", "-b", "feature")

	// Main moves forward so there's something to merge.
	require.NoError(t, os.WriteFile(filepath.Join(origin, "main-fix.txt"), []byte("fix on main\n"), 0o644))
	runGit(t, origin, "add", "main-fix.txt")
	runGit(t, origin, "commit", "-m", "fix landed on main")

	// Dirty the worktree on a path the incoming merge will also touch, so git refuses
	// the merge with "would be overwritten" rather than a content conflict.
	require.NoError(t, os.WriteFile(filepath.Join(work, "main-fix.txt"), []byte("uncommitted local edit\n"), 0o644))

	result, err := MergeMainIntoWorktree(work, "main")
	require.Error(t, err, "a non-conflict merge failure must be returned as an error")
	assert.Nil(t, result)

	// The offending uncommitted change must be left exactly as-is (no destructive abort
	// of a merge that never actually started).
	content, readErr := os.ReadFile(filepath.Join(work, "main-fix.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "uncommitted local edit\n", string(content))
}
