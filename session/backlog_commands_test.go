package session

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/session/git"
)

// setupTestGitRepo creates a temporary git repository with an initial commit and a
// configured test identity, mirroring session/git/worktree_creation_test.go's
// setupTestRepo helper (duplicated here rather than imported since that one lives in
// package git and is unexported). Uses go-git directly rather than shelling out — see
// the `prefer-go-git-over-subshells` skill.
func setupTestGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	repo, err := gogit.PlainInitWithOptions(dir, &gogit.PlainInitOptions{
		InitOptions: gogit.InitOptions{DefaultBranch: plumbing.NewBranchReferenceName("main")},
	})
	if err != nil {
		t.Fatalf("git init failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test\n"), 0o644); err != nil {
		t.Fatalf("failed to write README.md: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree failed: %v", err)
	}
	if _, err := wt.Add("README.md"); err != nil {
		t.Fatalf("git add failed: %v", err)
	}
	if _, err := wt.Commit("Initial commit", &gogit.CommitOptions{
		Author: &object.Signature{Name: "Test User", Email: "test@example.com", When: time.Now()},
	}); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}

	return dir
}

// setupLinkedWorktree builds a real linked worktree (git worktree add topology)
// on top of an existing main repo — the topology where --git-dir and
// --git-common-dir resolve to different directories, which a plain setupTestGitRepo
// fixture cannot exercise.
func setupLinkedWorktree(t *testing.T, repoPath, sessionName string) string {
	t.Helper()
	worktree, _, err := git.NewGitWorktree(repoPath, sessionName)
	if err != nil {
		t.Fatalf("git.NewGitWorktree failed: %v", err)
	}
	if err := worktree.Setup(); err != nil {
		t.Fatalf("worktree.Setup failed: %v", err)
	}
	t.Cleanup(func() {
		if err := worktree.Cleanup(); err != nil {
			t.Logf("worktree.Cleanup failed (non-fatal): %v", err)
		}
	})
	return worktree.GetWorktreePath()
}

// gitLsFiles returns the list of paths currently tracked in the git index at dir.
func gitLsFiles(t *testing.T, dir string) []string {
	t.Helper()
	cmd := safeexec.CommandContext(context.Background(), "git", "ls-files")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-files failed: %v", err)
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

// containsPath reports whether paths contains target.
func containsPath(paths []string, target string) bool {
	for _, p := range paths {
		if p == target {
			return true
		}
	}
	return false
}

// TestUntrackTrackedScaffolding_NoErrorOnNonGitDirectory verifies the self-heal helper
// degrades gracefully (no error, nothing untracked) for directory-mode sessions that
// have no git backing at all — mirroring addWorktreeExcludes' existing best-effort
// handling of the same case.
func TestUntrackTrackedScaffolding_NoErrorOnNonGitDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	removed, err := git.UntrackScaffolding(dir, git.ScaffoldingExcludePatterns)
	if err != nil {
		t.Errorf("expected no error for a non-git directory, got: %v", err)
	}
	if len(removed) != 0 {
		t.Errorf("expected nothing removed for a non-git directory, got: %v", removed)
	}
}

// TestUntrackTrackedScaffolding_LeavesUnrelatedTrackedFilesAlone verifies the self-heal
// only touches paths matching git.ScaffoldingExcludePatterns and leaves everything else in
// the index untouched.
func TestUntrackTrackedScaffolding_LeavesUnrelatedTrackedFilesAlone(t *testing.T) {
	t.Parallel()
	repo := setupTestGitRepo(t)

	before := gitLsFiles(t, repo)
	if !containsPath(before, "README.md") {
		t.Fatalf("test setup failed: README.md is not tracked")
	}

	removed, err := git.UntrackScaffolding(repo, git.ScaffoldingExcludePatterns)
	if err != nil {
		t.Fatalf("UntrackScaffolding returned error: %v", err)
	}
	if len(removed) != 0 {
		t.Errorf("expected nothing to be untracked (no scaffolding files present), got: %v", removed)
	}

	after := gitLsFiles(t, repo)
	if !containsPath(after, "README.md") {
		t.Errorf("README.md was unexpectedly untracked")
	}
}

// TestAddWorktreeExcludes_PlainRepo_GitCommonDirEqualsGitDir pins the mechanism by which
// the --git-common-dir fix is a no-op for plain (non-worktree) repos: both flags must
// resolve to the same directory there.
func TestAddWorktreeExcludes_PlainRepo_GitCommonDirEqualsGitDir(t *testing.T) {
	t.Parallel()
	repoPath := setupTestGitRepo(t)

	// Both sides must go through git's own --path-format=absolute resolution.
	// t.TempDir() on macOS returns a path through the /var -> /private/var
	// symlink; git's absolute-path formatting resolves it but a manual
	// filepath.Join of the unresolved repoPath onto a relative --git-dir does
	// not, producing a false mismatch unrelated to --git-dir vs
	// --git-common-dir semantics.
	gitDir := runGitRevParse(t, repoPath, "--path-format=absolute", "--git-dir")
	commonDir := runGitRevParse(t, repoPath, "--path-format=absolute", "--git-common-dir")

	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(repoPath, gitDir)
	}

	// Resolve symlinks on both sides: t.TempDir() on macOS returns a path under
	// /var/folders/..., which is itself a symlink to /private/var/.... git's
	// --path-format=absolute resolves through that symlink, but joining the
	// (unresolved) repoPath onto the relative --git-dir output does not, so the
	// two would otherwise mismatch on symlink resolution alone rather than on
	// any real --git-common-dir behavior difference.
	resolvedGitDir, err := filepath.EvalSymlinks(gitDir)
	if err != nil {
		t.Fatalf("failed to resolve symlinks for git-dir %s: %v", gitDir, err)
	}
	resolvedCommonDir, err := filepath.EvalSymlinks(commonDir)
	if err != nil {
		t.Fatalf("failed to resolve symlinks for git-common-dir %s: %v", commonDir, err)
	}
	if resolvedGitDir != resolvedCommonDir {
		t.Errorf("expected --git-dir (%s) and --git-common-dir (%s) to resolve identically for a plain repo", resolvedGitDir, resolvedCommonDir)
	}
}

func runGitRevParse(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := safeexec.CommandContext(context.Background(), "git", append([]string{"rev-parse"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse %v failed: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}
