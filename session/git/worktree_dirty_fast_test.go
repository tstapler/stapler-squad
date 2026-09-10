package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// TestWorktreeIsDirtyFast_MatchesWorktreeIsDirty_OnCleanUntrackedAndModified is the
// direct regression test for the algorithm swap in dirtyCheckerFunc (worktree.go):
// worktreeIsDirtyFast must agree with the go-git Worktree.Status()-backed
// worktreeIsDirty on the same scenarios TestWorktreeIsDirty_DetectsUntrackedAndModifiedFiles
// already covers, since it now runs in worktreeIsDirty's place on every production
// IsDirty()/HasStagedChanges() call.
func TestWorktreeIsDirtyFast_MatchesWorktreeIsDirty_OnCleanUntrackedAndModified(t *testing.T) {
	t.Parallel()
	repoDir := setupTestRepo(t)
	var cache gitignoreFSCache
	var headCache headTreeHashCache

	dirty, err := worktreeIsDirtyFast(repoDir, &cache, &headCache)
	require.NoError(t, err)
	assert.False(t, dirty, "freshly committed repo must report clean")

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "untracked.txt"), []byte("new"), 0o644))
	dirty, err = worktreeIsDirtyFast(repoDir, &cache, &headCache)
	require.NoError(t, err)
	assert.True(t, dirty, "an untracked file must report dirty")

	require.NoError(t, os.Remove(filepath.Join(repoDir, "untracked.txt")))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("changed"), 0o644))
	dirty, err = worktreeIsDirtyFast(repoDir, &cache, &headCache)
	require.NoError(t, err)
	assert.True(t, dirty, "a modified tracked file must report dirty")
}

// TestWorktreeIsDirtyFast_DetectsStagedAddition covers worktreeStagedDirty's "new or
// modified staged file" branch directly (not reachable via untracked/unstaged alone).
func TestWorktreeIsDirtyFast_DetectsStagedAddition(t *testing.T) {
	t.Parallel()
	repoDir := setupTestRepo(t)
	var cache gitignoreFSCache
	var headCache headTreeHashCache

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "staged.txt"), []byte("new"), 0o644))
	repo, err := OpenRepo(repoDir)
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)
	_, err = wt.Add("staged.txt")
	require.NoError(t, err)

	dirty, err := worktreeIsDirtyFast(repoDir, &cache, &headCache)
	require.NoError(t, err)
	assert.True(t, dirty, "a newly-staged file must report dirty")
}

// TestWorktreeIsDirtyFast_DetectsStagedDeletion covers worktreeStagedDirty's "staged
// deletion" branch: a file present in HEAD but removed from the index (via `git rm
// --cached`), with the working-tree copy still present unmodified.
func TestWorktreeIsDirtyFast_DetectsStagedDeletion(t *testing.T) {
	t.Parallel()
	repoDir := setupTestRepo(t)
	var cache gitignoreFSCache
	var headCache headTreeHashCache

	out, err := safeexec.CommandContext(context.Background(), "git", "-C", repoDir, "rm", "--cached", "README.md").CombinedOutput()
	require.NoError(t, err, "git rm --cached failed: %s", out)

	dirty, err := worktreeIsDirtyFast(repoDir, &cache, &headCache)
	require.NoError(t, err)
	assert.True(t, dirty, "a staged deletion must report dirty even though the worktree copy is untouched")
}

// TestWorktreeIsDirtyFast_DetectsMergeConflictStage covers worktreeStagedDirty's
// unresolved-merge-conflict branch (index.Entry.Stage != 0), which the mtime/hash
// short-circuit can't detect any other way since a conflicted entry has no single
// well-defined hash to compare against HEAD.
func TestWorktreeIsDirtyFast_DetectsMergeConflictStage(t *testing.T) {
	t.Parallel()
	repoDir := setupTestRepo(t)
	var cache gitignoreFSCache
	var headCache headTreeHashCache

	run := func(args ...string) {
		out, err := safeexec.CommandContext(context.Background(), "git", append([]string{"-C", repoDir}, args...)...).CombinedOutput()
		require.NoError(t, err, "git %v failed: %s", args, out)
	}
	run("checkout", "-b", "conflict-branch")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("branch content"), 0o644))
	run("commit", "-am", "branch change")
	run("checkout", "main")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("main content"), 0o644))
	run("commit", "-am", "main change")
	_, _ = safeexec.CommandContext(context.Background(), "git", "-C", repoDir, "merge", "conflict-branch").CombinedOutput() //nolint:errcheck // a merge conflict is the expected, non-error-checked outcome here

	dirty, err := worktreeIsDirtyFast(repoDir, &cache, &headCache)
	require.NoError(t, err)
	assert.True(t, dirty, "an unresolved merge conflict must report dirty")
}

