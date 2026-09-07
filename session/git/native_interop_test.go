package git

import (
	"os"
	"path/filepath"
	"strings"
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
		t.Cleanup(func() { useNativeWorktree = orig })
		require.NoError(t, wt.Setup())

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

// --- MUST FIX 3 (Phase 6 verify): migration-reversibility, per plan.md's Migration Plan
// ("a same-implementation round trip alone is not sufficient evidence"). The tests above
// only chain one native operation before checking cross-implementation compatibility;
// these two chain a fuller, more realistic sequence: native worktree creation, at least
// one native merge (including a real conflict) or an interrupted native Add, THEN a flag
// flip to legacy, proving THIS codebase's own legacy implementation (not just real git's
// CLI) operates correctly on the resulting on-disk state.

// newNativeWorktreeAfterConflictedMerge builds a worktree via the native create path, then
// runs a native merge that produces a real conflict (materialized-then-aborted per
// abortNativeMerge, leaving the worktree clean) — the on-disk state
// TestNativeGitRollout_migration_should_BeReversible_When_FlagFlippedOffAfterNativeWorktreeAndMergeExist
// requires proving the legacy implementation can still operate on, not just a freshly
// created worktree with no merge history.
func newNativeWorktreeAfterConflictedMerge(t *testing.T, branchSuffix string) (origin, mainRepo, worktreePath string, wt *GitWorktree) {
	t.Helper()
	origWorktree := useNativeWorktree
	useNativeWorktree = func(string) bool { return true }
	t.Cleanup(func() { useNativeWorktree = origWorktree })

	origMerge := useNativeMerge
	useNativeMerge = func(string) bool { return true }
	t.Cleanup(func() { useNativeMerge = origMerge })

	origin = setupTestRepo(t)
	mainRepo = cloneTestRepo(t, origin)
	branchName := "feature-migration-reversible-" + branchSuffix
	worktreePath = filepath.Join(t.TempDir(), branchName)
	wt = NewGitWorktreeFromStorageWithExecutor(mainRepo, worktreePath, "test-migration-reversible-"+branchSuffix, branchName, "")
	require.NoError(t, wt.Setup())

	require.NoError(t, os.WriteFile(filepath.Join(worktreePath, "README.md"), []byte("# feature edit\n"), 0o644))
	runGit(t, worktreePath, "add", "README.md")
	runGit(t, worktreePath, "commit", "-m", "feature edits README")

	require.NoError(t, os.WriteFile(filepath.Join(origin, "README.md"), []byte("# main edit\n"), 0o644))
	runGit(t, origin, "add", "README.md")
	runGit(t, origin, "commit", "-m", "main edits README")

	result, err := MergeMainIntoWorktree(worktreePath, "main")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Conflicted, "fixture requires a real conflict, not a clean merge")

	status := runGit(t, worktreePath, "status", "--porcelain")
	require.Empty(t, strings.TrimSpace(status), "materializeConflictOnAbort must leave the worktree clean before the flag flip")

	return origin, mainRepo, worktreePath, wt
}

// TestNativeGitRollout_migration_should_BeReversible_When_FlagFlippedOffAfterNativeWorktreeAndMergeExist
// covers validation.md's required-but-missing reversibility test: native create → native
// merge (including a conflict) → flag flip off → legacy operates correctly on that same
// worktree, exercised through findLiveWorktreeForBranch/Remove/Prune's public dispatch
// (which resolves to legacyFindLiveWorktreeForBranch/legacyRemoveWorktree/
// legacyWorktreePrune once the flag is off), not just a same-implementation round trip.
func TestNativeGitRollout_migration_should_BeReversible_When_FlagFlippedOffAfterNativeWorktreeAndMergeExist(t *testing.T) {
	t.Run("LegacyFindLiveWorktreeForBranch", func(t *testing.T) {
		_, _, worktreePath, wt := newNativeWorktreeAfterConflictedMerge(t, "find")

		useNativeWorktree = func(string) bool { return false }
		useNativeMerge = func(string) bool { return false }

		foundPath, found := wt.findLiveWorktreeForBranch()
		require.True(t, found, "legacyFindLiveWorktreeForBranch must recognize a native-created, native-merged (incl. a conflict) worktree")
		assert.Equal(t, CanonicalizeWorktreePath(worktreePath), CanonicalizeWorktreePath(foundPath))
	})

	t.Run("LegacyRemove", func(t *testing.T) {
		_, mainRepo, worktreePath, wt := newNativeWorktreeAfterConflictedMerge(t, "remove")
		branchName := filepath.Base(worktreePath)

		useNativeWorktree = func(string) bool { return false }
		useNativeMerge = func(string) bool { return false }

		require.NoError(t, wt.Remove())

		_, err := os.Stat(worktreePath)
		assert.True(t, os.IsNotExist(err), "legacy Remove must delete a native-created, native-merged worktree's working directory")

		adminDir := filepath.Join(mainRepo, ".git", "worktrees", branchName)
		_, err = os.Stat(adminDir)
		assert.True(t, os.IsNotExist(err), "legacy Remove must delete a native-created, native-merged worktree's admin dir")
	})

	t.Run("LegacyPrune", func(t *testing.T) {
		_, mainRepo, worktreePath, wt := newNativeWorktreeAfterConflictedMerge(t, "prune")
		branchName := filepath.Base(worktreePath)

		useNativeWorktree = func(string) bool { return false }
		useNativeMerge = func(string) bool { return false }

		// Make the entry prunable exactly like the existing native-prune tests do: delete
		// the working directory out from under git, leaving only the admin dir behind.
		require.NoError(t, os.RemoveAll(worktreePath))

		require.NoError(t, wt.Prune())

		adminDir := filepath.Join(mainRepo, ".git", "worktrees", branchName)
		_, err := os.Stat(adminDir)
		assert.True(t, os.IsNotExist(err), "legacy Prune must recognize and remove a native-created, native-merged worktree's admin dir once its working directory is gone")
	})
}

