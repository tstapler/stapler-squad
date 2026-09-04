package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// setupTestRepo creates a temporary git repository with an initial commit and configured
// user identity. It returns the repo directory path and a cleanup function.
//
// Uses go-git directly rather than shelling out — see
// the `prefer-go-git-over-subshells` skill.
func setupTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	repo, err := git.PlainInitWithOptions(dir, &git.PlainInitOptions{
		InitOptions: git.InitOptions{DefaultBranch: plumbing.NewBranchReferenceName("main")},
	})
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test"), 0644))

	wt, err := repo.Worktree()
	require.NoError(t, err)
	_, err = wt.Add(".")
	require.NoError(t, err)
	_, err = wt.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{Name: "Test User", Email: "test@example.com", When: time.Now()},
	})
	require.NoError(t, err)

	return dir
}

// TestNewGitWorktreeWithBranch_should_Error_When_RepoPathIsEmpty guards against a
// zero-value repoPath falling through to filepath.Abs("") (resolves to cwd).
func TestNewGitWorktreeWithBranch_should_Error_When_RepoPathIsEmpty(t *testing.T) {
	_, _, err := NewGitWorktreeWithBranch("", "test-empty-path", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "repoPath must not be empty")
}

// TestNewGitWorktreeFromCommitSHA_should_Error_When_RepoPathIsEmpty mirrors
// TestNewGitWorktreeWithBranch_should_Error_When_RepoPathIsEmpty for
// NewGitWorktreeFromCommitSHA, the other findGitRepoRoot-reaching constructor.
func TestNewGitWorktreeFromCommitSHA_should_Error_When_RepoPathIsEmpty(t *testing.T) {
	_, _, err := NewGitWorktreeFromCommitSHA("", "test-empty-path", "branch", "deadbeef")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "repoPath must not be empty")
}

// TestNewWorktreeSetup_SetsBaseCommitSHA verifies that Setup() on a brand-new worktree
// records the HEAD SHA as baseCommitSHA so Diff() can work immediately.
func TestNewWorktreeSetup_SetsBaseCommitSHA(t *testing.T) {
	repoDir := setupTestRepo(t)

	wt, _, err := NewGitWorktree(repoDir, "test-new-worktree")
	require.NoError(t, err)

	require.NoError(t, wt.Setup())

	defer func() { _ = wt.Cleanup() }()

	assert.NotEmpty(t, wt.GetBaseCommitSHA(), "baseCommitSHA must be set after Setup()")
	assert.Len(t, wt.GetBaseCommitSHA(), 40, "baseCommitSHA must be a full SHA-1")
}

// TestCleanup_PreservesBranchWithCommits verifies the fix for the branch-deletion bug: a
// worktree branch with a commit that exists nowhere else must survive Cleanup(). Cleanup()
// used to unconditionally delete the branch reference via go-git's RemoveReference, which
// would silently destroy unpushed/unmerged work with no way to recover it (found live via
// mcp__stapler-squad__stop_session — see docs/tasks/backlog-feature-improvement.md).
func TestCleanup_PreservesBranchWithCommits(t *testing.T) {
	repoDir := setupTestRepo(t)

	wt, branchName, err := NewGitWorktree(repoDir, "test-cleanup-preserves-branch")
	require.NoError(t, err)
	require.NoError(t, wt.Setup())

	// Make a commit inside the worktree that exists on this branch only.
	worktreePath := wt.GetWorktreePath()
	require.NoError(t, os.WriteFile(filepath.Join(worktreePath, "new-file.txt"), []byte("unpushed work"), 0644))
	run := func(args ...string) {
		t.Helper()
		cmd := safeexec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = worktreePath
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %s failed: %s", strings.Join(args, " "), out)
	}
	run("add", ".")
	run("commit", "-m", "unpushed commit that must survive cleanup")

	require.NoError(t, wt.Cleanup())

	// The worktree directory must be gone...
	_, statErr := os.Stat(worktreePath)
	assert.True(t, os.IsNotExist(statErr), "worktree directory should be removed by Cleanup()")

	// ...but the branch — and its commit — must still exist in the main repo.
	branchCmd := safeexec.CommandContext(context.Background(), "git", "rev-parse", "--verify", branchName)
	branchCmd.Dir = repoDir
	out, err := branchCmd.CombinedOutput()
	assert.NoError(t, err, "branch %s must still exist after Cleanup(): %s", branchName, out)

	logCmd := safeexec.CommandContext(context.Background(), "git", "log", branchName, "--oneline")
	logCmd.Dir = repoDir
	logOut, err := logCmd.CombinedOutput()
	require.NoError(t, err)
	assert.Contains(t, string(logOut), "unpushed commit that must survive cleanup")
}

