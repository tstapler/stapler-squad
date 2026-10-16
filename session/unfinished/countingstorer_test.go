package unfinished

// Permanent counting storer (Story 0.1.2b) and the DecodeCount / StatCount
// metric definitions every gate in this package is expressed in.

import (
	"io"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/go-git/go-billy/v5/memfs"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/storage"
	"github.com/go-git/go-git/v5/storage/memory"

	"github.com/tstapler/stapler-squad/session/unfinished/gogitstore"
	"github.com/tstapler/stapler-squad/session/unfinished/internal/testrepo"
)

// countingStorer wraps a storage.Storer and counts EncodedObject loads per
// object type. DecodeCount is the commit counter only; the tree and blob
// counters let tree-read and blob-read criteria fail when they regress.
type countingStorer struct {
	storage.Storer
	commits, trees, blobs atomic.Int64
}

var _ storer.EncodedObjectStorer = (*countingStorer)(nil)

func (s *countingStorer) EncodedObject(t plumbing.ObjectType, h plumbing.Hash) (plumbing.EncodedObject, error) {
	switch t {
	case plumbing.CommitObject:
		s.commits.Add(1)
	case plumbing.TreeObject:
		s.trees.Add(1)
	case plumbing.BlobObject:
		s.blobs.Add(1)
	}
	return s.Storer.EncodedObject(t, h)
}

// DecodeCount is the number of EncodedObject(CommitObject, _) reads.
func (s *countingStorer) DecodeCount() int64 { return s.commits.Load() }

// TreeReads is the number of EncodedObject(TreeObject, _) reads.
func (s *countingStorer) TreeReads() int64 { return s.trees.Load() }

// BlobReads is the number of EncodedObject(BlobObject, _) reads.
func (s *countingStorer) BlobReads() int64 { return s.blobs.Load() }

// Extension and CommonDirKey forward to the wrapped gogitstore storer so a
// repo opened through the counting wrapper still engages the shared RepoState.
func (s *countingStorer) Extension(newFn func() gogitstore.Extension) gogitstore.Extension {
	if h, ok := s.Storer.(extensionHolder); ok {
		return h.Extension(newFn)
	}
	return nil
}

func (s *countingStorer) CommonDirKey() string {
	if k, ok := s.Storer.(commonDirKeyer); ok {
		return k.CommonDirKey()
	}
	return ""
}

