// Package services contains integration tests for the workspace service.
package services

import (
	"context"
	"fmt"
	"os"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/vc"
)

// stubLiveFinder is a test double for LiveInstanceFinder that returns a fixed instance.
type stubLiveFinder struct {
	inst *session.Instance
}

func (s *stubLiveFinder) FindLiveInstance(_ string) *session.Instance {
	return s.inst
}

// workspaceTestFixture holds the WorkspaceService and its dependencies for a
// single test. The storage is backed by a real SQLite database in a temp dir.
type workspaceTestFixture struct {
	svc     *WorkspaceService
	bus     *events.EventBus
	storage *session.Storage
	cleanup func()
}

func setupWorkspaceTestFixture(t *testing.T) *workspaceTestFixture {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "workspace-svc-test-*")
	require.NoError(t, err)

	dbPath := fmt.Sprintf("%s/sessions.db", tmpDir)
	repo, err := session.NewEntRepository(session.WithDatabasePath(dbPath))
	require.NoError(t, err)

	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)

	bus := events.NewEventBus(16)
	svc := NewWorkspaceService(storage, bus)

	cleanup := func() {
		bus.Close()
		if err := repo.Close(); err != nil {
			t.Logf("cleanup: repo.Close: %v", err)
		}
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Logf("cleanup: os.RemoveAll: %v", err)
		}
	}

	return &workspaceTestFixture{
		svc:     svc,
		bus:     bus,
		storage: storage,
		cleanup: cleanup,
	}
}

// seedInstance creates a minimal Instance with the given title and persists it
// via AddInstance so that WorkspaceService.findInstance can resolve it.
//
// Status is set to Paused to avoid triggering tmux session start in LoadInstances;
// Running status causes FromInstanceData to call Start(), which spawns PTY
// subprocesses and times out in environments without a real tmux server.
func seedInstance(t *testing.T, storage *session.Storage, title string) {
	t.Helper()
	inst := &session.Instance{
		Title:   title,
		Path:    "/tmp/test-workspace",
		Status:  session.Stopped, // Stopped avoids tmux Start() in FromInstanceData
		Program: "claude",
	}
	require.NoError(t, storage.AddInstance(inst))
}

// --------------------------------------------------------------------------
// Input validation tests
// --------------------------------------------------------------------------

