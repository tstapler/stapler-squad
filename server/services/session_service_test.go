package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/creack/pty"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/classifier"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/detection/binaries"
	"github.com/tstapler/stapler-squad/session/tmux"
	"go.uber.org/goleak"
)

// createTestStorage creates a test storage backed by an in-memory Ent
// repository via session.NewTestEntRepository — see that function's doc
// comment for why this uses a named, shared-cache in-memory DSN rather than
// a bare ":memory:" literal.
//
// This package's fixtures are called 300+ times across its test files; each
// call still pays the full Ent schema-creation + backfill-migration cost,
// but backing it with an in-memory SQLite database (rather than a
// t.TempDir()-backed file) removes the disk I/O and WAL fsync overhead.
func createTestStorage(t *testing.T) *session.Storage {
	t.Helper()

	repo := session.NewTestEntRepository(t)

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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// TestShutdown_WaitsForDeleteSessionCleanup_LiveInstanceNil verifies that
// Shutdown blocks until DeleteSession's background cleanup goroutine (the
// KillTmuxSessionByTitle fallback used when FindLiveInstance returns nil) has
// actually finished, rather than returning while it is still running.
//
// The DeleteSession call below exercises the real KillTmuxSessionByTitle
// branch, but that goroutine finishes almost instantly against a non-real
// tmux session — a fast-finishing real cleanup goroutine looks the same
// whether or not Shutdown actually blocks on it, and goroutine-count
// before/after comparisons in a shared test binary are flaky by construction
// (see go.uber.org/goleak's rationale). So this test additionally tracks a
// second, artificially slow cleanup goroutine (same technique as
// TestShutdown_BlocksUntilTrackedCleanupCompletes) on the same deleteCleanupWG
// that DeleteSession's goroutine was tracked on, and asserts it has already
// finished by the time Shutdown returns — a check that fails against the
// pre-fix buggy code — plus goleak.VerifyNone to confirm nothing was leaked.
func TestShutdown_WaitsForDeleteSessionCleanup_LiveInstanceNil(t *testing.T) {
	// Not t.Parallel(): this test's goleak.IgnoreCurrent()/VerifyNone() baseline
	// can be polluted by other parallel tests' own background goroutines (e.g.
	// database/sql connection-pool cleaners from their own createTestStorage
	// pools) starting mid-flight and getting misattributed as "leaked" by this
	// test. See withShrunkIdleSettleTimers's identical rationale in
	// session/autonomous_driver_test.go.
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)

	require.NoError(t, storage.AddInstance(&session.Instance{
		Title:     "no-live-instance",
		Path:      "/tmp/test",
		Status:    session.Paused,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}))

	baseline := goleak.IgnoreCurrent()

	// No ReviewQueuePoller wired: FindLiveInstance returns nil, so DeleteSession
	// takes the KillTmuxSessionByTitle fallback branch.
	resp, err := svc.DeleteSession(context.Background(), connect.NewRequest(&sessionv1.DeleteSessionRequest{
		Id: "no-live-instance",
	}))
	require.NoError(t, err)
	require.True(t, resp.Msg.Success)

	var mu sync.Mutex
	finished := false
	svc.trackCleanup(func() {
		time.Sleep(200 * time.Millisecond)
		mu.Lock()
		finished = true
		mu.Unlock()
	})

	svc.Shutdown()

	mu.Lock()
	assert.True(t, finished, "Shutdown returned before the tracked cleanup goroutine finished")
	mu.Unlock()

	goleak.VerifyNone(t, baseline)
}

// TestShutdown_WaitsForDeleteSessionCleanup_LiveInstancePresent is the
// liveInst-present counterpart to TestShutdown_WaitsForDeleteSessionCleanup_LiveInstanceNil:
// it wires a ReviewQueuePoller with a live instance so FindLiveInstance returns
// non-nil and DeleteSession takes the liveInst.Destroy() branch (which runs
// waitForDestroyLoggingSlowCleanup — see TestWaitForDestroyLoggingSlowCleanup_*
// for direct coverage of that helper's own timeout/non-abandoning behavior),
// then uses the same artificial-delay + goleak technique to verify Shutdown
// still blocks until tracked cleanup exits and nothing leaks.
func TestShutdown_WaitsForDeleteSessionCleanup_LiveInstancePresent(t *testing.T) {
	// Not t.Parallel(): see TestShutdown_WaitsForDeleteSessionCleanup_LiveInstanceNil's
	// goleak-baseline-pollution rationale above.
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)

	testInst := &session.Instance{
		Title:     "live-instance",
		Path:      "/tmp/test",
		Status:    session.Paused,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, storage.AddInstance(testInst))

	queue := session.NewReviewQueue()
	statusMgr := session.NewInstanceStatusManager()
	poller := session.NewReviewQueuePoller(queue, statusMgr, nil)
	poller.SetInstances([]*session.Instance{testInst})
	svc.SetReviewQueuePoller(poller)

	baseline := goleak.IgnoreCurrent()

	resp, err := svc.DeleteSession(context.Background(), connect.NewRequest(&sessionv1.DeleteSessionRequest{
		Id: "live-instance",
	}))
	require.NoError(t, err)
	require.True(t, resp.Msg.Success)

	var mu sync.Mutex
	finished := false
	svc.trackCleanup(func() {
		time.Sleep(200 * time.Millisecond)
		mu.Lock()
		finished = true
		mu.Unlock()
	})

	svc.Shutdown()

	mu.Lock()
	assert.True(t, finished, "Shutdown returned before the tracked cleanup goroutine finished")
	mu.Unlock()

	goleak.VerifyNone(t, baseline)
}

// TestShutdown_BlocksUntilTrackedCleanupCompletes proves Shutdown's blocking
// contract directly: it tracks a cleanup goroutine that sleeps for a fixed
// duration before recording completion, then asserts that completion is
// already recorded by the time Shutdown returns. Unlike the goroutine-count
// comparisons above (which pass identically whether or not Shutdown actually
// blocks, since a fast-finishing goroutine looks the same either way — see
// the deleteCleanupWG.Wait() removal check in git history for this repo's own
// confirmation of that gap), this test fails if deleteCleanupWG.Wait() is
// ever removed or short-circuited from Shutdown.
func TestShutdown_BlocksUntilTrackedCleanupCompletes(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)

	var mu sync.Mutex
	finished := false
	svc.trackCleanup(func() {
		time.Sleep(200 * time.Millisecond)
		mu.Lock()
		finished = true
		mu.Unlock()
	})

	svc.Shutdown()

	mu.Lock()
	defer mu.Unlock()
	assert.True(t, finished, "Shutdown returned before the tracked cleanup goroutine finished")
}

// TestDestroyWithTimeout_ReturnsTimeoutError_When_WorkExceedsTimeout exercises
// destroyWithTimeout's timeout-exceeded branch, which had zero coverage: it
// passes a callback that deliberately sleeps far longer than the configured
// timeout and asserts destroyWithTimeout returns a timeout error at
// approximately the timeout, not after waiting for the full work duration.
// destroyWithTimeout takes destroy as a func() error (rather than a
// *session.Instance) specifically so this can be tested without needing a
// real Instance whose Destroy() can be made to hang on demand — see
// destroyWithTimeout's doc comment.
func TestDestroyWithTimeout_ReturnsTimeoutError_When_WorkExceedsTimeout(t *testing.T) {
	t.Parallel()
	const testTimeout = 20 * time.Millisecond
	const workDuration = 300 * time.Millisecond

	workDone := make(chan struct{})
	start := time.Now()
	err := destroyWithTimeout(func() error {
		time.Sleep(workDuration)
		close(workDone)
		return nil
	}, testTimeout)
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
	assert.Lessf(t, elapsed, workDuration,
		"destroyWithTimeout took %s, should have returned at ~%s (the configured timeout) rather than waiting for the full %s of work",
		elapsed, testTimeout, workDuration)

	// Drain the background goroutine so it doesn't outlive the test (it is
	// intentionally untracked past the timeout — see destroyWithTimeout's doc
	// comment — but leaving it running would otherwise trip goleak in other
	// tests in this package that run concurrently or immediately after).
	<-workDone
}

