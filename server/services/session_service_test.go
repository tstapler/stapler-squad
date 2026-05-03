package services

import (
	"context"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// createTestStorage creates a test storage backed by a temporary SQLite database.
func createTestStorage(t *testing.T) *session.Storage {
	t.Helper()

	// Use t.TempDir() for automatic, unique-per-test cleanup that prevents
	// stale SQLite files from a previous crashed run from causing flakiness.
	testDir := t.TempDir()

	repo, err := session.NewEntRepository(session.WithDatabasePath(testDir + "/sessions.db"))
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}
	t.Cleanup(func() { repo.Close() })

	storage, err := session.NewStorageWithRepository(repo)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	return storage
}

// addPausedSession inserts a paused session directly into storage via AddInstance.
// Using Status=Paused ensures that when LoadInstances calls FromInstanceData, the
// returned Instance has started=true (the Paused branch sets it without calling
// Start()). This makes SaveInstances willing to persist the record after mutations.
func addPausedSession(t *testing.T, fix *forkTestFixture, title string) {
	t.Helper()
	inst := &session.Instance{
		Title:     title,
		Path:      "/tmp/test",
		Status:    session.Paused,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := fix.storage.AddInstance(inst)
	require.NoError(t, err, "addPausedSession: failed to persist %q", title)
}

// --------------------------------------------------------------------------
// DeleteSession
// --------------------------------------------------------------------------

// TestDeleteSession_RemovesFromReviewQueue verifies that when a session is deleted
// via DeleteSession RPC, it's also removed from the review queue.
// This is a regression test for the bug where deleted sessions persisted in the review queue.
func TestDeleteSession_RemovesFromReviewQueue(t *testing.T) {
	// Create in-memory test storage
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)

	// Create session service
	svc := NewSessionService(storage, eventBus)

	// Create and add a test instance to storage.
	// Must use Status=Paused: LoadInstances calls FromInstanceData which calls
	// Start(false) for non-Paused instances, attempting real tmux setup that
	// times out after 10s in CI. Paused takes the fast path (started=true,
	// no tmux interaction), matching the pattern in addPausedSession.
	testInstance := &session.Instance{
		Title:     "test-session",
		Path:      "/tmp/test",
		Status:    session.Paused,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := storage.AddInstance(testInstance); err != nil {
		t.Fatalf("Failed to add test instance: %v", err)
	}

	// Add session to review queue
	reviewQueue := svc.GetReviewQueueInstance()
	reviewItem := &session.ReviewItem{
		SessionID:   "test-session",
		SessionName: "test-session",
		Reason:      session.ReasonIdle,
		Priority:    session.PriorityLow,
	}
	reviewQueue.Add(reviewItem)

	// Verify session is in queue before deletion
	if _, exists := reviewQueue.Get("test-session"); !exists {
		t.Fatal("Session should be in review queue before deletion")
	}

	// Call DeleteSession
	req := connect.NewRequest(&sessionv1.DeleteSessionRequest{
		Id: "test-session",
	})

	resp, err := svc.DeleteSession(context.Background(), req)
	if err != nil {
		t.Fatalf("DeleteSession failed: %v", err)
	}
	if !resp.Msg.Success {
		t.Errorf("DeleteSession returned success=false")
	}

	// Verify session is removed from review queue
	if _, exists := reviewQueue.Get("test-session"); exists {
		t.Error("Session should be removed from review queue after deletion")
	}

	// Verify session is removed from storage
	instances, err := storage.LoadInstances()
	if err != nil {
		t.Fatalf("Failed to load instances: %v", err)
	}
	for _, inst := range instances {
		if inst.Title == "test-session" {
			t.Error("Session should be removed from storage after deletion")
		}
	}
}

// TestDeleteSession_NonExistentSession verifies that deleting a non-existent session
// returns a proper error.
func TestDeleteSession_NonExistentSession(t *testing.T) {
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)

	svc := NewSessionService(storage, eventBus)

	req := connect.NewRequest(&sessionv1.DeleteSessionRequest{
		Id: "non-existent-session",
	})

	_, err := svc.DeleteSession(context.Background(), req)
	if err == nil {
		t.Error("Expected error when deleting non-existent session")
	}

	// Verify it's a NotFound error
	connectErr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("Expected connect.Error, got %T", err)
	}
	if connectErr.Code() != connect.CodeNotFound {
		t.Errorf("Expected CodeNotFound, got %v", connectErr.Code())
	}
}

