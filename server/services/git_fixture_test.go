package services

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// testGitSignature returns a fixed, per-commit author/committer identity for
// test fixtures so commits never depend on global/system git config.
func testGitSignature() *object.Signature {
	return &object.Signature{
		Name:  "Test",
		Email: "test@example.com",
		When:  time.Unix(0, 0),
	}
}

// initGitRepoForTest creates a new git repository at dir using go-git,
// replacing subprocess-based `git init` fixture setup.
//
// It also sets a local (repo-scoped, not --global) user.name/user.email via
// go-git's config API. go-git commits always supply their own per-commit
// CommitOptions.Author/Committer and don't need this, but several tests reuse
// this repo (or a clone of it, e.g. setupPRFixSyncRepo's originDir) for
// further commits made via the plain git CLI (runGitTestCmd), which reads
// identity from git config rather than any argument. That works on a dev
// machine with a global identity configured, but CI runners have none and
// fail with "Author identity unknown" — setting it here once, locally, fixes
// every downstream CLI commit against this repo.
func initGitRepoForTest(t *testing.T, dir string) *git.Repository {
	t.Helper()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("git init %s failed: %v", dir, err)
	}
	cfg, err := repo.Config()
	if err != nil {
		t.Fatalf("git config %s failed: %v", dir, err)
	}
	cfg.User.Name = "Test"
	cfg.User.Email = "test@example.com"
	if err := repo.SetConfig(cfg); err != nil {
		t.Fatalf("git set config %s failed: %v", dir, err)
	}
	return repo
}

// commitFileForTest writes name/content into dir and commits it into repo
// using go-git, with a fixed per-commit Author/Committer so the commit never
// relies on global git config.
func commitFileForTest(t *testing.T, repo *git.Repository, dir, name, content, message string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("get worktree: %v", err)
	}
	if _, err := wt.Add(name); err != nil {
		t.Fatalf("git add %s failed: %v", name, err)
	}
	sig := testGitSignature()
	if _, err := wt.Commit(message, &git.CommitOptions{Author: sig, Committer: sig}); err != nil {
		t.Fatalf("git commit %q failed: %v", message, err)
	}
}

// initGitRepoWithCommit initializes a git repo at dir with a single initial
// commit containing README.md. Consolidates the formerly duplicated
// subprocess-based initGitRepoWithCommit/initGitRepoForTest/harness
// initGitRepo helpers onto go-git.
func initGitRepoWithCommit(t *testing.T, dir string) {
	t.Helper()
	repo := initGitRepoForTest(t, dir)
	commitFileForTest(t, repo, dir, "README.md", "# Test Repo\n", "initial")
}
