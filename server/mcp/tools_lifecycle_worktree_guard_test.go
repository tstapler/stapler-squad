package mcp

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// liveSessionExecutor is an executor.Executor double answering the only two
// tmux subprocess calls SessionService.OtherLiveSessionInsideWorktree makes per
// candidate instance: list-sessions (CombinedOutput, behind
// IsBackendProcessAlive) and display-message -p '#{pane_current_path}' (Output,
// behind GetCurrentWorkingDirectory). No tmux server is involved.
type liveSessionExecutor struct {
	sessionName string
	paneCWD     string
}

func (e *liveSessionExecutor) CombinedOutput(_ *exec.Cmd) ([]byte, error) {
	return []byte(e.sessionName + "\n"), nil
}

func (e *liveSessionExecutor) Run(_ *exec.Cmd) error {
	return fmt.Errorf("liveSessionExecutor: Run is unsupported on this path")
}

func (e *liveSessionExecutor) Output(_ *exec.Cmd) ([]byte, error) {
	return []byte(e.paneCWD + "\n"), nil
}

// newWorktreeGuardHandlers wires a real *services.SessionService (the field is a
// concrete type, so there is no seam to stub OtherLiveSessionInsideWorktree
// behind) to a poller holding occupants, and returns lifecycleHandlers over it.
func newWorktreeGuardHandlers(t *testing.T, occupants ...*session.Instance) *lifecycleHandlers {
	t.Helper()
	storage := newTestBacklogStorage(t)
	bus := events.NewEventBus(16)
	svc := services.NewSessionService(storage, bus)
	t.Cleanup(func() { bus.Close() })
	t.Cleanup(func() { svc.Shutdown() })

	poller := session.NewReviewQueuePoller(session.NewReviewQueue(), session.NewInstanceStatusManager(), nil)
	poller.SetInstances(occupants)
	svc.SetReviewQueuePoller(poller)

	return &lifecycleHandlers{store: storage, svc: svc}
}

// newLiveOccupant builds an instance whose tmux backend reports alive and whose
// pane cwd is paneCWD. session.NewInstance (not a struct literal) is required:
// SetTmuxSession only takes effect once the constructor has wired a TmuxBackend.
// A nil PtyFactory is safe — neither the liveness probe nor the pane-path lookup
// touches it.
func newLiveOccupant(t *testing.T, title, paneCWD string) *session.Instance {
	t.Helper()
	inst, err := session.NewInstance(session.InstanceOptions{Title: title, Path: t.TempDir()})
	require.NoError(t, err)
	inst.SetTmuxSession(tmux.NewTmuxSessionWithDeps(title, "true", nil, &liveSessionExecutor{
		sessionName: tmux.NewSessionName(title, tmux.TmuxPrefix).String(),
		paneCWD:     paneCWD,
	}))
	return inst
}

// newWorktreeTargetInstance builds the pause/stop target: a session whose
// GetEffectiveRootDir resolves to worktreePath, which both pause_session and
// stop_session would delete.
func newWorktreeTargetInstance(uuid, worktreePath string) *session.Instance {
	inst := &session.Instance{Title: "guard-target-" + uuid, UUID: uuid, Path: worktreePath, Status: session.Active}
	inst.SetGitWorktree(git.NewGitWorktreeFromStorage(worktreePath, worktreePath, "guard-target", "guard-branch", ""))
	return inst
}

// TestRefuseIfWorktreeSharedWithOtherLiveSession_BlocksWhenSiblingStillRunning
// is the regression test for the incident where stop_session/pause_session
// deleted a worktree a second, still-running session was working in: rework
// rounds of one backlog item deliberately share a worktree, so the guard must
// refuse with CONFLICT and name the occupant. It fails against the pre-fix
// behavior, which had no guard at all and let the worktree be removed.
func TestRefuseIfWorktreeSharedWithOtherLiveSession_BlocksWhenSiblingStillRunning(t *testing.T) {
	worktree := t.TempDir()
	sibling := newLiveOccupant(t, "guard-sibling-"+t.Name(), filepath.Join(worktree, "web-app"))
	lh := newWorktreeGuardHandlers(t, sibling)

	target := newWorktreeTargetInstance("uuid-guard-target", worktree)
	require.True(t, target.HasGitWorktree(), "test setup: the target must be a worktree session for the guard to apply")

	res := lh.refuseIfWorktreeSharedWithOtherLiveSession(target)

	require.NotNil(t, res, "a worktree still occupied by another live session must not be deleted")
	parsed := parseResult(t, res)
	errObj, ok := parsed["error"].(map[string]interface{})
	require.True(t, ok, "blocked result must carry an MCPError, got %v", parsed)
	assert.Equal(t, ErrConflict, errObj["code"])
	assert.Contains(t, errObj["message"], sibling.UUID, "the message must name the session that has to be stopped first")
}

// TestRefuseIfWorktreeSharedWithOtherLiveSession_AllowsWhenNothingElseOccupiesIt
// covers the two ways the guard must stand aside, so it can never become a
// blanket refusal of pause/stop: a non-worktree session (nothing to delete) and
// a worktree whose only other live occupant is working somewhere else entirely.
func TestRefuseIfWorktreeSharedWithOtherLiveSession_AllowsWhenNothingElseOccupiesIt(t *testing.T) {
	worktree := t.TempDir()
	elsewhere := newLiveOccupant(t, "guard-elsewhere-"+t.Name(), t.TempDir())
	lh := newWorktreeGuardHandlers(t, elsewhere)

	t.Run("directory session has no worktree to protect", func(t *testing.T) {
		plain := &session.Instance{Title: "guard-plain", UUID: "uuid-plain", Path: worktree, Status: session.Active}
		require.False(t, plain.HasGitWorktree(), "test setup: this instance must have no worktree")
		assert.Nil(t, lh.refuseIfWorktreeSharedWithOtherLiveSession(plain))
	})

	t.Run("worktree with no other live occupant inside it", func(t *testing.T) {
		assert.Nil(t, lh.refuseIfWorktreeSharedWithOtherLiveSession(newWorktreeTargetInstance("uuid-lonely", worktree)))
	})
}
