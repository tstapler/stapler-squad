package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

func TestNewConflictEntries_ThreeStages(t *testing.T) {
	t.Parallel()
	h1 := plumbing.ComputeHash(plumbing.BlobObject, []byte("base"))
	h2 := plumbing.ComputeHash(plumbing.BlobObject, []byte("ours"))
	h3 := plumbing.ComputeHash(plumbing.BlobObject, []byte("theirs"))

	entries := NewConflictEntries("a.txt", h1, h2, h3, filemode.Regular)

	require.Len(t, entries, 3)
	byStage := map[index.Stage]*index.Entry{}
	for _, e := range entries {
		assert.Equal(t, "a.txt", e.Name)
		byStage[e.Stage] = e
	}
	require.Contains(t, byStage, index.AncestorMode)
	require.Contains(t, byStage, index.OurMode)
	require.Contains(t, byStage, index.TheirMode)
	assert.Equal(t, h1, byStage[index.AncestorMode].Hash)
	assert.Equal(t, h2, byStage[index.OurMode].Hash)
	assert.Equal(t, h3, byStage[index.TheirMode].Hash)
}

// TestNewConflictEntries_SkipsZeroHashStages covers the add/add and delete/modify
// partial-stage cases (stack.md §3): a zero hash for a stage means that side has no
// entry at all, not a literal zero-content blob.
func TestNewConflictEntries_SkipsZeroHashStages(t *testing.T) {
	t.Parallel()
	oursHash := plumbing.ComputeHash(plumbing.BlobObject, []byte("ours"))
	theirsHash := plumbing.ComputeHash(plumbing.BlobObject, []byte("theirs"))

	entries := NewConflictEntries("a.txt", plumbing.ZeroHash, oursHash, theirsHash, filemode.Regular)

	require.Len(t, entries, 2)
	for _, e := range entries {
		assert.NotEqual(t, index.AncestorMode, e.Stage)
	}
}

func TestSortConflictEntries_ScrambledInput(t *testing.T) {
	t.Parallel()
	entries := []*index.Entry{
		{Name: "z.txt", Stage: 0},
		{Name: "a.txt", Stage: index.TheirMode},
		{Name: "a.txt", Stage: index.AncestorMode},
		{Name: "a.txt", Stage: index.OurMode},
	}

	sortConflictEntries(entries)

	require.Len(t, entries, 4)
	assert.Equal(t, "a.txt", entries[0].Name)
	assert.Equal(t, index.AncestorMode, entries[0].Stage)
	assert.Equal(t, "a.txt", entries[1].Name)
	assert.Equal(t, index.OurMode, entries[1].Stage)
	assert.Equal(t, "a.txt", entries[2].Name)
	assert.Equal(t, index.TheirMode, entries[2].Stage)
	assert.Equal(t, "z.txt", entries[3].Name)
	assert.Equal(t, index.Stage(0), entries[3].Stage)
}

// commitBlobHash returns the blob hash path resolves to in commit's tree.
func commitBlobHash(t *testing.T, commit *object.Commit, path string) plumbing.Hash {
	t.Helper()
	f, err := commit.File(path)
	require.NoError(t, err)
	return f.Hash
}

func runRealGitIndexTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := safeexec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "real git command failed: %s", string(out))
	return string(out)
}

// indexTestRepo bundles an open repo/worktree with its on-disk path, so the
// write/commit/blob helpers below don't each need a 4-5 parameter primitive pile.
type indexTestRepo struct {
	t    *testing.T
	repo *git.Repository
	wt   *git.Worktree
	path string
}

// newIndexTestRepo opens setupTestRepo's fixture for direct object-store access.
func newIndexTestRepo(t *testing.T) *indexTestRepo {
	t.Helper()
	path := setupTestRepo(t)
	repo, err := git.PlainOpen(path)
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)
	return &indexTestRepo{t: t, repo: repo, wt: wt, path: path}
}

