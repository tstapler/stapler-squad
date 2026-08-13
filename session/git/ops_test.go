package git

import (
	"context"
	"fmt"
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

// TestResolveOriginBranchSHA_ReturnsFetchedTip_When_OriginHasAdvanced is the regression
// test for the stale-HEAD backlog-spawn bug's fetch primitive: it must return origin's
// true current tip, not whatever repoPath's cached origin/main ref happened to be at
// clone time (the same staleness that let a new backlog work session branch from a
// days-old checkout instead of main's real tip).
func TestResolveOriginBranchSHA_ReturnsFetchedTip_When_OriginHasAdvanced(t *testing.T) {
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)

	staleSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "origin/main"))

	require.NoError(t, os.WriteFile(filepath.Join(origin, "advance.txt"), []byte("advance\n"), 0o644))
	runGit(t, origin, "add", "advance.txt")
	runGit(t, origin, "commit", "-m", "advance origin")
	newTip := strings.TrimSpace(runGit(t, origin, "rev-parse", "HEAD"))
	require.NotEqual(t, staleSHA, newTip, "test setup must advance origin past the clone's cached ref")

	sha, err := ResolveOriginBranchSHA(work, "main")
	require.NoError(t, err)
	assert.Equal(t, newTip, sha, "must return origin's freshly-fetched tip, not the clone's stale cached ref")
}

// TestResolveOriginBranchSHA_ReturnsError_When_FetchFails verifies that an unreachable
// origin surfaces as an error rather than silently returning a stale or empty SHA —
// CreateBacklogWorktree relies on this to know when to fall back to the old
// ambient-HEAD behavior instead of branching from a bogus commit.
func TestResolveOriginBranchSHA_ReturnsError_When_FetchFails(t *testing.T) {
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	runGit(t, work, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "does-not-exist"))

	sha, err := ResolveOriginBranchSHA(work, "main")
	require.Error(t, err)
	assert.Empty(t, sha)
}

// TestIsCommitOnMain_should_ReturnTrue_When_CommitIsMainTipLocally verifies the
// simplest case: a commit that IS main's own local tip is trivially its own ancestor.
func TestIsCommitOnMain_should_ReturnTrue_When_CommitIsMainTipLocally(t *testing.T) {
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	mainTip := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))

	onMain, err := IsCommitOnMain(work, "main", mainTip)
	require.NoError(t, err)
	assert.True(t, onMain)
}

// TestIsCommitOnMain_should_ReturnFalse_When_CommitOnlyExistsOnUnmergedBranch verifies
// the core gap this function closes: a commit that was made on a feature branch and
// never merged anywhere must not be reported as shipped.
func TestIsCommitOnMain_should_ReturnFalse_When_CommitOnlyExistsOnUnmergedBranch(t *testing.T) {
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	runGit(t, work, "checkout", "-b", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(work, "feature.txt"), []byte("wip\n"), 0o644))
	runGit(t, work, "add", "feature.txt")
	runGit(t, work, "commit", "-m", "feature work, never merged")
	featureSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))

	onMain, err := IsCommitOnMain(work, "main", featureSHA)
	require.NoError(t, err)
	assert.False(t, onMain, "an unmerged feature commit must not read as shipped")
}

// TestIsCommitOnMain_should_ReturnTrue_When_CommitMergedRemotelyButNotPulledLocally
// verifies the "merged remotely" half of the fix (Tyler: "merged can happen remotely
// or locally... it effectively needs to be on main either locally or remotely") — a PR
// merged on GitHub advances origin's main, but the local clone's own main branch isn't
// automatically updated. IsCommitOnMain must still detect it via its own origin fetch.
func TestIsCommitOnMain_should_ReturnTrue_When_CommitMergedRemotelyButNotPulledLocally(t *testing.T) {
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)

	// Advance the "remote" (origin) past what "work" has locally — simulating a PR
	// merged on GitHub that this local clone hasn't fetched/pulled yet.
	require.NoError(t, os.WriteFile(filepath.Join(origin, "shipped.txt"), []byte("shipped via PR\n"), 0o644))
	runGit(t, origin, "add", "shipped.txt")
	runGit(t, origin, "commit", "-m", "merged via PR on GitHub")
	remoteSHA := strings.TrimSpace(runGit(t, origin, "rev-parse", "HEAD"))

	localMainTip := strings.TrimSpace(runGit(t, work, "rev-parse", "main"))
	require.NotEqual(t, remoteSHA, localMainTip, "sanity check: work's local main must NOT already have this commit")

	onMain, err := IsCommitOnMain(work, "main", remoteSHA)
	require.NoError(t, err)
	assert.True(t, onMain, "a commit merged remotely to origin/main must be detected even before a local pull")
}