// TestWaitForDestroyLoggingSlowCleanup_DoesNotAbandonWorkAfterTimeout exercises
// DeleteSession's actual liveInst-cleanup timeout logic (extracted into
// waitForDestroyLoggingSlowCleanup so it's directly testable — see that
// function's doc comment). Unlike destroyWithTimeout, this must NOT abandon
// the work at the timeout: it fires onSlow once the timeout elapses but keeps
// waiting for the real result, so the returned error (and elapsed time)
// reflect the full work duration, not the timeout.
func TestWaitForDestroyLoggingSlowCleanup_DoesNotAbandonWorkAfterTimeout(t *testing.T) {
	const testTimeout = 20 * time.Millisecond
	const workDuration = 150 * time.Millisecond

	var mu sync.Mutex
	onSlowCalled := false

	workDone := make(chan struct{})
	start := time.Now()
	err := waitForDestroyLoggingSlowCleanup(func() error {
		time.Sleep(workDuration)
		close(workDone)
		return nil
	}, testTimeout, func() {
		mu.Lock()
		onSlowCalled = true
		mu.Unlock()
	})
	elapsed := time.Since(start)

	require.NoError(t, err)

	mu.Lock()
	assert.True(t, onSlowCalled, "onSlow should fire once the timeout elapses")
	mu.Unlock()

	assert.GreaterOrEqualf(t, elapsed, workDuration,
		"waitForDestroyLoggingSlowCleanup returned after %s, should have waited for the full %s of work instead of abandoning it at the %s timeout",
		elapsed, workDuration, testTimeout)

	select {
	case <-workDone:
	default:
		t.Fatal("destroy work should already be complete by the time waitForDestroyLoggingSlowCleanup returned")
	}
}

// TestWaitForDestroyLoggingSlowCleanup_NoTimeout_OnSlowNeverCalled is the
// fast-path counterpart: when destroy() finishes before timeout, onSlow must
// never fire and the real error/result is returned directly.
func TestWaitForDestroyLoggingSlowCleanup_NoTimeout_OnSlowNeverCalled(t *testing.T) {
	var mu sync.Mutex
	onSlowCalled := false

	sentinel := errors.New("destroy failed")
	err := waitForDestroyLoggingSlowCleanup(func() error {
		return sentinel
	}, time.Second, func() {
		mu.Lock()
		onSlowCalled = true
		mu.Unlock()
	})

	require.ErrorIs(t, err, sentinel)
	mu.Lock()
	assert.False(t, onSlowCalled, "onSlow must not fire when destroy() finishes before the timeout")
	mu.Unlock()
}

// TestDeleteSession_LiveInstance_LogsWarningOnSlowCleanupButStillWaits wires
// waitForDestroyLoggingSlowCleanup's timeout branch through the actual
// DeleteSession RPC path (via SetDeleteSessionCleanupTimeout, the test-only
// override — see defaultDeleteSessionCleanupTimeout's doc comment) and
// asserts the expected warning is logged, then that Shutdown still blocks
// until the real cleanup goroutine finishes and nothing leaks.
func TestDeleteSession_LiveInstance_LogsWarningOnSlowCleanupButStillWaits(t *testing.T) {
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)

	// A near-zero timeout guarantees the slow-cleanup branch fires for any
	// real Destroy(), however fast, without needing a genuinely slow instance.
	svc.SetDeleteSessionCleanupTimeout(time.Nanosecond)

	testInst := &session.Instance{
		Title:     "live-instance-slow-cleanup",
		Path:      "/tmp/test",
		Status:    session.Paused,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, storage.AddInstance(testInst))

	queue := session.NewReviewQueue()
	statusMgr := session.NewInstanceStatusManager()
	poller := session.NewReviewQueuePoller(queue, statusMgr, nil)
	poller.SetInstances([]*session.Instance{testInst})
	svc.SetReviewQueuePoller(poller)

	baseline := goleak.IgnoreCurrent()
	buf := captureLogs(t)

	resp, err := svc.DeleteSession(context.Background(), connect.NewRequest(&sessionv1.DeleteSessionRequest{
		Id: "live-instance-slow-cleanup",
	}))
	require.NoError(t, err)
	require.True(t, resp.Msg.Success)

	svc.Shutdown()

	assert.Contains(t, buf.String(), "session cleanup still running in background after timeout",
		"DeleteSession's liveInst branch should log the slow-cleanup warning when deleteSessionCleanupTimeout elapses")

	goleak.VerifyNone(t, baseline)
}

