package session

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/git"
)

// fakeStopByUserProcessManager is a minimal ProcessManager test double that
// tracks whether Close() was ever called, so the rejection test below can
// assert deterministically that KillSession() never ran — without depending
// on a real PTY/tmux process (see fakePauseResumeProcessManager in
// pause_resume_test.go for the same pattern applied to Pause/Resume).
type fakeStopByUserProcessManager struct {
	ProcessManager
	hasSession  bool
	closeCalled bool
}

func (f *fakeStopByUserProcessManager) HasSession() bool { return f.hasSession }
func (f *fakeStopByUserProcessManager) IsAlive() bool    { return f.hasSession }
func (f *fakeStopByUserProcessManager) Close() error {
	f.closeCalled = true
	f.hasSession = false
	return nil
}

// TestStopByUser_should_KillTmuxAndTransitionToStopped_When_SessionIsActive verifies
// the happy path: an Active, non-worktree session transitions to Stopped.
// Follows the bare-&Instance{} construction pattern used by Pause()/Resume()'s
// own tests (see TestPause_should_skipGitOps_When_IsWorktreeIsFalse).
func TestStopByUser_should_KillTmuxAndTransitionToStopped_When_SessionIsActive(t *testing.T) {
	t.Parallel()
	inst := &Instance{
		Title:       "test-stop-active",
		Status:      Active,
		IsWorktree:  false,
		Permissions: GetManagedPermissions(),
	}
	inst.started.Store(true)

	err := inst.StopByUser()

	require.NoError(t, err, "StopByUser() on an Active session must not return an error")
	assert.Equal(t, Stopped, inst.Status)
}

// TestStopByUser_should_RejectTransition_When_SessionIsRestoring is the regression
// test for the pre-mortem P1 ordering fix: the legality check must run, and reject,
// before any destructive cleanup. Restoring has no outbound edge in transitionDefs,
// so Restoring->Stopped is illegal. The (fake) tmux session must still be alive and
// the on-disk worktree directory must still exist after the rejected call — i.e. no
// cleanup ran before the legality check rejected it.
func TestStopByUser_should_RejectTransition_When_SessionIsRestoring(t *testing.T) {
	t.Parallel()

	repoPath := t.TempDir()
	worktreePath := git.CanonicalizeWorktreePath(t.TempDir())

	inst := &Instance{
		Title:       "test-stop-restoring",
		Status:      Restoring,
		IsWorktree:  true,
		Path:        repoPath,
		Permissions: GetManagedPermissions(),
	}
	inst.started.Store(true)
	inst.gitManager.SetWorktree(newTestGitWorktree(repoPath, worktreePath))

	pm := &fakeStopByUserProcessManager{hasSession: true}
	inst.processManager = pm

	_, statErr := os.Stat(worktreePath)
	require.NoError(t, statErr, "precondition: worktree directory must exist before the rejected call")

	err := inst.StopByUser()

	var transErr ErrInvalidTransition
	require.True(t, errors.As(err, &transErr), "expected ErrInvalidTransition, got %T: %v", err, err)
	assert.Equal(t, Restoring, transErr.From)
	assert.Equal(t, Stopped, transErr.To)

	assert.False(t, pm.closeCalled, "rejected StopByUser must not kill the tmux session")
	assert.True(t, pm.IsAlive(), "rejected StopByUser must leave the tmux session alive")
	_, statErr = os.Stat(worktreePath)
	assert.NoError(t, statErr, "rejected StopByUser must not remove the worktree directory")
	assert.Equal(t, Restoring, inst.Status, "rejected StopByUser must not change the instance status")
}
