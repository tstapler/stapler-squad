package session

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// TestCaptureCurrentState_NotStarted_IsNoOp verifies that CaptureCurrentState
// returns nil without modifying WorkingDir when the instance has not been started.
func TestCaptureCurrentState_NotStarted_IsNoOp(t *testing.T) {
	t.Parallel()
	inst := &instance.Instance{Title: "test-session"}
	// inst.started == false by default

	err := inst.CaptureCurrentState()

	require.NoError(t, err)
	assert.Empty(t, inst.WorkingDir, "WorkingDir should not be set for unstarted instance")
}

// TestInstance_CaptureCurrentState_UpdatesWorkingDir verifies the happy path:
// a started, running instance with a live tmux session has WorkingDir populated.
func TestInstance_CaptureCurrentState_UpdatesWorkingDir(t *testing.T) {
	t.Parallel()
	const sessionName = "capture-happy"
	mockExec := &tmux.MockCmdExec{
		// list-sessions response: our session is alive
		CombinedOutputFunc: func(_ *exec.Cmd) ([]byte, error) {
			return []byte("staplersquad_capture-happy\n"), nil
		},
		// display-message response: current pane path
		OutputFunc: func(_ *exec.Cmd) ([]byte, error) {
			return []byte("/home/user/project\n"), nil
		},
	}
	mockSession := tmux.NewTmuxSessionWithDeps(sessionName, "echo", nil, mockExec)
	tpm := &TmuxProcessManager{}
	tpm.SetSession(mockSession)
	inst := (&instance.Instance{Title: sessionName}).WithProcessManagerForTest(NewTmuxBackend(tpm))
	inst.MarkStarted()

	err := inst.CaptureCurrentState()

	require.NoError(t, err)
	assert.Equal(t, "/home/user/project", inst.WorkingDir)
}

// --- BUG-033: pathEscapesRoot (the write-side gate CaptureCurrentState now
// applies, and the logic resolveStartPath's pre-existing read-side backstop
// was refactored to share) ---

// TestPathEscapesRoot_should_returnFalse_When_CandidateIsRootItself verifies
// the root path itself is never considered "escaped".
func TestPathEscapesRoot_should_returnFalse_When_CandidateIsRootItself(t *testing.T) {
	t.Parallel()
	assert.False(t, instance.PathEscapesRoot("/home/user/worktree", "/home/user/worktree"))
}

// TestPathEscapesRoot_should_returnFalse_When_CandidateIsInsideRoot verifies a
// subdirectory of the worktree is never considered "escaped".
func TestPathEscapesRoot_should_returnFalse_When_CandidateIsInsideRoot(t *testing.T) {
	t.Parallel()
	assert.False(t, instance.PathEscapesRoot("/home/user/worktree", "/home/user/worktree/subdir/file"))
}

// TestPathEscapesRoot_should_returnTrue_When_CandidateIsParentRepo is the
// direct regression check for BUG-033's live incident: an autonomous backlog
// session's agent cd'd from its isolated worktree into the shared parent repo
// checkout (e.g. /home/tstapler/Programming/stapler-squad while its worktree
// was /home/tstapler/.stapler-squad/workspaces/.../worktrees/<slug>) — that
// parent path must be detected as escaped so CaptureCurrentState refuses to
// persist it as the session's WorkingDir.
func TestPathEscapesRoot_should_returnTrue_When_CandidateIsParentRepo(t *testing.T) {
	t.Parallel()
	assert.True(t, instance.PathEscapesRoot(
		"/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/stapler-squad-add-cron-schedule-builder-widget_18c4ac95c784fad8",
		"/home/tstapler/Programming/stapler-squad",
	))
}

// TestPathEscapesRoot_should_returnTrue_When_CandidateIsSiblingWorktree
// verifies one worktree is correctly detected as outside a different sibling
// worktree, not just outside the bare parent repo.
func TestPathEscapesRoot_should_returnTrue_When_CandidateIsSiblingWorktree(t *testing.T) {
	t.Parallel()
	assert.True(t, instance.PathEscapesRoot(
		"/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/item-a",
		"/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/item-b",
	))
}

// TestPathEscapesRoot_should_returnTrue_When_PathsAreUnrelated verifies an
// error from filepath.Rel (e.g. incomparable paths on some platforms, or any
// other resolution failure) fails closed — treated as escaped rather than
// silently allowed through.
func TestPathEscapesRoot_should_returnTrue_When_PathsAreUnrelated(t *testing.T) {
	t.Parallel()
	// filepath.Rel doesn't error on plain unrelated absolute Unix paths (it just
	// returns a "../.." style relative path) — this exercises that shape
	// directly, since a genuine filepath.Rel error is platform-specific.
	assert.True(t, instance.PathEscapesRoot("/home/tstapler/worktree-a", "/var/other/path"))
}