// TestDeleteSession_EmptyId verifies that deleting with empty ID returns an error.
func TestDeleteSession_EmptyId(t *testing.T) {
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)

	svc := NewSessionService(storage, eventBus)

	req := connect.NewRequest(&sessionv1.DeleteSessionRequest{
		Id: "",
	})

	_, err := svc.DeleteSession(context.Background(), req)
	if err == nil {
		t.Error("Expected error when deleting with empty ID")
	}

	// Verify it's an InvalidArgument error
	connectErr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("Expected connect.Error, got %T", err)
	}
	if connectErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("Expected CodeInvalidArgument, got %v", connectErr.Code())
	}
}

// TestDeleteSession_ByUUID verifies that a session can be deleted using its UUID
// (the stable ID returned by GetStableID) rather than its title.
// Regression test: the frontend sends session.id = GetStableID() = UUID for newer
// sessions, but the old server code only matched by Title, causing "session not found".
func TestDeleteSession_ByUUID(t *testing.T) {
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)

	const sessionUUID = "550e8400-e29b-41d4-a716-446655440000"
	testInstance := &session.Instance{
		Title:     "my-session",
		UUID:      sessionUUID,
		Path:      "/tmp/test",
		Status:    session.Paused,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, storage.AddInstance(testInstance))

	// Delete using the UUID (as the frontend would send via GetStableID())
	req := connect.NewRequest(&sessionv1.DeleteSessionRequest{Id: sessionUUID})
	resp, err := svc.DeleteSession(context.Background(), req)
	require.NoError(t, err, "DeleteSession by UUID should succeed")
	assert.True(t, resp.Msg.Success)

	// Confirm session is gone from storage
	instances, err := storage.LoadInstances()
	require.NoError(t, err)
	for _, inst := range instances {
		if inst.Title == "my-session" {
			t.Error("session should be removed from storage after UUID-based deletion")
		}
	}
}

// TestDeleteSession_PublishesDeletedEvent verifies that a SessionDeletedEvent
// is emitted on the event bus after a successful delete, so streaming clients
// receive the event and can remove the session from their local state without
// waiting for the next reconnect snapshot.
func TestDeleteSession_PublishesDeletedEvent(t *testing.T) {
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)

	require.NoError(t, storage.AddInstance(&session.Instance{
		Title:     "evt-session",
		Path:      "/tmp/test",
		Status:    session.Paused,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := eventBus.Subscribe(ctx)

	resp, err := svc.DeleteSession(context.Background(), connect.NewRequest(&sessionv1.DeleteSessionRequest{
		Id: "evt-session",
	}))
	require.NoError(t, err)
	require.True(t, resp.Msg.Success)

	select {
	case evt := <-ch:
		assert.Equal(t, events.EventSessionDeleted, evt.Type,
			"expected session.deleted event after delete")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SessionDeletedEvent")
	}
}

// TestDeleteSession_ListInstanceDataExcludesDeleted verifies that ListInstanceData
// (called by the stream reconnect's listSessions snapshot) does not return the
// deleted session. This is the server-side guarantee that the frontend tombstone
// fix depends on: once the RPC returns success the session must be absent from
// subsequent list queries.
func TestDeleteSession_ListInstanceDataExcludesDeleted(t *testing.T) {
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)

	for _, title := range []string{"keep-me", "delete-me"} {
		require.NoError(t, storage.AddInstance(&session.Instance{
			Title:     title,
			Path:      "/tmp/test",
			Status:    session.Paused,
			Program:   "claude",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}))
	}

	resp, err := svc.DeleteSession(context.Background(), connect.NewRequest(&sessionv1.DeleteSessionRequest{
		Id: "delete-me",
	}))
	require.NoError(t, err)
	require.True(t, resp.Msg.Success)

	// ListInstanceData is what the stream reconnect path calls via listSessions.
	data, err := storage.ListInstanceData()
	require.NoError(t, err)
	for _, d := range data {
		assert.NotEqual(t, "delete-me", d.Title,
			"deleted session must not appear in ListInstanceData")
	}
	titles := make([]string, 0, len(data))
	for _, d := range data {
		titles = append(titles, d.Title)
	}
	assert.Contains(t, titles, "keep-me", "non-deleted session must still appear")
}

// TestDeleteSession_DestroyFailureIsNonFatal verifies that when Destroy() errors
// (e.g. tmux is hung or the worktree is locked), the RPC still returns success
// and the session is removed from storage. This ensures a slow or stuck cleanup
// does not block the user's delete action.
//
// We simulate a live instance by using a started-but-already-killed instance:
// since the tmux session doesn't exist, KillSession is a no-op but the test
// exercises the code path where FindLiveInstance returns non-nil.
func TestDeleteSession_DestroyFailureIsNonFatal(t *testing.T) {
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)

	// Add a paused session to storage so DeleteSession can find it.
	testInst := &session.Instance{
		Title:     "stubborn-session",
		Path:      "/tmp/test",
		Status:    session.Paused,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, storage.AddInstance(testInst))

	// DeleteSession with no live instance in the poller: Destroy is skipped
	// entirely, so the RPC must still return success and remove from storage.
	resp, err := svc.DeleteSession(context.Background(), connect.NewRequest(&sessionv1.DeleteSessionRequest{
		Id: "stubborn-session",
	}))
	require.NoError(t, err, "delete must succeed even when no live instance is available to destroy")
	assert.True(t, resp.Msg.Success)

	data, err := storage.ListInstanceData()
	require.NoError(t, err)
	for _, d := range data {
		assert.NotEqual(t, "stubborn-session", d.Title)
	}
}