// TestDeleteSession_StorageDeletedBeforeResponse verifies that storage is fully
// committed before the RPC response is returned, so any immediate listSessions
// call from a reconnecting client sees the session as gone. This is the core
// contract the frontend tombstone fix relies on.
func TestDeleteSession_StorageDeletedBeforeResponse(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// TestUpdateSession_should_StopSession_When_TargetIsStoppedFromActive verifies the
// board-view "drag into Complete" path: targeting SESSION_STATUS_STOPPED from an
// Active instance calls StopByUser() and lands the session in Stopped.
func TestUpdateSession_should_StopSession_When_TargetIsStoppedFromActive(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	inst := &session.Instance{
		Title:       "active-session-to-stop",
		Path:        "/tmp/test",
		Status:      session.Active,
		Program:     "claude",
		Permissions: session.GetManagedPermissions(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	addInstanceToPoller(fix.poller, inst)

	stopped := sessionv1.SessionStatus_SESSION_STATUS_STOPPED
	resp, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:     "active-session-to-stop",
		Status: &stopped,
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Session)
	assert.Equal(t, sessionv1.SessionStatus_SESSION_STATUS_STOPPED, resp.Msg.Session.Status)
}

// TestUpdateSession_should_RejectStop_When_TransitionIsIllegal verifies that an
// illegal target (Restoring -> Stopped, which has no entry in transitionDefs) is
// classified as FailedPrecondition, not CodeInternal.
func TestUpdateSession_should_RejectStop_When_TransitionIsIllegal(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	inst := &session.Instance{
		Title:       "restoring-session",
		Path:        "/tmp/test",
		Status:      session.Restoring,
		Program:     "claude",
		Permissions: session.GetManagedPermissions(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	addInstanceToPoller(fix.poller, inst)

	stopped := sessionv1.SessionStatus_SESSION_STATUS_STOPPED
	_, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:     "restoring-session",
		Status: &stopped,
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
	t.Parallel()
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
	t.Parallel()
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
// branch. Wires a real, pipe-backed *session.ClaudeController (see
// newRealControllerForTest) so this exercises an actual successful
// SendCommandImmediate call end-to-end through UpdateSession.
//
// Prior to Story 1.1.2 this test used an instance with no controller wired at
// all and asserted success — that assertion documented the swallowed-error
// bug Story 1.1.2 fixes (a nil controller now returns an error; see
// TestUpdateSession_SteerMessage_AutonomousNilController_NowReturnsError).
func TestUpdateSession_SteerMessage_AutonomousSession_StillUsesController(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	ctrl := newRealControllerForTest(t, "autonomous-session", true)

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
	inst.SetControllerForTest(ctrl)
	addInstanceToPoller(fix.poller, inst)

	msg := "focus on the auth module first"
	resp, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:           "autonomous-session",
		SteerMessage: &msg,
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Session)
}

// TestUpdateSession_SteerMessage_AutonomousNilController_NowReturnsError
// documents Story 1.1.2's intentional behavior change: an autonomous session
// with no controller started now surfaces a real RPC error instead of
// silently succeeding (the pre-1.1.2 behavior asserted by the previous
// version of TestUpdateSession_SteerMessage_AutonomousSession_StillUsesController).
func TestUpdateSession_SteerMessage_AutonomousNilController_NowReturnsError(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	inst := &session.Instance{
		Title:          "autonomous-no-controller",
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
	_, err := fix.svc.UpdateSession(context.Background(), connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:           "autonomous-no-controller",
		SteerMessage: &msg,
	}))
	require.Error(t, err)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeFailedPrecondition, connectErr.Code())
}

// --------------------------------------------------------------------------
// steerInstance — autonomous branch error/timeout paths (Story 1.1.2)
// --------------------------------------------------------------------------

// fakeControllerInstance is a minimal session.InstanceContext double that lets
// these tests construct a real *session.ClaudeController without a real PTY
// subprocess (see newRealControllerForTest).
type fakeControllerInstance struct {
	title     string
	ptyReader *os.File
}

func (f *fakeControllerInstance) GetTitle() string                    { return f.title }
func (f *fakeControllerInstance) GetStableID() string                 { return f.title }
func (f *fakeControllerInstance) GetPTYReader() (*os.File, error)     { return f.ptyReader, nil }
func (f *fakeControllerInstance) Preview() (string, error)            { return "", nil }
func (f *fakeControllerInstance) LastMeaningfulOutputTime() time.Time { return time.Time{} }
func (f *fakeControllerInstance) GetCreatedAt() time.Time             { return time.Time{} }
func (f *fakeControllerInstance) SetLastMeaningfulOutput(_ time.Time) {}
func (f *fakeControllerInstance) GetStatus() int                      { return 0 }
func (f *fakeControllerInstance) WriteToPTY(data []byte) (int, error) { return len(data), nil }
func (f *fakeControllerInstance) GetProgram() string                  { return "claude" }

// newRealControllerForTest builds a real *session.ClaudeController backed by
// a genuine pty pair (github.com/creack/pty, already a project dependency),
// without requiring a subprocess fork (which this sandboxed test environment
// does not permit; see
// TestUpdateSession_SteerMessage_NonAutonomousSession_SendsViaSendKeys's PTY skip
// above for the same constraint on the non-autonomous branch).
//
// cancelImmediately controls the two distinct scenarios these tests need:
//
//   - true: the controller is started against an already-cancelled context.
//     SendCommandImmediate's underlying executeCommand loop then observes
//     ctx.Done() as the only ready select case on its very first iteration —
//     deterministic, no real PTY-timing race — giving a fast, genuine
//     "success" outcome (steerInstance's autonomous branch treats a nil
//     SendCommandImmediate error as success regardless of the
//     *ExecutionResult's own internal Error field; see ExecuteImmediate in
//     session/command_executor.go). An earlier version of this helper
//     instead closed the slave fd immediately to force an EOF/hangup on the
//     master read — that raced the kernel's hangup delivery against
//     steerInstance's 5s bound and flaked under -count=5. Used by
//     TestUpdateSession_SteerMessage_AutonomousSession_StillUsesController.
//   - false: the context is left live and the slave stays open for the whole
//     test body (closed only in t.Cleanup, after assertions already ran), so
//     nothing ever completes the read/response cycle and SendCommandImmediate
//     genuinely blocks. Used by
//     TestSteerInstance_AutonomousSendCommandImmediateHangs_
//     TimesOutRatherThanBlockingForever.
//
// Deliberately never calls ctrl.Stop(): confirmed via a goroutine dump
// (test timeout with -timeout 40s) that ResponseStream's background read
// loop blocks in a genuine syscall.Read on this fd with no effective
// deadline in this sandboxed environment (PTYAccess's SetReadDeadline call
// discards its error and silently no-ops here), so Stop()'s
// rs.wg.Wait() — which joins that goroutine — hangs forever and takes the
// whole test binary down with it. Skipping Stop() leaks one background
// goroutine and one open pty pair for the remaining lifetime of the test
// process, which the OS reclaims on exit; it does not block go test's own
// completion and does not trip this file's goleak.VerifyNone checks (their
// baselines are captured after this goroutine already exists). This leak is
// harmless in the cancelImmediately=true case since streamLoop's own select
// also sees ctx.Done() as its only ready case and exits on its own almost
// immediately — nothing is actually left running there.
func newRealControllerForTest(t *testing.T, title string, cancelImmediately bool) *session.ClaudeController {
	t.Helper()
	master, slave, err := pty.Open()
	if err != nil {
		t.Skipf("pty not available in this environment: %v", err)
	}
	t.Cleanup(func() { _ = slave.Close() })

	ctx := context.Background()
	if cancelImmediately {
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(ctx)
		cancel()
	}

	ctrl, err := session.NewClaudeController(&fakeControllerInstance{title: title, ptyReader: master})
	require.NoError(t, err)
	require.NoError(t, ctrl.Start(ctx))
	return ctrl
}

// TestSteerInstance_AutonomousNilController_ReturnsError verifies the fixed
// autonomous branch returns a real error (not a swallowed log.Warn) when
// GetController() is nil.
func TestSteerInstance_AutonomousNilController_ReturnsError(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	inst := &session.Instance{
		Title:          "autonomous-nil-controller",
		AutonomousMode: true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	err := fix.svc.steerInstance(context.Background(), inst, "fix the conflict")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "controller not started")

	var connectErr *connect.Error
	assert.False(t, errors.As(err, &connectErr), "steerInstance must never return a connect.Error — UpdateSession alone translates to a connect.Code")
}

// TestSteerInstance_AutonomousSendCommandImmediateError_ReturnsError verifies
// the fixed autonomous branch returns SendCommandImmediate's own error
// (wrapped with context) instead of swallowing it — a real, non-nil
// *session.ClaudeController that was never Start()ed reproduces this
// deterministically (SendCommandImmediate's own "controller not started"
// check), without needing a PTY.
func TestSteerInstance_AutonomousSendCommandImmediateError_ReturnsError(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	ctrl, err := session.NewClaudeController(&fakeControllerInstance{title: "autonomous-erroring-controller"})
	require.NoError(t, err)

	inst := &session.Instance{
		Title:          "autonomous-erroring-controller",
		AutonomousMode: true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	inst.SetControllerForTest(ctrl)

	steerErr := fix.svc.steerInstance(context.Background(), inst, "fix the conflict")
	require.Error(t, steerErr)
	assert.Contains(t, steerErr.Error(), "steer autonomous session")

	var connectErr *connect.Error
	assert.False(t, errors.As(steerErr, &connectErr), "steerInstance must never return a connect.Error")
}

// TestSteerInstance_AutonomousSendCommandImmediateHangs_TimesOutRatherThanBlockingForever
// is the regression test for the unbounded-PTY-write bug: a wedged controller
// (real, started, but nothing ever drains/responds on its pty) must not be
// able to hang steerInstance forever — it must return within ~5s wrapping
// context.DeadlineExceeded.
func TestSteerInstance_AutonomousSendCommandImmediateHangs_TimesOutRatherThanBlockingForever(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	ctrl := newRealControllerForTest(t, "autonomous-wedged-controller", false)

	inst := &session.Instance{
		Title:          "autonomous-wedged-controller",
		AutonomousMode: true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	inst.SetControllerForTest(ctrl)

	type result struct {
		err error
	}
	done := make(chan result, 1)
	start := time.Now()
	go func() {
		done <- result{err: fix.svc.steerInstance(context.Background(), inst, "fix the conflict")}
	}()

	select {
	case r := <-done:
		elapsed := time.Since(start)
		require.Error(t, r.err)
		assert.True(t, errors.Is(r.err, context.DeadlineExceeded), "expected error to wrap context.DeadlineExceeded, got: %v", r.err)
		assert.Less(t, elapsed, 10*time.Second, "steerInstance must bound the PTY write to ~5s, not block indefinitely")
	case <-time.After(15 * time.Second):
		t.Fatal("steerInstance did not return within 15s — the PTY write timeout did not fire")
	}
}

// --------------------------------------------------------------------------
// SessionSteerer (Story 1.2.1) — SessionProgram / SteerActiveSession
// --------------------------------------------------------------------------
//
// validation.md flags these as a coverage gap: plan.md's Tasks 1.2.1a-d name
// only a compile-time interface assertion (var _ SessionSteerer =
// (*SessionService)(nil)) for SessionProgram/SteerActiveSession, with no
// direct Test... function for their own return-value behavior. Added here
// per validation.md's explicit recommendation.

// TestSessionService_SessionProgram_ReturnsProgramForLiveInstance verifies
// SessionProgram returns (instance.Program, true) for a live instance.
func TestSessionService_SessionProgram_ReturnsProgramForLiveInstance(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	inst := &session.Instance{
		Title:       "program-lookup-session",
		Program:     "claude",
		Permissions: session.GetManagedPermissions(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	addInstanceToPoller(fix.poller, inst)

	program, ok := fix.svc.SessionProgram("program-lookup-session")
	assert.True(t, ok)
	assert.Equal(t, "claude", program)
}

// TestSessionService_SessionProgram_ReturnsNotOkForUnknownUUID verifies
// SessionProgram returns ("", false) when no live instance is tracked for
// the given UUID.
func TestSessionService_SessionProgram_ReturnsNotOkForUnknownUUID(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	program, ok := fix.svc.SessionProgram("no-such-session")
	assert.False(t, ok)
	assert.Empty(t, program)
}

// TestSessionService_SteerActiveSession_ReturnsErrorForUnknownUUID verifies
// SteerActiveSession returns a non-nil error (and does not panic) when no
// live instance is tracked for the given UUID.
func TestSessionService_SteerActiveSession_ReturnsErrorForUnknownUUID(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	err := fix.svc.SteerActiveSession(context.Background(), "missing-uuid", "hello")
	require.Error(t, err)
}

// --------------------------------------------------------------------------
// IsReadyForSteer (PR #645 Gate 2 P1) — never assume ready when the
// detection signal is unavailable.
// --------------------------------------------------------------------------

// TestSessionService_IsReadyForSteer_ReturnsFalseForUnknownUUID verifies the
// not-live case degrades to false (matches SessionProgram's ok=false shape).
func TestSessionService_IsReadyForSteer_ReturnsFalseForUnknownUUID(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	assert.False(t, fix.svc.IsReadyForSteer("no-such-session"))
}

// TestSessionService_IsReadyForSteer_ReturnsFalse_When_StatusManagerNotWired
// verifies that a live instance with no statusManager wired (so readiness
// genuinely can't be determined) returns false rather than assuming ready.
func TestSessionService_IsReadyForSteer_ReturnsFalse_When_StatusManagerNotWired(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	inst := &session.Instance{
		Title:       "no-status-manager-session",
		Program:     "claude",
		Permissions: session.GetManagedPermissions(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	addInstanceToPoller(fix.poller, inst)

	assert.False(t, fix.svc.IsReadyForSteer("no-status-manager-session"))
}

// TestSessionService_IsReadyForSteer_ReturnsFalse_When_ControllerNotRegistered
// verifies the "unknown detection signal" case: a statusManager is wired but
// no controller was ever registered for this instance's title (e.g. it never
// started one). This must degrade to false, not true — the vulnerability
// this method exists to close is exactly a PTY write with no confirmed idle
// signal.
func TestSessionService_IsReadyForSteer_ReturnsFalse_When_ControllerNotRegistered(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)
	fix.svc.SetStatusManager(session.NewInstanceStatusManager())

	inst := &session.Instance{
		Title:       "no-controller-session",
		Program:     "claude",
		Permissions: session.GetManagedPermissions(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	addInstanceToPoller(fix.poller, inst)

	assert.False(t, fix.svc.IsReadyForSteer("no-controller-session"))
}

// TestSafeIdleStatusContexts_MatchClaudeIdlePatternDescriptions pins
// safeIdleStatusContexts against the actual Idle pattern descriptions in
// session/detection/binaries/claude.go (the single source of truth per that
// file's Patterns() doc comment) — both that the three safe entries match
// verbatim, and that the allowlist contains nothing else. A future wording
// change to claude.go's Idle patterns fails this test loudly instead of
// silently breaking IsReadyForSteer's security gate (either by no longer
// matching a real safe prompt, or by an unnoticed unsafe pattern reusing one
// of the pinned strings).
func TestSafeIdleStatusContexts_MatchClaudeIdlePatternDescriptions(t *testing.T) {
	t.Parallel()

	idlePatterns := binaries.NewClaudeDetector().Patterns().Idle
	byName := make(map[string]string, len(idlePatterns))
	for _, p := range idlePatterns {
		byName[p.Name] = p.Description
	}

	wantSafe := map[string]string{
		"claude_readline_prompt":  "Claude Code readline input prompt",
		"claude_shortcuts_prompt": "Claude Code idle prompt showing ? for shortcuts",
		"claude_accept_edits":     "Claude Code 'accept edits' review mode — session completed turn, user reviews proposed changes",
	}
	for name, wantDesc := range wantSafe {
		gotDesc, ok := byName[name]
		require.True(t, ok, "claude.go's Idle group no longer has a pattern named %q", name)
		require.Equal(t, wantDesc, gotDesc, "claude.go's %q description changed", name)
		require.True(t, safeIdleStatusContexts[gotDesc], "safeIdleStatusContexts missing current %q description %q", name, gotDesc)
	}
	require.Len(t, safeIdleStatusContexts, len(wantSafe), "safeIdleStatusContexts has entries beyond the three pinned safe patterns")

	// insert_mode is deliberately excluded (ambiguous with a real vim INSERT-mode
	// status line) and the raw shell/vim/editor prompts must never be treated as safe.
	for _, unsafeName := range []string{"insert_mode", "command_prompt", "vim_normal_mode", "bracket_insert_mode"} {
		desc, ok := byName[unsafeName]
		require.True(t, ok, "claude.go's Idle group no longer has a pattern named %q", unsafeName)
		require.False(t, safeIdleStatusContexts[desc], "%q's description %q must not be on the safe allowlist", unsafeName, desc)
	}
}

// TestSessionService_IsReadyForSteer_ReturnsTrue_When_RealDetectionSeesClaudeReadlinePrompt
// runs realistic Claude Code idle-prompt terminal text through the real
// detection.PatternSet.MatchLines pipeline (built from the actual
// binaries.NewClaudeDetector() pattern set, not a hand-fabricated status) and
// verifies isSafeSteerStatus — the exact function IsReadyForSteer defers to —
// accepts the resulting (status, description) pair. Building a fully live
// *session.ClaudeController here would additionally require a real PTY and
// Start()'s background goroutines (see session/claude_controller_test.go's
// newControllerWithMock, which stays inside package session for the same
// reason); exercising the real regexes directly is the documented fallback.
func TestSessionService_IsReadyForSteer_ReturnsTrue_When_RealDetectionSeesClaudeReadlinePrompt(t *testing.T) {
	t.Parallel()

	ps, err := detection.NewPatternSet(binaries.NewClaudeDetector().Patterns())
	require.NoError(t, err)

	text := "I've fixed the null check in validator.go.\n> "
	status, name, desc, _ := ps.MatchLines(text, nil)

	require.Equal(t, detection.StatusIdle, status, "matched pattern: %s", name)
	assert.True(t, isSafeSteerStatus(status, desc), "description %q (pattern %s) should be treated as safe to steer", desc, name)
}

// TestSessionService_IsReadyForSteer_ReturnsFalse_When_RealDetectionSeesShellPrompt
// is the regression test for the original StatusReady-vs-StatusIdle
// confusion this fix addresses: a raw shell prompt also reports
// detection.StatusIdle, so a naive `ClaudeStatus == StatusIdle` check (the
// unsafe "fix" this test guards against) would incorrectly treat it as safe
// to steer. Only the description allowlist in isSafeSteerStatus tells the
// two apart.
func TestSessionService_IsReadyForSteer_ReturnsFalse_When_RealDetectionSeesShellPrompt(t *testing.T) {
	t.Parallel()

	ps, err := detection.NewPatternSet(binaries.NewClaudeDetector().Patterns())
	require.NoError(t, err)

	text := "tstapler@dev-box:~/stapler-squad$ "
	status, name, desc, _ := ps.MatchLines(text, nil)

	require.Equal(t, detection.StatusIdle, status, "matched pattern: %s", name)
	assert.False(t, isSafeSteerStatus(status, desc), "raw shell prompt description %q (pattern %s) must not be treated as safe to steer", desc, name)
}

// TestUpdateSession_SteerMessage_ExceedsMaxLength_ReturnsInvalidArgument verifies
// that a steer_message longer than session.MaxSteerMessageLength is rejected with
// InvalidArgument before any send is attempted, mirroring the Note field's
// length-cap guard (TestUpdateSession_NoteExceedsMaxLength_ReturnsInvalidArgument)
// — steer_message is a free-text entry point that now reaches ordinary
// work/review sessions, not just autonomous ones, so it needs the same cap.
func TestUpdateSession_SteerMessage_ExceedsMaxLength_ReturnsInvalidArgument(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
// CreateSession — backend resolution (tymux-bundled-integration Epic 4.3):
// wiring session.ResolveSessionBackend's precedence (explicit request
// override > TymuxSessionOverrides, keyed by the sanitized tmux session name
// > the process-wide registered default) into instanceOpts.Backend.
// --------------------------------------------------------------------------

// TestCreateSession_HonorsExplicitBackendOverride verifies that a request's
// backend_override field wins over both this session's own TymuxSessionOverrides
// entry and the process-wide default, mirroring ResolveSessionBackend's top
// precedence tier (session/backend_resolution_test.go).
func TestCreateSession_HonorsExplicitBackendOverride(t *testing.T) {
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)

	testDir := t.TempDir()
	t.Setenv("STAPLER_SQUAD_TEST_DIR", testDir)
	const title = "explicit-override-session"
	sessionKey := tmux.NewSessionName(title, tmux.TmuxPrefix).String()
	// The session's own override forces tmux; the request override must still win.
	require.NoError(t, os.WriteFile(filepath.Join(testDir, "config.json"),
		[]byte(`{"tymux_session_overrides": {"`+sessionKey+`": false}}`), 0o644))

	backendOverride := string(session.BackendTymux)
	resp, err := svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:           title,
		Path:            t.TempDir(),
		Program:         "claude",
		BackendOverride: &backendOverride,
	}))
	require.NoError(t, err)
	t.Cleanup(func() { destroyCreatedSession(t, svc, resp.Msg.Session.Id) })

	inst := svc.FindLiveInstance(resp.Msg.Session.Id)
	require.NotNil(t, inst)
	assert.Equal(t, session.BackendTymux, inst.Backend,
		"request backend_override must win even though this session's TymuxSessionOverrides entry forces tmux")
}

// TestCreateSession_HonorsSessionNameOverrideMap verifies that, absent a
// request override, a TymuxSessionOverrides entry keyed by the sanitized tmux
// session name (tmux.NewSessionName's derivation of the request Title — not
// the raw Title itself) forces the session's backend.
func TestCreateSession_HonorsSessionNameOverrideMap(t *testing.T) {
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)

	testDir := t.TempDir()
	t.Setenv("STAPLER_SQUAD_TEST_DIR", testDir)
	const title = "session-override-map-test"
	sessionKey := tmux.NewSessionName(title, tmux.TmuxPrefix).String()
	require.NoError(t, os.WriteFile(filepath.Join(testDir, "config.json"),
		[]byte(`{"tymux_session_overrides": {"`+sessionKey+`": true}}`), 0o644))

	resp, err := svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:   title,
		Path:    t.TempDir(),
		Program: "claude",
	}))
	require.NoError(t, err)
	t.Cleanup(func() { destroyCreatedSession(t, svc, resp.Msg.Session.Id) })

	inst := svc.FindLiveInstance(resp.Msg.Session.Id)
	require.NotNil(t, inst)
	assert.Equal(t, session.BackendTymux, inst.Backend,
		"a TymuxSessionOverrides entry keyed by the sanitized tmux session name must force the backend with no request override present")
}

// TestCreateSession_FallsBackToGlobalDefaultWhenNoOverrides verifies that with
// no request override and no TymuxSessionOverrides entry for this session,
// CreateSession applies the process-wide registered backend
// (session.RegisterBackendProvider) rather than a hardcoded BackendTmux.
func TestCreateSession_FallsBackToGlobalDefaultWhenNoOverrides(t *testing.T) {
	session.RegisterBackendProvider(session.BackendTymux)
	t.Cleanup(func() { session.RegisterBackendProvider(session.BackendTmux) })

	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)

	testDir := t.TempDir()
	t.Setenv("STAPLER_SQUAD_TEST_DIR", testDir)
	require.NoError(t, os.WriteFile(filepath.Join(testDir, "config.json"), []byte(`{}`), 0o644))

	resp, err := svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:   "no-override-session",
		Path:    t.TempDir(),
		Program: "claude",
	}))
	require.NoError(t, err)
	t.Cleanup(func() { destroyCreatedSession(t, svc, resp.Msg.Session.Id) })

	inst := svc.FindLiveInstance(resp.Msg.Session.Id)
	require.NotNil(t, inst)
	assert.Equal(t, session.BackendTymux, inst.Backend,
		"with no request override and no session-name override, CreateSession must apply the process-wide registered default")
}

