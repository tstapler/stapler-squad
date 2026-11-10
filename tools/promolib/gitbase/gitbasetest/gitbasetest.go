// Package gitbasetest builds go-git fixture repositories for promo tool tests.
package gitbasetest

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// Files maps a repo path to content; a nil value deletes the path.
type Files map[string][]byte

func sig() *object.Signature {
	return &object.Signature{Name: "t", Email: "t@example.invalid", When: time.Unix(1700000000, 0)}
}

func apply(t *testing.T, dir string, wt *git.Worktree, files Files) {
	t.Helper()
	for p, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if content == nil {
			if _, err := wt.Remove(p); err != nil {
				t.Fatalf("remove %s: %v", p, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, content, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := wt.Add(p); err != nil {
			t.Fatalf("add %s: %v", p, err)
		}
	}
}

// Commit applies files and commits them on top of HEAD (or as a root commit).
func Commit(t *testing.T, dir string, files Files) plumbing.Hash {
	t.Helper()
	repo, wt := openRaw(t, dir)
	_ = repo
	apply(t, dir, wt, files)
	h, err := wt.Commit("c", &git.CommitOptions{Author: sig(), AllowEmptyCommits: true})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func openRaw(t *testing.T, dir string) (*git.Repository, *git.Worktree) {
	t.Helper()
	repo, err := git.PlainInit(dir, false)
	if err == git.ErrRepositoryAlreadyExists {
		repo, err = git.PlainOpen(dir) //nolint:norawgitopen // fixture builder, not production code
	}
	if err != nil {
		t.Fatal(err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	return repo, wt
}

// PRRepo returns a repo whose HEAD is a pull_request-style merge commit: its
// first parent holds the base files and its second parent the base files with
// pr applied. The returned directory is the checkout at the merge commit.
func PRRepo(t *testing.T, base, pr Files) string {
	t.Helper()
	dir := t.TempDir()
	baseHash := Commit(t, dir, base)
	prHash := Commit(t, dir, pr)
	_, wt := openRaw(t, dir)
	if _, err := wt.Commit("merge", &git.CommitOptions{
		Author: sig(), Parents: []plumbing.Hash{baseHash, prHash}, AllowEmptyCommits: true,
	}); err != nil {
		t.Fatal(err)
	}
	return dir
}

// LinearRepo returns a repo with a single root commit (one parent-less HEAD).
func LinearRepo(t *testing.T, files Files) string {
	t.Helper()
	dir := t.TempDir()
	Commit(t, dir, files)
	return dir
}
