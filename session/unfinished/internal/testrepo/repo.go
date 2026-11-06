// Package testrepo builds git fixtures for tests using go-git only: no native
// git, os/exec or executor/safeexec. It is a non-test package so every test
// package under session/unfinished (including external _test packages) can
// import it. It must not import unfinished, gogitstore or repostate.
package testrepo

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// DefaultBranch is the branch NewRepo points HEAD at.
const DefaultBranch = "main"

// Epoch is the deterministic base time; fixtures derive commit times from it.
var Epoch = time.Date(2024, time.January, 1, 12, 0, 0, 0, time.UTC)

// Signature returns the fixed author/committer identity at the given time.
func Signature(when time.Time) *object.Signature {
	return &object.Signature{Name: "Test", Email: "test@example.com", When: when}
}

// TempDir returns a t.TempDir() with symlinks resolved (macOS /var vs
// /private/var), so paths compare equal to what go-git reports.
func TempDir(t testing.TB) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("testrepo: EvalSymlinks: %v", err)
	}
	return dir
}

// NewRepo initialises an empty non-bare repository (HEAD -> refs/heads/main)
// in a fresh temp dir and returns it with its worktree path.
func NewRepo(t testing.TB) (*git.Repository, string) {
	t.Helper()
	dir := TempDir(t)
	repo, err := git.PlainInitWithOptions(dir, &git.PlainInitOptions{
		InitOptions: git.InitOptions{DefaultBranch: plumbing.NewBranchReferenceName(DefaultBranch)},
	})
	if err != nil {
		t.Fatalf("testrepo: PlainInit: %v", err)
	}
	return repo, dir
}

// Commit writes files (path -> content, slash separated), stages exactly those
// paths and commits on the current HEAD with deterministic signatures.
// Empty commits are allowed.
func Commit(t testing.TB, repo *git.Repository, files map[string]string, msg string, when time.Time) plumbing.Hash {
	t.Helper()
	return CommitWithParents(t, repo, files, msg, when, nil)
}

// CommitWithParents is Commit with explicit parents (nil means HEAD), for
// building merge commits on the checked-out branch.
func CommitWithParents(t testing.TB, repo *git.Repository, files map[string]string, msg string, when time.Time, parents []plumbing.Hash) plumbing.Hash {
	t.Helper()
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("testrepo: Worktree: %v", err)
	}
	WriteFiles(t, wt.Filesystem.Root(), files)
	for _, name := range sortedKeys(files) {
		if _, err := wt.Add(name); err != nil {
			t.Fatalf("testrepo: Add %s: %v", name, err)
		}
	}
	sig := Signature(when)
	h, err := wt.Commit(msg, &git.CommitOptions{Author: sig, Committer: sig, Parents: parents, AllowEmptyCommits: true})
	if err != nil {
		t.Fatalf("testrepo: Commit: %v", err)
	}
	return h
}

// WriteFiles writes files below root, creating parent directories.
func WriteFiles(t testing.TB, root string, files map[string]string) {
	t.Helper()
	for _, name := range sortedKeys(files) {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("testrepo: MkdirAll: %v", err)
		}
		if err := os.WriteFile(path, []byte(files[name]), 0o644); err != nil {
			t.Fatalf("testrepo: WriteFile %s: %v", name, err)
		}
	}
}

// EmptyTree stores and returns the empty tree object.
func EmptyTree(t testing.TB, repo *git.Repository) plumbing.Hash {
	t.Helper()
	obj := repo.Storer.NewEncodedObject()
	if err := (&object.Tree{}).Encode(obj); err != nil {
		t.Fatalf("testrepo: encode empty tree: %v", err)
	}
	h, err := repo.Storer.SetEncodedObject(obj)
	if err != nil {
		t.Fatalf("testrepo: store empty tree: %v", err)
	}
	return h
}

// RawCommit stores a commit object with the given tree and parents without
// touching the worktree, index or any ref. Use it for graph-only fixtures.
func RawCommit(t testing.TB, repo *git.Repository, tree plumbing.Hash, parents []plumbing.Hash, msg string, when time.Time) plumbing.Hash {
	t.Helper()
	sig := *Signature(when)
	c := &object.Commit{Author: sig, Committer: sig, Message: msg, TreeHash: tree, ParentHashes: parents}
	obj := repo.Storer.NewEncodedObject()
	if err := c.Encode(obj); err != nil {
		t.Fatalf("testrepo: encode commit: %v", err)
	}
	h, err := repo.Storer.SetEncodedObject(obj)
	if err != nil {
		t.Fatalf("testrepo: store commit: %v", err)
	}
	return h
}

// SetBranch points refs/heads/<name> at hash, creating or moving it.
func SetBranch(t testing.TB, repo *git.Repository, name string, hash plumbing.Hash) {
	t.Helper()
	ref := plumbing.NewHashReference(plumbing.NewBranchReferenceName(name), hash)
	if err := repo.Storer.SetReference(ref); err != nil {
		t.Fatalf("testrepo: SetBranch %s: %v", name, err)
	}
}

// SetHead points HEAD symbolically at refs/heads/<name> without touching the
// worktree (unlike Checkout).
func SetHead(t testing.TB, repo *git.Repository, name string) {
	t.Helper()
	ref := plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName(name))
	if err := repo.Storer.SetReference(ref); err != nil {
		t.Fatalf("testrepo: SetHead %s: %v", name, err)
	}
}

// Checkout switches to branch. With create, the new branch starts at HEAD.
func Checkout(t testing.TB, repo *git.Repository, branch string, create bool) {
	t.Helper()
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("testrepo: Worktree: %v", err)
	}
	opts := &git.CheckoutOptions{Branch: plumbing.NewBranchReferenceName(branch), Create: create}
	if err := wt.Checkout(opts); err != nil {
		t.Fatalf("testrepo: Checkout %s: %v", branch, err)
	}
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
