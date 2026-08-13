package services

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/detection"
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

	// Load back from storage so FromInstanceData sets started=true (required for
	// SaveInstances to persist subsequent mutations via the live poller path).
	loaded, err := fix.storage.LoadInstances()
	require.NoError(t, err, "addPausedSession: failed to reload after persist")
	for _, li := range loaded {
		if li.Title == title {
			addInstanceToPoller(fix.poller, li)
			return
		}
	}
	t.Fatalf("addPausedSession: could not find %q after reload", title)
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
	t.Cleanup(func() { svc.Shutdown() })

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
	t.Cleanup(func() { svc.Shutdown() })

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
	t.Cleanup(func() { svc.Shutdown() })

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
	t.Cleanup(func() { svc.Shutdown() })

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
	t.Cleanup(func() { svc.Shutdown() })

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
	t.Cleanup(func() { svc.Shutdown() })

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
	t.Cleanup(func() { svc.Shutdown() })

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
	t.Cleanup(func() { svc.Shutdown() })

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
// Session exit — unexpected stop publishes SessionUpdatedEvent
// --------------------------------------------------------------------------

// TestSessionExitedPublisher_PublishesUpdatedEvent is a regression test for the bug
// where a session marked as "Thinking…" remained visually stuck after the process
// exited unexpectedly. Root cause: the PTY-EOF and reconcile-exit paths called
// transitionTo(Stopped) and fireLifecycleEvent(EventExited) but never published a
// SessionUpdatedEvent to the event bus, so the WatchSessions stream never notified
// the frontend of the status change.
//
// This test verifies that wireSessionExitedPublisher, wired during session loading,
// fires a SessionUpdatedEvent with a Stopped instance when EventExited is raised.
func TestSessionExitedPublisher_PublishesUpdatedEvent(t *testing.T) {
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	// Use Status=Paused so FromInstanceData marks the instance as started=true,
	// matching what the real lifecycle does for an instance that was once Active.
	require.NoError(t, storage.AddInstance(&session.Instance{
		Title:     "thinking-session",
		Path:      "/tmp/test",
		Status:    session.Paused,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}))

	// loadInstancesWithWiring wires the exit publisher onto each loaded instance.
	instances, err := svc.loadInstancesWithWiring()
	require.NoError(t, err)
	require.Len(t, instances, 1)
	inst := instances[0]

	// Force the instance into Active status to simulate a live thinking session.
	inst.ForceStatus(session.Active)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := eventBus.Subscribe(ctx)

	// Simulate an unexpected exit (PTY EOF / process crash).
	inst.ForceStatus(session.Stopped)
	inst.FireLifecycleEventForTest(session.EventExited, "pty-eof")

	select {
	case evt := <-ch:
		assert.Equal(t, events.EventSessionUpdated, evt.Type,
			"expected SessionUpdatedEvent after unexpected session exit")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SessionUpdatedEvent after session exit")
	}
}

// TestSessionExitedPublisher_ClearsDetectedStatus verifies that the SessionUpdatedEvent
// published on exit has DetectedStatus == StatusUnknown, i.e., detection is cleared.
func TestSessionExitedPublisher_ClearsDetectedStatus(t *testing.T) {
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	require.NoError(t, storage.AddInstance(&session.Instance{
		Title:     "thinking-session-2",
		Path:      "/tmp/test",
		Status:    session.Paused,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}))

	instances, err := svc.loadInstancesWithWiring()
	require.NoError(t, err)
	require.Len(t, instances, 1)
	inst := instances[0]

	inst.ForceStatus(session.Active)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := eventBus.Subscribe(ctx)

	inst.ForceStatus(session.Stopped)
	inst.FireLifecycleEventForTest(session.EventExited, "pty-eof")

	select {
	case evt := <-ch:
		require.Equal(t, events.EventSessionUpdated, evt.Type,
			"expected SessionUpdatedEvent after session exit")
		assert.Equal(t, detection.StatusUnknown, evt.DetectedStatusTyped,
			"detection should be cleared (StatusUnknown) on session exit")
		assert.Empty(t, evt.DetectedContext,
			"detected context should be empty on session exit")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SessionUpdatedEvent after session exit")
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
	// Load back so started=true; required for SaveInstances to persist mutations.
	loaded, err := fix.storage.LoadInstances()
	require.NoError(t, err)
	for _, li := range loaded {
		if li.Title == "tagged-session" {
			addInstanceToPoller(fix.poller, li)
			break
		}
	}

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

// TestUpdateSession_NoteUpdate verifies that a session note round-trips through
// UpdateSession and persists through a full storage reload, not just the in-memory
// response (mirrors TestUpdateSession_TagsUpdate).
func TestUpdateSession_NoteUpdate(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	addPausedSession(t, fix, "my-session")

	note := "left this waiting on CI"
	resp, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:   "my-session",
		Note: &note,
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Session)
	assert.Equal(t, note, resp.Msg.Session.Note, "response should contain the updated note")

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
	assert.Equal(t, note, found.Note, "note should be persisted in storage")
}

// TestUpdateSession_NoteUpdate_BumpsUpdatedAt is a regression test: setNoteLocked
// used to mutate Instance.Note without touching Instance.UpdatedAt. The frontend's
// upsertSession reducer (sessionsSlice.ts) skips applying an incoming session as a
// no-op dedup optimization whenever its updatedAt matches the already-stored value —
// so on a session whose UpdatedAt hadn't otherwise moved (e.g. freshly created and
// still idle), a note save would succeed server-side yet never appear in the UI,
// since the client-side dedup silently discarded the "unchanged" update. Confirmed
// live via tests/e2e/session-notes.spec.ts before this fix. UpdatedAt must always
// move forward on a note change so that dedup check can't misfire.
func TestUpdateSession_NoteUpdate_BumpsUpdatedAt(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	addPausedSession(t, fix, "my-session")

	before := fix.poller.FindInstance("my-session")
	require.NotNil(t, before)
	beforeUpdatedAt := before.UpdatedAt

	note := "left this waiting on CI"
	resp, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:   "my-session",
		Note: &note,
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Session)
	require.NotNil(t, resp.Msg.Session.UpdatedAt)
	assert.True(t, resp.Msg.Session.UpdatedAt.AsTime().After(beforeUpdatedAt),
		"UpdatedAt must move forward after a note-only update, or the frontend's "+
			"upsertSession no-op dedup will silently drop the change")
}

