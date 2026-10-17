package testrepo

import (
	"os"
	"path/filepath"
	"testing"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/tstapler/stapler-squad/session/git/native"
)

// NewLinkedWorktree creates a linked worktree of the repository at
// mainRepoDir, named name and checked out on branch, without native git.
// It hand-writes <main>/.git/worktrees/<name>/{HEAD,commondir,gitdir} and the
// linked directory's .git pointer file, creates the branch at main's HEAD if
// it does not exist, then populates files and the per-worktree index with
// go-git (PlainOpenWithOptions EnableDotGitCommonDir + Reset Hard).
// mainRepoDir must already have at least one commit. It returns the linked
// worktree directory.
func NewLinkedWorktree(t testing.TB, mainRepoDir, name, branch string) string {
	t.Helper()
	mainGit := filepath.Join(mainRepoDir, ".git")
	adminDir := filepath.Join(mainGit, "worktrees", name)
	linked := filepath.Join(TempDir(t), name)
	mustMkdir(t, adminDir)
	mustMkdir(t, linked)

	mainRepo, err := native.OpenRepo(mainRepoDir)
	if err != nil {
		t.Fatalf("testrepo: open main repo: %v", err)
	}
	branchRef := plumbing.NewBranchReferenceName(branch)
	if _, err := mainRepo.Reference(branchRef, false); err != nil {
		head, herr := mainRepo.Head()
		if herr != nil {
			t.Fatalf("testrepo: main repo has no HEAD commit: %v", herr)
		}
		SetBranch(t, mainRepo, branch, head.Hash())
	}

	mustWrite(t, filepath.Join(adminDir, "HEAD"), "ref: "+branchRef.String()+"\n")
	mustWrite(t, filepath.Join(adminDir, "commondir"), "../..\n")
	mustWrite(t, filepath.Join(adminDir, "gitdir"), filepath.Join(linked, ".git")+"\n")
	mustWrite(t, filepath.Join(linked, ".git"), "gitdir: "+adminDir+"\n")

	repo, err := native.OpenRepo(linked)
	if err != nil {
		t.Fatalf("testrepo: open linked worktree: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("testrepo: linked Worktree: %v", err)
	}
	if err := wt.Reset(&git.ResetOptions{Mode: git.HardReset}); err != nil {
		t.Fatalf("testrepo: linked Reset(Hard): %v", err)
	}
	return linked
}

// OpenLinked opens a linked worktree directory with commondir support.
func OpenLinked(t testing.TB, linkedDir string) *git.Repository {
	t.Helper()
	repo, err := native.OpenRepo(linkedDir)
	if err != nil {
		t.Fatalf("testrepo: OpenLinked: %v", err)
	}
	return repo
}

func mustMkdir(t testing.TB, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("testrepo: MkdirAll %s: %v", dir, err)
	}
}

func mustWrite(t testing.TB, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("testrepo: write %s: %v", path, err)
	}
}