// TestNewWorktreeSetup_WorktreePathExists verifies that the worktree directory is created
// on disk after Setup().
func TestNewWorktreeSetup_WorktreePathExists(t *testing.T) {
	repoDir := setupTestRepo(t)

	wt, _, err := NewGitWorktree(repoDir, "test-wt-path-exists")
	require.NoError(t, err)
	require.NoError(t, wt.Setup())
	defer func() { _ = wt.Cleanup() }()

	_, statErr := os.Stat(wt.GetWorktreePath())
	assert.NoError(t, statErr, "worktree directory must exist after Setup()")
}

// TestNewWorktreeSetup_RepoPathIsRoot verifies that GetRepoPath() returns the main repo
// root, not the worktree subdirectory.
func TestNewWorktreeSetup_RepoPathIsRoot(t *testing.T) {
	repoDir := setupTestRepo(t)

	wt, _, err := NewGitWorktree(repoDir, "test-repo-path")
	require.NoError(t, err)
	require.NoError(t, wt.Setup())
	defer func() { _ = wt.Cleanup() }()

	assert.Equal(t, repoDir, wt.GetRepoPath(), "GetRepoPath() must be the main repo root")
	assert.NotEqual(t, repoDir, wt.GetWorktreePath(), "worktree path must differ from repo root")
}

// TestExistingBranchWorktree_SetsBaseCommitSHA verifies that when a worktree is created
// for a branch that already exists, baseCommitSHA is still resolved.
func TestExistingBranchWorktree_SetsBaseCommitSHA(t *testing.T) {
	repoDir := setupTestRepo(t)

	// Create a branch manually so it already exists when we call Setup().
	cmd := safeexec.CommandContext(context.Background(), "git", "branch", "existing-feature")
	cmd.Dir = repoDir
	require.NoError(t, cmd.Run())

	wt, _, err := NewGitWorktreeWithBranch(repoDir, "test-existing-branch", "existing-feature")
	require.NoError(t, err)
	require.NoError(t, wt.Setup())
	defer func() { _ = wt.Cleanup() }()

	assert.NotEmpty(t, wt.GetBaseCommitSHA(),
		"baseCommitSHA must be set even when the branch already existed before Setup()")
}

// TestDiff_EmptyWorktree_ReturnsZeroStats verifies that a freshly created worktree with
// no changes shows zero diff stats.
func TestDiff_EmptyWorktree_ReturnsZeroStats(t *testing.T) {
	repoDir := setupTestRepo(t)

	wt, _, err := NewGitWorktree(repoDir, "test-diff-empty")
	require.NoError(t, err)
	require.NoError(t, wt.Setup())
	defer func() { _ = wt.Cleanup() }()

	stats := wt.Diff()
	require.NoError(t, stats.Error)
	assert.Equal(t, 0, stats.Added, "no changes should produce 0 added lines")
	assert.Equal(t, 0, stats.Removed, "no changes should produce 0 removed lines")
}

// TestDiff_WithChanges_ReturnsNonZeroStats verifies that after writing a file in the
// worktree the diff is reflected in the stats.
func TestDiff_WithChanges_ReturnsNonZeroStats(t *testing.T) {
	repoDir := setupTestRepo(t)

	wt, _, err := NewGitWorktree(repoDir, "test-diff-changes")
	require.NoError(t, err)
	require.NoError(t, wt.Setup())
	defer func() { _ = wt.Cleanup() }()

	// Write a new file in the worktree.
	newFile := filepath.Join(wt.GetWorktreePath(), "new_feature.go")
	content := "package main\n\nfunc newFeature() {}\n"
	require.NoError(t, os.WriteFile(newFile, []byte(content), 0644))

	stats := wt.Diff()
	require.NoError(t, stats.Error)
	assert.Greater(t, stats.Added, 0, "adding a file should produce added lines in the diff")
}