// writeAndCommit writes content to path and commits it, returning the resulting commit.
func (r *indexTestRepo) writeAndCommit(relPath, content, msg string) *object.Commit {
	r.t.Helper()
	fullPath := filepath.Join(r.path, relPath)
	require.NoError(r.t, os.MkdirAll(filepath.Dir(fullPath), 0o750))
	require.NoError(r.t, os.WriteFile(fullPath, []byte(content), 0o600))
	_, err := r.wt.Add(relPath)
	require.NoError(r.t, err)
	hash, err := r.wt.Commit(msg, &git.CommitOptions{
		Author: &object.Signature{Name: "Test User", Email: "test@example.com", When: time.Now()},
	})
	require.NoError(r.t, err)
	commit, err := r.repo.CommitObject(hash)
	require.NoError(r.t, err)
	return commit
}

// storeBlob stores content as a blob object without adding it to any tree/commit —
// used to fabricate an "ours"/"theirs" conflict side that was never checked out.
func (r *indexTestRepo) storeBlob(content string) plumbing.Hash {
	r.t.Helper()
	obj := r.repo.Storer.NewEncodedObject()
	obj.SetType(plumbing.BlobObject)
	w, err := obj.Writer()
	require.NoError(r.t, err)
	_, err = w.Write([]byte(content))
	require.NoError(r.t, err)
	require.NoError(r.t, w.Close())
	hash, err := r.repo.Storer.SetEncodedObject(obj)
	require.NoError(r.t, err)
	return hash
}

// buildConflictedIndexFixture sets up a repo with a.txt committed at stage 0
// ("line1\n"), then returns the repo path plus stage-1/2/3 index entries for a.txt built
// from real blob hashes (base = the committed content; ours/theirs = two freshly stored
// blobs), for writeConflictedIndex's real-git-recognition tests.
func buildConflictedIndexFixture(t *testing.T) (repoPath string, entries []*index.Entry) {
	t.Helper()
	r := newIndexTestRepo(t)

	baseCommit := r.writeAndCommit("a.txt", "line1\n", "base")
	baseHash := commitBlobHash(t, baseCommit, "a.txt")

	oursHash := r.storeBlob("line1\nours\n")
	theirsHash := r.storeBlob("line1\ntheirs\n")

	entries = NewConflictEntries("a.txt", baseHash, oursHash, theirsHash, filemode.Regular)
	return r.path, entries
}

// TestWriteConflictedIndex_RealGitStatusRecognizesUU covers Story 3.3.2's third
// acceptance criterion: real `git status --porcelain`, not go-git's own reader
// (stack.md's explicit warning), must recognize the write as an unmerged conflict.
func TestWriteConflictedIndex_RealGitStatusRecognizesUU(t *testing.T) {
	t.Parallel()
	repoPath, entries := buildConflictedIndexFixture(t)

	require.NoError(t, writeConflictedIndex(repoPath, entries))

	out := runRealGitIndexTest(t, repoPath, "status", "--porcelain")
	found := false
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if strings.TrimSpace(line) == "UU a.txt" {
			found = true
		}
	}
	assert.True(t, found, "expected 'UU a.txt' in real git status --porcelain output, got: %q", out)
}

// TestWriteConflictedIndex_should_ReturnError_When_IndexPathUnwritable covers the error
// path: a read-only .git directory must surface as an error, not a torn index (relying
// on AdminFileWriter's own atomicity, which this test doesn't re-verify).
func TestWriteConflictedIndex_should_ReturnError_When_IndexPathUnwritable(t *testing.T) {
	t.Parallel()
	repoPath, entries := buildConflictedIndexFixture(t)

	gitDir := filepath.Join(repoPath, ".git")
	require.NoError(t, os.Chmod(gitDir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(gitDir, 0o750) })

	err := writeConflictedIndex(repoPath, entries)

	require.Error(t, err)
}