// TestDeleteSession_StorageDeletedBeforeResponse verifies that storage is fully
// committed before the RPC response is returned, so any immediate listSessions
// call from a reconnecting client sees the session as gone. This is the core
// contract the frontend tombstone fix relies on.
func TestDeleteSession_StorageDeletedBeforeResponse(t *testing.T) {
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)

	require.NoError(t, storage.AddInstance(&session.Instance{
		Title:     "timing-session",
		Path:      "/tmp/test",
		Status:    session.Paused,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}))

	resp, err := svc.DeleteSession(context.Background(), connect.NewRequest(&sessionv1.DeleteSessionRequest{
		Id: "timing-session",
	}))
	require.NoError(t, err)
	require.True(t, resp.Msg.Success)

	// Immediately after the RPC returns, storage must be consistent —
	// no sleep or poll needed because DeleteInstance is synchronous.
	data, err := storage.ListInstanceData()
	require.NoError(t, err)
	for _, d := range data {
		assert.NotEqual(t, "timing-session", d.Title,
			"session must be absent from storage the moment the RPC response is returned")
	}
}

// --------------------------------------------------------------------------
// UpdateSession – tags
// --------------------------------------------------------------------------

// TestUpdateSession_TagsUpdate verifies that a tags update is applied to the
// session and persisted to storage so a subsequent reload reflects the change.
func TestUpdateSession_TagsUpdate(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	addPausedSession(t, fix, "my-session")

	resp, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:   "my-session",
		Tags: []string{"frontend", "urgent"},
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Session)

	// Response must carry the new tags.
	assert.ElementsMatch(t, []string{"frontend", "urgent"}, resp.Msg.Session.Tags,
		"response should contain the updated tags")

	// Reload from storage to verify persistence.
	loaded, err := fix.storage.LoadInstances()
	require.NoError(t, err)

	var found *session.Instance
	for _, inst := range loaded {
		if inst.Title == "my-session" {
			found = inst
			break
		}
	}
	require.NotNil(t, found, "session should still exist in storage after update")
	assert.ElementsMatch(t, []string{"frontend", "urgent"}, found.Tags,
		"tags should be persisted in storage")
}

