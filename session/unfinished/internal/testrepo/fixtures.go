package testrepo

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/index"
)

// RawLinear stores n graph-only commits (all on tree) chained on top of parent
// and returns them oldest first (tag keeps sibling chains from hashing
// identically); a zero parent starts a new root. No ref, index
// or worktree is touched, so a 2,500-commit history costs milliseconds. Move a
// branch to the returned tip with SetBranch.
func RawLinear(t testing.TB, repo *git.Repository, tree, parent plumbing.Hash, n int, tag string) []plumbing.Hash {
	t.Helper()
	hashes := make([]plumbing.Hash, 0, n)
	for i := range n {
		var parents []plumbing.Hash
		if !parent.IsZero() {
			parents = []plumbing.Hash{parent}
		}
		parent = RawCommit(t, repo, tree, parents, fmt.Sprintf("%s %d", tag, i), at(i+1))
		hashes = append(hashes, parent)
	}
	return hashes
}

// LinkedWorktrees adds n linked worktrees ("wt1".."wtN", branches "wt-1".."wt-N",
// each starting at main's HEAD) to the repository at mainRepoDir and returns
// their directories. mainRepoDir must already have a commit.
func LinkedWorktrees(t testing.TB, mainRepoDir string, n int) []string {
	t.Helper()
	dirs := make([]string, 0, n)
	for i := 1; i <= n; i++ {
		dirs = append(dirs, NewLinkedWorktree(t, mainRepoDir, fmt.Sprintf("wt%d", i), fmt.Sprintf("wt-%d", i)))
	}
	return dirs
}

// Symlink returns a path in a fresh temp dir that is a symlink to target.
func Symlink(t testing.TB, target string) string {
	t.Helper()
	link := filepath.Join(TempDir(t), "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("testrepo: Symlink: %v", err)
	}
	return link
}

// AddSubmodule stages a gitlink (index entry mode filemode.Submodule) at path
// pointing at commit, without a .gitmodules file or a nested repository, the
// shape a not-yet-initialised submodule has in a parent checkout.
func AddSubmodule(t testing.TB, repo *git.Repository, path string, commit plumbing.Hash) {
	t.Helper()
	idx, err := repo.Storer.Index()
	if err != nil {
		t.Fatalf("testrepo: read index: %v", err)
	}
	idx.Entries = append(idx.Entries, &index.Entry{Name: path, Hash: commit, Mode: filemode.Submodule})
	if err := repo.Storer.SetIndex(idx); err != nil {
		t.Fatalf("testrepo: write index: %v", err)
	}
}

// EnableSparseCheckout turns on core.sparseCheckout and writes the pattern file
// of the main worktree whose checkout root is dir.
func EnableSparseCheckout(t testing.TB, repo *git.Repository, dir string, patterns ...string) {
	t.Helper()
	cfg, err := repo.Config()
	if err != nil {
		t.Fatalf("testrepo: Config: %v", err)
	}
	cfg.Raw.Section("core").SetOption("sparseCheckout", "true")
	if err := repo.SetConfig(cfg); err != nil {
		t.Fatalf("testrepo: SetConfig: %v", err)
	}
	infoDir := filepath.Join(dir, ".git", "info")
	mustMkdir(t, infoDir)
	body := ""
	for _, p := range patterns {
		body += p + "\n"
	}
	mustWrite(t, filepath.Join(infoDir, "sparse-checkout"), body)
}

// SkipWorktree sets the skip-worktree bit on the named index entries, the way a
// sparse checkout marks paths it did not materialise.
func SkipWorktree(t testing.TB, repo *git.Repository, paths ...string) {
	t.Helper()
	idx, err := repo.Storer.Index()
	if err != nil {
		t.Fatalf("testrepo: read index: %v", err)
	}
	for _, p := range paths {
		e, err := idx.Entry(p)
		if err != nil {
			t.Fatalf("testrepo: index entry %s: %v", p, err)
		}
		e.SkipWorktree = true
	}
	if err := repo.Storer.SetIndex(idx); err != nil {
		t.Fatalf("testrepo: write index: %v", err)
	}
}