// TestDiff_MissingBaseCommit_FallsBackToMergeBase verifies the dynamic merge-base
// fallback inside Diff(). If baseCommitSHA was never stored (simulating old sessions),
// Diff() should still return meaningful stats by resolving the merge-base at call time.
func TestDiff_MissingBaseCommit_FallsBackToMergeBase(t *testing.T) {
	repoDir := setupTestRepo(t)

	wt, _, err := NewGitWorktree(repoDir, "test-diff-fallback")
	require.NoError(t, err)
	require.NoError(t, wt.Setup())
	defer func() { _ = wt.Cleanup() }()

	// Manually clear the base commit SHA to simulate a session created before the fix.
	originalSHA := wt.GetBaseCommitSHA()
	require.NotEmpty(t, originalSHA)
	wt.baseCommitSHA = ""

	// Write a new file in the worktree.
	newFile := filepath.Join(wt.GetWorktreePath(), "fallback_test.go")
	require.NoError(t, os.WriteFile(newFile, []byte("package main\n"), 0644))

	stats := wt.Diff()
	require.NoError(t, stats.Error, "Diff() must not error when baseCommitSHA starts empty")
	assert.Greater(t, stats.Added, 0, "Diff() must fall back to merge-base and detect the added file")

	// The base commit SHA should now be cached after the fallback resolved it.
	assert.NotEmpty(t, wt.baseCommitSHA, "Diff() should cache the resolved merge-base")
}

// TestDiff_DeletedFile_ReturnsRemovedLines verifies that deleting a file that was present
// in the base commit shows up as removed lines in the diff.
func TestDiff_DeletedFile_ReturnsRemovedLines(t *testing.T) {
	repoDir := setupTestRepo(t)

	wt, _, err := NewGitWorktree(repoDir, "test-diff-deleted")
	require.NoError(t, err)
	require.NoError(t, wt.Setup())
	defer func() { _ = wt.Cleanup() }()

	// README.md was committed in the base commit (by setupTestRepo).
	// Deleting it from the worktree should appear as removed lines vs baseCommitSHA.
	readmePath := filepath.Join(wt.GetWorktreePath(), "README.md")
	require.NoError(t, os.Remove(readmePath))

	stats := wt.Diff()
	require.NoError(t, stats.Error)
	assert.Greater(t, stats.Removed, 0, "deleting README.md (present in base commit) must produce removed lines")
}

// TestNewGitWorktreeFromStorage_RoundTrip verifies that all fields survive a serialization
// round-trip through NewGitWorktreeFromStorage.
func TestNewGitWorktreeFromStorage_RoundTrip(t *testing.T) {
	repoDir := setupTestRepo(t)

	wt, _, err := NewGitWorktree(repoDir, "test-storage-roundtrip")
	require.NoError(t, err)
	require.NoError(t, wt.Setup())
	defer func() { _ = wt.Cleanup() }()

	// Capture fields before the round-trip.
	origRepoPath := wt.GetRepoPath()
	origWorktreePath := wt.GetWorktreePath()
	origBranchName := wt.GetBranchName()
	origBaseCommitSHA := wt.GetBaseCommitSHA()

	require.NotEmpty(t, origBaseCommitSHA, "baseCommitSHA must be set before testing round-trip")

	// Simulate serialization and deserialization via NewGitWorktreeFromStorage.
	restored := NewGitWorktreeFromStorage(origRepoPath, origWorktreePath, "test-storage-roundtrip", origBranchName, origBaseCommitSHA)
	require.NotNil(t, restored)

	assert.Equal(t, origRepoPath, restored.GetRepoPath())
	assert.Equal(t, origWorktreePath, restored.GetWorktreePath())
	assert.Equal(t, origBranchName, restored.GetBranchName())
	assert.Equal(t, origBaseCommitSHA, restored.GetBaseCommitSHA())
}

// TestNewGitWorktreeFromStorage_EmptyPaths_ReturnsNil ensures that passing empty repo
// and worktree paths produces a nil worktree (invalid data guard).
func TestNewGitWorktreeFromStorage_EmptyPaths_ReturnsNil(t *testing.T) {
	wt := NewGitWorktreeFromStorage("", "", "", "", "")
	assert.Nil(t, wt, "NewGitWorktreeFromStorage with empty paths must return nil")
}