// TestUpdateSession_TagsUpdate_Replaces verifies that calling UpdateSession with
// a new tag list replaces (not appends to) the previous tags.
func TestUpdateSession_TagsUpdate_Replaces(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	// Seed session with existing tags.
	inst := &session.Instance{
		Title:     "tagged-session",
		Path:      "/tmp/test",
		Status:    session.Paused,
		Program:   "claude",
		Tags:      []string{"old-tag"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := fix.storage.AddInstance(inst)
	require.NoError(t, err)

	// Replace tags with a new set.
	resp, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:   "tagged-session",
		Tags: []string{"new-tag", "another"},
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Session)

	assert.ElementsMatch(t, []string{"new-tag", "another"}, resp.Msg.Session.Tags,
		"tags should be fully replaced, not appended")
	assert.NotContains(t, resp.Msg.Session.Tags, "old-tag",
		"old tags must be removed after replacement")
}

// --------------------------------------------------------------------------
// UpdateSession – handler ordering: metadata before status
// --------------------------------------------------------------------------

// TestUpdateSession_HandlerOrdering_MetadataBeforeStatus verifies that a single
// UpdateSession call applying title, tags, AND a status change (no-op here, already
// Paused → Paused) commits all fields atomically.  The test acts as a contract
// check for the documented ordering: title/category/tags are applied before status.
func TestUpdateSession_HandlerOrdering_MetadataBeforeStatus(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	addPausedSession(t, fix, "combo-session")

	newTitle := "combo-session-renamed"
	paused := sessionv1.SessionStatus_SESSION_STATUS_PAUSED

	resp, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:     "combo-session",
		Title:  &newTitle,
		Tags:   []string{"backend", "infra"},
		Status: &paused,
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Session)

	// All three fields must appear in the response.
	assert.Equal(t, newTitle, resp.Msg.Session.Title, "title should be updated")
	assert.ElementsMatch(t, []string{"backend", "infra"}, resp.Msg.Session.Tags,
		"tags should be updated")
	assert.Equal(t, sessionv1.SessionStatus_SESSION_STATUS_PAUSED, resp.Msg.Session.Status,
		"status should remain paused")

	// Reload from storage to confirm all changes were persisted together.
	loaded, err := fix.storage.LoadInstances()
	require.NoError(t, err)

	var found *session.Instance
	for _, inst := range loaded {
		if inst.Title == newTitle {
			found = inst
			break
		}
	}
	require.NotNil(t, found, "renamed session must be present in storage")
	assert.ElementsMatch(t, []string{"backend", "infra"}, found.Tags,
		"tags must be persisted alongside the rename")
}

// --------------------------------------------------------------------------
// UpdateSession – title conflict
// --------------------------------------------------------------------------

// TestUpdateSession_TitleConflict verifies that attempting to rename a session to
// the title of an already-existing session returns CodeAlreadyExists.
func TestUpdateSession_TitleConflict(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	addPausedSession(t, fix, "session-alpha")
	addPausedSession(t, fix, "session-beta")

	conflictingTitle := "session-beta"
	_, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:    "session-alpha",
		Title: &conflictingTitle,
	}))
	require.Error(t, err)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeAlreadyExists, connectErr.Code(),
		"renaming to an existing title should return CodeAlreadyExists")
}

// TestUpdateSession_NotFound verifies that updating a non-existent session returns
// CodeNotFound.
func TestUpdateSession_NotFound(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	_, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:   "no-such-session",
		Tags: []string{"tag1"},
	}))
	require.Error(t, err)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}

// TestUpdateSession_MissingID verifies that UpdateSession with an empty ID returns
// CodeInvalidArgument.
func TestUpdateSession_MissingID(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	_, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:   "",
		Tags: []string{"tag1"},
	}))
	require.Error(t, err)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// --------------------------------------------------------------------------
// GetSession
// --------------------------------------------------------------------------