func TestWorkspaceService_SwitchWorkspace_MissingID(t *testing.T) {
	t.Parallel()
	fix := setupWorkspaceTestFixture(t)
	t.Cleanup(fix.cleanup)

	_, err := fix.svc.SwitchWorkspace(context.Background(), connect.NewRequest(&sessionv1.SwitchWorkspaceRequest{
		Id:     "",
		Target: "main",
	}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

func TestWorkspaceService_SwitchWorkspace_MissingTarget(t *testing.T) {
	t.Parallel()
	fix := setupWorkspaceTestFixture(t)
	t.Cleanup(fix.cleanup)

	_, err := fix.svc.SwitchWorkspace(context.Background(), connect.NewRequest(&sessionv1.SwitchWorkspaceRequest{
		Id:     "some-session",
		Target: "",
	}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// --------------------------------------------------------------------------
// Concurrent switch guard tests
// --------------------------------------------------------------------------

// TestWorkspaceService_ConcurrentSwitchReturnsUnavailable verifies that a
// second SwitchWorkspace call for the same session ID while one is already
// in progress returns CodeUnavailable.
//
// The test simulates an in-progress switch by pre-loading the session ID into
// inFlightSwitches before issuing the RPC call, mirroring exactly what the handler
// does at the top of SwitchWorkspace.
func TestWorkspaceService_ConcurrentSwitchReturnsUnavailable(t *testing.T) {
	t.Parallel()
	fix := setupWorkspaceTestFixture(t)
	t.Cleanup(fix.cleanup)

	sessionID := "concurrent-switch-session"
	seedInstance(t, fix.storage, sessionID)

	// Simulate an in-progress switch by pre-storing the session ID in the guard
	// map, exactly as the handler does with LoadOrStore.
	fix.svc.inFlightSwitches.Store(sessionID, true)

	_, err := fix.svc.SwitchWorkspace(context.Background(), connect.NewRequest(&sessionv1.SwitchWorkspaceRequest{
		Id:     sessionID,
		Target: "main",
	}))

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeUnavailable, connectErr.Code())
	assert.Contains(t, connectErr.Message(), sessionID,
		"error message should mention the session ID")
	assert.Contains(t, connectErr.Message(), "already in progress",
		"error message should indicate the switch is already in progress")
}

// TestWorkspaceService_SwitchGuardCleansUpOnCompletion verifies that the
// concurrent switch guard key is removed from inFlightSwitches after SwitchWorkspace
// returns, regardless of whether the call succeeds or fails. The test exercises
// the failure path (session not found) because spinning up a real VCS workspace
// is not required to verify the guard lifecycle.
//
// After the call completes, a second call must NOT receive CodeUnavailable —
// demonstrating that the defer ws.inFlightSwitches.Delete(req.Msg.Id) fired.
func TestWorkspaceService_SwitchGuardCleansUpOnCompletion(t *testing.T) {
	t.Parallel()
	fix := setupWorkspaceTestFixture(t)
	t.Cleanup(fix.cleanup)

	sessionID := "cleanup-guard-session"
	// Do NOT seed the session so the first call fails with CodeNotFound after
	// passing the guard check. The guard key must still be cleaned up.

	// First call: passes the guard (no pre-existing entry), fails with
	// CodeNotFound because the session does not exist in storage.
	_, firstErr := fix.svc.SwitchWorkspace(context.Background(), connect.NewRequest(&sessionv1.SwitchWorkspaceRequest{
		Id:     sessionID,
		Target: "main",
	}))
	require.Error(t, firstErr)
	var firstConnectErr *connect.Error
	require.ErrorAs(t, firstErr, &firstConnectErr)
	assert.Equal(t, connect.CodeNotFound, firstConnectErr.Code(),
		"first call should fail with CodeNotFound (session not in storage), not CodeUnavailable")

	// Verify the guard key is gone after the call returned.
	_, stillLocked := fix.svc.inFlightSwitches.Load(sessionID)
	assert.False(t, stillLocked,
		"inFlightSwitches entry must be deleted after SwitchWorkspace returns")

	// Second call must also fail with CodeNotFound — NOT CodeUnavailable — which
	// proves the guard was cleaned up and does not block subsequent requests.
	_, secondErr := fix.svc.SwitchWorkspace(context.Background(), connect.NewRequest(&sessionv1.SwitchWorkspaceRequest{
		Id:     sessionID,
		Target: "main",
	}))
	require.Error(t, secondErr)
	var secondConnectErr *connect.Error
	require.ErrorAs(t, secondErr, &secondConnectErr)
	assert.Equal(t, connect.CodeNotFound, secondConnectErr.Code(),
		"second call should still fail with CodeNotFound, not CodeUnavailable — guard was cleaned up")
}

// TestWorkspaceService_SwitchGuardIsPerSession verifies that concurrent switch
// guards are session-scoped: locking session A must not block a concurrent call
// for session B.
//
// Note: SwitchWorkspace wraps VCS-level failures into the response body (not
// a connect error), so a call that passes the guard returns nil error even when
// the underlying workspace operation fails. The key assertion is that the call
// for session B is NOT rejected with CodeUnavailable.
func TestWorkspaceService_SwitchGuardIsPerSession(t *testing.T) {
	t.Parallel()
	fix := setupWorkspaceTestFixture(t)
	t.Cleanup(fix.cleanup)

	sessionA := "guard-session-a"
	sessionB := "guard-session-b"

	seedInstance(t, fix.storage, sessionB)

	// Simulate an in-progress switch for session A.
	fix.svc.inFlightSwitches.Store(sessionA, true)

	// A call for session B must NOT be blocked by session A's lock.
	// The call may succeed (nil error with response body) or fail with a
	// non-Unavailable code — but never with CodeUnavailable.
	_, err := fix.svc.SwitchWorkspace(context.Background(), connect.NewRequest(&sessionv1.SwitchWorkspaceRequest{
		Id:     sessionB,
		Target: "main",
	}))

	if err != nil {
		var connectErr *connect.Error
		require.ErrorAs(t, err, &connectErr)
		assert.NotEqual(t, connect.CodeUnavailable, connectErr.Code(),
			"lock on session A must not block calls for session B")
	}
	// err == nil also means the guard did not trigger, which is the desired
	// outcome: session B proceeded past the guard independently.
}

// --------------------------------------------------------------------------
// UUID ID lookup tests (regression: frontend now passes UUIDs, not Titles)
// --------------------------------------------------------------------------

// TestWorkspaceService_GetVCSStatus_FindsByUUID verifies that GetVCSStatus
// accepts a session UUID as the ID parameter. Before the UUID migration, only
// the Title was accepted; now MatchesID checks both UUID and Title.
func TestWorkspaceService_GetVCSStatus_FindsByUUID(t *testing.T) {
	t.Parallel()
	fix := setupWorkspaceTestFixture(t)
	t.Cleanup(fix.cleanup)

	const testUUID = "11111111-1111-1111-1111-111111111111"
	require.NoError(t, fix.storage.AddInstance(&session.Instance{
		UUID:    testUUID,
		Title:   "my-uuid-session",
		Path:    "/tmp/test-workspace",
		Status:  session.Paused, // Paused avoids tmux setup in LoadInstances (no CI tmux)
		Program: "claude",
	}))

	_, err := fix.svc.GetVCSStatus(context.Background(), connect.NewRequest(&sessionv1.GetVCSStatusRequest{
		Id: testUUID,
	}))

	// Must NOT return CodeNotFound — UUID lookup must succeed.
	// /tmp/test-workspace is not a VCS dir, so the response body may carry
	// an error string, but the RPC itself returns nil.
	require.NoError(t, err, "GetVCSStatus should find the session by UUID")
}

// TestWorkspaceService_GetVCSStatus_FindsByTitle verifies that the legacy
// Title-based lookup still works after the UUID migration.
func TestWorkspaceService_GetVCSStatus_FindsByTitle(t *testing.T) {
	t.Parallel()
	fix := setupWorkspaceTestFixture(t)
	t.Cleanup(fix.cleanup)

	require.NoError(t, fix.storage.AddInstance(&session.Instance{
		Title:   "title-only-session",
		Path:    "/tmp/test-workspace",
		Status:  session.Paused, // Paused avoids tmux setup in LoadInstances (no CI tmux)
		Program: "claude",
	}))

	_, err := fix.svc.GetVCSStatus(context.Background(), connect.NewRequest(&sessionv1.GetVCSStatusRequest{
		Id: "title-only-session",
	}))

	require.NoError(t, err, "GetVCSStatus should still find the session by Title")
}

// TestWorkspaceService_GetVCSStatus_UnknownIDReturnsNotFound verifies that an
// ID that matches neither UUID nor Title of any session returns CodeNotFound.
func TestWorkspaceService_GetVCSStatus_UnknownIDReturnsNotFound(t *testing.T) {
	t.Parallel()
	fix := setupWorkspaceTestFixture(t)
	t.Cleanup(fix.cleanup)

	_, err := fix.svc.GetVCSStatus(context.Background(), connect.NewRequest(&sessionv1.GetVCSStatusRequest{
		Id: "no-such-session",
	}))

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}

// --------------------------------------------------------------------------
// LiveInstanceFinder fast-path tests
// --------------------------------------------------------------------------

// TestWorkspaceService_FindInstanceFast_LiveFinderHit_BypassesStorage verifies
// that when SetLiveFinder is wired and FindLiveInstance returns an instance,
// that instance is used directly without consulting storage.
func TestWorkspaceService_FindInstanceFast_LiveFinderHit_BypassesStorage(t *testing.T) {
	t.Parallel()
	fix := setupWorkspaceTestFixture(t)
	t.Cleanup(fix.cleanup)

	liveInst := &session.Instance{
		Title:   "live-session",
		Path:    "/tmp/live-workspace",
		Status:  session.Active,
		Program: "claude",
	}
	fix.svc.SetLiveFinder(&stubLiveFinder{inst: liveInst})

	// Session is NOT in storage — only the live finder can resolve it.
	_, err := fix.svc.GetVCSStatus(context.Background(), connect.NewRequest(&sessionv1.GetVCSStatusRequest{
		Id: "live-session",
	}))

	// GetVCSStatus will fail (no real VCS dir), but it must NOT be CodeNotFound —
	// that would mean the session wasn't resolved at all.
	require.NoError(t, err, "live instance with no working dir should return a response with an error field, not a gRPC error")
}

// TestWorkspaceService_FindInstanceFast_LiveFinderMiss_FallsBackToStorage verifies
// that when the live finder returns nil, findInstanceFast falls back to storage.
func TestWorkspaceService_FindInstanceFast_LiveFinderMiss_FallsBackToStorage(t *testing.T) {
	t.Parallel()
	fix := setupWorkspaceTestFixture(t)
	t.Cleanup(fix.cleanup)

	// Live finder always misses.
	fix.svc.SetLiveFinder(&stubLiveFinder{inst: nil})

	// Session is in storage.
	seedInstance(t, fix.storage, "storage-session")

	_, err := fix.svc.GetVCSStatus(context.Background(), connect.NewRequest(&sessionv1.GetVCSStatusRequest{
		Id: "storage-session",
	}))

	// Same logic: a missing working dir produces a response with an error field,
	// but the session must be found (no CodeNotFound).
	require.NoError(t, err, "session found via storage fallback should not return a gRPC error")
}

// --------------------------------------------------------------------------
// fileChangeToProto: Additions/Deletions threading (numstat feature)
// --------------------------------------------------------------------------

// TestFileChangeToProto verifies that fileChangeToProto correctly threads
// every vc.FileChange field — including the Additions/Deletions numstat
// counts added alongside per-file insertion/deletion stats — through to the
// sessionv1.FileChange proto.
func TestFileChangeToProto(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   vc.FileChange
		want *sessionv1.FileChange
	}{
		{
			name: "modified file threads additions and deletions",
			in: vc.FileChange{
				Path:      "main.go",
				Status:    vc.FileModified,
				IsStaged:  false,
				Additions: 12,
				Deletions: 3,
			},
			want: &sessionv1.FileChange{
				Path:      "main.go",
				Status:    sessionv1.FileStatus_FILE_STATUS_MODIFIED,
				IsStaged:  false,
				OldPath:   "",
				Additions: 12,
				Deletions: 3,
			},
		},
		{
			name: "staged rename threads old path and stats",
			in: vc.FileChange{
				Path:      "renamed.txt",
				OldPath:   "old.txt",
				Status:    vc.FileRenamed,
				IsStaged:  true,
				Additions: 5,
				Deletions: 1,
			},
			want: &sessionv1.FileChange{
				Path:      "renamed.txt",
				OldPath:   "old.txt",
				Status:    sessionv1.FileStatus_FILE_STATUS_RENAMED,
				IsStaged:  true,
				Additions: 5,
				Deletions: 1,
			},
		},
		{
			name: "untracked file has zero additions/deletions",
			in: vc.FileChange{
				Path:      "new.txt",
				Status:    vc.FileUntracked,
				IsStaged:  false,
				Additions: 0,
				Deletions: 0,
			},
			want: &sessionv1.FileChange{
				Path:      "new.txt",
				Status:    sessionv1.FileStatus_FILE_STATUS_UNTRACKED,
				IsStaged:  false,
				Additions: 0,
				Deletions: 0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := fileChangeToProto(tt.in)

			assert.Equal(t, tt.want.Path, got.Path)
			assert.Equal(t, tt.want.OldPath, got.OldPath)
			assert.Equal(t, tt.want.Status, got.Status)
			assert.Equal(t, tt.want.IsStaged, got.IsStaged)
			assert.Equal(t, tt.want.Additions, got.Additions)
			assert.Equal(t, tt.want.Deletions, got.Deletions)
		})
	}
}