// TestNewGitWorktreeFromStorage_CanonicalizesRawPathOnRead verifies AC4: a worktreePath
// value persisted before CanonicalizeWorktreePath existed (a raw, symlink-unresolved
// spelling) must come back canonicalized on rehydration -- without any data migration
// script -- so in-memory comparisons against freshly-computed (already-canonical) paths
// don't spuriously mismatch after a storage round-trip.
func TestNewGitWorktreeFromStorage_CanonicalizesRawPathOnRead(t *testing.T) {
	real := t.TempDir()
	rawAlias := filepath.Join(t.TempDir(), "raw-alias-worktree-dir")
	if err := os.Symlink(real, rawAlias); err != nil {
		t.Skipf("symlinks not supported on this platform: %v", err)
	}

	resolvedReal, err := filepath.EvalSymlinks(real)
	require.NoError(t, err)

	restored := NewGitWorktreeFromStorage("/some/repo", rawAlias, "session", "branch", "deadbeef")
	require.NotNil(t, restored)

	assert.Equal(t, resolvedReal, restored.GetWorktreePath(),
		"rehydration must canonicalize a raw persisted worktreePath, not return it as-is")
}

// TestNewGitWorktreeFromStorage_MissingDirectory_NonFatal verifies AC5: rehydrating a
// worktree whose directory no longer exists on disk (deleted worktree, stale storage
// entry) must not error, panic, or block on EvalSymlinks -- normalization is skipped and
// the original path is returned unchanged.
func TestNewGitWorktreeFromStorage_MissingDirectory_NonFatal(t *testing.T) {
	deletedPath := filepath.Join(t.TempDir(), "no-longer-on-disk")

	restored := NewGitWorktreeFromStorage("/some/repo", deletedPath, "session", "branch", "deadbeef")
	require.NotNil(t, restored, "rehydration of a deleted worktree's storage entry must not fail")

	assert.Equal(t, deletedPath, restored.GetWorktreePath(),
		"a since-deleted worktree path must be returned unchanged, not blocked by a failed canonicalization attempt")
}

// TestNewGitWorktreeFromExisting_DetectsBranchAndBase verifies that
// NewGitWorktreeFromExisting can detect the branch name and HEAD commit from an already
// existing worktree directory.
func TestNewGitWorktreeFromExisting_DetectsBranchAndBase(t *testing.T) {
	repoDir := setupTestRepo(t)

	wt, _, err := NewGitWorktree(repoDir, "test-from-existing")
	require.NoError(t, err)
	require.NoError(t, wt.Setup())
	defer func() { _ = wt.Cleanup() }()

	// Re-open the same worktree path as if we were connecting to an existing worktree.
	reopened, err := NewGitWorktreeFromExisting(wt.GetWorktreePath(), "reconnected-session")
	require.NoError(t, err, "must be able to open an existing worktree path")

	assert.NotEmpty(t, reopened.GetBranchName(), "branch name must be detected")
	assert.NotEmpty(t, reopened.GetBaseCommitSHA(), "base commit SHA must be detected for existing worktree")
	// The worktree path must match the path we passed in.
	assert.Equal(t, wt.GetWorktreePath(), reopened.GetWorktreePath(), "worktree path must match")

	// Regression guard: the resolved repo root must be the actual main repo, not the
	// worktree's own directory. findGitRepoRoot (previously used here) misreads a
	// worktree whose repo.Head() fails as "an uninitialized repo needing a fresh
	// commit", plants a brand-new disconnected initial commit directly inside the
	// worktree directory, and returns the worktree path itself as the "repo root" —
	// severing the worktree from its real branch/history entirely.
	// git rev-parse --path-format=absolute (used by GetRepoPath's underlying
	// implementation) canonicalizes through symlinks, so on macOS it returns
	// /private/var/... for a t.TempDir() path under the /var/... symlink.
	// Resolve repoDir the same way before comparing -- this is a path
	// representation difference, not a behavioral one.
	wantRepoDir, err := filepath.EvalSymlinks(repoDir)
	require.NoError(t, err)
	assert.Equal(t, wantRepoDir, reopened.GetRepoPath(),
		"repo root must resolve to the main repo, not be conflated with the worktree's own path")

	// Regression guard: the detected base SHA must resolve to a real object. In
	// production, go-git's repo.Head() read (the previous implementation of
	// getHeadCommitSHA) was observed returning a syntactically-valid 40-hex-char
	// SHA that did not correspond to any object in the repository at all — not
	// even a stale/unreachable one (absent from git cat-file -t, git rev-list
	// --all, git reflog show --all, and git fsck --unreachable). getHeadCommitSHA
	// now shells out to `git rev-parse HEAD` instead, which is what GetGitDiff
	// later uses to actually consume this value, keeping producer and consumer
	// in agreement.
	catFileCmd := safeexec.CommandContext(context.Background(), "git", "cat-file", "-t", reopened.GetBaseCommitSHA())
	catFileCmd.Dir = repoDir
	out, catErr := catFileCmd.Output()
	require.NoErrorf(t, catErr, "base commit SHA %q must resolve to a real object", reopened.GetBaseCommitSHA())
	assert.Equal(t, "commit", strings.TrimSpace(string(out)))
}