// DivergedHashes names the commits of Diverged.
type DivergedHashes struct {
	Root, Fork, Head, Base plumbing.Hash
}

// Diverged builds, with go-git only, a repository whose "main" (checked out)
// is `ahead` commits past a fork point on top of a `depth`-commit linear
// history, and whose "base" branch is `behind` commits past the same fork:
//
//	root --- ... --- fork --- a1..aN   (main, HEAD)
//	                    \
//	                     b1..bM         (base)
//
// AheadBehind(main, base) is therefore (ahead, behind). Everything past the
// first commit is graph-only (RawLinear), so a 2,500-commit history is cheap;
// the first commit carries README.md and is the tree of every later commit.
func Diverged(t testing.TB, depth, ahead, behind int) (*git.Repository, string, DivergedHashes) {
	t.Helper()
	repo, dir := NewRepo(t)
	var h DivergedHashes
	h.Root = Commit(t, repo, map[string]string{"README.md": "hello\n"}, "root", Epoch)
	rootCommit, err := repo.CommitObject(h.Root)
	if err != nil {
		t.Fatalf("testrepo: root commit: %v", err)
	}
	tree := rootCommit.TreeHash

	trunk := append([]plumbing.Hash{h.Root}, RawLinear(t, repo, tree, h.Root, depth, "trunk")...)
	h.Fork = trunk[len(trunk)-1]
	h.Head = h.Fork
	if ahead > 0 {
		h.Head = RawLinear(t, repo, tree, h.Fork, ahead, "ahead")[ahead-1]
	}
	h.Base = h.Fork
	if behind > 0 {
		h.Base = RawLinear(t, repo, tree, h.Fork, behind, "behind")[behind-1]
	}
	SetBranch(t, repo, DefaultBranch, h.Head)
	SetBranch(t, repo, "base", h.Base)
	return repo, dir, h
}

// CommitAll is Commit for many files: it stages the whole worktree with one
// index rewrite instead of one per file, which keeps 1,000+ file fixtures fast.
func CommitAll(t testing.TB, repo *git.Repository, files map[string]string, msg string, when time.Time) plumbing.Hash {
	t.Helper()
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("testrepo: Worktree: %v", err)
	}
	WriteFiles(t, wt.Filesystem.Root(), files)
	if err := wt.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		t.Fatalf("testrepo: Add all: %v", err)
	}
	sig := Signature(when)
	h, err := wt.Commit(msg, &git.CommitOptions{Author: sig, Committer: sig, AllowEmptyCommits: true})
	if err != nil {
		t.Fatalf("testrepo: Commit: %v", err)
	}
	return h
}

// CommitManyFast is CommitAll for tens of thousands of files: it writes them,
// builds the index entries straight from stat and the content hash, and
// commits. Blob objects are NOT stored, so use it for index/stat/walk
// fixtures only, never where HEAD blobs are read (diff counting).
func CommitManyFast(t testing.TB, repo *git.Repository, files map[string]string, msg string, when time.Time) plumbing.Hash {
	t.Helper()
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("testrepo: Worktree: %v", err)
	}
	root := wt.Filesystem.Root()
	WriteFiles(t, root, files)
	idx := &index.Index{Version: 2}
	for _, name := range sortedKeys(files) {
		fi, err := os.Lstat(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("testrepo: stat %s: %v", name, err)
		}
		idx.Entries = append(idx.Entries, &index.Entry{
			Hash:       plumbing.ComputeHash(plumbing.BlobObject, []byte(files[name])),
			Name:       name,
			Mode:       filemode.Regular,
			Size:       uint32(fi.Size()), //nolint:gosec // fixture files are tiny
			ModifiedAt: fi.ModTime(),
			CreatedAt:  fi.ModTime(),
		})
	}
	if err := repo.Storer.SetIndex(idx); err != nil {
		t.Fatalf("testrepo: SetIndex: %v", err)
	}
	sig := Signature(when)
	h, err := wt.Commit(msg, &git.CommitOptions{Author: sig, Committer: sig, AllowEmptyCommits: true})
	if err != nil {
		t.Fatalf("testrepo: Commit: %v", err)
	}
	return h
}
