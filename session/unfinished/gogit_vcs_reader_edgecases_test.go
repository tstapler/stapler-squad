package unfinished

// Characterization tests (Story 0.1.4a): each pins what the reader does TODAY
// for an edge-case repository shape, so that sharing work cannot silently
// change it. They assert current behaviour, including known parity bugs; they
// are not a statement that the behaviour is right. Fixtures are go-git only.

import (
	"os"
	"path/filepath"
	"testing"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/unfinished/internal/testrepo"
)

// storeTree encodes and stores a tree object holding entries.
func storeTree(t *testing.T, repo *git.Repository, entries []object.TreeEntry) plumbing.Hash {
	t.Helper()
	obj := repo.Storer.NewEncodedObject()
	require.NoError(t, (&object.Tree{Entries: entries}).Encode(obj))
	h, err := repo.Storer.SetEncodedObject(obj)
	require.NoError(t, err)
	return h
}

func TestEdgeCase_Shallow_AheadBehind_ShouldWalkToBoundary(t *testing.T) {
	t.Parallel()
	repo, dir := testrepo.NewRepo(t)
	hashes := testrepo.Linear(t, repo, 4)
	testrepo.SetBranch(t, repo, "base", hashes[1])
	// Boundary at hashes[1]: its parent hashes[0] is "not available".
	testrepo.Shallow(t, repo, hashes[1])

	g := &GoGitVCSReader{}
	ahead, behind, err := g.AheadBehind(dir, "base")
	require.NoError(t, err)
	assert.Equal(t, 2, ahead, "HEAD is two commits past the shallow boundary")
	assert.Equal(t, 0, behind)

	// The boundary is honoured by go-git's own reader, so the walk cannot
	// reach hashes[0] even though the object is still in the store.
	boundary, err := repo.Storer.Shallow()
	require.NoError(t, err)
	assert.Equal(t, []plumbing.Hash{hashes[1]}, boundary)
}

// Resolves Unresolved Question 9: `shallow` lives in the COMMON dir, shared by
// every linked worktree; a linked worktree's own gitdir never holds one.
func TestEdgeCase_Shallow_ShouldLiveInCommonDir_WhenLinkedWorktreeExists(t *testing.T) {
	t.Parallel()
	repo, dir := testrepo.NewRepo(t)
	hashes := testrepo.Linear(t, repo, 3)
	linked := testrepo.NewLinkedWorktree(t, dir, "wt1", "feature")

	// Written through the linked worktree's own handle, as a depth-limited
	// fetch run from that worktree would.
	linkedRepo := testrepo.OpenLinked(t, linked)
	require.NoError(t, linkedRepo.Storer.SetShallow([]plumbing.Hash{hashes[1]}))

	commonShallow := filepath.Join(dir, ".git", "shallow")
	adminShallow := filepath.Join(dir, ".git", "worktrees", "wt1", "shallow")
	assert.FileExists(t, commonShallow, "shallow is written to the common dir")
	assert.NoFileExists(t, adminShallow, "no per-worktree shallow file")

	// ... and so the main worktree (and any other linked one) sees it.
	mainBoundary, err := repo.Storer.Shallow()
	require.NoError(t, err)
	assert.Equal(t, []plumbing.Hash{hashes[1]}, mainBoundary)
	linkedBoundary, err := testrepo.OpenLinked(t, linked).Storer.Shallow()
	require.NoError(t, err)
	assert.Equal(t, []plumbing.Hash{hashes[1]}, linkedBoundary)
}

func TestEdgeCase_SymlinkedPath_ShouldShareOneRegistryEntry_ButTwoCommonDirStrings(t *testing.T) {
	t.Parallel()
	repo, real := testrepo.NewRepo(t)
	testrepo.Linear(t, repo, 2)
	testrepo.SetBranch(t, repo, "base", mustHead(t, repo))

	link := filepath.Join(testrepo.TempDir(t), "via-symlink")
	require.NoError(t, os.Symlink(real, link))

	g := &GoGitVCSReader{}
	_, _, err := g.AheadBehind(real, "base")
	require.NoError(t, err)
	_, _, err = g.AheadBehind(link, "base")
	require.NoError(t, err)

	// gogitstore canonicalizes (EvalSymlinks) its registry key: one store,
	// referenced once per path-keyed repoCache entry.
	refCounts := g.gogitstoreRegistry().RefCounts()
	require.Len(t, refCounts, 1, "both routes resolve to one shared object store")
	for _, rc := range refCounts {
		assert.Equal(t, int32(2), rc)
	}
	assert.Equal(t, int64(2), g.RepoCacheStats().ColdOpens, "repoCache itself is keyed by the raw path")

	// gitCommonDir does no symlink resolution, so the two routes differ as strings.
	assert.Equal(t, filepath.Join(real, ".git"), gitCommonDir(real))
	assert.Equal(t, filepath.Join(link, ".git"), gitCommonDir(link))
	assert.NotEqual(t, gitCommonDir(real), gitCommonDir(link))
}

