package session

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/internal/sessiontest"
)

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
	repo := sessiontest.SetupTestGitRepo(t)

	before := sessiontest.GitLsFiles(t, repo)
	if !sessiontest.ContainsPath(before, "README.md") {
		t.Fatalf("test setup failed: README.md is not tracked")
	}

	removed, err := git.UntrackScaffolding(repo, git.ScaffoldingExcludePatterns)
	if err != nil {
		t.Fatalf("UntrackScaffolding returned error: %v", err)
	}
	if len(removed) != 0 {
		t.Errorf("expected nothing to be untracked (no scaffolding files present), got: %v", removed)
	}

	after := sessiontest.GitLsFiles(t, repo)
	if !sessiontest.ContainsPath(after, "README.md") {
		t.Errorf("README.md was unexpectedly untracked")
	}
}

// TestAddWorktreeExcludes_PlainRepo_GitCommonDirEqualsGitDir pins the mechanism by which
// the --git-common-dir fix is a no-op for plain (non-worktree) repos: both flags must
// resolve to the same directory there.
func TestAddWorktreeExcludes_PlainRepo_GitCommonDirEqualsGitDir(t *testing.T) {
	t.Parallel()
	repoPath := sessiontest.SetupTestGitRepo(t)

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