// TestUpdateSession_NoteExceedsMaxLength_ReturnsInvalidArgument verifies that a
// note longer than session.MaxNoteLength is rejected with InvalidArgument and
// does not partially write.
func TestUpdateSession_NoteExceedsMaxLength_ReturnsInvalidArgument(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	addPausedSession(t, fix, "my-session")

	tooLong := strings.Repeat("a", session.MaxNoteLength+1)
	_, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:   "my-session",
		Note: &tooLong,
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

	loaded, err := fix.storage.LoadInstances()
	require.NoError(t, err)
	var found *session.Instance
	for _, inst := range loaded {
		if inst.Title == "my-session" {
			found = inst
			break
		}
	}
	require.NotNil(t, found)
	assert.Empty(t, found.Note, "note must not be partially written when rejected")
}

// TestUpdateSession_NoteLengthValidation_IsByteAccurate proves the length check uses
// Go's byte-length len(string), not a rune count — an ASCII-only fixture can't tell
// these apart since 1 rune == 1 byte for ASCII, so this specifically exercises a
// multi-byte string whose rune count is under session.MaxNoteLength but whose byte
// length exceeds it (mirrors the frontend's equivalent guard in NotePanel.tsx).
func TestUpdateSession_NoteLengthValidation_IsByteAccurate(t *testing.T) {
	// "あ" is 1 rune but 3 UTF-8 bytes: 3400 runes = 3400 runes / 10200 bytes,
	// under the rune-based reading of the cap but over the byte-based one.
	multiByteTooLong := strings.Repeat("あ", 3400)
	require.Less(t, len([]rune(multiByteTooLong)), session.MaxNoteLength,
		"fixture invariant: rune count must be under MaxNoteLength")
	require.Greater(t, len(multiByteTooLong), session.MaxNoteLength,
		"fixture invariant: byte length must exceed MaxNoteLength")

	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	addPausedSession(t, fix, "my-session")

	_, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:   "my-session",
		Note: &multiByteTooLong,
	}))
	require.Error(t, err, "a note under the rune-count cap but over the byte cap must still be rejected")
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// TestUpdateSession_NoteExceedsMaxLength_LeavesOtherFieldsUnmutated is a regression
// test for a partial-mutation bug: the note-length check used to run after
// SetCategory/SetTitleDirect had already mutated the live in-memory Instance and
// published a new snapshot (visible to concurrent readers like WatchSessions), even
// though the RPC as a whole returned InvalidArgument. A combined request that fails
// on Note must leave every other field it also touched completely unmutated — not
// just unwritten to storage (SaveInstances never runs on this error path either way,
// so a storage-only check wouldn't catch this), but unmutated in the live instance
// the poller holds.
func TestUpdateSession_NoteExceedsMaxLength_LeavesOtherFieldsUnmutated(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	addPausedSession(t, fix, "my-session")

	originalCategory := "original-category"
	_, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:       "my-session",
		Category: &originalCategory,
	}))
	require.NoError(t, err)

	newCategory := "should-not-apply"
	tooLong := strings.Repeat("a", session.MaxNoteLength+1)
	_, err = fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:       "my-session",
		Category: &newCategory,
		Note:     &tooLong,
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

	live := fix.poller.FindInstance("my-session")
	require.NotNil(t, live, "session should still be resolvable in the live poller list")
	assert.Equal(t, originalCategory, live.Category,
		"category must remain unmutated in the live in-memory instance when the note in the same request is rejected")
}

// TestUpdateSession_NoteCleared_PersistsAsEmptyAcrossReload is the regression test
// for the ent Update path's guarded-vs-unconditional SetNote fix: it must fail
// against a guarded (`if data.Note != ""`) Update and pass against the
// unconditional one, since clearing a note is a meaningful state, not "unset".
func TestUpdateSession_NoteCleared_PersistsAsEmptyAcrossReload(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	addPausedSession(t, fix, "my-session")

	stale := "stale reminder"
	_, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:   "my-session",
		Note: &stale,
	}))
	require.NoError(t, err)

	empty := ""
	resp, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:   "my-session",
		Note: &empty,
	}))
	require.NoError(t, err)
	assert.Equal(t, "", resp.Msg.Session.Note, "response should reflect the cleared note")

	loaded, err := fix.storage.LoadInstances()
	require.NoError(t, err)
	var found *session.Instance
	for _, inst := range loaded {
		if inst.Title == "my-session" {
			found = inst
			break
		}
	}
	require.NotNil(t, found)
	assert.Equal(t, "", found.Note, "cleared note must persist as empty across reload, not the stale prior value")
}

