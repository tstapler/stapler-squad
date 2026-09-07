package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mergeAbortFixture builds a repo where HEAD (branch "feature") has a.txt = "line1\nours\n",
// with a distinct "theirs" commit on "main" carrying a.txt = "line1\ntheirs\n" and a
// common ancestor carrying "line1\n" — real blob hashes for all three, and the
// then-current (pre-conflict) working-tree content, for writeConflictedIndex +
// writeMergeStateFiles + abortNativeMerge's real-git-recognized round trip.
type mergeAbortFixture struct {
	repoPath        string
	conflictEntries []*index.Entry
	preMergeEntry   *index.Entry
	preMergeContent []byte
	theirsSHA       string
}

func buildMergeAbortFixture(t *testing.T) mergeAbortFixture {
	t.Helper()
	r := newIndexTestRepo(t)

	baseCommit := r.writeAndCommit("a.txt", "line1\n", "base")
	baseHash := commitBlobHash(t, baseCommit, "a.txt")

	require.NoError(t, r.wt.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("feature"),
		Create: true,
	}))
	oursCommit := r.writeAndCommit("a.txt", "line1\nours\n", "ours")
	oursHash := commitBlobHash(t, oursCommit, "a.txt")

	require.NoError(t, r.wt.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("main"),
	}))
	theirsCommit := r.writeAndCommit("a.txt", "line1\ntheirs\n", "theirs")
	theirsHash := commitBlobHash(t, theirsCommit, "a.txt")

	require.NoError(t, r.wt.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("feature"),
	}))

	return mergeAbortFixture{
		repoPath:        r.path,
		conflictEntries: NewConflictEntries("a.txt", baseHash, oursHash, theirsHash, filemode.Regular),
		preMergeEntry:   &index.Entry{Name: "a.txt", Hash: oursHash, Mode: filemode.Regular},
		preMergeContent: []byte("line1\nours\n"),
		theirsSHA:       theirsCommit.Hash.String(),
	}
}

// materializeFixtureConflict writes the fixture's conflicted index and conflict-marker
// working-tree content plus merge-state files — the "materialize" half of
// materializeConflictOnAbort, built from Story 3.3.1/3.3.2/3.3.3's functions together.
func materializeFixtureConflict(t *testing.T, f mergeAbortFixture) {
	t.Helper()
	require.NoError(t, writeConflictedIndex(f.repoPath, f.conflictEntries))

	hunk := MergeHunk{Kind: RegionConflict, Ours: []string{"ours"}, Theirs: []string{"theirs"}}
	markerText, err := renderConflictHunk(hunk, "HEAD", "origin/main")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(f.repoPath, "a.txt"), []byte(markerText), 0o644))

	require.NoError(t, writeMergeStateFiles(f.repoPath, f.theirsSHA, "main"))
}

// TestWriteMergeStateFiles_RealGitMergeAbortSucceeds covers Story 3.3.3's first
// acceptance criterion: a real `git merge --abort` subprocess run against the
// materialized state must exit 0 and leave a clean working tree.
func TestWriteMergeStateFiles_RealGitMergeAbortSucceeds(t *testing.T) {
	t.Parallel()
	f := buildMergeAbortFixture(t)
	materializeFixtureConflict(t, f)

	runRealGitIndexTest(t, f.repoPath, "merge", "--abort")

	statusOut := runRealGitIndexTest(t, f.repoPath, "status", "--porcelain")
	assert.Empty(t, strings.TrimSpace(statusOut), "worktree must be clean after a real git merge --abort")
}

// TestAbortNativeMerge_ClearsStateAndResetsWorkingTree covers Story 3.3.3's second
// acceptance criterion: after this project's own abort path, the three merge-state
// files are gone and the working tree matches its pre-merge state.
func TestAbortNativeMerge_ClearsStateAndResetsWorkingTree(t *testing.T) {
	t.Parallel()
	f := buildMergeAbortFixture(t)
	materializeFixtureConflict(t, f)

	snapshot := &PreMergeIndexSnapshot{
		Entries: []*index.Entry{f.preMergeEntry},
		Content: map[string][]byte{"a.txt": f.preMergeContent},
	}
	require.NoError(t, abortNativeMerge(f.repoPath, snapshot))

	gitDir := filepath.Join(f.repoPath, ".git")
	for _, name := range []string{mergeHeadFile, mergeMsgFile, mergeModeFile} {
		_, err := os.Stat(filepath.Join(gitDir, name))
		assert.True(t, os.IsNotExist(err), "%s must be removed after abortNativeMerge", name)
	}

	content, err := os.ReadFile(filepath.Join(f.repoPath, "a.txt"))
	require.NoError(t, err)
	assert.Equal(t, f.preMergeContent, content)

	statusOut := runRealGitIndexTest(t, f.repoPath, "status", "--porcelain")
	assert.Empty(t, strings.TrimSpace(statusOut), "worktree must be clean after abortNativeMerge")
}

func TestAbortNativeMerge_should_ReturnError_When_PreMergeIndexSnapshotMissing(t *testing.T) {
	t.Parallel()
	f := buildMergeAbortFixture(t)
	materializeFixtureConflict(t, f)

	err := abortNativeMerge(f.repoPath, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "pre-merge index snapshot")
}

// TestNativeMergeMainIntoWorktree_UpToDate covers Task 3.4.1a's up-to-date short-circuit:
// a branch that already contains origin/main's tip must be reported UpToDate with no ref
// change, matching legacyMergeMainIntoWorktree's existing semantics exactly.
func TestNativeMergeMainIntoWorktree_UpToDate(t *testing.T) {
	t.Parallel()
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	runGit(t, work, "checkout", "-b", "feature")

	beforeSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))

	result, err := nativeMergeMainIntoWorktree(work, "main")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.UpToDate)
	assert.False(t, result.Merged)
	assert.False(t, result.Conflicted)

	afterSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))
	assert.Equal(t, beforeSHA, afterSHA, "an up-to-date merge must not touch the branch ref")
}

