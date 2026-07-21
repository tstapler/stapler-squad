package git

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCommitChanges_UntracksIgnoredPreTrackedFiles is the regression test for the
// root cause of the ".backlog-context.md keeps getting committed" bug: a branch
// that at some point got a scaffolding file committed (a stale branch predating
// the write-time self-heal, or any other route) must never have that file
// re-staged/re-committed by a later CommitChanges call, even though `git add .`
// alone would happily restage anything already tracked.
func TestCommitChanges_UntracksIgnoredPreTrackedFiles(t *testing.T) {
	repoDir := setupTestRepo(t)

	// Simulate a stale branch that already committed the scaffolding file —
	// exactly the state that predates the write-time self-heal (or any commit
	// made outside stapler-squad's own write path).
	contextPath := filepath.Join(repoDir, ".backlog-context.md")
	require.NoError(t, os.WriteFile(contextPath, []byte("stale context"), 0o644))
	runGit(t, repoDir, "add", ".backlog-context.md")
	runGit(t, repoDir, "commit", "-m", "stale: commit scaffolding file")
	require.Contains(t, runGit(t, repoDir, "ls-files"), ".backlog-context.md", "test setup failed: file must be tracked before CommitChanges runs")

	// Mid-session: the scaffolding file gets rewritten (as a real session
	// would) and a real source file changes too.
	require.NoError(t, os.WriteFile(contextPath, []byte("fresh context"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("# Updated"), 0o644))

	wt := NewGitWorktreeFromStorage(repoDir, repoDir, "test-commit", "main", "")
	require.NoError(t, wt.CommitChanges("wip: real change"))

	tracked := runGit(t, repoDir, "ls-files")
	assert.NotContains(t, tracked, ".backlog-context.md", "scaffolding file must be untracked, not recommitted")

	// git-rm-cached semantics: the working-tree file itself is left alone.
	data, err := os.ReadFile(contextPath)
	require.NoError(t, err, "scaffolding file must still exist on disk after being untracked")
	assert.Equal(t, "fresh context", string(data))

	// The real change must still have been committed.
	assert.Contains(t, runGit(t, repoDir, "show", "--stat", "HEAD"), "README.md")
}

// TestCommitChanges_SkipsCommitGracefully_WhenOnlyScaffoldingStaged covers the
// edge case where the only dirty change in the worktree is a brand-new
// scaffolding file (never previously tracked in HEAD — e.g. a worktree where
// the info/exclude rules weren't set up) that `git add .` stages and
// UntrackScaffolding then removes again: net zero change vs. HEAD.
// CommitChanges must skip the commit rather than erroring on git's "nothing to
// commit".
func TestCommitChanges_SkipsCommitGracefully_WhenOnlyScaffoldingStaged(t *testing.T) {
	repoDir := setupTestRepo(t)
	headBefore := runGit(t, repoDir, "rev-parse", "HEAD")

	contextPath := filepath.Join(repoDir, ".backlog-context.md")
	require.NoError(t, os.WriteFile(contextPath, []byte("fresh context"), 0o644))

	wt := NewGitWorktreeFromStorage(repoDir, repoDir, "test-commit", "main", "")
	require.NoError(t, wt.CommitChanges("wip: should be a no-op"))

	assert.Equal(t, headBefore, runGit(t, repoDir, "rev-parse", "HEAD"), "HEAD must not move when the only staged change was untracked scaffolding")
	assert.NotContains(t, runGit(t, repoDir, "ls-files"), ".backlog-context.md")
	_, statErr := os.Stat(contextPath)
	assert.NoError(t, statErr, "scaffolding file must still exist on disk, untracked")
}

// TestStageAllExceptScaffolding_LeavesUnrelatedTrackedFilesAlone verifies the
// staging guard stages a normal new file as usual, while a scaffolding file
// added alongside it never ends up staged.
func TestStageAllExceptScaffolding_LeavesUnrelatedTrackedFilesAlone(t *testing.T) {
	repoDir := setupTestRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "new-file.txt"), []byte("hello"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, ".backlog-context.md"), []byte("scaffolding"), 0o644))

	wt := NewGitWorktreeFromStorage(repoDir, repoDir, "test-stage", "main", "")
	require.NoError(t, wt.StageAllExceptScaffolding())

	staged := runGit(t, repoDir, "diff", "--cached", "--name-only")
	assert.Contains(t, staged, "new-file.txt")
	assert.NotContains(t, staged, ".backlog-context.md")
}
