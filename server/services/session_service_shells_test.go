package services

// session_service_shells_test.go — unit tests for the shell RPC handlers:
// SpawnShell, StopShell, RestartShell, ListShells, DeleteShell.
//
// Test strategy:
//   - Error-path tests (missing session ID, session not in DB, session not running)
//     require no tmux and use only storage + poller fixture plumbing.
//   - Success-path tests pre-populate an Instance's in-memory shell registry via
//     session.Instance.AddShellInMemory to avoid spawning real tmux processes.
//   - RestartShell_Success is omitted: RestartShell always spawns a new sibling
//     tmux session (handle.Spawn), which requires a live tmux server; this belongs
//     in integration tests rather than unit tests.

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// makeStoppedShell returns a Shell in the Stopped state with closed channels so
// that DeleteShell's watcherDone drain completes immediately.
func makeStoppedShell(id, name string) *session.Shell {
	exitCh := make(chan struct{})
	close(exitCh)
	watcherDone := make(chan struct{})
	close(watcherDone)
	return &session.Shell{
		ID:          id,
		Name:        name,
		Command:     "/bin/sh",
		WorkingDir:  "/tmp",
		Status:      session.ShellStatusStopped,
		OrderIndex:  0,
		StartedAt:   time.Now(),
	}
}

// makeRunningShell returns a Shell in the Running state.
// watcherDone is left as nil — callers that need DeleteShell to drain
// must close it or use makeStoppedShell.
func makeRunningShell(id, name string) *session.Shell {
	return &session.Shell{
		ID:         id,
		Name:       name,
		Command:    "/bin/sh",
		WorkingDir: "/tmp",
		Status:     session.ShellStatusRunning,
		OrderIndex: 0,
		StartedAt:  time.Now(),
	}
}

// shellsFixture extends forkTestFixture with a convenience method for adding a
// session to both storage (so resolveSessionTitle finds it) and to the poller
// (so FindLiveInstance returns the live instance).
type shellsFixture struct {
	*forkTestFixture
}

func setupShellsFixture(t *testing.T) *shellsFixture {
	t.Helper()
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)
	return &shellsFixture{fix}
}

// addLiveSession creates a paused session in storage (so ListInstanceData finds it)
// and returns the loaded Instance registered with the poller.
func (f *shellsFixture) addLiveSession(t *testing.T, title string) *session.Instance {
	t.Helper()
	addPausedSession(t, f.forkTestFixture, title)
	// addPausedSession already calls addInstanceToPoller with the loaded instance.
	// Retrieve it from the poller so the caller can manipulate it.
	inst := f.svc.FindLiveInstance(title)
	require.NotNilf(t, inst, "addLiveSession: poller must hold %q after addPausedSession", title)
	return inst
}