// TestNewGitWorktreeWithBranch_FreshPathMatchesGitRediscoveredPath verifies AC1: a
// worktree path computed fresh via joinWithinDir(worktreeDir, sanitizedName) at creation
// time, and the same directory rediscovered later via `git worktree list --porcelain`
// (the "already checked out elsewhere" fallback in findExistingWorktreeForBranch), must
// be byte-identical strings -- not merely equal after a test-side filepath.EvalSymlinks
// normalization. On macOS this is only true because getWorktreeDirectory() resolves
// symlinks on the base dir up front (see its doc comment) and the rediscovery path
// canonicalizes git's porcelain output the same way before returning it.
func TestNewGitWorktreeWithBranch_FreshPathMatchesGitRediscoveredPath(t *testing.T) {
	repoDir := setupTestRepo(t)
	const branch = "backlog/ac1-fresh-vs-rediscovered"

	wt1, _, err := NewGitWorktreeWithBranch(repoDir, "ac1-item", branch)
	require.NoError(t, err)
	require.NoError(t, wt1.Setup())
	freshPath := wt1.GetWorktreePath()
	defer func() { _ = wt1.Cleanup() }()

	// Rediscover the same worktree the way a second, independent construction would:
	// findExistingWorktreeForBranch parses `git worktree list --porcelain`, which
	// reports git's own realpath'd view of the directory.
	rediscoveredPath, found := findExistingWorktreeForBranch(repoDir, branch)
	require.True(t, found, "git must report the worktree just created via Setup()")
	rediscoveredPath = CanonicalizeWorktreePath(rediscoveredPath)

	assert.Equal(t, freshPath, rediscoveredPath,
		"freshly-computed worktree path and git-rediscovered path must be byte-identical, not just equal-after-normalization")
}

// TestWorktreeSetup_BranchNameSet verifies that GetBranchName() returns the expected
// branch after Setup() for both new and custom branch names.
func TestWorktreeSetup_BranchNameSet(t *testing.T) {
	repoDir := setupTestRepo(t)

	t.Run("AutoGeneratedBranch", func(t *testing.T) {
		wt, branchName, err := NewGitWorktree(repoDir, "my-session")
		require.NoError(t, err)
		require.NoError(t, wt.Setup())
		defer func() { _ = wt.Cleanup() }()

		assert.NotEmpty(t, wt.GetBranchName())
		assert.Equal(t, branchName, wt.GetBranchName())
	})

	t.Run("CustomBranch", func(t *testing.T) {
		const custom = "feat/my-custom-branch"
		wt, branchName, err := NewGitWorktreeWithBranch(repoDir, "custom-session", custom)
		require.NoError(t, err)
		require.NoError(t, wt.Setup())
		defer func() { _ = wt.Cleanup() }()

		assert.Equal(t, custom, branchName)
		assert.Equal(t, custom, wt.GetBranchName())
	})
}

// TestWorktreeSetup_ReusesHealthyWorktreeInPlace is a regression test for the reopen
// worktree-recreation bug: a second Setup() call for the same branch/path (mirroring a
// backlog rework/reopen) must reuse the existing checkout rather than tearing it down,
// so uncommitted state written after the first Setup() survives.
func TestWorktreeSetup_ReusesHealthyWorktreeInPlace(t *testing.T) {
	repoDir := setupTestRepo(t)
	const branch = "backlog/reuse-test-item"

	wt1, _, err := NewGitWorktreeWithBranch(repoDir, "reuse-test-item", branch)
	require.NoError(t, err)
	require.NoError(t, wt1.Setup())
	path := wt1.GetWorktreePath()
	defer func() { _ = wt1.Cleanup() }()

	marker := filepath.Join(path, "not-yet-committed.txt")
	require.NoError(t, os.WriteFile(marker, []byte("uncommitted work\n"), 0644))

	wt2, _, err := NewGitWorktreeWithBranch(repoDir, "reuse-test-item", branch)
	require.NoError(t, err)
	require.NoError(t, wt2.Setup())

	assert.Equal(t, path, wt2.GetWorktreePath(), "reopen must reuse the same worktree path")
	assert.FileExists(t, marker, "reusing in place must not wipe the existing worktree's uncommitted content")
}

