package git

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/utils/merkletrie"
	"github.com/stretchr/testify/require"

	ssqlog "github.com/tstapler/stapler-squad/log"
)

// mergeBaseFixture wraps a throwaway repo for building commit/tree graphs directly
// against the object store (rather than via checkout+commit for every node), which is
// what Task 3.1.1b's "construct a criss-cross history via go-git commit creation"
// needs: criss-cross graphs require synthetic multi-parent commits real git wouldn't
// even let you check out in the sequence a test needs.
type mergeBaseFixture struct {
	t         *testing.T
	repo      *git.Repository
	dir       string
	emptyTree plumbing.Hash
}

func newMergeBaseFixture(t *testing.T) *mergeBaseFixture {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	require.NoError(t, err)

	obj := repo.Storer.NewEncodedObject()
	require.NoError(t, (&object.Tree{}).Encode(obj))
	emptyTree, err := repo.Storer.SetEncodedObject(obj)
	require.NoError(t, err)

	return &mergeBaseFixture{t: t, repo: repo, dir: dir, emptyTree: emptyTree}
}

// commit stores a commit object directly, with an arbitrary parent list, using the
// fixture's empty placeholder tree — content doesn't matter for these graph-only tests.
func (f *mergeBaseFixture) commit(msg string, when time.Time, parents ...plumbing.Hash) *object.Commit {
	f.t.Helper()
	c := &object.Commit{
		Author:       object.Signature{Name: "Test User", Email: "test@example.com", When: when},
		Committer:    object.Signature{Name: "Test User", Email: "test@example.com", When: when},
		Message:      msg,
		TreeHash:     f.emptyTree,
		ParentHashes: parents,
	}
	obj := f.repo.Storer.NewEncodedObject()
	require.NoError(f.t, c.Encode(obj))
	hash, err := f.repo.Storer.SetEncodedObject(obj)
	require.NoError(f.t, err)
	got, err := object.GetCommit(f.repo.Storer, hash)
	require.NoError(f.t, err)
	return got
}

// commitFile writes content to path and commits it via go-git's Worktree.Commit
// directly rather than shelling out to git — see the `prefer-go-git-over-subshells`
// skill. Used where the test needs a real, diffable tree (unlike commit, above).
func (f *mergeBaseFixture) commitFile(path, content, msg string) *object.Commit {
	f.t.Helper()
	fullPath := filepath.Join(f.dir, path)
	require.NoError(f.t, os.MkdirAll(filepath.Dir(fullPath), 0o750))
	require.NoError(f.t, os.WriteFile(fullPath, []byte(content), 0o600))

	wt, err := f.repo.Worktree()
	require.NoError(f.t, err)
	_, err = wt.Add(path)
	require.NoError(f.t, err)
	hash, err := wt.Commit(msg, &git.CommitOptions{
		Author: &object.Signature{Name: "Test User", Email: "test@example.com", When: time.Now()},
	})
	require.NoError(f.t, err)
	commit, err := f.repo.CommitObject(hash)
	require.NoError(f.t, err)
	return commit
}

// tree stores a synthetic tree object built from the given entries directly against
// the fixture's object store, without requiring the entries' targets to exist —
// used to fabricate a dangling/unresolvable blob reference.
func (f *mergeBaseFixture) tree(entries ...object.TreeEntry) *object.Tree {
	f.t.Helper()
	tree := &object.Tree{Entries: entries}
	obj := f.repo.Storer.NewEncodedObject()
	require.NoError(f.t, tree.Encode(obj))
	hash, err := f.repo.Storer.SetEncodedObject(obj)
	require.NoError(f.t, err)
	got, err := object.GetTree(f.repo.Storer, hash)
	require.NoError(f.t, err)
	return got
}

// blob stores content as a real blob object and returns its hash, for building tree
// entries that must actually resolve (as opposed to the deliberately-dangling hash
// used in the unresolvable-blob test).
func (f *mergeBaseFixture) blob(content string) plumbing.Hash {
	f.t.Helper()
	obj := f.repo.Storer.NewEncodedObject()
	obj.SetType(plumbing.BlobObject)
	w, err := obj.Writer()
	require.NoError(f.t, err)
	_, err = w.Write([]byte(content))
	require.NoError(f.t, err)
	require.NoError(f.t, w.Close())
	hash, err := f.repo.Storer.SetEncodedObject(obj)
	require.NoError(f.t, err)
	return hash
}

func TestMergeBaseResolver_SingleBase(t *testing.T) {
	t.Parallel()
	f := newMergeBaseFixture(t)
	when := time.Now()

	base := f.commit("base", when)
	ours := f.commit("ours", when.Add(time.Minute), base.Hash)
	theirs := f.commit("theirs", when.Add(time.Minute), base.Hash)

	got, err := MergeBaseResolver(ours, theirs)
	require.NoError(t, err)
	require.Equal(t, base.Hash, got.Hash)
}