// TestNativeGitRollout_migration_should_BeReversible_When_FlagFlippedOffAfterInterruptedNativeAdd
// covers validation.md's second required-but-missing reversibility test. Unlike
// TestNativeSetupNewWorktree_CrashBetweenGitdirAndCommondir_IsPrunableToRealGit (which only
// proves real git's OWN `worktree list --porcelain` recognizes this partial state), this
// exercises THIS codebase's own legacy implementation
// (legacyFindLiveWorktreeForBranch/legacyWorktreePrune) against the same on-disk state
// after a flag flip — the crash-simulation approach (stop after LockedMarker+GitdirFile,
// mirroring real git's own write order) is reused verbatim from that test.
func TestNativeGitRollout_migration_should_BeReversible_When_FlagFlippedOffAfterInterruptedNativeAdd(t *testing.T) {
	branchName := "feature-migration-reversible-interrupted"
	repoPath, worktreePath := newNativeAddTarget(t, branchName)

	adminDir, err := AllocateAdminDirName(repoPath, filepath.Base(worktreePath))
	require.NoError(t, err)

	w := NewAdminFileWriter(adminDir)
	require.NoError(t, w.WriteFile("locked", []byte("initializing")))
	require.NoError(t, w.WriteFile("gitdir", []byte(filepath.Join(worktreePath, ".git")+"\n")))
	// Deliberately stop here, mirroring
	// TestNativeSetupNewWorktree_CrashBetweenGitdirAndCommondir_IsPrunableToRealGit —
	// commondir/HEAD/the worktree's own .git redirect file are never written, and
	// worktreePath's own working directory never gets created at all.

	origWorktree := useNativeWorktree
	useNativeWorktree = func(string) bool { return false }
	t.Cleanup(func() { useNativeWorktree = origWorktree })

	wt := NewGitWorktreeFromStorageWithExecutor(repoPath, worktreePath, "test-migration-reversible-interrupted", branchName, "")

	// legacyFindLiveWorktreeForBranch must not crash or wrongly report this half-written,
	// working-directory-less admin dir as a live worktree for branchName: no HEAD/branch
	// ref was ever written for it, so real `git worktree list --porcelain` (which
	// legacyFindLiveWorktreeForBranch shells out to) has no "branch" line to match against.
	_, found := wt.legacyFindLiveWorktreeForBranch()
	assert.False(t, found, "an interrupted native Add's half-written admin dir must not be reported as a live worktree by the legacy implementation")

	// legacyWorktreePrune (a thin wrapper over real `git worktree prune`) must handle this
	// state without erroring. Real git itself recognizes a LockedMarker-present entry as
	// "locked", not prunable
	// (TestNativeSetupNewWorktree_CrashBetweenGitdirAndCommondir_IsPrunableToRealGit), so
	// the interrupted admin dir is expected to survive prune untouched, not be silently
	// removed or crash the call.
	require.NoError(t, wt.legacyWorktreePrune())
	_, statErr := os.Stat(adminDir)
	assert.NoError(t, statErr, "a locked, interrupted admin dir must survive prune untouched, matching real git's own locked-takes-priority-over-prunable rule")
}