// TestWorktreeSetup_RecreatesLockedInterruptedWorktree is a regression test for a bug
// introduced by the reuse-in-place fix itself: a worktree left registered but locked
// with git's "initializing" marker — the state `worktree add` leaves behind when
// interrupted mid-checkout (e.g. killed by runGitCommand's timeout under load) — must
// NOT be reused as-is (that would silently hand back a half-populated checkout). Setup()
// must detect the lock, clean up, and rebuild a healthy worktree instead.
func TestWorktreeSetup_RecreatesLockedInterruptedWorktree(t *testing.T) {
	repoDir := setupTestRepo(t)
	const branch = "backlog/interrupted-item"

	wt1, _, err := NewGitWorktreeWithBranch(repoDir, "interrupted-item", branch)
	require.NoError(t, err)
	require.NoError(t, wt1.Setup())
	path := wt1.GetWorktreePath()

	// Simulate an interrupted `worktree add`: lock the worktree as git itself would
	// mid-checkout, then delete a real tracked file to simulate a half-populated
	// directory left behind by the kill.
	_, err = wt1.runGitCommand(repoDir, "worktree", "lock", "--reason", "initializing", path)
	require.NoError(t, err)
	trackedFile := filepath.Join(path, "README.md")
	require.NoError(t, os.Remove(trackedFile))

	wt2, _, err := NewGitWorktreeWithBranch(repoDir, "interrupted-item", branch)
	require.NoError(t, err)
	require.NoError(t, wt2.Setup())
	defer func() { _ = wt2.Cleanup() }()

	assert.FileExists(t, filepath.Join(wt2.GetWorktreePath(), "README.md"),
		"a locked/interrupted worktree must be rebuilt, not reused half-populated")
}

// TestDiff_Content_MatchesSHA verifies that the diff content returned by Diff() contains
// diff markers and is non-empty when there are changes.
func TestDiff_Content_MatchesSHA(t *testing.T) {
	repoDir := setupTestRepo(t)

	wt, _, err := NewGitWorktree(repoDir, "test-diff-content")
	require.NoError(t, err)
	require.NoError(t, wt.Setup())
	defer func() { _ = wt.Cleanup() }()

	// Add a new file.
	require.NoError(t, os.WriteFile(filepath.Join(wt.GetWorktreePath(), "feature.go"), []byte("package main\n"), 0644))

	stats := wt.Diff()
	require.NoError(t, stats.Error)
	assert.NotEmpty(t, stats.Content, "Diff content must be non-empty when there are changes")
	assert.Contains(t, stats.Content, "+++", "diff content must contain unified diff markers")
}

// TestIsDirty_Clean returns false for an unmodified worktree.
func TestIsDirty_Clean(t *testing.T) {
	repoDir := setupTestRepo(t)

	wt, _, err := NewGitWorktree(repoDir, "test-isdirty-clean")
	require.NoError(t, err)
	require.NoError(t, wt.Setup())
	defer func() { _ = wt.Cleanup() }()

	dirty, err := wt.IsDirty()
	require.NoError(t, err)
	assert.False(t, dirty, "fresh worktree must not be dirty")
}

// TestIsDirty_WithChanges returns true after writing a file.
func TestIsDirty_WithChanges(t *testing.T) {
	repoDir := setupTestRepo(t)

	wt, _, err := NewGitWorktree(repoDir, "test-isdirty-dirty")
	require.NoError(t, err)
	require.NoError(t, wt.Setup())
	defer func() { _ = wt.Cleanup() }()

	require.NoError(t, os.WriteFile(filepath.Join(wt.GetWorktreePath(), "new.go"), []byte("package main\n"), 0644))

	dirty, err := wt.IsDirty()
	require.NoError(t, err)
	assert.True(t, dirty, "worktree with untracked file must be dirty")
}