// --------------------------------------------------------------------------
// CreateDirectorySession / CreateWorktreeSession — backend resolution
// (tymux-bundled-integration Epic 4.4.2): these internal session creators
// have no per-request override field, so they wire session.ResolveSessionBackend
// with requestOverride="" -- only the session-name override map (or the
// process-wide default) can influence their Backend.
// --------------------------------------------------------------------------

// TestCreateDirectorySession_HonorsSessionNameOverrideMap verifies that a
// TymuxSessionOverrides entry keyed by the sanitized tmux session name
// (tmux.NewSessionName's derivation of the title passed to
// CreateDirectorySession) forces the resulting instance's backend even
// though the process-wide default is registered as tymux.
func TestCreateDirectorySession_HonorsSessionNameOverrideMap(t *testing.T) {
	session.RegisterBackendProvider(session.BackendTymux)
	t.Cleanup(func() { session.RegisterBackendProvider(session.BackendTmux) })

	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)

	testDir := t.TempDir()
	t.Setenv("STAPLER_SQUAD_TEST_DIR", testDir)
	const title = "directory-session-override-map-test"
	sessionKey := tmux.NewSessionName(title, tmux.TmuxPrefix).String()
	require.NoError(t, os.WriteFile(filepath.Join(testDir, "config.json"),
		[]byte(`{"default_program": "claude", "tymux_session_overrides": {"`+sessionKey+`": false}}`), 0o644))

	inst, err := svc.CreateDirectorySession(context.Background(), title, t.TempDir(), "", nil, true, false)
	require.NoError(t, err)
	t.Cleanup(func() { _ = inst.Destroy() })

	assert.Equal(t, session.BackendTmux, inst.Backend,
		"a TymuxSessionOverrides entry keyed by the sanitized tmux session name must force the backend even though the process-wide default is tymux")
}

