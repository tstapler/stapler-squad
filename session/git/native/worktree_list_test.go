package native

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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

	entries, err := ListWorktrees(repoPath)
	require.NoError(t, err)

	require.Len(t, entries, 1)
	entry := entries[0]
	assert.Equal(t, branchName, entry.Name)
	assert.False(t, entry.Locked)
	assert.False(t, entry.Prunable)
	assert.Equal(t, CanonicalizeWorktreePath(worktreePath), CanonicalizeWorktreePath(entry.WorktreePath))
}

// TestListWorktrees_should_ReturnSameEntries_When_ComparedToGitWorktreeList compares
// ListWorktrees against `git worktree list --porcelain` run by the real git CLI on a
// worktree the CLI itself created, so the parser is checked against git, not itself.
func TestListWorktrees_should_ReturnSameEntries_When_ComparedToGitWorktreeList(t *testing.T) {
	t.Parallel()
	repoPath := setupTestRepo(t)
	branchName := "work/b8ccca59"
	worktreePath := filepath.Join(t.TempDir(), "cli-created-wt")
	runGit(t, repoPath, "worktree", "add", "-b", branchName, worktreePath)

	got, err := ListWorktrees(repoPath)
	require.NoError(t, err)

	// Porcelain records are blank-line separated; the first is the main worktree.
	type cliEntry struct{ path, branch string }
	var linked []cliEntry
	for i, rec := range strings.Split(strings.TrimSpace(runGit(t, repoPath, "worktree", "list", "--porcelain")), "\n\n") {
		if i == 0 {
			continue
		}
		var e cliEntry
		for _, line := range strings.Split(rec, "\n") {
			switch {
			case strings.HasPrefix(line, "worktree "):
				e.path = strings.TrimPrefix(line, "worktree ")
			case strings.HasPrefix(line, "branch "):
				e.branch = strings.TrimPrefix(line, "branch ")
			}
		}
		linked = append(linked, e)
	}

	require.Len(t, linked, 1)
	require.Len(t, got, len(linked))
	assert.Equal(t, CanonicalizeWorktreePath(linked[0].path), CanonicalizeWorktreePath(got[0].WorktreePath))
	assert.Equal(t, linked[0].branch, got[0].BranchRef)
	assert.Equal(t, "refs/heads/"+branchName, got[0].BranchRef)
}

// TestListWorktrees_should_ReturnError_When_WorktreesDirUnreadable covers
// validation.md's noted gap for Story 1.1.1 (no error-path test for the exported wrapper):
// ListWorktrees is a pass-through, so an underlying read failure must propagate unchanged.
func TestListWorktrees_should_ReturnError_When_WorktreesDirUnreadable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permission checks, cannot exercise this failure mode")
	}
	t.Parallel()
	branchName := "feature-wrapper-unreadable"
	repoPath, _ := newNativeRemoveFixture(t, branchName)
	worktreesDir := filepath.Join(repoPath, ".git", "worktrees")

	require.NoError(t, os.Chmod(worktreesDir, 0o000))
	t.Cleanup(func() { _ = os.Chmod(worktreesDir, 0o755) })

	_, err := ListWorktrees(repoPath)
	require.Error(t, err)
	assert.True(t, errors.Is(err, fs.ErrPermission), "expected a permission-denied error (wrapped), got: %v", err)
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

	entries, err := ListWorktrees(repoPath)
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

	entries, err := ListWorktrees(repoPath)
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

	_, err := ListWorktrees(repoPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nativeListWorktrees")
}

// TestNativeListWorktrees_NoWorktreesDir_ReturnsEmptyNoError covers the "repo has never
// had a linked worktree" case: a missing .git/worktrees/ directory is not itself an
// error.
func TestNativeListWorktrees_NoWorktreesDir_ReturnsEmptyNoError(t *testing.T) {
	t.Parallel()
	repoPath := setupTestRepo(t)

	entries, err := ListWorktrees(repoPath)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// TestNativeListWorktrees_CrashPartialAdminDir_IsPrunableAlongsideHealthyEntries covers
// the fix for a crash between this project's own "locked" and "gitdir" admin-file writes
// (native_worktree_add.go's WriteAdminFiles) — research/pitfalls.md (citing
// GitoxideLabs/gitoxide#2959) documents this as real git's own well-defined, tolerable
// partial state. A single such entry must not abort the whole repo's listing: healthy
// entries must still list correctly, the broken one must be classified Prunable, and
// PruneWorktrees must be able to clean it up afterward.
func TestNativeListWorktrees_CrashPartialAdminDir_IsPrunableAlongsideHealthyEntries(t *testing.T) {
	t.Parallel()
	branchName := "feature-healthy"
	repoPath, worktreePath := newNativeRemoveFixture(t, branchName)

	brokenAdminDir := filepath.Join(repoPath, ".git", "worktrees", "crash-partial")
	require.NoError(t, os.MkdirAll(brokenAdminDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(brokenAdminDir, "locked"), []byte("initializing"), 0o644))
	// Deliberately no "gitdir" file — this is the crash-partial state under test.

	entries, err := ListWorktrees(repoPath)
	require.NoError(t, err, "one crash-partial admin dir must not abort the whole repo's listing")
	require.Len(t, entries, 2)

	var healthy, broken *WorktreeEntry
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

	require.NoError(t, PruneWorktrees(repoPath), "prune must be able to clean up the crash-partial entry")
	_, statErr := os.Stat(brokenAdminDir)
	assert.True(t, os.IsNotExist(statErr), "nativeWorktreePrune must remove the crash-partial admin dir")

	_, statErr = os.Stat(filepath.Join(repoPath, ".git", "worktrees", branchName))
	assert.NoError(t, statErr, "the healthy worktree's admin dir must survive prune")
}