// TestMergeBaseResolver_MultipleBases_PicksFirstAndWarns builds the classic criss-cross
// history (two independent merge-base candidates, matching go-git's own C/D → CD1/CD2
// fixture shape documented in its merge_base_test.go): a root commit with two children
// a1/b1, each merged into the other (a2 has parents [a1, b1], b2 has parents [b1, a1]).
// Neither a1 nor b1 is an ancestor of the other, so MergeBase legitimately returns both
// as candidates.
func TestMergeBaseResolver_MultipleBases_PicksFirstAndWarns(t *testing.T) {
	f := newMergeBaseFixture(t)
	when := time.Now()

	root := f.commit("root", when)
	a1 := f.commit("a1", when.Add(1*time.Minute), root.Hash)
	b1 := f.commit("b1", when.Add(1*time.Minute), root.Hash)
	a2 := f.commit("a2 (merges b1 into a-line)", when.Add(2*time.Minute), a1.Hash, b1.Hash)
	b2 := f.commit("b2 (merges a1 into b-line)", when.Add(2*time.Minute), b1.Hash, a1.Hash)

	var logBuf bytes.Buffer
	prevLogger := ssqlog.SetSlogDefaultForTest(slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	defer ssqlog.SetSlogDefaultForTest(prevLogger)

	got, err := MergeBaseResolver(a2, b2)
	require.NoError(t, err)

	// The chosen base must be exactly what go-git's own MergeBase would return first —
	// asserting equality against a direct call pins "pick the first candidate" without
	// re-deriving go-git's internal ordering here.
	wantBases, err := a2.MergeBase(b2)
	require.NoError(t, err)
	require.Len(t, wantBases, 2, "test fixture must produce a genuine criss-cross (2 candidates) history")
	require.Equal(t, wantBases[0].Hash, got.Hash)

	logOutput := logBuf.String()
	require.Contains(t, logOutput, "multiple merge-base candidates", "expected a WARN log line naming the ambiguity")
	require.Contains(t, logOutput, a1.Hash.String())
	require.Contains(t, logOutput, b1.Hash.String())
}

func TestMergeBaseResolver_should_ReturnError_When_CommitsShareNoHistory(t *testing.T) {
	t.Parallel()
	f := newMergeBaseFixture(t)
	when := time.Now()

	ours := f.commit("unrelated root 1", when)
	theirs := f.commit("unrelated root 2", when.Add(time.Minute))

	_, err := MergeBaseResolver(ours, theirs)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no merge base found")
}

func TestTreeDiffPair_TwoSidedEdit(t *testing.T) {
	t.Parallel()
	f := newMergeBaseFixture(t)

	baseCommit := f.commitFile("a.txt", "line1\n", "base")
	oursCommit := f.commitFile("a.txt", "line1\nline2-ours\n", "ours")
	theirsCommit := f.commitFile("a.txt", "line1\nline2-theirs\n", "theirs")

	baseTree, err := baseCommit.Tree()
	require.NoError(t, err)
	oursTree, err := oursCommit.Tree()
	require.NoError(t, err)
	theirsTree, err := theirsCommit.Tree()
	require.NoError(t, err)

	baseToOurs, baseToTheirs, err := TreeDiffPair(baseTree, oursTree, theirsTree)
	require.NoError(t, err)

	requireSingleModify(t, baseToOurs, "a.txt")
	requireSingleModify(t, baseToTheirs, "a.txt")
}

func requireSingleModify(t *testing.T, changes object.Changes, path string) {
	t.Helper()
	require.Len(t, changes, 1)
	action, err := changes[0].Action()
	require.NoError(t, err)
	require.Equal(t, merkletrie.Modify, action)
	require.Equal(t, path, changes[0].To.Name)
}

// TestTreeDiffPair_should_ReturnError_When_TreeHashUnresolvable builds a "theirs" tree
// that deletes base's only file (a.txt) and adds a same-shaped file (b.txt) whose blob
// hash was never stored — a dangling/unresolvable blob reference simulating a
// corrupt/incomplete object store. go-git's tree walker silently drops a dangling
// *tree* (directory) entry rather than erroring (documented behavior: "objects which
// cannot be found... will be skipped automatically"), so a missing blob is the one
// unresolvable-reference shape that actually surfaces as a propagated error: Diff's
// default rename detection (DefaultDiffTreeOptions) tries to score the delete/add pair
// for a possible rename, which requires reading both files' content and fails loudly
// instead of silently reporting no changes.
func TestTreeDiffPair_should_ReturnError_When_TreeHashUnresolvable(t *testing.T) {
	t.Parallel()
	f := newMergeBaseFixture(t)

	realBlob := f.blob("line1\n")
	danglingBlob := plumbing.ComputeHash(plumbing.BlobObject, []byte("dangling-blob-content"))

	baseTree := f.tree(object.TreeEntry{Name: "a.txt", Mode: filemode.Regular, Hash: realBlob})
	oursTree := f.tree(object.TreeEntry{Name: "a.txt", Mode: filemode.Regular, Hash: realBlob})
	theirsTree := f.tree(object.TreeEntry{Name: "b.txt", Mode: filemode.Regular, Hash: danglingBlob})

	_, _, err := TreeDiffPair(baseTree, oursTree, theirsTree)
	require.Error(t, err)
	require.Contains(t, err.Error(), "diff base tree against theirs")
}
