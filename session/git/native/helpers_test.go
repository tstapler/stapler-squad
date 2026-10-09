package native

import (
	"testing"

	"github.com/tstapler/stapler-squad/session/git/internal/gittest"
)

func setupTestRepo(t *testing.T) string { return gittest.SetupTestRepo(t) }

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	return gittest.RunGit(t, dir, args...)
}

func runRealGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	return gittest.RunGit(t, dir, args...)
}

// testWorktree stands in for session/git's *GitWorktree in tests: it carries the inputs
// SetupNewWorktree needs and caches the resolved base SHA the way the wrapper does.
type testWorktree struct {
	repoPath, worktreePath, branchName, baseCommitSHA string
}

// newTestWorktree mirrors the argument order of git.NewGitWorktreeFromStorageWithExecutor;
// the session-name argument is unused here.
func newTestWorktree(repoPath, worktreePath, _, branchName, baseCommitSHA string) *testWorktree {
	return &testWorktree{repoPath, worktreePath, branchName, baseCommitSHA}
}

func (w *testWorktree) nativeSetupNewWorktree() error {
	sha, err := SetupNewWorktree(SetupParams{
		RepoPath:      w.repoPath,
		WorktreePath:  w.worktreePath,
		BranchName:    w.branchName,
		BaseCommitSHA: w.baseCommitSHA,
	})
	if sha != "" {
		w.baseCommitSHA = sha
	}
	return err
}

func (w *testWorktree) GetBaseCommitSHA() string { return w.baseCommitSHA }