// TestIsCommitOnMain_should_ReturnError_When_ShaDoesNotExist verifies that an invalid
// or unknown commit SHA surfaces as an error rather than a false "not shipped".
func TestIsCommitOnMain_should_ReturnError_When_ShaDoesNotExist(t *testing.T) {
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)

	_, err := IsCommitOnMain(work, "main", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	require.Error(t, err)
}

// TestBranchAheadBehind_should_ReportBranchExistsFalse_When_BranchWasDeleted verifies
// the expected post-ship state for a done item: the branch has been cleaned up, and
// that must read as "nothing to show", not an error.
func TestBranchAheadBehind_should_ReportBranchExistsFalse_When_BranchWasDeleted(t *testing.T) {
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)

	status, err := BranchAheadBehind(work, "feature-long-gone", "main")
	require.NoError(t, err)
	assert.False(t, status.BranchExists)
}

// TestBranchAheadBehind_should_ReportAheadCount_When_BranchHasUnmergedCommits verifies
// the ahead count for a branch that's diverged from main with its own commits.
func TestBranchAheadBehind_should_ReportAheadCount_When_BranchHasUnmergedCommits(t *testing.T) {
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	runGit(t, work, "checkout", "-b", "feature")
	for i := 0; i < 3; i++ {
		fname := fmt.Sprintf("feature-%d.txt", i)
		require.NoError(t, os.WriteFile(filepath.Join(work, fname), []byte("work\n"), 0o644))
		runGit(t, work, "add", fname)
		runGit(t, work, "commit", "-m", fmt.Sprintf("feature commit %d", i))
	}

	status, err := BranchAheadBehind(work, "feature", "main")
	require.NoError(t, err)
	assert.True(t, status.BranchExists)
	assert.Equal(t, 3, status.AheadOfMain)
	assert.Equal(t, 0, status.BehindMain)
}

// TestBranchAheadBehind_should_ReportBehindCount_When_MainAdvancedPastBranch verifies
// the behind count when main has moved on since the branch was created.
func TestBranchAheadBehind_should_ReportBehindCount_When_MainAdvancedPastBranch(t *testing.T) {
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	runGit(t, work, "checkout", "-b", "feature")

	require.NoError(t, os.WriteFile(filepath.Join(origin, "main-fix.txt"), []byte("fix\n"), 0o644))
	runGit(t, origin, "add", "main-fix.txt")
	runGit(t, origin, "commit", "-m", "fix landed on main")
	runGit(t, work, "fetch", "origin", "main")
	runGit(t, work, "branch", "-f", "main", "origin/main")

	status, err := BranchAheadBehind(work, "feature", "main")
	require.NoError(t, err)
	assert.True(t, status.BranchExists)
	assert.Equal(t, 0, status.AheadOfMain)
	assert.Equal(t, 1, status.BehindMain)
}

// TestListShippedCommits_should_ReturnNewestFirst_When_MultipleCommitsShipped verifies
// the commit list (Tyler: "identify which commits were shipped to main from the
// branch") returns every commit in the range, newest first, like a PR's commits tab.
func TestListShippedCommits_should_ReturnNewestFirst_When_MultipleCommitsShipped(t *testing.T) {
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	baseSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))

	runGit(t, work, "checkout", "-b", "feature")
	var shas []string
	for i := 0; i < 3; i++ {
		fname := fmt.Sprintf("feature-%d.txt", i)
		require.NoError(t, os.WriteFile(filepath.Join(work, fname), []byte("work\n"), 0o644))
		runGit(t, work, "add", fname)
		runGit(t, work, "commit", "-m", fmt.Sprintf("feature commit %d", i))
		shas = append(shas, strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD")))
	}
	headSHA := shas[len(shas)-1]

	commits, err := ListShippedCommits(work, baseSHA, headSHA)
	require.NoError(t, err)
	require.Len(t, commits, 3)
	assert.Equal(t, headSHA, commits[0].SHA, "newest commit must come first")
	assert.Equal(t, "feature commit 2", commits[0].Summary)
	assert.Equal(t, "feature commit 0", commits[2].Summary, "oldest of the three shipped commits must be last")
}