// TestGetSession_EmptyID verifies that GetSession returns CodeInvalidArgument
// when no session ID is provided.
func TestGetSession_EmptyID(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	_, err := fix.svc.GetSession(context.Background(), connect.NewRequest(&sessionv1.GetSessionRequest{
		Id: "",
	}))
	require.Error(t, err)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestGetSession_FoundByTitle verifies that GetSession can find a session by Title
// when the poller is wired.
func TestGetSession_FoundByTitle(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	inst := &session.Instance{
		Title:   "title-session",
		UUID:    "aaaaaaaa-0000-0000-0000-000000000001",
		Status:  session.Running,
		Program: "claude",
		Path:    "/tmp/test",
	}
	fix.poller.AddInstance(inst)

	resp, err := fix.svc.GetSession(context.Background(), connect.NewRequest(&sessionv1.GetSessionRequest{
		Id: "title-session",
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Session)
	assert.Equal(t, "title-session", resp.Msg.Session.Title)
}

// TestGetSession_FoundByUUID verifies that GetSession can find a session by UUID
// when the poller is wired.
func TestGetSession_FoundByUUID(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	const testUUID = "bbbbbbbb-0000-0000-0000-000000000002"
	inst := &session.Instance{
		Title:   "uuid-session",
		UUID:    testUUID,
		Status:  session.Running,
		Program: "claude",
		Path:    "/tmp/test",
	}
	fix.poller.AddInstance(inst)

	resp, err := fix.svc.GetSession(context.Background(), connect.NewRequest(&sessionv1.GetSessionRequest{
		Id: testUUID,
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Session)
	assert.Equal(t, "uuid-session", resp.Msg.Session.Title)
}

// TestGetSession_NotFound verifies that GetSession returns CodeNotFound when the
// poller is wired but no session matches the requested ID.
func TestGetSession_NotFound(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	// Poller is wired but empty — no sessions registered.
	_, err := fix.svc.GetSession(context.Background(), connect.NewRequest(&sessionv1.GetSessionRequest{
		Id: "does-not-exist",
	}))
	require.Error(t, err)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}

// --------------------------------------------------------------------------
// ListSessions
// --------------------------------------------------------------------------

// TestListSessions_ReturnsAllSessions verifies that ListSessions returns all sessions
// registered in the poller when no filter is applied.
func TestListSessions_ReturnsAllSessions(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	fix.poller.AddInstance(&session.Instance{
		Title:   "session-one",
		UUID:    "cccccccc-0000-0000-0000-000000000001",
		Status:  session.Running,
		Program: "claude",
		Path:    "/tmp/test",
	})
	fix.poller.AddInstance(&session.Instance{
		Title:   "session-two",
		UUID:    "cccccccc-0000-0000-0000-000000000002",
		Status:  session.Paused,
		Program: "claude",
		Path:    "/tmp/test",
	})

	resp, err := fix.svc.ListSessions(context.Background(), connect.NewRequest(&sessionv1.ListSessionsRequest{}))
	require.NoError(t, err)
	assert.Len(t, resp.Msg.Sessions, 2)
}

// TestListSessions_WithStatusFilter verifies that ListSessions filters sessions by
// the requested status, returning only those that match.
func TestListSessions_WithStatusFilter(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	fix.poller.AddInstance(&session.Instance{
		Title:   "running-session",
		UUID:    "dddddddd-0000-0000-0000-000000000001",
		Status:  session.Running,
		Program: "claude",
		Path:    "/tmp/test",
	})
	fix.poller.AddInstance(&session.Instance{
		Title:   "paused-session",
		UUID:    "dddddddd-0000-0000-0000-000000000002",
		Status:  session.Paused,
		Program: "claude",
		Path:    "/tmp/test",
	})

	filterStatus := sessionv1.SessionStatus_SESSION_STATUS_PAUSED
	resp, err := fix.svc.ListSessions(context.Background(), connect.NewRequest(&sessionv1.ListSessionsRequest{
		Status: &filterStatus,
	}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Sessions, 1)
	assert.Equal(t, "paused-session", resp.Msg.Sessions[0].Title)
}

// TestListSessions_WithCategoryFilter verifies that ListSessions filters sessions by
// the requested category, returning only those that match.
func TestListSessions_WithCategoryFilter(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	fix.poller.AddInstance(&session.Instance{
		Title:    "backend-session",
		UUID:     "eeeeeeee-0000-0000-0000-000000000001",
		Status:   session.Running,
		Program:  "claude",
		Path:     "/tmp/test",
		Category: "backend",
	})
	fix.poller.AddInstance(&session.Instance{
		Title:    "frontend-session",
		UUID:     "eeeeeeee-0000-0000-0000-000000000002",
		Status:   session.Running,
		Program:  "claude",
		Path:     "/tmp/test",
		Category: "frontend",
	})

	category := "backend"
	resp, err := fix.svc.ListSessions(context.Background(), connect.NewRequest(&sessionv1.ListSessionsRequest{
		Category: &category,
	}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Sessions, 1)
	assert.Equal(t, "backend-session", resp.Msg.Sessions[0].Title)
}

// --------------------------------------------------------------------------
// RenameSession
// --------------------------------------------------------------------------

// TestRenameSession_EmptyID verifies that RenameSession returns CodeInvalidArgument
// when no session ID is provided.
func TestRenameSession_EmptyID(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	_, err := fix.svc.RenameSession(context.Background(), connect.NewRequest(&sessionv1.RenameSessionRequest{
		Id:       "",
		NewTitle: "new-name",
	}))
	require.Error(t, err)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestRenameSession_EmptyNewTitle verifies that RenameSession returns CodeInvalidArgument
// when no new title is provided.
func TestRenameSession_EmptyNewTitle(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	_, err := fix.svc.RenameSession(context.Background(), connect.NewRequest(&sessionv1.RenameSessionRequest{
		Id:       "some-session",
		NewTitle: "",
	}))
	require.Error(t, err)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestRenameSession_NotFound verifies that RenameSession returns CodeNotFound when
// the target session does not exist in storage.
func TestRenameSession_NotFound(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	_, err := fix.svc.RenameSession(context.Background(), connect.NewRequest(&sessionv1.RenameSessionRequest{
		Id:       "no-such-session",
		NewTitle: "new-name",
	}))
	require.Error(t, err)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}

// TestRenameSession_ConflictsWithExisting verifies that RenameSession returns
// CodeAlreadyExists when the desired new title is already taken by another session.
func TestRenameSession_ConflictsWithExisting(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	addPausedSession(t, fix, "session-alpha")
	addPausedSession(t, fix, "session-beta")

	_, err := fix.svc.RenameSession(context.Background(), connect.NewRequest(&sessionv1.RenameSessionRequest{
		Id:       "session-alpha",
		NewTitle: "session-beta",
	}))
	require.Error(t, err)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeAlreadyExists, connectErr.Code())
}

// TestRenameSession_Success verifies that RenameSession renames a session and returns
// the updated session in the response with the new title. It also confirms the new
// title record is persisted to storage.
func TestRenameSession_Success(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	addPausedSession(t, fix, "old-name")

	resp, err := fix.svc.RenameSession(context.Background(), connect.NewRequest(&sessionv1.RenameSessionRequest{
		Id:       "old-name",
		NewTitle: "new-name",
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Session)
	assert.Equal(t, "new-name", resp.Msg.Session.Title)

	// Confirm the new title record is present in storage using raw InstanceData
	// to avoid FromInstanceData's Start() side effect.
	data, err := fix.storage.ListInstanceData()
	require.NoError(t, err)

	var foundNew bool
	for _, d := range data {
		if d.Title == "new-name" {
			foundNew = true
			break
		}
	}
	assert.True(t, foundNew, "new title should exist in storage after rename")
}

// --------------------------------------------------------------------------
// CreateSession – validation only (no tmux)
// --------------------------------------------------------------------------

// TestCreateSession_EmptyTitle verifies that CreateSession returns CodeInvalidArgument
// when no title is provided.
func TestCreateSession_EmptyTitle(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	_, err := fix.svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title: "",
		Path:  "/tmp/test",
	}))
	require.Error(t, err)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestCreateSession_EmptyPath verifies that CreateSession returns CodeInvalidArgument
// when no path is provided.
func TestCreateSession_EmptyPath(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	_, err := fix.svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title: "some-session",
		Path:  "",
	}))
	require.Error(t, err)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestCreateSession_TitleAlreadyExists verifies that CreateSession returns
// CodeAlreadyExists when a session with the same title already exists in storage.
func TestCreateSession_TitleAlreadyExists(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	// Seed storage with an existing session.
	addPausedSession(t, fix, "existing-session")

	_, err := fix.svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title: "existing-session",
		Path:  "/tmp/test",
	}))
	require.Error(t, err)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeAlreadyExists, connectErr.Code())
}
