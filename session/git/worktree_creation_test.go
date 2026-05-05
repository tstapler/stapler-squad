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

// setupTestRepo creates a temporary git repository with an initial commit and configured
// user identity. It returns the repo directory path and a cleanup function.
func setupTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := safeexec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %s failed: %s", strings.Join(args, " "), out)
	}

	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test User")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test"), 0644))
	run("add", ".")
	run("commit", "-m", "Initial commit")

	// Rename default branch to "main" (git default can vary)
	run("branch", "-M", "main")

	return dir
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