// TestListShippedCommits_should_ReturnEmpty_When_HeadEqualsBase verifies the
// degenerate no-op range (nothing was actually committed) returns no commits rather
// than erroring.
func TestListShippedCommits_should_ReturnEmpty_When_HeadEqualsBase(t *testing.T) {
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	sha := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))

	commits, err := ListShippedCommits(work, sha, sha)
	require.NoError(t, err)
	assert.Empty(t, commits)
}

// TestFileStatsBetween_ShouldReturnPerFileCounts_WhenCommitsAddAndDeleteLines
// verifies the happy path (Story 3.2.1): a two-commit range that adds 5 lines to one
// file and deletes 2 from another must report per-file addition/deletion counts with
// no error, and without shelling out to git (.claude/rules/prefer-go-git-over-subshells.md).
func TestFileStatsBetween_ShouldReturnPerFileCounts_WhenCommitsAddAndDeleteLines(t *testing.T) {
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)

	// bar.go starts with content so a later commit can delete lines from it.
	barContent := "line1\nline2\nline3\n"
	require.NoError(t, os.WriteFile(filepath.Join(work, "bar.go"), []byte(barContent), 0o644))
	runGit(t, work, "add", "bar.go")
	runGit(t, work, "commit", "-m", "add bar.go")
	baseSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))

	// foo.go is a brand new file adding 5 lines.
	fooContent := "a\nb\nc\nd\ne\n"
	require.NoError(t, os.WriteFile(filepath.Join(work, "foo.go"), []byte(fooContent), 0o644))
	runGit(t, work, "add", "foo.go")

	// bar.go loses its last 2 lines.
	require.NoError(t, os.WriteFile(filepath.Join(work, "bar.go"), []byte("line1\n"), 0o644))
	runGit(t, work, "add", "bar.go")

	runGit(t, work, "commit", "-m", "add foo.go, trim bar.go")
	headSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))

	stats, err := FileStatsBetween(work, baseSHA, headSHA)
	require.NoError(t, err)
	require.Len(t, stats, 2)

	byPath := map[string]FileStat{}
	for _, s := range stats {
		byPath[s.Path] = s
	}

	foo, ok := byPath["foo.go"]
	require.True(t, ok, "expected an entry for foo.go, got %+v", stats)
	assert.Equal(t, "added", foo.Status)
	assert.Equal(t, 5, foo.Additions)
	assert.Equal(t, 0, foo.Deletions)

	bar, ok := byPath["bar.go"]
	require.True(t, ok, "expected an entry for bar.go, got %+v", stats)
	assert.Equal(t, "modified", bar.Status)
	assert.Equal(t, 0, bar.Additions)
	assert.Equal(t, 2, bar.Deletions)
}

// TestFileStatsBetween_ShouldReturnError_WhenBaseSHADoesNotExistInRepo verifies that
// an invalid/unresolvable baseSHA surfaces as a non-nil error, matching
// IsCommitOnMain's existing error-wrapping style — not a panic, not an empty silent
// slice.
func TestFileStatsBetween_ShouldReturnError_WhenBaseSHADoesNotExistInRepo(t *testing.T) {
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	headSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))

	stats, err := FileStatsBetween(work, "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", headSHA)
	require.Error(t, err)
	assert.Nil(t, stats)
}