// Close forwards to the wrapped storer so a cachedRepo holding the wrapper
// still releases its gogitstore registry reference on eviction.
func (s *countingStorer) Close() error {
	if c, ok := s.Storer.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

func TestCountingStorer_ShouldCountOnlyCommitEncodedObjects_WhenDecodeCountRead(t *testing.T) {
	mem := memory.NewStorage()
	fs := memfs.New()
	seed, err := git.Init(mem, fs)
	if err != nil {
		t.Fatal(err)
	}
	wt, _ := seed.Worktree()
	f, _ := fs.Create("a.txt")
	_, _ = f.Write([]byte("payload\n"))
	_ = f.Close()
	_, _ = wt.Add("a.txt")
	sig := testrepo.Signature(testrepo.Epoch)
	head, err := wt.Commit("one", &git.CommitOptions{Author: sig, Committer: sig})
	if err != nil {
		t.Fatal(err)
	}

	cs := &countingStorer{Storer: mem}
	repo, err := git.Open(cs, fs)
	if err != nil {
		t.Fatal(err)
	}
	assertCounts := func(step string, commits, trees, blobs int64) {
		t.Helper()
		if cs.DecodeCount() != commits || cs.TreeReads() != trees || cs.BlobReads() != blobs {
			t.Fatalf("%s: commits/trees/blobs = %d/%d/%d, want %d/%d/%d",
				step, cs.DecodeCount(), cs.TreeReads(), cs.BlobReads(), commits, trees, blobs)
		}
	}

	c, err := repo.CommitObject(head)
	if err != nil {
		t.Fatal(err)
	}
	assertCounts("commit read", 1, 0, 0)

	tree, err := c.Tree()
	if err != nil {
		t.Fatal(err)
	}
	assertCounts("tree read", 1, 1, 0)

	file, err := tree.File("a.txt")
	if err != nil {
		t.Fatal(err)
	}
	assertCounts("blob read", 1, 1, 1)
	if _, err := file.Contents(); err != nil {
		t.Fatal(err)
	}
	assertCounts("contents re-read reuses the loaded blob", 1, 1, 1)
}

// divergedWorktrees builds the shared fixture of the multi-worktree tests: a
// repo with a 2,500-commit trunk, main 3 ahead of and base 2 behind the fork,
// and w worktrees (main plus w-1 linked) all on the same head and base.
func divergedWorktrees(tb testing.TB, w int) []string {
	tb.Helper()
	_, dir, _ := testrepo.Diverged(tb, 2500, 3, 2)
	return append([]string{dir}, testrepo.LinkedWorktrees(tb, dir, w-1)...)
}

// coldAheadBehindDecodes runs one cold AheadBehind per worktree on a fresh
// reader and returns the DecodeCount it caused.
func coldAheadBehindDecodes(t *testing.T, paths []string) int64 {
	t.Helper()
	r := NewCountingReader()
	for _, p := range paths {
		r.Track(t, p)
	}
	for _, p := range paths {
		ahead, behind, err := r.AheadBehind(p, "base")
		if err != nil {
			t.Fatalf("AheadBehind %s: %v", p, err)
		}
		if ahead != 3 || behind != 2 {
			t.Fatalf("AheadBehind %s = (%d, %d), want (3, 2)", p, ahead, behind)
		}
	}
	return r.DecodeCount()
}

// Ungated characterization (Story 0.1.2b): no shared-commit-state flag exists
// yet, so this measures the legacy per-worktree walk and keeps the O(W x R)
// gap visible until gate G2 replaces it.
func TestDecodeCount_ShouldRecordWTimesBaseline_WhenThreeWorktreesShareOneRepo(t *testing.T) {
	paths := divergedWorktrees(t, 3)
	w1 := coldAheadBehindDecodes(t, paths[:1])
	w3 := coldAheadBehindDecodes(t, paths)
	t.Logf("legacy baseline, shared head and base, 2500-commit trunk, 3 ahead / 2 behind: DecodeCount W=1 = %d, W=3 = %d", w1, w3)
	if w1 == 0 {
		t.Fatal("DecodeCount W=1 is 0: the counting storer is not on the walk path")
	}
	if w3 <= w1 {
		t.Errorf("legacy DecodeCount(W=3) = %d, want > DecodeCount(W=1) = %d", w3, w1)
	}
}

func TestStatFn_ShouldCountLstatCalls_When1000FileWorktreeScanned(t *testing.T) {
	const files = 1000
	repo, dir := testrepo.NewRepo(t)
	content := make(map[string]string, files)
	for i := range files {
		content["d"+strconv.Itoa(i%10)+"/f"+strconv.Itoa(i)+".txt"] = strconv.Itoa(i) + "\n"
	}
	testrepo.CommitAll(t, repo, content, "many files", testrepo.Epoch)
	idx, err := repo.Storer.Index()
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Entries) != files {
		t.Fatalf("fixture has %d index entries, want %d", len(idx.Entries), files)
	}

	r := NewCountingReader()
	dirty, err := r.HasUncommitted(dir)
	if err != nil || dirty {
		t.Fatalf("HasUncommitted = %v, %v; want clean", dirty, err)
	}
	if got := r.StatCount(); got != files {
		t.Errorf("StatCount after HasUncommitted = %d, want %d (one lstat per tracked file)", got, files)
	}

	before := r.StatCount()
	if _, err := r.DiffShortstat(dir); err != nil {
		t.Fatal(err)
	}
	t.Logf("StatCount: HasUncommitted = %d, DiffShortstat = %d (duplicate pass)", before, r.StatCount()-before)
}