// TestCreateWorktreeSession_HonorsSessionNameOverrideMap is the
// CreateWorktreeSession analogue of the CreateDirectorySession test above.
func TestCreateWorktreeSession_HonorsSessionNameOverrideMap(t *testing.T) {
	session.RegisterBackendProvider(session.BackendTymux)
	t.Cleanup(func() { session.RegisterBackendProvider(session.BackendTmux) })

	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)

	testDir := t.TempDir()
	t.Setenv("STAPLER_SQUAD_TEST_DIR", testDir)
	const title = "worktree-session-override-map-test"
	sessionKey := tmux.NewSessionName(title, tmux.TmuxPrefix).String()
	require.NoError(t, os.WriteFile(filepath.Join(testDir, "config.json"),
		[]byte(`{"default_program": "claude", "tymux_session_overrides": {"`+sessionKey+`": false}}`), 0o644))

	worktreePath := t.TempDir()
	initGitRepoWithCommit(t, worktreePath)

	inst, err := svc.CreateWorktreeSession(context.Background(), title, t.TempDir(), worktreePath, "", nil, true, false)
	require.NoError(t, err)
	t.Cleanup(func() { _ = inst.Destroy() })

	assert.Equal(t, session.BackendTmux, inst.Backend,
		"a TymuxSessionOverrides entry keyed by the sanitized tmux session name must force the backend even though the process-wide default is tymux")
}

