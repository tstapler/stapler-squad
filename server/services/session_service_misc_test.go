package services

import (
	"context"
	"os"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/unfinished"
)

// ---------------------------------------------------------------------------
// RestartSession
// ---------------------------------------------------------------------------

// TestRestartSession_EmptyID verifies that an empty session id returns
// CodeInvalidArgument without touching storage.
func TestRestartSession_EmptyID(t *testing.T) {
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)

	_, err := svc.RestartSession(context.Background(), connect.NewRequest(&sessionv1.RestartSessionRequest{
		Id: "",
	}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestRestartSession_SessionNotFound verifies that a non-existent session id
// returns CodeNotFound.
func TestRestartSession_SessionNotFound(t *testing.T) {
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)

	_, err := svc.RestartSession(context.Background(), connect.NewRequest(&sessionv1.RestartSessionRequest{
		Id: "does-not-exist",
	}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}

// ---------------------------------------------------------------------------
// ClearConversationState
// ---------------------------------------------------------------------------

// TestClearConversationState_EmptyID verifies that an empty id returns
// CodeInvalidArgument.
func TestClearConversationState_EmptyID(t *testing.T) {
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)

	_, err := svc.ClearConversationState(context.Background(), connect.NewRequest(&sessionv1.ClearConversationStateRequest{
		Id: "",
	}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestClearConversationState_SessionNotFound verifies that a non-existent id
// returns CodeNotFound.
func TestClearConversationState_SessionNotFound(t *testing.T) {
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)

	_, err := svc.ClearConversationState(context.Background(), connect.NewRequest(&sessionv1.ClearConversationStateRequest{
		Id: "ghost-session",
	}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}

// TestClearConversationState_Success adds a Paused session, registers it with
// the ReviewQueuePoller (so FindLiveInstance resolves it), then calls
// ClearConversationState and expects success=true.
func TestClearConversationState_Success(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	inst := &session.Instance{
		Title:     "clear-test-session",
		Path:      "/tmp/test",
		Status:    session.Paused,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, fix.storage.AddInstance(inst))
	addInstanceToPoller(fix.poller, inst)

	resp, err := fix.svc.ClearConversationState(context.Background(), connect.NewRequest(&sessionv1.ClearConversationStateRequest{
		Id: inst.Title,
	}))
	require.NoError(t, err)
	assert.True(t, resp.Msg.Success)
}

// ---------------------------------------------------------------------------
// ListBranches
// ---------------------------------------------------------------------------

// TestListBranches_EmptyPath verifies that an empty repo_path returns
// CodeInvalidArgument.
func TestListBranches_EmptyPath(t *testing.T) {
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)

	_, err := svc.ListBranches(context.Background(), connect.NewRequest(&sessionv1.ListBranchesRequest{
		RepoPath: "",
	}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestListBranches_PathOutsideHome verifies that a path outside the user's
// home directory is rejected with CodeInvalidArgument.
func TestListBranches_PathOutsideHome(t *testing.T) {
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)

	// /proc exists on Linux but is never inside the user home directory.
	_, err := svc.ListBranches(context.Background(), connect.NewRequest(&sessionv1.ListBranchesRequest{
		RepoPath: "/proc",
	}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestListBranches_NonExistentPath verifies that a path that does not exist
// (but is within the home tree) returns CodeInvalidArgument.
func TestListBranches_NonExistentPath(t *testing.T) {
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)

	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)

	nonExistent := homeDir + "/this-path-does-not-exist-12345678"
	_, listErr := svc.ListBranches(context.Background(), connect.NewRequest(&sessionv1.ListBranchesRequest{
		RepoPath: nonExistent,
	}))
	require.Error(t, listErr)
	var connectErr *connect.Error
	require.ErrorAs(t, listErr, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// ---------------------------------------------------------------------------
// GetTerminalSnapshot
// ---------------------------------------------------------------------------

// TestGetTerminalSnapshot_EmptyID verifies that an empty session_id returns
// CodeInvalidArgument.
func TestGetTerminalSnapshot_EmptyID(t *testing.T) {
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)

	_, err := svc.GetTerminalSnapshot(context.Background(), connect.NewRequest(&sessionv1.GetTerminalSnapshotRequest{
		SessionId: "",
	}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestGetTerminalSnapshot_SessionNotFound verifies that a non-existent session
// id returns CodeNotFound (both poller and externalDiscovery are nil).
func TestGetTerminalSnapshot_SessionNotFound(t *testing.T) {
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)

	_, err := svc.GetTerminalSnapshot(context.Background(), connect.NewRequest(&sessionv1.GetTerminalSnapshotRequest{
		SessionId: "no-such-session",
	}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}

// ---------------------------------------------------------------------------
// GetWorktreeDiff (UnfinishedWorkService)
// ---------------------------------------------------------------------------

// newMinimalUnfinishedWorkService constructs an UnfinishedWorkService with a
// real (but empty) Scanner backed by a temp-dir StateStore. No background
// scan goroutines are started, so it is safe to use in unit tests.
func newMinimalUnfinishedWorkService(t *testing.T) *UnfinishedWorkService {
	t.Helper()
	tmpDir := t.TempDir()
	stateStore, err := unfinished.NewStateStore(tmpDir + "/state.json")
	require.NoError(t, err)
	bus := events.NewEventBus(16)
	scanner := unfinished.NewScanner(bus, stateStore)
	return NewUnfinishedWorkService(scanner, stateStore, bus, nil)
}

// TestGetWorktreeDiff_EmptyRepoPath verifies that missing repo_path or branch
// yields CodeNotFound (the scanner returns !ok, which the handler maps to
// CodeNotFound — the handler has no explicit empty-field validation).
func TestGetWorktreeDiff_EmptyRepoPath(t *testing.T) {
	svc := newMinimalUnfinishedWorkService(t)

	_, err := svc.GetWorktreeDiff(context.Background(), connect.NewRequest(&sessionv1.GetWorktreeDiffRequest{
		RepoPath: "",
		Branch:   "",
	}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}

// TestGetWorktreeDiff_SessionNotFound verifies that a repo+branch pair that
// has not been scanned returns CodeNotFound.
func TestGetWorktreeDiff_SessionNotFound(t *testing.T) {
	svc := newMinimalUnfinishedWorkService(t)

	_, err := svc.GetWorktreeDiff(context.Background(), connect.NewRequest(&sessionv1.GetWorktreeDiffRequest{
		RepoPath: "/nonexistent/repo",
		Branch:   "main",
	}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}
