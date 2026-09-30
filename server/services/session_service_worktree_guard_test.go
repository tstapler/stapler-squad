package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/git"
)

// mustMkdirSiblingCwd creates dir on disk before it's used as a live probe
// instance's simulated pane cwd. A real tmux pane's cwd always exists on
// disk -- filepath.EvalSymlinks (used by the production symlink-canonical
// comparison in OtherLiveSessionInsideWorktree) errors on a path that
// doesn't exist and silently falls back to leaving it unresolved, which
// would make these guard tests pass for the wrong reason (or fail outright
// on hosts like macOS where t.TempDir()'s real path has a symlink hop, e.g.
// /var -> /private/var) if this directory were never actually created.
func mustMkdirSiblingCwd(t *testing.T, dir string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0755))
	return dir
}

// newWorktreeGuardTarget builds a session.Instance whose GetEffectiveRootDir
// resolves to worktreePath -- the shape UpdateSession's pause/stop status
// transitions and DeleteSession would otherwise delete without AC4's guard.
// A struct literal is safe here (unlike a live tmux-backed instance):
// GitWorktreeManager is a value-typed field, not an interface, so its zero
// value is usable, and SetGitWorktree only touches that field.
func newWorktreeGuardTarget(title, uuid, worktreePath string) *session.Instance {
	inst := &session.Instance{
		Title:       title,
		UUID:        uuid,
		Path:        worktreePath,
		Status:      session.Active,
		Program:     "claude",
		Permissions: session.GetManagedPermissions(),
	}
	inst.SetGitWorktree(git.NewGitWorktreeFromStorage(worktreePath, worktreePath, title, "guard-branch", "0000000000000000000000000000000000000000"))
	return inst
}

// TestUpdateSession_Pause_RefusesWhenWorktreeSharedWithOtherLiveSession and its
// Stop sibling below are AC4's RPC-level regression tests: RPC UpdateSession's
// pause/stop status transitions -- the web UI's pause/stop controls -- delete
// the target's git worktree (StopByUser/Pause), so a live sibling round still
// running inside a shared worktree must block them, same as the MCP
// pause_session/stop_session guard in server/mcp/tools_lifecycle.go. Both now
// delegate to SessionService.RefuseIfWorktreeSharedWithOtherLiveSession.
func TestUpdateSession_Pause_RefusesWhenWorktreeSharedWithOtherLiveSession(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	worktree := t.TempDir()
	sibling := newLiveProbeInstance(t, "guard-sibling-pause-"+t.Name(), mustMkdirSiblingCwd(t, filepath.Join(worktree, "server")))
	target := newWorktreeGuardTarget("guard-target-pause", "uuid-guard-target-pause", worktree)
	addInstanceToPoller(fix.poller, sibling)
	addInstanceToPoller(fix.poller, target)

	paused := sessionv1.SessionStatus_SESSION_STATUS_PAUSED
	_, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:     "guard-target-pause",
		Status: &paused,
	}))

	require.Error(t, err, "pausing must refuse while a sibling live session's real cwd is inside the shared worktree")
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeFailedPrecondition, connectErr.Code())
	assert.Contains(t, connectErr.Message(), sibling.UUID, "the error must name the blocking session so the operator knows what to stop first")
	assert.Equal(t, session.Active, target.Status, "the target must not have been paused (and its worktree not removed) once the guard refused")
}

func TestUpdateSession_Stop_RefusesWhenWorktreeSharedWithOtherLiveSession(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	worktree := t.TempDir()
	sibling := newLiveProbeInstance(t, "guard-sibling-stop-"+t.Name(), mustMkdirSiblingCwd(t, filepath.Join(worktree, "web-app")))
	target := newWorktreeGuardTarget("guard-target-stop", "uuid-guard-target-stop", worktree)
	addInstanceToPoller(fix.poller, sibling)
	addInstanceToPoller(fix.poller, target)

	stopped := sessionv1.SessionStatus_SESSION_STATUS_STOPPED
	_, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:     "guard-target-stop",
		Status: &stopped,
	}))

	require.Error(t, err, "stopping must refuse while a sibling live session's real cwd is inside the shared worktree")
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeFailedPrecondition, connectErr.Code())
	assert.Contains(t, connectErr.Message(), sibling.UUID)
	assert.Equal(t, session.Active, target.Status, "the target must not have been stopped (and its worktree not removed) once the guard refused")
}