// TestSessionService_CreateSession_DelegatesToCreateManagedInstance_When_HandlerInvoked
// is the Story 1.2.0a regression test: it proves CreateSession's handler no
// longer contains its own inline path-existence check but instead delegates
// construction to session.CreateManagedInstance. It does this by observing
// CreateManagedInstance's specific sentinel-error contract (session.ErrPathNotExist,
// wrapped and mapped to connect.CodeNotFound) surface unchanged through the
// handler for a Directory-mode session whose path does not exist and
// CreateIfMissing is unset -- behavior that only holds if the handler is
// calling into the extracted domain function rather than duplicating (or
// dropping) the check itself.
func TestSessionService_CreateSession_DelegatesToCreateManagedInstance_When_HandlerInvoked(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	tmpDir := t.TempDir()
	missingPath := tmpDir + "/does-not-exist"

	_, err := fix.svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:       "delegation-regression",
		Path:        missingPath,
		SessionType: sessionv1.SessionType_SESSION_TYPE_DIRECTORY,
	}))
	require.Error(t, err)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code(),
		"CreateSession must surface session.CreateManagedInstance's ErrPathNotExist as CodeNotFound, proving the handler delegates rather than duplicating path-existence logic")

	// No instance should have been persisted -- CreateManagedInstance must
	// fail before any Storage.AddInstance call.
	data, listErr := fix.storage.ListInstanceData()
	require.NoError(t, listErr)
	for _, d := range data {
		assert.NotEqual(t, "delegation-regression", d.Title, "no instance should be persisted when path resolution fails")
	}
}

// --------------------------------------------------------------------------
// CreateSession — restart_from_session_id (Story 2.3.1): path derivation,
// lineage, and the still-live guard.
// --------------------------------------------------------------------------

// TestCreateSession_RestartFromSessionId_DerivesPathFromSource verifies that
// when restart_from_session_id is set and path is left empty, the new
// session's path is resolved from the (persisted, not live) source session's
// Path, and the created session records RestartedFromSessionId.
func TestCreateSession_RestartFromSessionId_DerivesPathFromSource(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	sourcePath := t.TempDir()
	require.NoError(t, fix.storage.AddInstance(&session.Instance{
		Title:   "restart-source",
		Path:    sourcePath,
		Program: "claude",
		Status:  session.Paused,
	}))

	resp, err := fix.svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:                "restarted-session",
		Path:                 "",
		Program:              "claude",
		RestartFromSessionId: "restart-source",
	}))
	require.NoError(t, err)
	t.Cleanup(func() { destroyCreatedSession(t, fix.svc, resp.Msg.Session.Id) })

	assert.Equal(t, sourcePath, resp.Msg.Session.Path, "path should be derived from the source session")
	assert.Equal(t, "restart-source", resp.Msg.Session.RestartedFromSessionId)
}

// TestCreateSession_RestartFromSessionId_ExplicitPathWins verifies that an
// explicit request Path always wins over the restart-derived path -- it must
// never be silently overridden, even though lineage is still recorded.
func TestCreateSession_RestartFromSessionId_ExplicitPathWins(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	sourcePath := t.TempDir()
	require.NoError(t, fix.storage.AddInstance(&session.Instance{
		Title:   "restart-source-explicit",
		Path:    sourcePath,
		Program: "claude",
		Status:  session.Paused,
	}))

	explicitPath := t.TempDir()
	require.NotEqual(t, sourcePath, explicitPath)

	resp, err := fix.svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:                "restarted-session-explicit-path",
		Path:                 explicitPath,
		Program:              "claude",
		RestartFromSessionId: "restart-source-explicit",
	}))
	require.NoError(t, err)
	t.Cleanup(func() { destroyCreatedSession(t, fix.svc, resp.Msg.Session.Id) })

	assert.Equal(t, explicitPath, resp.Msg.Session.Path, "explicit path must win over the restart-derived path")
	assert.Equal(t, "restart-source-explicit", resp.Msg.Session.RestartedFromSessionId, "lineage must still be recorded even when path is explicit")
}

// TestCreateSession_RestartFromSessionId_MissingSourceReturnsNotFound verifies
// that a restart_from_session_id naming a session that exists neither live nor
// in storage returns CodeNotFound naming the missing source -- not a silent
// fallback (e.g. to cwd).
func TestCreateSession_RestartFromSessionId_MissingSourceReturnsNotFound(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	_, err := fix.svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:                "restarted-session-missing-source",
		Path:                 t.TempDir(),
		Program:              "claude",
		RestartFromSessionId: "does-not-exist",
	}))
	require.Error(t, err)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
	assert.Contains(t, err.Error(), "does-not-exist")
}