func TestEdgeCase_UnbornBranch_ShouldReturnNilNil_AndAheadBehindErrorsUncached(t *testing.T) {
	t.Parallel()
	_, dir := testrepo.NewRepo(t) // HEAD -> refs/heads/main, no commits

	g := &GoGitVCSReader{}
	entry, err := g.openRepoEntry(dir)
	require.NoError(t, err)
	hashes, err := resolveHeadTreeHashes(g, entry, entry.repo)
	require.NoError(t, err)
	assert.Nil(t, hashes, "unborn HEAD is the documented (nil, nil) sentinel")

	_, _, err = g.AheadBehind(dir, "main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "head:")
	_, cached := g.aheadBehindCache.Load(dir + "\x00" + "main")
	assert.False(t, cached, "a failed AheadBehind must not be cached")

	dirty, err := g.HasUncommitted(dir)
	require.NoError(t, err)
	assert.False(t, dirty, "an empty unborn repo has nothing staged or untracked")
}

// Current parity bug, documented not fixed: the HEAD tree walk skips only
// filemode.Dir, so a gitlink (submodule) entry is recorded like a blob, keyed
// by the submodule commit hash.
func TestEdgeCase_SubmoduleGitlink_ShouldBeRecordedAsBlobLikeEntry(t *testing.T) {
	t.Parallel()
	repo, dir := testrepo.NewRepo(t)
	blob := repo.Storer.NewEncodedObject()
	blob.SetType(plumbing.BlobObject)
	w, err := blob.Writer()
	require.NoError(t, err)
	_, err = w.Write([]byte("hello\n"))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	blobHash, err := repo.Storer.SetEncodedObject(blob)
	require.NoError(t, err)

	gitlinkTarget := plumbing.NewHash("1111111111111111111111111111111111111111")
	tree := storeTree(t, repo, []object.TreeEntry{
		{Name: "README", Mode: filemode.Regular, Hash: blobHash},
		{Name: "sub", Mode: filemode.Submodule, Hash: gitlinkTarget},
	})
	commit := testrepo.RawCommit(t, repo, tree, nil, "with gitlink", testrepo.Epoch)
	testrepo.SetBranch(t, repo, "main", commit)

	g := &GoGitVCSReader{}
	entry, err := g.openRepoEntry(dir)
	require.NoError(t, err)
	hashes, err := resolveHeadTreeHashes(g, entry, entry.repo)
	require.NoError(t, err)
	assert.Equal(t, map[string]plumbing.Hash{"README": blobHash, "sub": gitlinkTarget}, hashes,
		"gitlink recorded as a blob-like name->hash entry")
}

// Sparse checkout: go-git persists the skip-worktree bit, but HasUncommitted
// never reads it, so a skip-worktree entry whose file is absent is reported as
// a tracked-file deletion (dirty). Characterizes today's behaviour; Story 4.2.2
// calls the guard a deliberate fix.
func TestEdgeCase_SparseCheckout_SkipWorktreeAbsentFile_ShouldReportDirtyToday(t *testing.T) {
	t.Parallel()
	repo, dir := testrepo.NewRepo(t)
	testrepo.Commit(t, repo, map[string]string{"keep.txt": "k\n", "sparse.txt": "s\n"}, "two files", testrepo.Epoch)

	g := &GoGitVCSReader{}
	dirty, err := g.HasUncommitted(dir)
	require.NoError(t, err)
	require.False(t, dirty, "precondition: clean checkout")

	idx, err := repo.Storer.Index()
	require.NoError(t, err)
	var marked bool
	for _, e := range idx.Entries {
		if e.Name == "sparse.txt" {
			e.SkipWorktree = true
			marked = true
		}
	}
	require.True(t, marked)
	require.NoError(t, repo.Storer.SetIndex(idx))
	require.NoError(t, os.Remove(filepath.Join(dir, "sparse.txt")))

	reread, err := repo.Storer.Index()
	require.NoError(t, err)
	assert.True(t, skipWorktreeSet(reread, "sparse.txt"), "go-git round-trips the skip-worktree bit")

	g2 := &GoGitVCSReader{} // fresh reader: no result cache from the clean probe
	dirty, err = g2.HasUncommitted(dir)
	require.NoError(t, err)
	assert.True(t, dirty, "skip-worktree is not consulted: the absent file reads as deleted")
}

func skipWorktreeSet(idx *index.Index, name string) bool {
	for _, e := range idx.Entries {
		if e.Name == name {
			return e.SkipWorktree
		}
	}
	return false
}

// A bare repo has no `.git` entry, which gogitstore.Open requires, so every
// reader entry point errors rather than reading the bare layout.
func TestEdgeCase_BareRepo_ShouldFailToOpen(t *testing.T) {
	t.Parallel()
	dir := testrepo.TempDir(t)
	_, err := git.PlainInit(dir, true)
	require.NoError(t, err)

	g := &GoGitVCSReader{}
	_, err = g.openRepoEntry(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stat .git")

	_, err = g.ListWorktrees(dir)
	require.Error(t, err)
	_, _, err = g.AheadBehind(dir, "main")
	require.Error(t, err)
	assert.Empty(t, g.gogitstoreRegistry().RefCounts(), "a failed open registers nothing")
}

func mustHead(t *testing.T, repo *git.Repository) plumbing.Hash {
	t.Helper()
	head, err := repo.Head()
	require.NoError(t, err)
	return head.Hash()
}