// TestUpdateSession_Pause_AllowsWhenNoOtherLiveSessionInsideWorktree confirms
// the guard is not a blanket refusal: a worktree session with no other live
// occupant still reaches instance.Pause() and transitions normally.
func TestUpdateSession_Pause_AllowsWhenNoOtherLiveSessionInsideWorktree(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	worktree := t.TempDir()
	elsewhere := newLiveProbeInstance(t, "guard-elsewhere-"+t.Name(), t.TempDir())
	target := newWorktreeGuardTarget("guard-target-alone", "uuid-guard-target-alone", worktree)
	addInstanceToPoller(fix.poller, elsewhere)
	addInstanceToPoller(fix.poller, target)

	paused := sessionv1.SessionStatus_SESSION_STATUS_PAUSED
	resp, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:     "guard-target-alone",
		Status: &paused,
	}))

	require.NoError(t, err, "no other live session occupies the worktree, so pause must proceed")
	require.NotNil(t, resp.Msg.Session)
	assert.Equal(t, sessionv1.SessionStatus_SESSION_STATUS_PAUSED, resp.Msg.Session.Status)
}

// TestDeleteSession_RefusesWhenWorktreeSharedWithOtherLiveSession is
// DeleteSession's AC4 regression test: DeleteSession's liveInst branch calls
// instance.Destroy(), which removes the git worktree exactly like
// StopByUser()/Pause() -- so it needs the same guard, applied before any
// cleanup goroutine is scheduled.
func TestDeleteSession_RefusesWhenWorktreeSharedWithOtherLiveSession(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	worktree := t.TempDir()
	sibling := newLiveProbeInstance(t, "guard-sibling-delete-"+t.Name(), mustMkdirSiblingCwd(t, filepath.Join(worktree, "session")))
	target := newWorktreeGuardTarget("guard-target-delete", "uuid-guard-target-delete", worktree)
	addInstanceToPoller(fix.poller, sibling)
	addInstanceToPoller(fix.poller, target)
	require.NoError(t, fix.storage.AddInstance(target))

	_, err := fix.svc.DeleteSession(context.Background(), connect.NewRequest(&sessionv1.DeleteSessionRequest{
		Id: "guard-target-delete",
	}))

	require.Error(t, err, "deleting must refuse while a sibling live session's real cwd is inside the shared worktree")
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeFailedPrecondition, connectErr.Code())
	assert.Contains(t, connectErr.Message(), sibling.UUID)

	instances, err := fix.storage.LoadInstances()
	require.NoError(t, err)
	found := false
	for _, inst := range instances {
		if inst.Title == "guard-target-delete" {
			found = true
		}
	}
	assert.True(t, found, "the target session must still exist in storage once the guard refused the delete")
}

// TestDeleteSession_AllowsWhenNoOtherLiveSessionInsideWorktree is
// TestDeleteSession_RefusesWhenWorktreeSharedWithOtherLiveSession's positive
// counterpart: a worktree-backed delete with no sibling live session inside
// it must still succeed — the guard is not a blanket refusal for every
// worktree-backed session, only one whose worktree is genuinely shared.
func TestDeleteSession_AllowsWhenNoOtherLiveSessionInsideWorktree(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	worktree := t.TempDir()
	elsewhere := newLiveProbeInstance(t, "guard-elsewhere-delete-"+t.Name(), t.TempDir())
	target := newWorktreeGuardTarget("guard-target-delete-alone", "uuid-guard-target-delete-alone", worktree)
	addInstanceToPoller(fix.poller, elsewhere)
	addInstanceToPoller(fix.poller, target)
	require.NoError(t, fix.storage.AddInstance(target))

	resp, err := fix.svc.DeleteSession(context.Background(), connect.NewRequest(&sessionv1.DeleteSessionRequest{
		Id: "guard-target-delete-alone",
	}))

	require.NoError(t, err, "no other live session occupies the worktree, so delete must proceed")
	assert.True(t, resp.Msg.Success)
}