// TestCreateSession_RestartFromSessionId_RejectsStillLiveSourceWithoutConfirmation
// verifies the still-live guard: when the source session named by
// restart_from_session_id is currently live (found via FindLiveInstance) and
// confirm_restart_with_live_source is not set, CreateSession returns
// CodeFailedPrecondition and creates nothing.
func TestCreateSession_RestartFromSessionId_RejectsStillLiveSourceWithoutConfirmation(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	liveSource := &session.Instance{
		Title:   "restart-source-live",
		Path:    t.TempDir(),
		Program: "claude",
		Status:  session.Active,
	}
	addInstanceToPoller(fix.poller, liveSource)

	_, err := fix.svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:                "restarted-session-rejected",
		Path:                 t.TempDir(),
		Program:              "claude",
		RestartFromSessionId: "restart-source-live",
	}))
	require.Error(t, err)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeFailedPrecondition, connectErr.Code())
	assert.Contains(t, err.Error(), "restart-source-live")

	data, listErr := fix.storage.ListInstanceData()
	require.NoError(t, listErr)
	for _, d := range data {
		assert.NotEqual(t, "restarted-session-rejected", d.Title, "no session should be created when the still-live guard rejects the request")
	}
}

// TestCreateSession_RestartFromSessionId_ProceedsWhenLiveSourceConfirmed
// verifies that the same still-live source, with
// confirm_restart_with_live_source: true, lets CreateSession proceed normally
// -- the explicit confirmation overrides the guard.
func TestCreateSession_RestartFromSessionId_ProceedsWhenLiveSourceConfirmed(t *testing.T) {
	fix := setupForkTestFixture(t)
	t.Cleanup(fix.cleanup)

	sourcePath := t.TempDir()
	liveSource := &session.Instance{
		Title:   "restart-source-live-confirmed",
		Path:    sourcePath,
		Program: "claude",
		Status:  session.Active,
	}
	addInstanceToPoller(fix.poller, liveSource)

	resp, err := fix.svc.CreateSession(context.Background(), connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:                        "restarted-session-confirmed",
		Path:                         "",
		Program:                      "claude",
		RestartFromSessionId:         "restart-source-live-confirmed",
		ConfirmRestartWithLiveSource: true,
	}))
	require.NoError(t, err)
	t.Cleanup(func() { destroyCreatedSession(t, fix.svc, resp.Msg.Session.Id) })

	assert.Equal(t, sourcePath, resp.Msg.Session.Path, "path should be derived from the confirmed live source")
	assert.Equal(t, "restart-source-live-confirmed", resp.Msg.Session.RestartedFromSessionId)
}

// --------------------------------------------------------------------------
// DeleteSession + CancelSession ordering (F10)
// --------------------------------------------------------------------------