// addStorageOnlySession creates a session in storage but does NOT add it to the poller,
// simulating a session that exists in the DB but has no live running instance.
func (f *shellsFixture) addStorageOnlySession(t *testing.T, title string) {
	t.Helper()
	inst := &session.Instance{
		Title:     title,
		Path:      "/tmp/test",
		Status:    session.Paused,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := f.storage.AddInstance(inst)
	require.NoErrorf(t, err, "addStorageOnlySession: failed to persist %q", title)
}

// --------------------------------------------------------------------------
// SpawnShell tests
// --------------------------------------------------------------------------

func TestSpawnShell_EmptySessionId(t *testing.T) {
	fix := setupShellsFixture(t)

	_, err := fix.svc.SpawnShell(context.Background(), connect.NewRequest(&sessionv1.SpawnShellRequest{
		SessionId: "",
		Command:   "/bin/sh",
	}))

	assertConnectCode(t, err, connect.CodeInvalidArgument)
}

func TestSpawnShell_SessionNotFound(t *testing.T) {
	fix := setupShellsFixture(t)

	_, err := fix.svc.SpawnShell(context.Background(), connect.NewRequest(&sessionv1.SpawnShellRequest{
		SessionId: "no-such-session",
		Command:   "/bin/sh",
	}))

	assertConnectCode(t, err, connect.CodeNotFound)
}

func TestSpawnShell_SessionNotRunning(t *testing.T) {
	fix := setupShellsFixture(t)
	// Session exists in DB but is not registered with the poller (no live instance).
	fix.addStorageOnlySession(t, "paused-session")

	_, err := fix.svc.SpawnShell(context.Background(), connect.NewRequest(&sessionv1.SpawnShellRequest{
		SessionId: "paused-session",
		Command:   "/bin/sh",
	}))

	assertConnectCode(t, err, connect.CodeFailedPrecondition)
}

// --------------------------------------------------------------------------
// StopShell tests
// --------------------------------------------------------------------------

func TestStopShell_SessionNotFound(t *testing.T) {
	fix := setupShellsFixture(t)

	_, err := fix.svc.StopShell(context.Background(), connect.NewRequest(&sessionv1.StopShellRequest{
		SessionId: "no-such-session",
		ShellId:   "shell-1",
	}))

	assertConnectCode(t, err, connect.CodeNotFound)
}

func TestStopShell_Success(t *testing.T) {
	fix := setupShellsFixture(t)
	inst := fix.addLiveSession(t, "stop-test-session")

	const shellID = "shell-stop-001"
	inst.AddShellInMemory(makeRunningShell(shellID, "bash"))

	resp, err := fix.svc.StopShell(context.Background(), connect.NewRequest(&sessionv1.StopShellRequest{
		SessionId: "stop-test-session",
		ShellId:   shellID,
	}))

	require.NoError(t, err)
	require.NotNil(t, resp.Msg)
	assert.True(t, resp.Msg.Success)
	assert.Contains(t, resp.Msg.Message, shellID)
}

// --------------------------------------------------------------------------
// RestartShell tests
// --------------------------------------------------------------------------

func TestRestartShell_SessionNotFound(t *testing.T) {
	fix := setupShellsFixture(t)

	_, err := fix.svc.RestartShell(context.Background(), connect.NewRequest(&sessionv1.RestartShellRequest{
		SessionId: "no-such-session",
		ShellId:   "shell-1",
	}))

	assertConnectCode(t, err, connect.CodeNotFound)
}

// TestRestartShell_ShellNotFound verifies that RestartShell returns CodeInternal when
// the shell ID is not found in the live instance (the Instance method returns an error
// which the handler wraps as CodeInternal).
func TestRestartShell_ShellNotFound(t *testing.T) {
	fix := setupShellsFixture(t)
	fix.addLiveSession(t, "restart-test-session")

	_, err := fix.svc.RestartShell(context.Background(), connect.NewRequest(&sessionv1.RestartShellRequest{
		SessionId: "restart-test-session",
		ShellId:   "no-such-shell",
	}))

	// Instance.RestartShell returns an error for unknown shell IDs, wrapped as CodeInternal.
	assertConnectCode(t, err, connect.CodeInternal)
}

// --------------------------------------------------------------------------
// ListShells tests
// --------------------------------------------------------------------------

func TestListShells_EmptySession(t *testing.T) {
	fix := setupShellsFixture(t)
	fix.addLiveSession(t, "list-empty-session")

	resp, err := fix.svc.ListShells(context.Background(), connect.NewRequest(&sessionv1.ListShellsRequest{
		SessionId: "list-empty-session",
	}))

	require.NoError(t, err)
	require.NotNil(t, resp.Msg)
	assert.Empty(t, resp.Msg.Shells, "expected no shells for a freshly created session")
}

func TestListShells_ReturnsAll(t *testing.T) {
	fix := setupShellsFixture(t)
	inst := fix.addLiveSession(t, "list-populated-session")

	inst.AddShellInMemory(makeStoppedShell("shell-a", "alpha"))
	inst.AddShellInMemory(makeStoppedShell("shell-b", "beta"))

	resp, err := fix.svc.ListShells(context.Background(), connect.NewRequest(&sessionv1.ListShellsRequest{
		SessionId: "list-populated-session",
	}))

	require.NoError(t, err)
	require.NotNil(t, resp.Msg)
	assert.Len(t, resp.Msg.Shells, 2, "expected exactly 2 shells")

	ids := make(map[string]bool, 2)
	for _, sh := range resp.Msg.Shells {
		ids[sh.Id] = true
	}
	assert.True(t, ids["shell-a"], "shell-a must be in response")
	assert.True(t, ids["shell-b"], "shell-b must be in response")
}

// TestListShells_SessionNotRunning verifies that ListShells returns an empty list
// (not an error) when the session exists in storage but has no live instance.
// The handler intentionally returns [] rather than an error to support frontend
// polling during session startup.
func TestListShells_SessionNotRunning(t *testing.T) {
	fix := setupShellsFixture(t)
	fix.addStorageOnlySession(t, "list-not-running")

	resp, err := fix.svc.ListShells(context.Background(), connect.NewRequest(&sessionv1.ListShellsRequest{
		SessionId: "list-not-running",
	}))

	require.NoError(t, err)
	require.NotNil(t, resp.Msg)
	assert.Empty(t, resp.Msg.Shells)
}

// --------------------------------------------------------------------------
// DeleteShell tests
// --------------------------------------------------------------------------

func TestDeleteShell_SessionNotFound(t *testing.T) {
	fix := setupShellsFixture(t)

	_, err := fix.svc.DeleteShell(context.Background(), connect.NewRequest(&sessionv1.DeleteShellRequest{
		SessionId: "no-such-session",
		ShellId:   "shell-1",
	}))

	assertConnectCode(t, err, connect.CodeNotFound)
}

func TestDeleteShell_Success(t *testing.T) {
	fix := setupShellsFixture(t)
	inst := fix.addLiveSession(t, "delete-test-session")

	const shellID = "shell-delete-001"
	// Use a stopped shell. DeleteShell calls StopShell (no-op on already-stopped shells)
	// and then checks sh.watcherDone — a nil watcherDone is safe (the nil guard is in
	// DeleteShell itself). No real tmux interaction occurs.
	inst.AddShellInMemory(&session.Shell{
		ID:         shellID,
		Name:       "bash",
		Command:    "/bin/sh",
		WorkingDir: "/tmp",
		Status:     session.ShellStatusStopped,
		OrderIndex: 0,
		StartedAt:  time.Now(),
	})

	resp, err := fix.svc.DeleteShell(context.Background(), connect.NewRequest(&sessionv1.DeleteShellRequest{
		SessionId: "delete-test-session",
		ShellId:   shellID,
	}))

	require.NoError(t, err)
	require.NotNil(t, resp.Msg)
	assert.True(t, resp.Msg.Success)
	assert.Contains(t, resp.Msg.Message, shellID)

	// Verify the shell is no longer returned by ListShells.
	listResp, listErr := fix.svc.ListShells(context.Background(), connect.NewRequest(&sessionv1.ListShellsRequest{
		SessionId: "delete-test-session",
	}))
	require.NoError(t, listErr)
	assert.Empty(t, listResp.Msg.Shells, "shell must be removed from memory after DeleteShell")
}