// TestNativeMergeMainIntoWorktree_FastForward covers Task 3.4.1a's fast-forward
// short-circuit: a strict-ancestor branch must be reported Merged with its ref advanced
// to origin/main's exact SHA (subprocess-verified per the acceptance criteria), and its
// working tree updated to match.
func TestNativeMergeMainIntoWorktree_FastForward(t *testing.T) {
	t.Parallel()
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	runGit(t, work, "checkout", "-b", "feature")

	require.NoError(t, os.WriteFile(filepath.Join(origin, "main-fix.txt"), []byte("fix on main\n"), 0o644))
	runGit(t, origin, "add", "main-fix.txt")
	runGit(t, origin, "commit", "-m", "fix landed on main")

	result, err := nativeMergeMainIntoWorktree(work, "main")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Merged)
	assert.False(t, result.UpToDate)
	assert.False(t, result.Conflicted)

	wantSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "origin/main"))
	gotSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "feature"))
	assert.Equal(t, wantSHA, gotSHA, "the checked-out branch ref must now match origin/main's SHA exactly")

	assert.FileExists(t, filepath.Join(work, "main-fix.txt"))
	status := runGit(t, work, "status", "--porcelain")
	assert.Empty(t, strings.TrimSpace(status), "a real git status must see the fast-forward as clean")
}

// TestNativeMergeMainIntoWorktree_CleanThreeWayMerge covers Task 3.4.1b: a divergence
// with no overlapping edits must produce a real two-parent merge commit (subprocess-
// verified via `git log -1 --format=%P`), with both sides' files present and the
// worktree left clean.
func TestNativeMergeMainIntoWorktree_CleanThreeWayMerge(t *testing.T) {
	t.Parallel()
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	runGit(t, work, "checkout", "-b", "feature")

	require.NoError(t, os.WriteFile(filepath.Join(work, "feature.txt"), []byte("feature work\n"), 0o644))
	runGit(t, work, "add", "feature.txt")
	runGit(t, work, "commit", "-m", "feature work")

	require.NoError(t, os.WriteFile(filepath.Join(origin, "main-fix.txt"), []byte("fix on main\n"), 0o644))
	runGit(t, origin, "add", "main-fix.txt")
	runGit(t, origin, "commit", "-m", "fix landed on main")

	result, err := nativeMergeMainIntoWorktree(work, "main")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Merged)
	assert.False(t, result.UpToDate)
	assert.False(t, result.Conflicted)

	assert.FileExists(t, filepath.Join(work, "feature.txt"))
	assert.FileExists(t, filepath.Join(work, "main-fix.txt"))

	parents := strings.Fields(runGit(t, work, "log", "-1", "--format=%P"))
	assert.Len(t, parents, 2, "a native clean merge must produce a real two-parent commit")

	status := runGit(t, work, "status", "--porcelain")
	assert.Empty(t, strings.TrimSpace(status), "a real git status must see the merge as clean")
}

// TestNativeMergeMainIntoWorktree_Conflicted_LeavesWorktreeClean covers Task 3.4.1c: an
// overlapping edit must be reported Conflicted with the correct file list, and — per
// materializeConflictAndAbort's "always materialize, then abort" decision — leave the
// worktree exactly as clean as a real `git merge --abort` would, verified via a real `git
// status --porcelain` subprocess, not just internal state.
func TestNativeMergeMainIntoWorktree_Conflicted_LeavesWorktreeClean(t *testing.T) {
	t.Parallel()
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	runGit(t, work, "checkout", "-b", "feature")

	require.NoError(t, os.WriteFile(filepath.Join(work, "README.md"), []byte("# Feature Edit\n"), 0o644))
	runGit(t, work, "add", "README.md")
	runGit(t, work, "commit", "-m", "feature edits README")

	require.NoError(t, os.WriteFile(filepath.Join(origin, "README.md"), []byte("# Main Edit\n"), 0o644))
	runGit(t, origin, "add", "README.md")
	runGit(t, origin, "commit", "-m", "main edits README")

	beforeSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))

	result, err := nativeMergeMainIntoWorktree(work, "main")
	require.NoError(t, err, "a real conflict must be reported via the result, not returned as an error")
	require.NotNil(t, result)
	assert.True(t, result.Conflicted)
	assert.False(t, result.UpToDate)
	assert.False(t, result.Merged)
	assert.Equal(t, []string{"README.md"}, result.ConflictedFiles)

	afterSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))
	assert.Equal(t, beforeSHA, afterSHA, "a conflicted native merge must leave HEAD untouched")
	assert.NoFileExists(t, filepath.Join(work, ".git", "MERGE_HEAD"))

	status := runGit(t, work, "status", "--porcelain")
	assert.Empty(t, strings.TrimSpace(status), "worktree must be exactly as clean as a real git merge --abort would leave it")
}

// TestNativeMergeMainIntoWorktree_should_ReturnError_When_FetchFails mirrors the existing
// legacy-path test of the same shape: an unreachable origin must surface as an error, not
// any MergeMainResult state, and must leave no partial merge state on disk.
func TestNativeMergeMainIntoWorktree_should_ReturnError_When_FetchFails(t *testing.T) {
	t.Parallel()
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)
	runGit(t, work, "checkout", "-b", "feature")
	runGit(t, work, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "does-not-exist"))

	result, err := nativeMergeMainIntoWorktree(work, "main")
	require.Error(t, err)
	assert.Nil(t, result)
	assert.NoFileExists(t, filepath.Join(work, ".git", "MERGE_HEAD"))
}
