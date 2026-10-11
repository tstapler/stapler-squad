package session

import (
	"os"
	"path/filepath"
	"testing"

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

	repo, err := git.OpenRepo(repoPath)
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

// A stale cached "clean" must not make CommitChanges skip the commit that the
// uncached check just called for.
func TestWorktreeNeedsCommit_should_ReachCommit_When_DirtyCacheIsStaleClean(t *testing.T) {
	t.Parallel()
	repoPath := t.TempDir()
	initTestRepoForReviewQueue(t, repoPath)
	wt, branch, err := git.NewGitWorktree(repoPath, "stale-clean")
	require.NoError(t, err)
	require.NoError(t, wt.Setup())
	t.Cleanup(func() { _ = wt.Cleanup() })
	inst := &Instance{IsWorktree: true, gitManager: GitWorktreeManager{worktree: wt}}

	dirty, err := inst.gitManager.IsDirty() // primes the clean cache
	require.NoError(t, err)
	require.False(t, dirty)
	require.NoError(t, os.WriteFile(filepath.Join(wt.GetWorktreePath(), "late.txt"), []byte("late"), 0o644))

	needs, err := inst.worktreeNeedsCommit()
	require.NoError(t, err)
	require.True(t, needs)
	require.NoError(t, inst.gitManager.CommitChanges("save"))

	repo, err := git.OpenRepo(repoPath)
	require.NoError(t, err)
	ref, err := repo.Reference(plumbing.NewBranchReferenceName(branch), true)
	require.NoError(t, err)
	commit, err := repo.CommitObject(ref.Hash())
	require.NoError(t, err)
	_, err = commit.File("late.txt")
	require.NoError(t, err, "late.txt must be committed despite the stale clean cache")
}

func TestCommitBeforeRemove_should_FailClosed_When_DirtyCheckErrors(t *testing.T) {
	t.Parallel()
	inst, precious := brokenWorktreeInstance(t)

	err := inst.commitBeforeRemove("msg")

	require.ErrorIs(t, err, ErrDirtyStateUnknown)
	assert.FileExists(t, precious)
}
