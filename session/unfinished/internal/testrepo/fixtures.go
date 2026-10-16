package testrepo

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/index"
)

// RawLinear stores n graph-only commits (all on tree) chained on top of parent
// and returns them oldest first; a zero parent starts a new root. No ref, index
// or worktree is touched, so a 2,500-commit history costs milliseconds. Move a
// branch to the returned tip with SetBranch.
func RawLinear(t testing.TB, repo *git.Repository, tree, parent plumbing.Hash, n int) []plumbing.Hash {
	t.Helper()
	hashes := make([]plumbing.Hash, 0, n)
	for i := range n {
		var parents []plumbing.Hash
		if !parent.IsZero() {
			parents = []plumbing.Hash{parent}
		}
		parent = RawCommit(t, repo, tree, parents, fmt.Sprintf("raw %d", i), at(i+1))
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
