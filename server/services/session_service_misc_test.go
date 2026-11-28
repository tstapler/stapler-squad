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
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

// ---------------------------------------------------------------------------
// RestartSession
// ---------------------------------------------------------------------------

// TestRestartSession_EmptyID verifies that an empty session id returns
// CodeInvalidArgument without touching storage.
func TestRestartSession_EmptyID(t *testing.T) {
	t.Parallel()
	storage := svctest.CreateTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

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
	t.Parallel()
	storage := svctest.CreateTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

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
	t.Parallel()
	storage := svctest.CreateTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

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
	t.Parallel()
	storage := svctest.CreateTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

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
	t.Parallel()
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	inst := &instance.Instance{
		Title:     "clear-test-session",
		Path:      "/tmp/test",
		Status:    domain.Paused,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, fix.storage.AddInstance(inst))
	svctest.AddInstanceToPoller(fix.poller, inst)

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
	t.Parallel()
	storage := svctest.CreateTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

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
	t.Parallel()
	storage := svctest.CreateTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

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
	t.Parallel()
	storage := svctest.CreateTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

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
	t.Parallel()
	storage := svctest.CreateTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

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
	t.Parallel()
	storage := svctest.CreateTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	_, err := svc.GetTerminalSnapshot(context.Background(), connect.NewRequest(&sessionv1.GetTerminalSnapshotRequest{
		SessionId: "no-such-session",
	}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}