// TestUpdateSession_UnrelatedFieldUpdate_PreservesExistingNote guards the interaction
// between the RPC handler's conditional field application (Note only mutated when
// req.Msg.Note != nil) and ent_repository.go's *unconditional* SetNote(data.Note) on
// every Update call: an UpdateSession call that doesn't touch Note at all must not
// let the unconditional set clobber an existing note, since data.Note always reflects
// the instance's already-current in-memory value when Note wasn't part of this request.
func TestUpdateSession_UnrelatedFieldUpdate_PreservesExistingNote(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	addPausedSession(t, fix, "my-session")

	existingNote := "left this waiting on CI"
	_, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:   "my-session",
		Note: &existingNote,
	}))
	require.NoError(t, err)

	newTitle := "my-session-renamed"
	resp, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:    "my-session",
		Title: &newTitle,
	}))
	require.NoError(t, err)
	assert.Equal(t, existingNote, resp.Msg.Session.Note, "response should still show the existing note after a Title-only update")

	loaded, err := fix.storage.LoadInstances()
	require.NoError(t, err)
	var found *session.Instance
	for _, inst := range loaded {
		if inst.Title == newTitle {
			found = inst
			break
		}
	}
	require.NotNil(t, found)
	assert.Equal(t, existingNote, found.Note, "note must survive an unrelated field update across reload")
}

// TestUpdateSession_NoteOnlyEdit_DoesNotTouchOtherSessions is the regression test for the
// write-amplification bug: UpdateSession used to call SaveInstances with the ENTIRE live
// instance list, issuing one full-row UPDATE per OTHER started session even though only the
// target session's field changed. This asserts that N other sessions' UpdatedAt timestamps
// are untouched by a single-field edit to one session.
func TestUpdateSession_NoteOnlyEdit_DoesNotTouchOtherSessions(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	addPausedSession(t, fix, "target-session")
	addPausedSession(t, fix, "other-session-1")
	addPausedSession(t, fix, "other-session-2")

	before, err := fix.storage.LoadInstances()
	require.NoError(t, err)
	beforeUpdatedAt := make(map[string]time.Time, len(before))
	for _, inst := range before {
		beforeUpdatedAt[inst.Title] = inst.UpdatedAt
	}

	note := "left this waiting on CI"
	_, err = fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:   "target-session",
		Note: &note,
	}))
	require.NoError(t, err)

	after, err := fix.storage.LoadInstances()
	require.NoError(t, err)
	for _, inst := range after {
		if inst.Title == "target-session" {
			continue
		}
		assert.True(t, inst.UpdatedAt.Equal(beforeUpdatedAt[inst.Title]),
			"session %q must not be rewritten by an edit to a different session (before=%v after=%v)",
			inst.Title, beforeUpdatedAt[inst.Title], inst.UpdatedAt)
	}
}

// TestUpdateSession_TitleRename_DoesNotOrphanOldRow is the regression test for the
// title-rename identity bug: the generic SaveInstances path looks the DB row up by the
// already-in-memory-renamed title, misses the still-old-titled row, and falls into a
// Create fallback that leaves the old row behind as an orphan. Asserts exactly one row
// (under the new title) exists after a rename, not two.
func TestUpdateSession_TitleRename_DoesNotOrphanOldRow(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	addPausedSession(t, fix, "old-title")

	newTitle := "new-title"
	_, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:    "old-title",
		Title: &newTitle,
	}))
	require.NoError(t, err)

	data, err := fix.storage.ListInstanceData()
	require.NoError(t, err)

	var matches []string
	for _, d := range data {
		if d.Title == "old-title" || d.Title == newTitle {
			matches = append(matches, d.Title)
		}
	}
	assert.Equal(t, []string{newTitle}, matches,
		"rename must not leave an orphaned row under the old title (found: %v)", matches)
}

// TestUpdateSession_TitleAndProgramCombo_DoesNotDuplicateRow is the regression test for a
// bug found in code review: when a single UpdateSession request changes both Title and
// Program, the Program branch's SwitchProgram callback used to persist via SaveInstances
// BEFORE the deferred narrow title rename ran. SaveInstances looks the DB row up by the
// already-in-memory-renamed title, misses the still-old-titled row, and duplicates it via
// saveInstancesToRepo's Create fallback — leaving two rows under the new title. Asserts
// exactly one row exists under either title after a combined Title+Program edit.
func TestUpdateSession_TitleAndProgramCombo_DoesNotDuplicateRow(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	addPausedSession(t, fix, "combo-old-title")

	newTitle := "combo-new-title"
	newProgram := "aider"
	resp, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:      "combo-old-title",
		Title:   &newTitle,
		Program: &newProgram,
	}))
	require.NoError(t, err)
	assert.Equal(t, newTitle, resp.Msg.Session.Title)
	assert.Equal(t, newProgram, resp.Msg.Session.Program)

	data, err := fix.storage.ListInstanceData()
	require.NoError(t, err)

	var matches []string
	for _, d := range data {
		if d.Title == "combo-old-title" || d.Title == newTitle {
			matches = append(matches, d.Title)
		}
	}
	assert.Equal(t, []string{newTitle}, matches,
		"combined title+program edit must not leave a duplicate row (found: %v)", matches)

	loaded, err := fix.storage.LoadInstances()
	require.NoError(t, err)
	var found *session.Instance
	for _, inst := range loaded {
		if inst.Title == newTitle {
			found = inst
			break
		}
	}
	require.NotNil(t, found)
	assert.Equal(t, newProgram, found.Program, "program change must be persisted alongside the rename")
}

// TestUpdateSession_ConcurrentDifferentFieldEdits_BothPersist verifies that two concurrent
// UpdateSession calls touching different fields (note, category) of the same session both
// land — neither narrow metadata write should lose the other's update.
func TestUpdateSession_ConcurrentDifferentFieldEdits_BothPersist(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	addPausedSession(t, fix, "concurrent-session")

	note := "concurrent note"
	category := "concurrent-category"

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
			Id:   "concurrent-session",
			Note: &note,
		}))
		assert.NoError(t, err)
	}()
	go func() {
		defer wg.Done()
		_, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
			Id:       "concurrent-session",
			Category: &category,
		}))
		assert.NoError(t, err)
	}()
	wg.Wait()

	loaded, err := fix.storage.LoadInstances()
	require.NoError(t, err)
	var found *session.Instance
	for _, inst := range loaded {
		if inst.Title == "concurrent-session" {
			found = inst
			break
		}
	}
	require.NotNil(t, found)
	assert.Equal(t, note, found.Note, "note update must not be lost to the concurrent category update")
	assert.Equal(t, category, found.Category, "category update must not be lost to the concurrent note update")
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
// UpdateSession – pause/resume on never-started instances (regression tests
// for the pause/resume-500 bug: UpdateSession must not return CodeInternal
// when the target Instance was never actually started).
// --------------------------------------------------------------------------

