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
