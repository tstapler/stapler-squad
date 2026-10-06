package git

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNativeWorktreePrune_RemovesOnlyPrunableEntries covers Story 2.4.1's acceptance
// criterion: two worktrees under one repo — one live, one with its working directory
// deleted (making it Prunable per Story 2.3.1) — pruning removes only the prunable
// entry's admin dir, leaving the live one untouched.
func TestNativeWorktreePrune_RemovesOnlyPrunableEntries(t *testing.T) {
	t.Parallel()
	repoPath := setupTestRepo(t)

	liveBranch := "feature-x"
	liveWorktreePath := filepath.Join(t.TempDir(), liveBranch)
	liveWt := NewGitWorktreeFromStorageWithExecutor(repoPath, liveWorktreePath, "native-prune-live", liveBranch, "")
	require.NoError(t, liveWt.nativeSetupNewWorktree())

	prunableBranch := "feature-y"
	prunableWorktreePath := filepath.Join(t.TempDir(), prunableBranch)
	prunableWt := NewGitWorktreeFromStorageWithExecutor(repoPath, prunableWorktreePath, "native-prune-prunable", prunableBranch, "")
	require.NoError(t, prunableWt.nativeSetupNewWorktree())
	require.NoError(t, os.RemoveAll(prunableWorktreePath))

	require.NoError(t, nativeWorktreePrune(repoPath))

	liveAdminDir := filepath.Join(repoPath, ".git", "worktrees", liveBranch)
	_, err := os.Stat(liveAdminDir)
	assert.NoError(t, err, "live entry's admin dir must be untouched")

	prunableAdminDir := filepath.Join(repoPath, ".git", "worktrees", prunableBranch)
	_, err = os.Stat(prunableAdminDir)
	assert.True(t, os.IsNotExist(err), "prunable entry's admin dir must be removed")
}

// TestNativeWorktreePrune_should_ReturnError_When_ListingFails covers validation.md's
// error case: when nativeListWorktrees fails, nativeWorktreePrune must propagate the
// error rather than silently pruning nothing (or partially pruning).
func TestNativeWorktreePrune_should_ReturnError_When_ListingFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permission checks, cannot exercise this failure mode")
	}
	t.Parallel()
	branchName := "feature-unreadable"
	repoPath, _ := newNativeRemoveFixture(t, branchName)
	worktreesDir := filepath.Join(repoPath, ".git", "worktrees")

	require.NoError(t, os.Chmod(worktreesDir, 0o000))
	t.Cleanup(func() { _ = os.Chmod(worktreesDir, 0o755) })

	err := nativeWorktreePrune(repoPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nativeWorktreePrune")
}

// TestNativeWorktreePrune_ContinuesPastOneFailure_RemovesOtherPrunableEntries covers the
// fix for nativeWorktreePrune aborting entirely on the first os.RemoveAll failure: two
// independently-prunable entries, one whose admin dir can't be removed (permission
// denied), must still both be attempted — the failure is reported, but the other entry
// is still cleaned up in the same pass.
func TestNativeWorktreePrune_ContinuesPastOneFailure_RemovesOtherPrunableEntries(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permission checks, cannot exercise this failure mode")
	}
	t.Parallel()
	repoPath := setupTestRepo(t)

	blockedBranch := "feature-blocked"
	blockedWorktreePath := filepath.Join(t.TempDir(), blockedBranch)
	blockedWt := NewGitWorktreeFromStorageWithExecutor(repoPath, blockedWorktreePath, "native-prune-blocked", blockedBranch, "")
	require.NoError(t, blockedWt.nativeSetupNewWorktree())
	require.NoError(t, os.RemoveAll(blockedWorktreePath))

	prunableBranch := "feature-prunable"
	prunableWorktreePath := filepath.Join(t.TempDir(), prunableBranch)
	prunableWt := NewGitWorktreeFromStorageWithExecutor(repoPath, prunableWorktreePath, "native-prune-prunable", prunableBranch, "")
	require.NoError(t, prunableWt.nativeSetupNewWorktree())
	require.NoError(t, os.RemoveAll(prunableWorktreePath))

	blockedAdminDir := filepath.Join(repoPath, ".git", "worktrees", blockedBranch)
	require.NoError(t, os.Chmod(blockedAdminDir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(blockedAdminDir, 0o755) })

	err := nativeWorktreePrune(repoPath)
	require.Error(t, err, "the blocked entry's removal failure must still be reported")
	assert.Contains(t, err.Error(), "nativeWorktreePrune")

	prunableAdminDir := filepath.Join(repoPath, ".git", "worktrees", prunableBranch)
	_, statErr := os.Stat(prunableAdminDir)
	assert.True(t, os.IsNotExist(statErr), "the other prunable entry must still be removed despite the first one's failure")

	_, statErr = os.Stat(blockedAdminDir)
	assert.NoError(t, statErr, "the blocked entry's admin dir must still exist (removal failed)")
}