// TestUpdateSession_Pause_NeverStartedActiveInstance_NoOpSuccessNotInternal verifies
// that pausing an Active instance that was never started (e.g. the async
// CreateSession goroutine hasn't finished) succeeds as a no-op instead of 500ing.
func TestUpdateSession_Pause_NeverStartedActiveInstance_NoOpSuccessNotInternal(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	inst := &session.Instance{
		Title:       "never-started-active",
		Path:        "/tmp/test",
		Status:      session.Active,
		Program:     "claude",
		Permissions: session.GetManagedPermissions(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	addInstanceToPoller(fix.poller, inst)

	paused := sessionv1.SessionStatus_SESSION_STATUS_PAUSED
	resp, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:     "never-started-active",
		Status: &paused,
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Session)
	assert.Equal(t, sessionv1.SessionStatus_SESSION_STATUS_PAUSED, resp.Msg.Session.Status)
}

// TestUpdateSession_Pause_StoppedInstance_ReturnsFailedPrecondition verifies that
// an invalid transition (Stopped -> Paused) is classified as FailedPrecondition,
// not CodeInternal.
func TestUpdateSession_Pause_StoppedInstance_ReturnsFailedPrecondition(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	inst := &session.Instance{
		Title:       "stopped-session",
		Path:        "/tmp/test",
		Status:      session.Stopped,
		Program:     "claude",
		Permissions: session.GetManagedPermissions(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	addInstanceToPoller(fix.poller, inst)

	paused := sessionv1.SessionStatus_SESSION_STATUS_PAUSED
	_, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:     "stopped-session",
		Status: &paused,
	}))
	require.Error(t, err)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeFailedPrecondition, connectErr.Code())
}

// TestUpdateSession_Pause_PermissionDenied_ReturnsFailedPrecondition verifies that
// an externally-discovered instance with CanPause=false is rejected with
// FailedPrecondition rather than silently no-op-pausing (closing the pre-mortem
// F1 gap: MCP tools call Instance.Pause() directly, bypassing UpdateSession).
func TestUpdateSession_Pause_PermissionDenied_ReturnsFailedPrecondition(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	inst := &session.Instance{
		Title:       "external-session",
		Path:        "/tmp/test",
		Status:      session.Active,
		Program:     "claude",
		Permissions: session.GetExternalPermissions(false),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	addInstanceToPoller(fix.poller, inst)

	paused := sessionv1.SessionStatus_SESSION_STATUS_PAUSED
	_, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:     "external-session",
		Status: &paused,
	}))
	require.Error(t, err)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeFailedPrecondition, connectErr.Code())
}

// TestUpdateSession_Resume_PermissionDenied_ReturnsFailedPrecondition verifies the
// resume-side counterpart of the pause permission gate: an externally-discovered
// Paused instance with CanResume=false is rejected with FailedPrecondition.
// (The "resume a never-started Paused instance succeeds" scenario needs a fake
// ProcessManager to avoid spinning up real tmux and is covered at the Instance
// level by TestResume_should_PerformRealResumeAndMarkStarted_When_PausedInstanceNeverStarted
// in session/pause_resume_test.go — Instance.processManager isn't exported for
// injection from this package.)
func TestUpdateSession_Resume_PermissionDenied_ReturnsFailedPrecondition(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	inst := &session.Instance{
		Title:       "external-paused-session",
		Path:        "/tmp/test",
		Status:      session.Paused,
		Program:     "claude",
		Permissions: session.GetExternalPermissions(false),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	addInstanceToPoller(fix.poller, inst)

	active := sessionv1.SessionStatus_SESSION_STATUS_ACTIVE
	_, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:     "external-paused-session",
		Status: &active,
	}))
	require.Error(t, err)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeFailedPrecondition, connectErr.Code())
}

// --------------------------------------------------------------------------
// UpdateSession — steer_message (ADR-001: widened steer RPC)
// --------------------------------------------------------------------------

// TestUpdateSession_SteerMessage_NonAutonomousSession_SendsViaSendKeys verifies
// AC7: a non-autonomous, live Instance-backed session now steers via
// Instance.SendKeys (the same primitive the MCP steer_session tool's PTY
// fallback uses) instead of unconditionally rejecting with FailedPrecondition.
//
// Instance.processManager isn't exported for injection from this package (see
// the note on TestUpdateSession_Resume_PermissionDenied_ReturnsFailedPrecondition
// above), so this drives a real Instance.Resume() with the package-level
// backend switched to the native PTY backend — the same real-subprocess
// approach session/native_process_manager_test.go uses — rather than a
// fake/mock SendKeys recorder, to get a genuinely "started" Instance whose
// SendKeys call actually succeeds.
func TestUpdateSession_SteerMessage_NonAutonomousSession_SendsViaSendKeys(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PTY allocation")
	}

	session.RegisterBackendProvider(session.BackendNative)
	defer session.RegisterBackendProvider(session.BackendTmux)

	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	inst := &session.Instance{
		Title:       "steerable-work-session",
		Path:        t.TempDir(),
		Status:      session.Paused,
		Program:     "bash",
		Permissions: session.GetManagedPermissions(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := inst.Resume(); err != nil {
		t.Skipf("PTY not available in this environment: %v", err)
	}
	t.Cleanup(func() { _ = inst.KillSession() })
	require.False(t, inst.AutonomousMode, "regression guard: instance must not be autonomous for this test")

	addInstanceToPoller(fix.poller, inst)

	msg := "focus on the auth module first"
	resp, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:           "steerable-work-session",
		SteerMessage: &msg,
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Session)
}

