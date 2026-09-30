package git

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// WorktreeAdminFixtureResult is what WorktreeAdminFixture returns: a real repo plus a
// real, git-CLI-created linked worktree — Story 5.1.2's verified baseline every
// native-worktree interop test in this phase (and Epics 5.2/5.3's fuzz/golden tests)
// starts from, rather than each test hand-rolling its own `git worktree add` sequence.
type WorktreeAdminFixtureResult struct {
	RepoPath     string
	WorktreePath string
	BranchName   string
}

// WorktreeAdminFixture builds a fresh repo (`git init`/commit) and adds a linked
// worktree for branchName via real `git worktree add -b` (subprocess, via
// safeexec.CommandContext per this package's existing test convention — see
// runRealGit in native_worktree_add_test.go), entirely inside t.TempDir() (Task 5.1.2a).
func WorktreeAdminFixture(t *testing.T, branchName string) WorktreeAdminFixtureResult {
	t.Helper()

	repoPath := t.TempDir()
	runGit(t, repoPath, "init", "-b", "main")
	runGit(t, repoPath, "config", "user.email", "test@example.com")
	runGit(t, repoPath, "config", "user.name", "Test User")
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("# fixture\n"), 0o644))
	runGit(t, repoPath, "add", ".")
	runGit(t, repoPath, "commit", "-m", "initial commit")

	worktreePath := filepath.Join(t.TempDir(), branchName)
	runGit(t, repoPath, "worktree", "add", "-b", branchName, worktreePath)

	return WorktreeAdminFixtureResult{
		RepoPath:     repoPath,
		WorktreePath: worktreePath,
		BranchName:   branchName,
	}
}

// TestWorktreeAdminFixture_ProducesRealGitRecognizedWorktree covers Story 5.1.2's
// acceptance criterion: the fixture's worktree must be indistinguishable, to real git
// itself, from a hand-run `git worktree add` — listed live, non-prunable, non-locked.
func TestWorktreeAdminFixture_ProducesRealGitRecognizedWorktree(t *testing.T) {
	t.Parallel()

	fx := WorktreeAdminFixture(t, "feature-x")

	// runGit (defined package-wide in ops_test.go) is this package's existing
	// safeexec.CommandContext-based real-git-subprocess test helper — reused here rather
	// than redeclared.
	out := runGit(t, fx.RepoPath, "worktree", "list", "--porcelain")

	assert.Contains(t, out, fx.WorktreePath, "fixture's worktree path must be listed")
	assert.NotContains(t, out, "prunable", "a freshly created worktree must not be prunable")
	assert.NotContains(t, out, "locked", "a freshly created worktree must not be locked")
}
