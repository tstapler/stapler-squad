package git

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file covers Epic 5.3, Story 5.3.1: proof that either the native or legacy
// worktree/merge implementation can safely operate on state the other created — the
// migration-shaped test gap architecture.md §5 names explicitly (a same-implementation
// round trip proves less than a cross-implementation one). Every test here mutates the
// shared useNativeWorktree/useNativeMerge package vars (worktree_ops_test.go's
// established toggle pattern) and is deliberately not t.Parallel() for the same reason
// that file's existing overrides aren't: a non-parallel test always runs to completion
// before the next one starts, so the shared var is never read concurrently by two tests.

// TestCrossImplementation_LegacyCreates_NativeRemoves covers Story 5.3.1's first
// acceptance criterion (plan.md Task 5.3.1a): a worktree created via the legacy
// subprocess implementation must be fully, cleanly removable by the native
// implementation once the flag flips on.
func TestCrossImplementation_LegacyCreates_NativeRemoves(t *testing.T) {
	repoDir := setupTestRepo(t)
	branchName := "feature-interop-legacy-create-native-remove"
	worktreePath := filepath.Join(t.TempDir(), branchName)
	wt := NewGitWorktreeFromStorageWithExecutor(repoDir, worktreePath, "test-interop-legacy-create", branchName, "")

	// useNativeWorktree defaults to legacy (no override in effect yet).
	require.NoError(t, wt.Setup())

	listOut := runGit(t, repoDir, "worktree", "list", "--porcelain")
	assert.Contains(t, listOut, worktreePath, "legacy Setup must register the worktree with real git")

	orig := useNativeWorktree
	useNativeWorktree = func(string) bool { return true }
	t.Cleanup(func() { useNativeWorktree = orig })

	require.NoError(t, wt.Remove())

	_, err := os.Stat(worktreePath)
	assert.True(t, os.IsNotExist(err), "native Remove must delete a legacy-created worktree's working directory")

	adminDir := filepath.Join(repoDir, ".git", "worktrees", branchName)
	_, err = os.Stat(adminDir)
	assert.True(t, os.IsNotExist(err), "native Remove must delete a legacy-created worktree's admin dir")

	branchOut := runGit(t, repoDir, "show-ref", "refs/heads/"+branchName)
	assert.NotEmpty(t, branchOut, "branch ref must survive removal regardless of which implementation removed the worktree")
}

// TestCrossImplementation_NativeCreates_LegacyRemovesListsPrunes covers Story 5.3.1's
// second acceptance criterion (plan.md Task 5.3.1b): a worktree created via the native
// implementation must be correctly recognized and operated on by all three legacy
// operations once the flag flips off — Remove, findLiveWorktreeForBranch, and Prune,
// each exercised against its own fixture since Remove already tears down the state
// Prune/find would otherwise need.
func TestCrossImplementation_NativeCreates_LegacyRemovesListsPrunes(t *testing.T) {
	newNativeCreatedWorktree := func(t *testing.T, branchSuffix string) (repoDir, worktreePath string, wt *GitWorktree) {
		t.Helper()
		repoDir = setupTestRepo(t)
		branchName := "feature-interop-native-create-" + branchSuffix
		worktreePath = filepath.Join(t.TempDir(), branchName)
		wt = NewGitWorktreeFromStorageWithExecutor(repoDir, worktreePath, "test-interop-native-create-"+branchSuffix, branchName, "")

		orig := useNativeWorktree
		useNativeWorktree = func(string) bool { return true }
		require.NoError(t, wt.Setup())
		useNativeWorktree = orig

		return repoDir, worktreePath, wt
	}

	t.Run("LegacyRemove", func(t *testing.T) {
		repoDir, worktreePath, wt := newNativeCreatedWorktree(t, "remove")
		branchName := filepath.Base(worktreePath)

		require.NoError(t, wt.Remove())

		_, err := os.Stat(worktreePath)
		assert.True(t, os.IsNotExist(err), "legacy Remove must delete a native-created worktree's working directory")

		adminDir := filepath.Join(repoDir, ".git", "worktrees", branchName)
		_, err = os.Stat(adminDir)
		assert.True(t, os.IsNotExist(err), "legacy Remove must delete a native-created worktree's admin dir")
	})

	t.Run("LegacyFindLiveWorktreeForBranch", func(t *testing.T) {
		_, worktreePath, wt := newNativeCreatedWorktree(t, "find")

		foundPath, found := wt.findLiveWorktreeForBranch()

		require.True(t, found, "legacyFindLiveWorktreeForBranch must recognize a native-created worktree's admin-file layout")
		assert.Equal(t, CanonicalizeWorktreePath(worktreePath), CanonicalizeWorktreePath(foundPath))
	})

	t.Run("LegacyPrune", func(t *testing.T) {
		repoDir, worktreePath, wt := newNativeCreatedWorktree(t, "prune")
		branchName := filepath.Base(worktreePath)

		// Make the entry prunable exactly like the existing native-prune tests do:
		// delete the working directory out from under git, leaving only the
		// native-written admin dir behind.
		require.NoError(t, os.RemoveAll(worktreePath))

		require.NoError(t, wt.Prune())

		adminDir := filepath.Join(repoDir, ".git", "worktrees", branchName)
		_, err := os.Stat(adminDir)
		assert.True(t, os.IsNotExist(err), "legacy Prune must recognize and remove a native-created admin dir once its working directory is gone")
	})
}

// TestCrossImplementation_MergeAgainstEitherWorktreeProvenance covers Story 5.3.1's
// third acceptance criterion (plan.md Task 5.3.1c): the native merge pipeline must run
// correctly against a worktree regardless of which implementation created it — no error
// attributable to creation provenance.
func TestCrossImplementation_MergeAgainstEitherWorktreeProvenance(t *testing.T) {
	scenarios := []struct {
		name            string
		createdByNative bool
	}{
		{name: "LegacyCreatedWorktree", createdByNative: false},
		{name: "NativeCreatedWorktree", createdByNative: true},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			origWorktree := useNativeWorktree
			useNativeWorktree = func(string) bool { return sc.createdByNative }
			t.Cleanup(func() { useNativeWorktree = origWorktree })

			origMerge := useNativeMerge
			useNativeMerge = func(string) bool { return true }
			t.Cleanup(func() { useNativeMerge = origMerge })

			origin := setupTestRepo(t)
			mainRepo := cloneTestRepo(t, origin)
			branchName := "feature-interop-merge-" + sc.name
			worktreePath := filepath.Join(t.TempDir(), branchName)
			wt := NewGitWorktreeFromStorageWithExecutor(mainRepo, worktreePath, "test-interop-merge-"+sc.name, branchName, "")

			require.NoError(t, wt.Setup())

			// Diverge origin's main on a path the worktree's branch never touched, so
			// the native pipeline resolves this as a clean fast-forward/merge rather
			// than a conflict — provenance is this test's variable, not conflict
			// handling (covered separately by the golden conflict-marker test).
			require.NoError(t, os.WriteFile(filepath.Join(origin, "from-main.txt"), []byte("main change\n"), 0o644))
			runGit(t, origin, "add", "from-main.txt")
			runGit(t, origin, "commit", "-m", "change on main")

			result, err := MergeMainIntoWorktree(worktreePath, "main")
			require.NoError(t, err, "native merge must succeed against a worktree regardless of which implementation created it")
			assert.True(t, result.Merged, "expected a clean fast-forward/merge, not a conflict, for this scenario")
			assert.False(t, result.Conflicted)
		})
	}
}
