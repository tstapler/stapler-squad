package git

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNativeListWorktrees_LiveWorktree covers Story 2.3.1's first acceptance criterion:
// a live, on-disk worktree is listed with Locked=false, Prunable=false.
func TestNativeListWorktrees_LiveWorktree(t *testing.T) {
	t.Parallel()
	branchName := "feature-x"
	repoPath, worktreePath := newNativeRemoveFixture(t, branchName)

	entries, err := nativeListWorktrees(repoPath)
	require.NoError(t, err)

	require.Len(t, entries, 1)
	entry := entries[0]
	assert.Equal(t, branchName, entry.Name)
	assert.False(t, entry.Locked)
	assert.False(t, entry.Prunable)
	assert.Equal(t, CanonicalizeWorktreePath(worktreePath), CanonicalizeWorktreePath(entry.WorktreePath))
}

// TestNativeFindExistingWorktreeForBranch_FindsLiveWorktree_ReportsNotFoundForOtherBranch
// covers PR #730 Gate 2's noted gap: nativeFindExistingWorktreeForBranch (worktree.go),
// findExistingWorktreeForBranch's native counterpart dispatched to when useNativeWorktree
// is on (worktree.go's findOrCreateWorktree), had no direct test of its own — every
// existing test of this dispatch point (worktree_creation_test.go) only exercised the
// legacy path.
func TestNativeFindExistingWorktreeForBranch_FindsLiveWorktree_ReportsNotFoundForOtherBranch(t *testing.T) {
	t.Parallel()
	branchName := "feature-native-find-existing"
	repoPath, worktreePath := newNativeRemoveFixture(t, branchName)

	path, found := nativeFindExistingWorktreeForBranch(repoPath, branchName)
	require.True(t, found)
	assert.Equal(t, CanonicalizeWorktreePath(worktreePath), CanonicalizeWorktreePath(path))

	_, found = nativeFindExistingWorktreeForBranch(repoPath, "some-other-branch")
	assert.False(t, found, "a branch with no registered worktree must report not-found, not a stale match")
}

// TestNativeListWorktrees_DeletedWorkingDir_IsPrunable covers Story 2.3.1's second
// acceptance criterion: a worktree whose target directory was deleted out from under git
// is classified Prunable=true, per this project's directory-exists-only scope cut
// (stack.md §2.2) — no mtime grace period.
func TestNativeListWorktrees_DeletedWorkingDir_IsPrunable(t *testing.T) {
	t.Parallel()
	branchName := "feature-deleted"
	repoPath, worktreePath := newNativeRemoveFixture(t, branchName)

	require.NoError(t, os.RemoveAll(worktreePath))

	entries, err := nativeListWorktrees(repoPath)
	require.NoError(t, err)

	require.Len(t, entries, 1)
	assert.True(t, entries[0].Prunable)
	assert.False(t, entries[0].Locked)
}

// TestNativeListWorktrees_LockedWorktree_NeverPrunable covers Story 2.3.1's third
// acceptance criterion: a LockedMarker overrides prunability regardless of the target
// directory's state.
func TestNativeListWorktrees_LockedWorktree_NeverPrunable(t *testing.T) {
	t.Parallel()
	branchName := "feature-locked"
	repoPath, worktreePath := newNativeRemoveFixture(t, branchName)

	require.NoError(t, os.RemoveAll(worktreePath))

	adminDir := filepath.Join(repoPath, ".git", "worktrees", branchName)
	require.NoError(t, os.WriteFile(filepath.Join(adminDir, "locked"), []byte("locked for testing"), 0644))

	entries, err := nativeListWorktrees(repoPath)
	require.NoError(t, err)

	require.Len(t, entries, 1)
	assert.True(t, entries[0].Locked)
	assert.False(t, entries[0].Prunable)
}

// TestNativeListWorktrees_should_ReturnError_When_WorktreesDirUnreadable covers
// validation.md's error case: a permission-denied `.git/worktrees` must surface as an
// error, not an empty/wrong list.
func TestNativeListWorktrees_should_ReturnError_When_WorktreesDirUnreadable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permission checks, cannot exercise this failure mode")
	}
	t.Parallel()
	branchName := "feature-unreadable"
	repoPath, _ := newNativeRemoveFixture(t, branchName)
	worktreesDir := filepath.Join(repoPath, ".git", "worktrees")

	require.NoError(t, os.Chmod(worktreesDir, 0o000))
	t.Cleanup(func() { _ = os.Chmod(worktreesDir, 0o755) })

	_, err := nativeListWorktrees(repoPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nativeListWorktrees")
}

// TestNativeListWorktrees_NoWorktreesDir_ReturnsEmptyNoError covers the "repo has never
// had a linked worktree" case: a missing .git/worktrees/ directory is not itself an
// error.
func TestNativeListWorktrees_NoWorktreesDir_ReturnsEmptyNoError(t *testing.T) {
	t.Parallel()
	repoPath := setupTestRepo(t)

	entries, err := nativeListWorktrees(repoPath)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// TestNativeListWorktrees_CrashPartialAdminDir_IsPrunableAlongsideHealthyEntries covers
// the fix for a crash between this project's own "locked" and "gitdir" admin-file writes
// (native_worktree_add.go's writeNativeWorktreeAdminFiles) — research/pitfalls.md (citing
// GitoxideLabs/gitoxide#2959) documents this as real git's own well-defined, tolerable
// partial state. A single such entry must not abort the whole repo's listing: healthy
// entries must still list correctly, the broken one must be classified Prunable, and
// nativeWorktreePrune must be able to clean it up afterward.
func TestNativeListWorktrees_CrashPartialAdminDir_IsPrunableAlongsideHealthyEntries(t *testing.T) {
	t.Parallel()
	branchName := "feature-healthy"
	repoPath, worktreePath := newNativeRemoveFixture(t, branchName)

	brokenAdminDir := filepath.Join(repoPath, ".git", "worktrees", "crash-partial")
	require.NoError(t, os.MkdirAll(brokenAdminDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(brokenAdminDir, "locked"), []byte("initializing"), 0o644))
	// Deliberately no "gitdir" file — this is the crash-partial state under test.

	entries, err := nativeListWorktrees(repoPath)
	require.NoError(t, err, "one crash-partial admin dir must not abort the whole repo's listing")
	require.Len(t, entries, 2)

	var healthy, broken *NativeWorktreeEntry
	for i := range entries {
		switch entries[i].Name {
		case branchName:
			healthy = &entries[i]
		case "crash-partial":
			broken = &entries[i]
		}
	}
	require.NotNil(t, healthy, "healthy entry must still be listed")
	require.NotNil(t, broken, "crash-partial entry must still be listed, not dropped")

	assert.False(t, healthy.Prunable)
	assert.Equal(t, CanonicalizeWorktreePath(worktreePath), CanonicalizeWorktreePath(healthy.WorktreePath))

	assert.True(t, broken.Prunable, "an admin dir with no gitdir file must be classified prunable")
	assert.Empty(t, broken.WorktreePath)

	require.NoError(t, nativeWorktreePrune(repoPath), "prune must be able to clean up the crash-partial entry")
	_, statErr := os.Stat(brokenAdminDir)
	assert.True(t, os.IsNotExist(statErr), "nativeWorktreePrune must remove the crash-partial admin dir")

	_, statErr = os.Stat(filepath.Join(repoPath, ".git", "worktrees", branchName))
	assert.NoError(t, statErr, "the healthy worktree's admin dir must survive prune")
}
