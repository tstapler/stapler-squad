package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/git"
)

// startedWorktreeInstance returns a started Instance backed by a real, clean git worktree.
func startedWorktreeInstance(t *testing.T, name string) *Instance {
	t.Helper()
	repoPath := t.TempDir()
	initTestRepoForReviewQueue(t, repoPath)

	wt, _, err := git.NewGitWorktree(repoPath, name)
	require.NoError(t, err)
	require.NoError(t, wt.Setup())
	t.Cleanup(func() { _ = wt.Cleanup() })

	inst := &Instance{
		Title:       name,
		Status:      Running,
		IsWorktree:  true,
		Permissions: GetManagedPermissions(),
		gitManager:  GitWorktreeManager{worktree: wt},
	}
	inst.started.Store(true)
	return inst
}

// dirtyWorktree leaves an untracked file in inst's worktree and returns its path.
func dirtyWorktree(t *testing.T, inst *Instance) string {
	t.Helper()
	precious := filepath.Join(inst.gitManager.GetWorktreePath(), "precious.txt")
	require.NoError(t, os.WriteFile(precious, []byte("uncommitted work"), 0o644))
	return precious
}

func TestCheckWorktreeRemovable(t *testing.T) {
	t.Parallel()

	t.Run("clean worktree is removable", func(t *testing.T) {
		t.Parallel()
		inst := startedWorktreeInstance(t, "check-clean")
		assert.NoError(t, inst.CheckWorktreeRemovable())
	})

	t.Run("untracked file blocks removal", func(t *testing.T) {
		t.Parallel()
		inst := startedWorktreeInstance(t, "check-dirty")
		dirtyWorktree(t, inst)
		assert.ErrorIs(t, inst.CheckWorktreeRemovable(), ErrWorktreeDirty)
	})

	t.Run("unreadable dirty state fails closed", func(t *testing.T) {
		t.Parallel()
		inst, _ := brokenWorktreeInstance(t)
		assert.ErrorIs(t, inst.CheckWorktreeRemovable(), ErrDirtyStateUnknown)
	})

	t.Run("worktree gone from disk is removable", func(t *testing.T) {
		t.Parallel()
		inst := &Instance{
			IsWorktree: true,
			gitManager: GitWorktreeManager{worktree: git.NewGitWorktreeFromStorage(t.TempDir(), filepath.Join(t.TempDir(), "gone"), "s", "b", "")},
		}
		assert.NoError(t, inst.CheckWorktreeRemovable())
	})

	t.Run("session without a worktree is removable", func(t *testing.T) {
		t.Parallel()
		inst := &Instance{IsWorktree: true}
		assert.NoError(t, inst.CheckWorktreeRemovable())
	})
}

func TestDestroy_should_KeepDirtyWorktree_When_NotForced(t *testing.T) {
	t.Parallel()
	inst := startedWorktreeInstance(t, "destroy-dirty")
	precious := dirtyWorktree(t, inst)

	err := inst.Destroy()

	require.ErrorIs(t, err, ErrWorktreeDirty)
	assert.FileExists(t, precious)
}

func TestForceDestroy_should_RemoveDirtyWorktree(t *testing.T) {
	t.Parallel()
	inst := startedWorktreeInstance(t, "destroy-force")
	precious := dirtyWorktree(t, inst)

	require.NoError(t, inst.ForceDestroy())

	assert.NoFileExists(t, precious)
	assert.NoDirExists(t, inst.gitManager.GetWorktreePath())
}

func TestDestroy_should_RemoveCleanWorktree(t *testing.T) {
	t.Parallel()
	inst := startedWorktreeInstance(t, "destroy-clean")

	require.NoError(t, inst.Destroy())

	assert.NoDirExists(t, inst.gitManager.GetWorktreePath())
}

// A failed restore must leave the worktree alone even when it is clean: it predates the
// attempt, and a transient failure (e.g. tmux killed at startup) is not a reason to delete it.
func TestCleanupAfterFailedStart_should_KeepWorktree_When_Restoring(t *testing.T) {
	t.Parallel()
	inst := startedWorktreeInstance(t, "restore-failed")

	require.NoError(t, inst.cleanupAfterFailedStart(false))

	assert.DirExists(t, inst.gitManager.GetWorktreePath())
}

func TestCleanupAfterFailedStart_should_RemoveCleanWorktree_When_FirstTimeSetup(t *testing.T) {
	t.Parallel()
	inst := startedWorktreeInstance(t, "setup-failed")

	require.NoError(t, inst.cleanupAfterFailedStart(true))

	assert.NoDirExists(t, inst.gitManager.GetWorktreePath())
}