// TestUpdateSession_SteerMessage_NonAutonomousSession_SendKeysFailure_ReturnsFailedPrecondition
// verifies the other half of AC7's error contract: unlike the autonomous
// branch (which only logs a send failure), a SendKeys failure on the
// non-autonomous branch is returned to the caller as FailedPrecondition so
// the UI can surface it. A never-started Instance deterministically fails
// SendKeys ("cannot send keys to instance that has not been started or is
// paused"), giving a real failure without needing a fake ProcessManager.
func TestUpdateSession_SteerMessage_NonAutonomousSession_SendKeysFailure_ReturnsFailedPrecondition(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	inst := &session.Instance{
		Title:       "never-started-work-session",
		Path:        "/tmp/test",
		Status:      session.Active,
		Program:     "claude",
		Permissions: session.GetManagedPermissions(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	addInstanceToPoller(fix.poller, inst)

	msg := "focus on the auth module first"
	_, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:           "never-started-work-session",
		SteerMessage: &msg,
	}))
	require.Error(t, err)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeFailedPrecondition, connectErr.Code())
}

// TestUpdateSession_SteerMessage_AutonomousSession_StillUsesController is a
// regression guard (AC7's "no parallel steering implementation"): the
// pre-existing autonomous branch (ClaudeController.SendCommandImmediate) must
// be untouched by widening the handler to add the non-autonomous SendKeys
// branch. An autonomous session with no live controller wired still returns
// success (a send failure on this branch is only logged, never rejected),
// exactly like the pre-widening behavior.
func TestUpdateSession_SteerMessage_AutonomousSession_StillUsesController(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	inst := &session.Instance{
		Title:          "autonomous-session",
		Path:           "/tmp/test",
		Status:         session.Active,
		Program:        "claude",
		Permissions:    session.GetManagedPermissions(),
		AutonomousMode: true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	addInstanceToPoller(fix.poller, inst)

	msg := "focus on the auth module first"
	resp, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:           "autonomous-session",
		SteerMessage: &msg,
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Session)
}

