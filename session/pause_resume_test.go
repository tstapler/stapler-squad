package session

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakePauseResumeProcessManager is a minimal ProcessManager test double for
// exercising Resume()'s "tmux session is dead" branch without touching real
// tmux. Embeds a nil ProcessManager so unused methods panic loudly if a code
// path we didn't anticipate starts calling them.
type fakePauseResumeProcessManager struct {
	ProcessManager
	alive     bool
	startErr  error
	startCall int
}

func (f *fakePauseResumeProcessManager) IsAlive() bool { return f.alive }

func (f *fakePauseResumeProcessManager) Start(dir string) error {
	f.startCall++
	return f.startErr
}

// TestPause_should_NoOpSucceed_When_ActiveInstanceNeverStarted verifies the
// fix for the pause/resume-500 bug: pausing an Active instance that was never
// actually started (e.g. the async CreateSession goroutine hasn't finished
// yet) performs a state-only transition instead of erroring.
func TestPause_should_NoOpSucceed_When_ActiveInstanceNeverStarted(t *testing.T) {
	t.Parallel()
	inst := &Instance{
		Title:       "never-started",
		Status:      Active,
		Permissions: GetManagedPermissions(),
	}

	err := inst.Pause()

	require.NoError(t, err)
	assert.Equal(t, Paused, inst.Status)
	assert.True(t, inst.Started(), "no-op pause should mark the instance as started")
}

// TestPause_should_ReturnErrPauseNotPermitted_When_PermissionDenied verifies the
// permission gate added to close the MCP-tool bypass (server/mcp/tools_lifecycle.go
// calls Instance.Pause() directly, so the guard must live here, not just in
// UpdateSession).
func TestPause_should_ReturnErrPauseNotPermitted_When_PermissionDenied(t *testing.T) {
	t.Parallel()
	inst := &Instance{
		Title:       "no-pause-perm",
		Status:      Active,
		Permissions: GetExternalPermissions(false),
	}
	inst.started.Store(true)

	err := inst.Pause()

	require.ErrorIs(t, err, ErrPauseNotPermitted)
}

// TestPause_should_ReturnAlreadyPausedError_When_AlreadyPaused verifies the
// already-Paused no-op guard still runs before the not-started branch.
func TestPause_should_ReturnAlreadyPausedError_When_AlreadyPaused(t *testing.T) {
	t.Parallel()
	inst := &Instance{
		Title:       "already-paused",
		Status:      Paused,
		Permissions: GetManagedPermissions(),
	}

	err := inst.Pause()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "already paused")
}

// TestPause_should_ReturnInvalidTransition_When_NeverStartedFromNonPauseableState
// verifies that the not-started no-op path still defers to the state machine:
// Creating/Stopped/Hibernated -> Paused are not valid transitions.
func TestPause_should_ReturnInvalidTransition_When_NeverStartedFromNonPauseableState(t *testing.T) {
	t.Parallel()
	for _, status := range []Status{Creating, Stopped, Hibernated} {
		t.Run(status.String(), func(t *testing.T) {
			t.Parallel()
			inst := &Instance{
				Title:       "never-started-" + status.String(),
				Status:      status,
				Permissions: GetManagedPermissions(),
			}

			err := inst.Pause()

			var transErr ErrInvalidTransition
			require.ErrorAs(t, err, &transErr)
			assert.Equal(t, status, transErr.From)
			assert.Equal(t, Paused, transErr.To)
		})
	}
}

// TestResume_should_PerformRealResumeAndMarkStarted_When_PausedInstanceNeverStarted
// verifies the other half of the bug fix: Resume() no longer refuses to run
// just because `started` was never set (e.g. a Paused instance loaded from a
// state where Start() was never actually called).
func TestResume_should_PerformRealResumeAndMarkStarted_When_PausedInstanceNeverStarted(t *testing.T) {
	t.Parallel()
	inst := &Instance{
		Title:       "never-started-paused",
		Status:      Paused,
		Permissions: GetManagedPermissions(),
		Path:        t.TempDir(),
	}
	fakePM := &fakePauseResumeProcessManager{alive: false}
	inst.processManager = fakePM

	err := inst.Resume()

	require.NoError(t, err)
	assert.Equal(t, Active, inst.Status)
	assert.True(t, inst.Started())
	assert.Equal(t, 1, fakePM.startCall, "Resume should start a fresh session when the old one is dead")
}

// TestResume_should_ReturnErrResumeNotPermitted_When_PermissionDenied mirrors
// the pause-side permission gate for the MCP-tool bypass.
func TestResume_should_ReturnErrResumeNotPermitted_When_PermissionDenied(t *testing.T) {
	t.Parallel()
	inst := &Instance{
		Title:       "no-resume-perm",
		Status:      Paused,
		Permissions: GetExternalPermissions(false),
	}

	err := inst.Resume()

	require.ErrorIs(t, err, ErrResumeNotPermitted)
}

// TestResume_should_NotFailFatally_When_ClaudeSessionReattachmentFails verifies
// that Resume() on a never-started instance still tolerates a failing/irrelevant
// claudeSession reattachment attempt (handleClaudeSessionReattachment errors are
// logged, not fatal).
func TestResume_should_NotFailFatally_When_ClaudeSessionReattachmentFails(t *testing.T) {
	t.Parallel()
	inst := &Instance{
		Title:       "never-started-paused-claude",
		Status:      Paused,
		Permissions: GetManagedPermissions(),
		Path:        t.TempDir(),
	}
	inst.claudeSession = &ClaudeSessionData{
		ConversationUUID: "does-not-exist",
		Settings:         ClaudeSettings{AutoReattach: true},
	}
	fakePM := &fakePauseResumeProcessManager{alive: false}
	inst.processManager = fakePM

	err := inst.Resume()

	require.NoError(t, err)
	assert.Equal(t, Active, inst.Status)
	assert.True(t, inst.Started())
}

// TestPauseResume_should_UseSentinelErrors verifies errors.Is compatibility for
// the classifyPauseResumeErr helper in server/services/session_service.go.
func TestPauseResume_should_UseSentinelErrors(t *testing.T) {
	t.Parallel()
	assert.True(t, errors.Is(ErrPauseNotPermitted, ErrPauseNotPermitted))
	assert.True(t, errors.Is(ErrResumeNotPermitted, ErrResumeNotPermitted))
}
