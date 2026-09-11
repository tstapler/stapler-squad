package git

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newNativeRemoveFixture creates a real native-created worktree for branchName (reusing
// newNativeAddTarget/nativeSetupNewWorktree from native_worktree_add_test.go), ready for
// nativeRemoveWorktree to tear down.
func newNativeRemoveFixture(t *testing.T, branchName string) (repoPath, worktreePath string) {
	t.Helper()
	repoPath, worktreePath = newNativeAddTarget(t, branchName)
	wt := NewGitWorktreeFromStorageWithExecutor(repoPath, worktreePath, "native-remove-fixture-"+branchName, branchName, "")
	require.NoError(t, wt.nativeSetupNewWorktree())
	return repoPath, worktreePath
}

// TestNativeRemoveWorktree_RemovesAdminDirAndWorkingTree_PreservesBranch covers Story
// 2.2.1's first acceptance criterion.
func TestNativeRemoveWorktree_RemovesAdminDirAndWorkingTree_PreservesBranch(t *testing.T) {
	t.Parallel()
	branchName := "feature-remove"
	repoPath, worktreePath := newNativeRemoveFixture(t, branchName)
	adminDir := filepath.Join(repoPath, ".git", "worktrees", branchName)

	require.NoError(t, nativeRemoveWorktree(repoPath, worktreePath))

	_, err := os.Stat(worktreePath)
	assert.True(t, os.IsNotExist(err), "working directory must be removed")

	_, err = os.Stat(adminDir)
	assert.True(t, os.IsNotExist(err), "admin dir must be removed")

	output := runRealGit(t, repoPath, "show-ref", "refs/heads/"+branchName)
	assert.NotEmpty(t, output, "branch ref must still resolve after removal — removal must never touch refs/heads/<branch>")
}

// TestNativeRemoveWorktree_MissingWorkingDirectory_NonFatal covers Story 2.2.1's second
// acceptance criterion: removal degrades gracefully when the working directory was
// already deleted out from under git (matches today's removeLocked behavior).
func TestNativeRemoveWorktree_MissingWorkingDirectory_NonFatal(t *testing.T) {
	t.Parallel()
	branchName := "feature-remove-missing-dir"
	repoPath, worktreePath := newNativeRemoveFixture(t, branchName)
	adminDir := filepath.Join(repoPath, ".git", "worktrees", branchName)

	require.NoError(t, os.RemoveAll(worktreePath))

	require.NoError(t, nativeRemoveWorktree(repoPath, worktreePath))

	_, err := os.Stat(adminDir)
	assert.True(t, os.IsNotExist(err), "admin dir must still be removed even when the working directory was already gone")
}

// TestNativeRemoveWorktree_should_ReturnError_When_AdminDirIsReadOnly covers
// validation.md's error case: a genuine removal failure (admin dir's parent made
// read-only) must surface as a non-nil error rather than being silently swallowed and
// leaving a partial admin dir behind.
func TestNativeRemoveWorktree_should_ReturnError_When_AdminDirIsReadOnly(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permission checks, cannot exercise this failure mode")
	}
	t.Parallel()
	branchName := "feature-remove-readonly"
	repoPath, worktreePath := newNativeRemoveFixture(t, branchName)
	worktreesDir := filepath.Join(repoPath, ".git", "worktrees")

	require.NoError(t, os.Chmod(worktreesDir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(worktreesDir, 0o755) })

	err := nativeRemoveWorktree(repoPath, worktreePath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nativeRemoveWorktree")
}
