package session

import (
	"os"
	"path/filepath"
	"testing"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/git"
)

// brokenWorktreeInstance returns a started worktree Instance whose worktree
// directory exists but is not a git checkout, so the dirty check errors.
func brokenWorktreeInstance(t *testing.T) (*Instance, string) {
	t.Helper()
	wtDir := t.TempDir()
	precious := filepath.Join(wtDir, "precious.txt")
	require.NoError(t, os.WriteFile(precious, []byte("uncommitted work"), 0o644))

	inst := &Instance{
		Title:       "dirty-check-fails",
		Status:      Running,
		IsWorktree:  true,
		Permissions: GetManagedPermissions(),
		gitManager: GitWorktreeManager{
			worktree: git.NewGitWorktreeFromStorage(t.TempDir(), wtDir, "dirty-check-fails", "branch", ""),
		},
	}
	inst.started.Store(true)
	return inst, precious
}

func TestPause_should_FailClosedAndKeepWorktree_When_DirtyCheckErrors(t *testing.T) {
	t.Parallel()
	inst, precious := brokenWorktreeInstance(t)

	err := inst.Pause()

	require.ErrorIs(t, err, ErrDirtyStateUnknown)
	assert.FileExists(t, precious)
	assert.Equal(t, Running, inst.Status, "status must not advance when pause was refused")
}

func TestStopByUser_should_FailClosedAndKeepWorktree_When_DirtyCheckErrors(t *testing.T) {
	t.Parallel()
	inst, precious := brokenWorktreeInstance(t)

	err := inst.StopByUser()

	require.ErrorIs(t, err, ErrDirtyStateUnknown)
	assert.FileExists(t, precious)
	assert.Equal(t, Running, inst.Status, "status must not advance when stop was refused")
}

// An untracked file must be reported as work to commit and must survive
// worktree removal on the branch (go-git's hard reset would have deleted it).
func TestWorktreeNeedsCommit_should_PreserveUntrackedFile_When_WorktreeRemoved(t *testing.T) {
	t.Parallel()
	repoPath := t.TempDir()
	initTestRepoForReviewQueue(t, repoPath)

	wt, branch, err := git.NewGitWorktree(repoPath, "untracked-survives")
	require.NoError(t, err)
	require.NoError(t, wt.Setup())
	t.Cleanup(func() { _ = wt.Cleanup() })

	inst := &Instance{IsWorktree: true, gitManager: GitWorktreeManager{worktree: wt}}
	require.NoError(t, os.WriteFile(filepath.Join(wt.GetWorktreePath(), "untracked.txt"), []byte("keep me"), 0o644))

	needs, err := inst.worktreeNeedsCommit()
	require.NoError(t, err)
	require.True(t, needs, "untracked file must count as uncommitted work")

	require.NoError(t, inst.gitManager.CommitChanges("save"))
	require.NoError(t, inst.gitManager.Remove())

	repo, err := gogit.PlainOpen(repoPath)
	require.NoError(t, err)
	ref, err := repo.Reference(plumbing.NewBranchReferenceName(branch), true)
	require.NoError(t, err)
	commit, err := repo.CommitObject(ref.Hash())
	require.NoError(t, err)
	f, err := commit.File("untracked.txt")
	require.NoError(t, err)
	content, err := f.Contents()
	require.NoError(t, err)
	assert.Equal(t, "keep me", content)
}

func TestWorktreeNeedsCommit_should_BeFalse_When_WorktreeDirMissing(t *testing.T) {
	t.Parallel()
	inst := &Instance{
		IsWorktree: true,
		gitManager: GitWorktreeManager{worktree: git.NewGitWorktreeFromStorage(t.TempDir(), filepath.Join(t.TempDir(), "gone"), "s", "b", "")},
	}

	needs, err := inst.worktreeNeedsCommit()

	require.NoError(t, err)
	assert.False(t, needs)
}