// TestDeleteSession_CancelsPendingApprovals verifies that DeleteSession cancels
// all pending approvals for the session before removing it from storage.
// This ensures approval goroutines can exit cleanly while the session still exists.
func TestDeleteSession_CancelsPendingApprovals(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// newRateLimitHiddenTestFixture builds an isolated storage/eventBus/service
// triple plus a Hidden test instance for the rate-limit Hidden-gate tests
// below. Each caller must get its own fixture rather than sharing one across
// t.Parallel() subtests: the eventBus broadcasts every publish to every
// subscriber, so a shared instance let a sibling subtest's SessionUpdated
// event land in another subtest's channel and flip an exact-count assertion
// (see TestWireRateLimitCallbacks_StillPublishesSessionUpdated_When_InstanceHidden's
// doc comment for the flake this caused).
func newRateLimitHiddenTestFixture(t *testing.T, title string) (*SessionService, *events.EventBus, *session.Instance) {
	t.Helper()
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(8)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

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
	return svc, eventBus, inst
}

// TestWireRateLimitCallbacks_SuppressesNotification_When_InstanceHidden verifies
// the Epic 5 Story 5.1 Hidden gate: a Hidden instance (e.g. a headless review
// session spawned via SpawnReviewSession) must never receive a rate-limit
// detected/recovery notification, since rate-limit events have no
// AttentionReason to preserve as a narrowing safety net — suppression here is
// unconditional on Hidden, matching Epic 3's generic done/stuck notifier.
func TestWireRateLimitCallbacks_SuppressesNotification_When_InstanceHidden(t *testing.T) {
	t.Parallel()

	t.Run("onDetected", func(t *testing.T) {
		t.Parallel()
		svc, eventBus, inst := newRateLimitHiddenTestFixture(t, "rl-hidden-detected")
		subCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		ch, _ := eventBus.Subscribe(subCtx)

		svc.onRateLimitDetected(inst, inst.UUID, time.Time{})

		notifs := drainNotificationEvents(ch)
		assert.Empty(t, notifs, "a Hidden instance must never receive a rate-limit-detected notification")
	})

	t.Run("onRecovery", func(t *testing.T) {
		t.Parallel()
		svc, eventBus, inst := newRateLimitHiddenTestFixture(t, "rl-hidden-recovery")
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
	t.Parallel()

	newHiddenInstance := func(t *testing.T, storage *session.Storage, title string) *session.Instance {
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

	// Each subtest gets its own storage/eventBus/service — see the matching
	// comment in TestWireRateLimitCallbacks_SuppressesNotification_When_InstanceHidden.
	// This test's exact require.Len(updates, 1) assertion is what actually
	// surfaced the shared-eventBus crosstalk as a flake: a sibling subtest's
	// SessionUpdated publish would occasionally land in this subtest's channel
	// before it drained, making the count 2 instead of 1.
	t.Run("onDetected", func(t *testing.T) {
		t.Parallel()
		storage := createTestStorage(t)
		eventBus := events.NewEventBus(8)
		svc := NewSessionService(storage, eventBus)
		t.Cleanup(func() { svc.Shutdown() })
		inst := newHiddenInstance(t, storage, "rl-hidden-detected-sync")
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
		t.Parallel()
		storage := createTestStorage(t)
		eventBus := events.NewEventBus(8)
		svc := NewSessionService(storage, eventBus)
		t.Cleanup(func() { svc.Shutdown() })
		inst := newHiddenInstance(t, storage, "rl-hidden-recovery-sync")
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
	t.Parallel()
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

// TestOnColdRestoreLostHistory_PublishesNotification_UnlessHidden verifies the
// session-revive-uuid-loss AC3 notification: a non-Hidden instance whose cold
// restore was forced fresh despite prior conversation history gets a durable
// WARNING/MEDIUM notification, while a Hidden instance (e.g. a headless
// review session) never does — mirroring onRateLimitRecovery's Hidden gate.
func TestOnColdRestoreLostHistory_PublishesNotification_UnlessHidden(t *testing.T) {
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(8)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	newInstance := func(title string, hidden bool) *session.Instance {
		inst := &session.Instance{
			Title:     title,
			UUID:      title + "-uuid",
			Path:      "/tmp/test",
			Status:    session.Paused,
			Program:   "claude",
			Hidden:    hidden,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		require.NoError(t, storage.AddInstance(inst))
		return inst
	}

	t.Run("not hidden publishes notification", func(t *testing.T) {
		inst := newInstance("cold-restore-visible", false)
		subCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		ch, _ := eventBus.Subscribe(subCtx)

		svc.onColdRestoreLostHistory(inst)

		notifs := drainNotificationEvents(ch)
		require.Len(t, notifs, 1, "expected exactly one cold-restore-lost-history notification")
		assert.Equal(t, int32(8), notifs[0].NotificationType, "must be NotificationType_WARNING")
		assert.Equal(t, int32(2), notifs[0].NotificationPriority, "must be NotificationPriority_MEDIUM")
		assert.Contains(t, notifs[0].NotificationTitle, inst.Title)
	})

	t.Run("hidden suppresses notification", func(t *testing.T) {
		inst := newInstance("cold-restore-hidden", true)
		subCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		ch, _ := eventBus.Subscribe(subCtx)

		svc.onColdRestoreLostHistory(inst)

		notifs := drainNotificationEvents(ch)
		assert.Empty(t, notifs, "a Hidden instance must never receive a cold-restore-lost-history notification")
	})
}

// TestColdRestoreOutcomeListener_FiltersOnReason verifies
// coldRestoreOutcomeListener only calls onColdRestoreLostHistory for an
// EventStarted carrying exactly session.ReasonColdRestoreLostHistory — a
// normal start (empty reason) or any other event/reason combination must not
// notify (session-revive-uuid-loss AC3).
func TestColdRestoreOutcomeListener_FiltersOnReason(t *testing.T) {
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(8)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	inst := &session.Instance{
		Title:     "cold-restore-listener",
		UUID:      "cold-restore-listener-uuid",
		Path:      "/tmp/test",
		Status:    session.Paused,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, storage.AddInstance(inst))

	listener := &coldRestoreOutcomeListener{svc: svc, inst: inst}

	subCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := eventBus.Subscribe(subCtx)

	listener.OnLifecycleEvent(session.EventStarted, "")
	assert.Empty(t, drainNotificationEvents(ch), "a normal start (empty reason) must not notify")

	listener.OnLifecycleEvent(session.EventExited, session.ReasonColdRestoreLostHistory)
	assert.Empty(t, drainNotificationEvents(ch), "the reason must only be honored on EventStarted")

	listener.OnLifecycleEvent(session.EventStarted, session.ReasonColdRestoreLostHistory)
	notifs := drainNotificationEvents(ch)
	require.Len(t, notifs, 1, "EventStarted with the lost-history reason must notify")
}

// TestSessionService_Shutdown_StopsAnalyticsFlushGoroutine confirms Shutdown() calls
// through to AnalyticsStore.Stop(), is idempotent, and leaves Record() as a safe no-op
// afterward. Shutdown() joins the flush goroutine synchronously (AnalyticsStore.Stop
// blocks on <-s.done), so a hung flush loop would make this test time out rather than
// silently pass — no goleak/process-wide check needed to catch that regression.
func TestSessionService_Shutdown_StopsAnalyticsFlushGoroutine(t *testing.T) {
	t.Parallel()
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

// TestLoadClaudeSettingsRulesAtStartup_MergesRulesIntoClassifier is the Story 2.1.1
// root-cause regression test: LoadClaudeSettingsRules had zero call sites before this
// project, so ~/.claude/settings.json permissions.allow entries never took effect as
// stapler-squad auto-approval rules. Exercises the extracted
// loadClaudeSettingsRulesAtStartup helper directly rather than going through
// NewSessionService, which skips this under config.IsTestMode() precisely so the rest of
// this repo's test suite doesn't depend on the developer's real ~/.claude/settings.json —
// see loadClaudeSettingsRulesAtStartup's call site in NewSessionServiceWithSearchEngine.
func TestLoadClaudeSettingsRulesAtStartup_MergesRulesIntoClassifier(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeSettingsFile(t, filepath.Join(home, ".claude", "settings.json"),
		`{"permissions":{"allow":["Bash(git log*)"]}}`)

	classifierObj := classifier.NewRuleBasedClassifier()
	loadClaudeSettingsRulesAtStartup(classifierObj, "", events.NewEventBus(100))

	toolName := ""
	found := false
	for _, r := range classifierObj.Rules() {
		if r.Source == "claude-settings" {
			found = true
			toolName = r.ToolName
		}
	}
	require.True(t, found, "claude-settings rule from ~/.claude/settings.json must be present at startup")
	assert.Equal(t, "Bash", toolName)
}

// TestNewSessionService_ClaudeSettingsWatcherWiredAndReachable is the Story 4.3.1
// lifecycle-wiring regression test: GetClaudeSettingsWatcher must return a non-nil watcher
// after NewSessionService, so wireDepsIntoServer has something to Start with the server's
// lifecycle context.
func TestNewSessionService_ClaudeSettingsWatcherWiredAndReachable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })

	assert.NotNil(t, svc.GetClaudeSettingsWatcher())
}

// TestLoadClaudeSettingsRulesAtStartup_CwdEqualsHome_NoDuplicateClaudeSettingsRules is the
// Blocker 2 end-to-end regression test: the live deployed systemd unit runs with
// WorkingDirectory=$HOME (see scripts/install-service.sh), so the server's own cwd equals
// home. Without the settingsPaths dedup fix, the classifier would load the same file's rules
// twice (once tagged "global", once tagged "project"). Exercises
// loadClaudeSettingsRulesAtStartup directly — see the sibling
// TestLoadClaudeSettingsRulesAtStartup_MergesRulesIntoClassifier's doc comment for why.
func TestLoadClaudeSettingsRulesAtStartup_CwdEqualsHome_NoDuplicateClaudeSettingsRules(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeSettingsFile(t, filepath.Join(home, ".claude", "settings.json"),
		`{"permissions":{"allow":["Bash(git *)"]}}`)

	classifierObj := classifier.NewRuleBasedClassifier()
	loadClaudeSettingsRulesAtStartup(classifierObj, home, events.NewEventBus(100)) // cwd == home, mirrors the live deployed config

	type key struct {
		pattern  string
		priority int
	}
	seen := make(map[key]int)
	for _, r := range classifierObj.Rules() {
		if r.Source != "claude-settings" {
			continue
		}
		pattern := ""
		if r.CommandPattern != nil {
			pattern = r.CommandPattern.String()
		}
		seen[key{pattern: pattern, priority: r.Priority}]++
	}
	require.NotEmpty(t, seen)
	for k, count := range seen {
		assert.Equal(t, 1, count, "claude-settings rule %+v must not be duplicated when cwd == home", k)
	}
}

// TestClaudeSettingsWatcher_StartsWiredCallback_InitialPrimingReloadPublishesNoNotification
// is the end-to-end regression test for the reviewed defect against the ACTUAL callback
// wired up in NewSessionServiceWithSearchEngine (not just the isolated watcher unit test):
// Start()'s initial priming reload must never publish a "Claude Settings Reloaded"
// EventNotification, whether or not any claude-settings rules are configured. Covers both
// halves the reviewer flagged: a spurious "0 claude-settings rule(s) reloaded" toast on
// every restart with no rules configured, and a redundant duplicate of the startup
// activation notification when rules do exist.
func TestClaudeSettingsWatcher_StartsWiredCallback_InitialPrimingReloadPublishesNoNotification(t *testing.T) {
	for _, tc := range []struct {
		name      string
		withRules bool
	}{
		{name: "zero rules configured", withRules: false},
		{name: "rules configured", withRules: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			if tc.withRules {
				writeSettingsFile(t, filepath.Join(home, ".claude", "settings.json"),
					`{"permissions":{"allow":["Bash(git log*)"]}}`)
			}

			storage := createTestStorage(t)
			eventBus := events.NewEventBus(100)
			svc := NewSessionService(storage, eventBus)
			t.Cleanup(func() { svc.Shutdown() })

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			eventCh, _ := eventBus.Subscribe(ctx)

			watcher := svc.GetClaudeSettingsWatcher()
			require.NotNil(t, watcher)
			watcher.Start(ctx) // exercises the exact NewSessionServiceWithSearchEngine-wired callback

			select {
			case ev := <-eventCh:
				t.Fatalf("Start's initial priming reload must not publish a notification, got: %+v", ev)
			case <-time.After(200 * time.Millisecond):
				// No notification observed within the window — expected.
			}
		})
	}
}
