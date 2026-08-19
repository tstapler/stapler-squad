package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEnsureDirectorySessionPath_CreatesAndGitInitsMissingDirectory verifies that
// a not-yet-existing path is created and git-initialized, matching the same
// behavior the inline SessionTypeDirectory/CreateIfMissing branch had before this
// logic was extracted into an exported, reusable function.
func TestEnsureDirectorySessionPath_CreatesAndGitInitsMissingDirectory(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	target := filepath.Join(base, "new-repo")

	err := EnsureDirectorySessionPath(target)
	require.NoError(t, err)

	info, statErr := os.Stat(target)
	require.NoError(t, statErr)
	require.True(t, info.IsDir())

	_, gitErr := os.Stat(filepath.Join(target, ".git"))
	require.NoError(t, gitErr, "expected target to be git-initialized")
}

// TestEnsureDirectorySessionPath_NoopWhenAlreadyExists verifies calling it on an
// already-existing directory (already a git repo, from a prior call) is a no-op
// that doesn't error — this is what makes it safe to call once before spawning a
// session and rely on the spawn's own CreateIfMissing check finding the directory
// already present.
func TestEnsureDirectorySessionPath_NoopWhenAlreadyExists(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	target := filepath.Join(base, "existing-repo")

	require.NoError(t, EnsureDirectorySessionPath(target))
	// Second call on the same, now-existing (and git-initialized) path must also succeed.
	require.NoError(t, EnsureDirectorySessionPath(target))

	_, gitErr := os.Stat(filepath.Join(target, ".git"))
	require.NoError(t, gitErr)
}

// TestEnsureDirectorySessionPath_ReturnsErrorOnNonNotExistStatFailure verifies that
// a stat failure other than "does not exist" (e.g. a path component that isn't a
// directory, producing ENOTDIR) surfaces as an error instead of being silently
// treated as "already exists, no-op" — a real path a plain os.IsNotExist check
// would otherwise mask.
func TestEnsureDirectorySessionPath_ReturnsErrorOnNonNotExistStatFailure(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	fileAsParent := filepath.Join(base, "not-a-dir")
	require.NoError(t, os.WriteFile(fileAsParent, []byte("x"), 0o644))

	// fileAsParent is a file, so treating it as a directory component (target
	// "underneath" it) fails stat with ENOTDIR, not IsNotExist.
	target := filepath.Join(fileAsParent, "child")

	err := EnsureDirectorySessionPath(target)
	require.Error(t, err)
}

// TestEnsureDirectorySessionPath_NoopWhenPathIsAnExistingFile documents the actual
// (pre-existing, unchanged-by-extraction) behavior: the function only acts when
// os.Stat reports the path does not exist at all — an existing plain file is left
// untouched rather than triggering git.InitializeProjectDirectory's own
// file-collision error, because the exists-check short-circuits first. This
// matches the original inline SessionTypeDirectory/CreateIfMissing logic exactly.
func TestEnsureDirectorySessionPath_NoopWhenPathIsAnExistingFile(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	target := filepath.Join(base, "not-a-dir")
	require.NoError(t, os.WriteFile(target, []byte("x"), 0o644))

	require.NoError(t, EnsureDirectorySessionPath(target))

	info, statErr := os.Stat(target)
	require.NoError(t, statErr)
	require.False(t, info.IsDir(), "the existing file must be left untouched, not replaced with a directory")
}

// TestGetEffectiveRootDir_ReturnsWorktreePath_EvenWhenMissingFromDisk documents
// that GetEffectiveRootDir must stay disk-existence-agnostic: HistoryLinker
// correlates sessions by the nominal worktree path string (see
// TestHistoryLinker_CorrelateSession_UsesWorktreePath_NotBasePath in
// history_linker_test.go), which requires the raw path regardless of whether
// the directory is actually present on disk.
func TestGetEffectiveRootDir_ReturnsWorktreePath_EvenWhenMissingFromDisk(t *testing.T) {
	t.Parallel()
	repoPath := t.TempDir()
	worktreePath := filepath.Join(t.TempDir(), "nonexistent-worktree")

	inst := &Instance{Title: "worktree-session", Path: repoPath, Status: Running}
	inst.gitManager.SetWorktree(newTestGitWorktree(repoPath, worktreePath))

	require.Equal(t, worktreePath, inst.GetEffectiveRootDir())
}

// TestWorkspace_FallsBackToRepoRoot_WhenWorktreePathMissingFromDisk is a
// regression test for the "directory not found: ." ListFiles bug: a worktree
// removed from disk (e.g. by pause_session, which preserves the branch but
// deletes the worktree directory) left GetEffectiveRootDir's stale path
// flowing straight into filesystem-reading callers like FileService.ListFiles.
// Workspace() must fall back to RepoRoot when the worktree path no longer
// exists, while GetEffectiveRootDir itself stays unchanged (see the test
// above) for HistoryLinker's path-string correlation use case.
func TestWorkspace_FallsBackToRepoRoot_WhenWorktreePathMissingFromDisk(t *testing.T) {
	t.Parallel()
	repoPath := t.TempDir()
	worktreePath := filepath.Join(t.TempDir(), "nonexistent-worktree")

	inst := &Instance{Title: "worktree-session", Path: repoPath, Status: Running}
	inst.gitManager.SetWorktree(newTestGitWorktree(repoPath, worktreePath))

	ws := inst.Workspace()
	require.Equal(t, repoPath, ws.EffectivePath, "must fall back to RepoRoot when the worktree path is gone")
	require.Equal(t, repoPath, ws.RepoRoot)
}

// TestWorkspace_UsesWorktreePath_WhenPresentOnDisk verifies Workspace() does
// NOT fall back when the worktree path genuinely exists — the fallback must
// be a real disk-existence check, not an unconditional preference for RepoRoot.
func TestWorkspace_UsesWorktreePath_WhenPresentOnDisk(t *testing.T) {
	t.Parallel()
	repoPath := t.TempDir()
	worktreePath := t.TempDir()

	inst := &Instance{Title: "worktree-session", Path: repoPath, Status: Running}
	inst.gitManager.SetWorktree(newTestGitWorktree(repoPath, worktreePath))

	// NewGitWorktreeFromStorage canonicalizes worktreePath via
	// CanonicalizeWorktreePath since the directory exists on disk (e.g. macOS's
	// /var -> /private/var), so the expected value must go through the same
	// resolution rather than comparing against the raw t.TempDir() string.
	resolvedWorktreePath, err := filepath.EvalSymlinks(worktreePath)
	require.NoError(t, err)

	ws := inst.Workspace()
	require.Equal(t, resolvedWorktreePath, ws.EffectivePath)
	require.Equal(t, repoPath, ws.RepoRoot)
}