// TestWorktreeIsDirtyFast_UnbornHEAD_ReportsCleanRegardlessOfIndex documents
// worktreeStagedDirty's existing, deliberately-preserved behavior for an unborn HEAD
// (headHashes == nil): it treats the staged-vs-HEAD comparison as a no-op rather than
// as "everything in the index is newly staged", matching
// session/unfinished.GoGitVCSReader.hasUncommittedGoGitPhase's identical rule exactly
// (see worktreeStagedDirty's doc comment) rather than introducing a new interpretation.
func TestWorktreeIsDirtyFast_UnbornHEAD_ReportsCleanRegardlessOfIndex(t *testing.T) {
	t.Parallel()
	repoDir := t.TempDir()
	repo, err := git.PlainInit(repoDir, false)
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "file.txt"), []byte("x"), 0o644))
	_, err = wt.Add("file.txt")
	require.NoError(t, err)

	_, err = repo.Head()
	require.ErrorIs(t, err, plumbing.ErrReferenceNotFound, "sanity check: repo must genuinely have no HEAD yet")

	var cache gitignoreFSCache
	var headCache headTreeHashCache
	dirty, err := worktreeIsDirtyFast(repoDir, &cache, &headCache)
	require.NoError(t, err)
	assert.False(t, dirty)
}

// TestCachedHeadTreeHashes_ReusesCacheUntilHeadMoves is PerfFix-4's
// enforcement: a second call against an unchanged HEAD must return the
// exact same cached map (proving headTreeHashes was not re-walked), and a
// call after HEAD moves must return a freshly computed, correctly updated
// map (proving the cache doesn't serve stale data across a commit).
func TestCachedHeadTreeHashes_ReusesCacheUntilHeadMoves(t *testing.T) {
	t.Parallel()
	repoDir := setupTestRepo(t)
	repo, err := OpenRepo(repoDir)
	require.NoError(t, err)
	var cache headTreeHashCache

	first, err := cachedHeadTreeHashes(repo, &cache)
	require.NoError(t, err)
	_, hasNewFile := first["new.txt"]
	assert.False(t, hasNewFile, "new.txt must not exist in the tree yet")

	second, err := cachedHeadTreeHashes(repo, &cache)
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("%p", first), fmt.Sprintf("%p", second),
		"an unchanged HEAD must return the cached map, not recompute it")

	// Move HEAD by committing a new file.
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "new.txt"), []byte("x"), 0o644))
	wt, err := repo.Worktree()
	require.NoError(t, err)
	_, err = wt.Add("new.txt")
	require.NoError(t, err)
	_, err = wt.Commit("add new.txt", &git.CommitOptions{
		Author: &object.Signature{Name: "Test User", Email: "test@example.com", When: time.Now()},
	})
	require.NoError(t, err)

	third, err := cachedHeadTreeHashes(repo, &cache)
	require.NoError(t, err)
	assert.NotEqual(t, fmt.Sprintf("%p", first), fmt.Sprintf("%p", third),
		"a moved HEAD must invalidate the cache and recompute")
	_, hasNewFile = third["new.txt"]
	assert.True(t, hasNewFile, "the recomputed map must reflect the new HEAD tree")
}

// setupLargeTreeBenchRepo builds a repo with a wide tracked tree (many files across
// nested directories, all committed) so merkletrie.DiffTree has real tree-diff work to
// do -- nestedDirsForBench's fixture only adds untracked .gitignore files, leaving the
// tracked tree trivial (just README.md) and understating go-git Status()'s cost.
func setupLargeTreeBenchRepo(b *testing.B, dirs, filesPerDir int) string {
	b.Helper()
	repoDir := setupBenchRepo(b)
	for d := 0; d < dirs; d++ {
		dir := filepath.Join(repoDir, "pkg", "group"+string(rune('a'+d%26)), "sub"+string(rune('a'+d/26)))
		require.NoError(b, os.MkdirAll(dir, 0o750))
		for f := 0; f < filesPerDir; f++ {
			require.NoError(b, os.WriteFile(filepath.Join(dir, "file"+string(rune('a'+f))+".go"), []byte("package pkg"), 0o600))
		}
	}
	repo, err := OpenRepo(repoDir)
	require.NoError(b, err)
	wt, err := repo.Worktree()
	require.NoError(b, err)
	_, err = wt.Add(".")
	require.NoError(b, err)
	_, err = wt.Commit("add tracked tree", &git.CommitOptions{
		Author: &object.Signature{Name: "Bench User", Email: "bench@example.com", When: time.Now()},
	})
	require.NoError(b, err)
	return repoDir
}

// BenchmarkWorktreeIsDirty_FastVsGoGit quantifies the algorithm swap: worktreeIsDirtyFast
// (mtime/hash short-circuit) against worktreeIsDirtyWithFS (go-git Worktree.Status(),
// full merkletrie tree-diff) on a repo with a wide tracked tree. Run with -benchmem; live
// production profiling found merkletrie.DiffTree/diffNodes at 16.58% cum CPU inside the
// go-git path even with gitignoreFSCache already warm.
func BenchmarkWorktreeIsDirty_FastVsGoGit(b *testing.B) {
	repoDir := setupLargeTreeBenchRepo(b, 40, 20)

	b.Run("Fast", func(b *testing.B) {
		var cache gitignoreFSCache
		var headCache headTreeHashCache
		if _, err := worktreeIsDirtyFast(repoDir, &cache, &headCache); err != nil {
			b.Fatal(err)
		}
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := worktreeIsDirtyFast(repoDir, &cache, &headCache); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("GoGit", func(b *testing.B) {
		var cache gitignoreFSCache
		if _, err := worktreeIsDirtyWithFS(repoDir, &cache); err != nil {
			b.Fatal(err)
		}
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := worktreeIsDirtyWithFS(repoDir, &cache); err != nil {
				b.Fatal(err)
			}
		}
	})
}