// TestFileStatsBetween_ShouldReportSingleRenameEntry_WhenFileRenamedWithNoContentChange
// verifies go-git's native rename detection against real git plumbing (not a mocked
// call): a pure rename must produce ONE entry keyed by the new path, not a delete+add
// pair a hand-parsed `git diff --numstat` would risk mishandling.
func TestFileStatsBetween_ShouldReportSingleRenameEntry_WhenFileRenamedWithNoContentChange(t *testing.T) {
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)

	content := "line1\nline2\nline3\n"
	require.NoError(t, os.WriteFile(filepath.Join(work, "old.go"), []byte(content), 0o644))
	runGit(t, work, "add", "old.go")
	runGit(t, work, "commit", "-m", "add old.go")
	baseSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))

	runGit(t, work, "mv", "old.go", "new.go")
	runGit(t, work, "commit", "-m", "rename old.go to new.go")
	headSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))

	stats, err := FileStatsBetween(work, baseSHA, headSHA)
	require.NoError(t, err)
	require.Len(t, stats, 1, "a pure rename must produce exactly one entry, not a delete+add pair: got %+v", stats)
	assert.Equal(t, "new.go", stats[0].Path)
	assert.Equal(t, "renamed", stats[0].Status)
	assert.Equal(t, 0, stats[0].Additions)
	assert.Equal(t, 0, stats[0].Deletions)
}

// TestFileStatsBetween_ShouldReturnEmpty_WhenBaseEqualsHead verifies the degenerate
// no-op range returns an empty slice with no error, mirroring
// TestListShippedCommits_should_ReturnEmpty_When_HeadEqualsBase's convention.
func TestFileStatsBetween_ShouldReturnEmpty_WhenBaseEqualsHead(t *testing.T) {
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	sha := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))

	stats, err := FileStatsBetween(work, sha, sha)
	require.NoError(t, err)
	assert.Empty(t, stats)
}

// TestFileStatsBetween_ShouldOmitBinaryFiles_WhenBinaryContentChanges verifies the
// spike finding (see FileStatsBetween's doc comment): go-git produces zero diff
// chunks for a changed binary file, so it must be silently omitted from the result
// rather than reported as a synthetic 0/0 entry or causing an error.
func TestFileStatsBetween_ShouldOmitBinaryFiles_WhenBinaryContentChanges(t *testing.T) {
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)

	binA := make([]byte, 64)
	for i := range binA {
		binA[i] = byte(i)
	}
	require.NoError(t, os.WriteFile(filepath.Join(work, "image.bin"), binA, 0o644))
	runGit(t, work, "add", "image.bin")
	runGit(t, work, "commit", "-m", "add image.bin")
	baseSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))

	binB := make([]byte, 64)
	for i := range binB {
		binB[i] = byte(255 - i)
	}
	require.NoError(t, os.WriteFile(filepath.Join(work, "image.bin"), binB, 0o644))
	// Also touch a text file so the range isn't degenerate.
	require.NoError(t, os.WriteFile(filepath.Join(work, "notes.txt"), []byte("hello\n"), 0o644))
	runGit(t, work, "add", "image.bin", "notes.txt")
	runGit(t, work, "commit", "-m", "change image.bin, add notes.txt")
	headSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))

	stats, err := FileStatsBetween(work, baseSHA, headSHA)
	require.NoError(t, err, "a changed binary file must not cause an error")
	require.Len(t, stats, 1, "the binary file must be omitted, leaving only notes.txt: got %+v", stats)
	assert.Equal(t, "notes.txt", stats[0].Path)
}

// TestDiffHashBetween_ShouldReturnSameHash_WhenSameCommitRangeHashedTwice verifies the
// core property IsFlakyVerdictFlipFlop depends on: hashing the identical base..head
// range twice must be fully deterministic.
func TestDiffHashBetween_ShouldReturnSameHash_WhenSameCommitRangeHashedTwice(t *testing.T) {
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	baseSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))

	require.NoError(t, os.WriteFile(filepath.Join(work, "foo.go"), []byte("a\nb\n"), 0o644))
	runGit(t, work, "add", "foo.go")
	runGit(t, work, "commit", "-m", "add foo.go")
	headSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))

	h1, err := DiffHashBetween(work, baseSHA, headSHA)
	require.NoError(t, err)
	h2, err := DiffHashBetween(work, baseSHA, headSHA)
	require.NoError(t, err)
	assert.Equal(t, h1, h2, "the same commit range must hash identically every time")
	assert.NotEmpty(t, h1)
}