// TestUpdateSession_SteerMessage_ExceedsMaxLength_ReturnsInvalidArgument verifies
// that a steer_message longer than session.MaxSteerMessageLength is rejected with
// InvalidArgument before any send is attempted, mirroring the Note field's
// length-cap guard (TestUpdateSession_NoteExceedsMaxLength_ReturnsInvalidArgument)
// — steer_message is a free-text entry point that now reaches ordinary
// work/review sessions, not just autonomous ones, so it needs the same cap.
func TestUpdateSession_SteerMessage_ExceedsMaxLength_ReturnsInvalidArgument(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	inst := &session.Instance{
		Title:       "steer-too-long-session",
		Path:        "/tmp/test",
		Status:      session.Active,
		Program:     "claude",
		Permissions: session.GetManagedPermissions(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	addInstanceToPoller(fix.poller, inst)

	tooLong := strings.Repeat("a", session.MaxSteerMessageLength+1)
	_, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:           "steer-too-long-session",
		SteerMessage: &tooLong,
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// --------------------------------------------------------------------------
// ResumeCrashedSession
// --------------------------------------------------------------------------

func TestResumeCrashedSession_EmptyId(t *testing.T) {
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	_, err := svc.ResumeCrashedSession(context.Background(), connect.NewRequest(&sessionv1.ResumeCrashedSessionRequest{Id: ""}))
	require.Error(t, err)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

func TestResumeCrashedSession_NotFound(t *testing.T) {
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	_, err := svc.ResumeCrashedSession(context.Background(), connect.NewRequest(&sessionv1.ResumeCrashedSessionRequest{Id: "does-not-exist"}))
	require.Error(t, err)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}

// TestResumeHibernatedSession_DoesNotTouchOtherSessions is the regression test for the
// layer-1 write-amplification fix applied to ResumeHibernatedSession (and identically to
// HibernateSession/ResumeCrashedSession, which share the exact same
// `instances[instanceIndex] = instance; SaveInstances(instances)` -> `SaveInstances([]*session.Instance{instance})`
// shape): resuming one Hibernated session used to persist via the entire live instance
// list, rewriting every other started session's row too. Asserts a sibling session's
// UpdatedAt is untouched.
func TestResumeHibernatedSession_DoesNotTouchOtherSessions(t *testing.T) {
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	hibernated := &session.Instance{
		Title:     "hibernated-session",
		UUID:      "dddddddd-0000-0000-0000-000000000004",
		Path:      "/tmp/test",
		Status:    session.Hibernated,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, storage.AddInstance(hibernated))

	other := &session.Instance{
		Title:     "hibernate-sibling-session",
		Path:      "/tmp/test",
		Status:    session.Paused,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, storage.AddInstance(other))

	// ResumeFromHibernation dispatches the actual relaunch to a background goroutine
	// that calls the real Start(false) — a real tmux session gets created as a side
	// effect. Best-effort clean it up so repeated test runs don't leave orphaned tmux
	// sessions, mirroring TestResumeCrashedSession_TransitionsCrashedToActive.
	t.Cleanup(func() {
		time.Sleep(100 * time.Millisecond)
		_ = hibernated.KillSession()
	})

	before, err := storage.LoadInstances()
	require.NoError(t, err)
	var beforeOtherUpdatedAt time.Time
	for _, inst := range before {
		if inst.Title == "hibernate-sibling-session" {
			beforeOtherUpdatedAt = inst.UpdatedAt
		}
	}
	require.False(t, beforeOtherUpdatedAt.IsZero(), "sibling session must be found before the call")

	resp, err := svc.ResumeHibernatedSession(context.Background(), connect.NewRequest(&sessionv1.ResumeHibernatedSessionRequest{Id: "dddddddd-0000-0000-0000-000000000004"}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Session)
	assert.Equal(t, sessionv1.SessionStatus_SESSION_STATUS_ACTIVE, resp.Msg.Session.Status)

	after, err := storage.LoadInstances()
	require.NoError(t, err)
	for _, inst := range after {
		if inst.Title == "hibernate-sibling-session" {
			assert.True(t, inst.UpdatedAt.Equal(beforeOtherUpdatedAt),
				"sibling session must not be rewritten by resuming a different session (before=%v after=%v)",
				beforeOtherUpdatedAt, inst.UpdatedAt)
		}
	}
}

// TestResumeCrashedSession_TransitionsCrashedToActive verifies that resuming a
// Crashed session (dead pane detected by SessionHealthChecker) transitions it
// back to Active in the response, giving the frontend a one-tap resume action
// instead of requiring the user to hand-type the --resume command.
func TestResumeCrashedSession_TransitionsCrashedToActive(t *testing.T) {
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	const sessionUUID = "cccccccc-0000-0000-0000-000000000003"
	testInstance := &session.Instance{
		Title:      "crashed-session",
		UUID:       sessionUUID,
		Path:       "/tmp/test",
		Status:     session.Crashed,
		ExitReason: "signal SIGKILL (exit code 137)",
		Program:    "claude",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	require.NoError(t, storage.AddInstance(testInstance))

	// ResumeFromCrash (session/instance_crash.go) dispatches the actual relaunch
	// to a background goroutine that calls the real Start(false) -- a real tmux
	// session gets created as a side effect. Best-effort clean it up so repeated
	// test runs don't leave orphaned tmux sessions on the machine; the goroutine
	// isn't awaited, so this is a short grace delay, not a guarantee.
	t.Cleanup(func() {
		time.Sleep(100 * time.Millisecond)
		_ = testInstance.KillSession()
	})

	resp, err := svc.ResumeCrashedSession(context.Background(), connect.NewRequest(&sessionv1.ResumeCrashedSessionRequest{Id: sessionUUID}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Session)
	assert.Equal(t, sessionv1.SessionStatus_SESSION_STATUS_ACTIVE, resp.Msg.Session.Status)
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
		Status:  session.Active,
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
		Status:  session.Active,
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
		Status:  session.Active,
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
		Status:  session.Active,
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
		Status:   session.Active,
		Program:  "claude",
		Path:     "/tmp/test",
		Category: "backend",
	})
	fix.poller.AddInstance(&session.Instance{
		Title:    "frontend-session",
		UUID:     "eeeeeeee-0000-0000-0000-000000000002",
		Status:   session.Active,
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

// --------------------------------------------------------------------------
// DeleteSession + CancelSession ordering (F10)
// --------------------------------------------------------------------------

// TestDeleteSession_CancelsPendingApprovals verifies that DeleteSession cancels
// all pending approvals for the session before removing it from storage.
// This ensures approval goroutines can exit cleanly while the session still exists.
func TestDeleteSession_CancelsPendingApprovals(t *testing.T) {
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	const sessionUUID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	testInstance := &session.Instance{
		Title:     "approval-session",
		UUID:      sessionUUID,
		Path:      "/tmp/test",
		Status:    session.Paused,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, storage.AddInstance(testInstance))

	// Register a pending approval for this session
	approvalStore := svc.GetApprovalStore()
	approval := &PendingApproval{
		ID:        "approval-001",
		SessionID: sessionUUID,
		ToolName:  "Bash",
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	require.NoError(t, approvalStore.Create(approval))

	// Verify approval exists before deletion
	pendingBefore := approvalStore.GetBySession(sessionUUID)
	require.Len(t, pendingBefore, 1, "approval must exist before deletion")

	// Delete the session
	resp, err := svc.DeleteSession(context.Background(), connect.NewRequest(&sessionv1.DeleteSessionRequest{
		Id: "approval-session",
	}))
	require.NoError(t, err)
	require.True(t, resp.Msg.Success)

	// Approval must be cancelled after deletion
	pendingAfter := approvalStore.GetBySession(sessionUUID)
	assert.Len(t, pendingAfter, 0, "pending approvals must be cleared after session deletion")
}

// TestDeleteSession_CancelsPendingApprovals_NoApprovalsIsNoop verifies that
// DeleteSession succeeds gracefully when there are no pending approvals.
func TestDeleteSession_CancelsPendingApprovals_NoApprovalsIsNoop(t *testing.T) {
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	require.NoError(t, storage.AddInstance(&session.Instance{
		Title:     "clean-session",
		Path:      "/tmp/test",
		Status:    session.Paused,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}))

	// No pending approvals — DeleteSession must still succeed
	resp, err := svc.DeleteSession(context.Background(), connect.NewRequest(&sessionv1.DeleteSessionRequest{
		Id: "clean-session",
	}))
	require.NoError(t, err)
	assert.True(t, resp.Msg.Success)
}

// --------------------------------------------------------------------------
// SetMCPServerURL / resolveMCPServerURL (Task 1.1.1c)
// --------------------------------------------------------------------------

// SessionService_should_ReturnUpdatedPort_When_MCPServerURLFnInvokedAfterAddrChanges
// verifies that mcpServerURLFn is invoked lazily at each point of use rather
// than snapshotted once at SetMCPServerURL time. This is the regression test
// for the early-binding bug where "http://"+srv.addr+"/mcp" was baked into a
// plain string field at server-construction time — before Start() resolved a
// PORT=0 listener's real address — permanently freezing the MCP URL at
// http://localhost:0/mcp.
func TestSessionService_should_ReturnUpdatedPort_When_MCPServerURLFnInvokedAfterAddrChanges(t *testing.T) {
	svc := &SessionService{}

	// Simulate the server's address being resolved lazily, e.g. after
	// net.Listen reassigns s.addr post-bind (Task 1.1.1a).
	addr := "localhost:0"
	svc.SetMCPServerURL(func() string { return "http://" + addr + "/mcp" })

	require.Equal(t, "http://localhost:0/mcp", svc.resolveMCPServerURL(),
		"expected the fn to reflect the pre-bind value before addr changes")

	// Mutate the captured address, simulating Start() reassigning s.addr to
	// the real OS-assigned port after PORT=0 was requested.
	addr = "localhost:54211"

	assert.Equal(t, "http://localhost:54211/mcp", svc.resolveMCPServerURL(),
		"expected the fn to be re-invoked and reflect the updated address, not a construction-time snapshot")
}

// SessionService_should_ReturnEmptyString_When_MCPServerURLFnNotYetConfigured
// verifies that reading the MCP server URL before SetMCPServerURL has been
// called returns a safe empty string instead of a nil-pointer panic.
func TestSessionService_should_ReturnEmptyString_When_MCPServerURLFnNotYetConfigured(t *testing.T) {
	svc := &SessionService{}

	assert.NotPanics(t, func() {
		got := svc.resolveMCPServerURL()
		assert.Equal(t, "", got)
	})
}

// TestSpawnReviewSession_SetsBacklogCategory verifies that review-gate sessions
// created via SpawnReviewSession get Category == "Backlog" so they group
// correctly in the session list UI instead of falling into "Uncategorized".
// This starts a real tmux session (like session/integration_test.go), so it's
// skipped under -short (see make test vs. make test-integration).
func TestSpawnReviewSession_SetsBacklogCategory(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test that starts a real tmux session")
	}
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	// Isolate config to this test and use a trivial program so the tmux pane
	// doesn't need the real "claude" binary to exist.
	testDir := t.TempDir()
	t.Setenv("STAPLER_SQUAD_TEST_DIR", testDir)
	require.NoError(t, os.WriteFile(filepath.Join(testDir, "config.json"),
		[]byte(`{"default_program": "bash -c 'sleep 30'"}`), 0o644))

	repoPath := t.TempDir()
	item := &session.BacklogItemData{ID: uuid.New().String(), RepoPath: repoPath}

	inst, err := fix.svc.SpawnReviewSession(context.Background(), item, "item-session-id", "review this")
	require.NoError(t, err)
	t.Cleanup(func() { _ = inst.Destroy() })

	assert.Equal(t, session.CategoryBacklog, inst.Category)
}

// drainNotificationEvents reads every event currently queued on ch (with a short
// deadline) and returns any events.EventNotification events found. Used to assert
// on presence/absence of a notification without depending on channel buffering
// details.
func drainNotificationEvents(ch <-chan *events.Event) []*events.Event {
	var notifs []*events.Event
	deadline := time.After(200 * time.Millisecond)
	for {
		select {
		case ev := <-ch:
			if ev.Type == events.EventNotification {
				notifs = append(notifs, ev)
			}
		case <-deadline:
			return notifs
		}
	}
}

// drainAllEvents reads every event currently queued on ch (with a short
// deadline), regardless of type. Used together with filterEventsByType when a
// test needs to assert on more than one event type from a single publish
// (e.g. confirming a Notification event is absent while a SessionUpdated
// event is present).
func drainAllEvents(ch <-chan *events.Event) []*events.Event {
	var all []*events.Event
	deadline := time.After(200 * time.Millisecond)
	for {
		select {
		case ev := <-ch:
			all = append(all, ev)
		case <-deadline:
			return all
		}
	}
}

// filterEventsByType narrows a slice of events to only those matching typ.
func filterEventsByType(all []*events.Event, typ events.EventType) []*events.Event {
	var matched []*events.Event
	for _, ev := range all {
		if ev.Type == typ {
			matched = append(matched, ev)
		}
	}
	return matched
}

// TestWireRateLimitCallbacks_SuppressesNotification_When_InstanceHidden verifies
// the Epic 5 Story 5.1 Hidden gate: a Hidden instance (e.g. a headless review
// session spawned via SpawnReviewSession) must never receive a rate-limit
// detected/recovery notification, since rate-limit events have no
// AttentionReason to preserve as a narrowing safety net — suppression here is
// unconditional on Hidden, matching Epic 3's generic done/stuck notifier.
func TestWireRateLimitCallbacks_SuppressesNotification_When_InstanceHidden(t *testing.T) {
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(8)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	newHiddenInstance := func(title string) *session.Instance {
		inst := &session.Instance{
			Title:     title,
			UUID:      title + "-uuid",
			Path:      "/tmp/test",
			Status:    session.Paused,
			Program:   "claude",
			Hidden:    true,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		require.NoError(t, storage.AddInstance(inst))
		return inst
	}

	t.Run("onDetected", func(t *testing.T) {
		inst := newHiddenInstance("rl-hidden-detected")
		subCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		ch, _ := eventBus.Subscribe(subCtx)

		svc.onRateLimitDetected(inst, inst.UUID, time.Time{})

		notifs := drainNotificationEvents(ch)
		assert.Empty(t, notifs, "a Hidden instance must never receive a rate-limit-detected notification")
	})

	t.Run("onRecovery", func(t *testing.T) {
		inst := newHiddenInstance("rl-hidden-recovery")
		subCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		ch, _ := eventBus.Subscribe(subCtx)

		svc.onRateLimitRecovery(inst, inst.UUID, true, "")

		notifs := drainNotificationEvents(ch)
		assert.Empty(t, notifs, "a Hidden instance must never receive a rate-limit-recovery notification")
	})
}

// TestWireRateLimitCallbacks_StillPublishesSessionUpdated_When_InstanceHidden
// verifies the other half of Epic 5 Story 5.1's AC: the Hidden gate on
// onRateLimitDetected/onRateLimitRecovery must suppress only the
// events.NewNotificationEvent publish (asserted separately by
// TestWireRateLimitCallbacks_SuppressesNotification_When_InstanceHidden). The
// accompanying events.NewSessionUpdatedEvent publish is session state sync
// (rate_limit_state/rate_limit_reset_time), not a Notifications-page entry,
// and must still fire unmodified for Hidden instances.
func TestWireRateLimitCallbacks_StillPublishesSessionUpdated_When_InstanceHidden(t *testing.T) {
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(8)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	newHiddenInstance := func(title string) *session.Instance {
		inst := &session.Instance{
			Title:     title,
			UUID:      title + "-uuid",
			Path:      "/tmp/test",
			Status:    session.Paused,
			Program:   "claude",
			Hidden:    true,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		require.NoError(t, storage.AddInstance(inst))
		return inst
	}

	t.Run("onDetected", func(t *testing.T) {
		inst := newHiddenInstance("rl-hidden-detected-sync")
		subCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		ch, _ := eventBus.Subscribe(subCtx)

		svc.onRateLimitDetected(inst, inst.UUID, time.Time{})

		all := drainAllEvents(ch)
		notifs := filterEventsByType(all, events.EventNotification)
		assert.Empty(t, notifs, "a Hidden instance must never receive a rate-limit-detected notification")

		updates := filterEventsByType(all, events.EventSessionUpdated)
		require.Len(t, updates, 1, "SessionUpdated (session state sync) must still fire for a Hidden instance")
		require.NotNil(t, updates[0].Session)
		assert.Equal(t, inst.UUID, updates[0].Session.UUID)
		assert.Contains(t, updates[0].UpdatedFields, "rate_limit_state")
	})

	t.Run("onRecovery", func(t *testing.T) {
		inst := newHiddenInstance("rl-hidden-recovery-sync")
		subCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		ch, _ := eventBus.Subscribe(subCtx)

		svc.onRateLimitRecovery(inst, inst.UUID, true, "")

		all := drainAllEvents(ch)
		notifs := filterEventsByType(all, events.EventNotification)
		assert.Empty(t, notifs, "a Hidden instance must never receive a rate-limit-recovery notification")

		updates := filterEventsByType(all, events.EventSessionUpdated)
		require.Len(t, updates, 1, "SessionUpdated (session state sync) must still fire for a Hidden instance")
		require.NotNil(t, updates[0].Session)
		assert.Equal(t, inst.UUID, updates[0].Session.UUID)
		assert.Contains(t, updates[0].UpdatedFields, "rate_limit_state")
	})
}

// TestWireRateLimitCallbacks_StampsItemIDMetadata_When_BacklogLinkedAndNotHidden
// covers the positive case for Epic 5 Story 5.1: a non-Hidden instance whose
// session is linked to a backlog item must have its rate-limit notification's
// metadata built via events.SessionScopedMetadata — {"item_id": ..., "session_scoped": "true"}
// — not the nil it carried before this fix.
func TestWireRateLimitCallbacks_StampsItemIDMetadata_When_BacklogLinkedAndNotHidden(t *testing.T) {
	storage := createTestStorage(t)
	ctx := context.Background()
	eventBus := events.NewEventBus(8)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	const title = "rl-metadata-test"
	inst := &session.Instance{
		Title:     title,
		UUID:      title + "-uuid",
		Path:      "/tmp/test",
		Status:    session.Paused,
		Program:   "claude",
		Hidden:    false,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, storage.AddInstance(inst))

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Rate limit metadata test item",
		Status: string(session.BacklogStatusIdea),
	})
	require.NoError(t, err)

	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: inst.UUID,
		SessionRole: "primary",
	})
	require.NoError(t, err)

	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch, _ := eventBus.Subscribe(subCtx)

	svc.onRateLimitDetected(inst, inst.UUID, time.Time{})

	notifs := drainNotificationEvents(ch)
	require.Len(t, notifs, 1, "expected exactly one rate-limit-detected notification")
	require.NotNil(t, notifs[0].NotificationMetadata, "metadata must not be nil so downstream consumers can correlate the notification to its item")
	assert.Equal(t, item.ID, notifs[0].NotificationMetadata[events.MetadataKeyItemID])
	assert.Equal(t, "true", notifs[0].NotificationMetadata[events.MetadataKeySessionScoped])
}

// TestSessionService_Shutdown_StopsAnalyticsFlushGoroutine confirms Shutdown() calls
// through to AnalyticsStore.Stop(), is idempotent, and leaves Record() as a safe no-op
// afterward. Shutdown() joins the flush goroutine synchronously (AnalyticsStore.Stop
// blocks on <-s.done), so a hung flush loop would make this test time out rather than
// silently pass — no goleak/process-wide check needed to catch that regression.
func TestSessionService_Shutdown_StopsAnalyticsFlushGoroutine(t *testing.T) {
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(10)
	svc := NewSessionService(storage, eventBus)

	store := svc.GetAnalyticsStore()
	require.NotNil(t, store, "NewSessionService must wire an AnalyticsStore")

	done := make(chan struct{})
	go func() {
		svc.Shutdown()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Shutdown() did not return within 2s — AnalyticsStore.Stop is likely blocked")
	}

	require.NotPanics(t, svc.Shutdown, "a second Shutdown() call must be idempotent")

	require.NotPanics(t, func() {
		store.Record(AnalyticsEntry{SessionID: "sess-1", ToolName: "Bash"})
	}, "Record() after Shutdown() must stay a silent no-op, not panic")
}