// TestDiffHashBetween_ShouldReturnDifferentHash_WhenDiffContentDiffers verifies the
// converse: a genuinely different diff must not collide.
func TestDiffHashBetween_ShouldReturnDifferentHash_WhenDiffContentDiffers(t *testing.T) {
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	baseSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))

	require.NoError(t, os.WriteFile(filepath.Join(work, "foo.go"), []byte("a\nb\n"), 0o644))
	runGit(t, work, "add", "foo.go")
	runGit(t, work, "commit", "-m", "add foo.go")
	headSHA1 := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))

	require.NoError(t, os.WriteFile(filepath.Join(work, "foo.go"), []byte("a\nb\nc\n"), 0o644))
	runGit(t, work, "add", "foo.go")
	runGit(t, work, "commit", "-m", "extend foo.go")
	headSHA2 := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))

	h1, err := DiffHashBetween(work, baseSHA, headSHA1)
	require.NoError(t, err)
	h2, err := DiffHashBetween(work, baseSHA, headSHA2)
	require.NoError(t, err)
	assert.NotEqual(t, h1, h2, "a different diff must not hash the same")
}

// TestDiffHashBetween_ShouldReturnStableHash_WhenBaseEqualsHead verifies the no-op
// range (nothing changed) still returns a valid, non-error hash rather than a
// distinguishable-from-real-hashes empty string.
func TestDiffHashBetween_ShouldReturnStableHash_WhenBaseEqualsHead(t *testing.T) {
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	sha := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))

	hash, err := DiffHashBetween(work, sha, sha)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
}

// TestDiffHashBetween_ShouldReturnDifferentHash_WhenSameLineCountsButDifferentContent
// guards the exact false-collision shape a counts-only hash (path+status+
// addition-count+deletion-count) would miss: two attempts that both replace
// exactly one line of the same file (same +1/-1 shape) but with genuinely
// different replacement text must not hash the same.
func TestDiffHashBetween_ShouldReturnDifferentHash_WhenSameLineCountsButDifferentContent(t *testing.T) {
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	require.NoError(t, os.WriteFile(filepath.Join(work, "foo.go"), []byte("line1\nline2\nline3\n"), 0o644))
	runGit(t, work, "add", "foo.go")
	runGit(t, work, "commit", "-m", "add foo.go")
	baseSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))

	require.NoError(t, os.WriteFile(filepath.Join(work, "foo.go"), []byte("line1\nCHANGED_A\nline3\n"), 0o644))
	runGit(t, work, "add", "foo.go")
	runGit(t, work, "commit", "-m", "attempt 1: replace line2 with CHANGED_A")
	headSHA1 := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))

	runGit(t, work, "reset", "--hard", baseSHA)
	require.NoError(t, os.WriteFile(filepath.Join(work, "foo.go"), []byte("line1\nCHANGED_B\nline3\n"), 0o644))
	runGit(t, work, "add", "foo.go")
	runGit(t, work, "commit", "-m", "attempt 2: replace line2 with CHANGED_B")
	headSHA2 := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))

	// Sanity check: both attempts really do produce the identical +1/-1 shape
	// a counts-only hash would have collapsed to the same tuple.
	stats1, err := FileStatsBetween(work, baseSHA, headSHA1)
	require.NoError(t, err)
	stats2, err := FileStatsBetween(work, baseSHA, headSHA2)
	require.NoError(t, err)
	require.Equal(t, stats1, stats2, "test setup must produce identical FileStat shape for this regression test to be meaningful")

	h1, err := DiffHashBetween(work, baseSHA, headSHA1)
	require.NoError(t, err)
	h2, err := DiffHashBetween(work, baseSHA, headSHA2)
	require.NoError(t, err)
	assert.NotEqual(t, h1, h2, "two genuinely different edits with the identical addition/deletion counts must not collide")
}

// TestDiffHashBetween_ShouldReturnError_WhenBaseSHADoesNotExistInRepo mirrors
// FileStatsBetween's identical error-propagation test — DiffHashBetween is a thin
// wrapper and must not swallow the underlying resolution error.
func TestDiffHashBetween_ShouldReturnError_WhenBaseSHADoesNotExistInRepo(t *testing.T) {
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	headSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))

	hash, err := DiffHashBetween(work, "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", headSHA)
	require.Error(t, err)
	assert.Empty(t, hash)
}
