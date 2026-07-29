package session

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	stdlog "log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/git"
)

// waitWithTimeout waits for the done channel to be closed or fails the test after 2 seconds.
func waitWithTimeout(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for goroutine to complete")
	}
}

// TestBacklogLifecycleListener_OnSessionStarted verifies that when a session UUID
// maps to an ItemSession, UpdateItemSessionStarted is called. When session UUID
// has no ItemSession (ErrNotFound), no error is propagated.
func TestBacklogLifecycleListener_OnSessionStarted(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Create a BacklogItem with status "in_progress".
	itemData := BacklogItemData{
		Title:              "Test Item",
		Description:        "A test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
	}
	createdItem, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)
	require.NotNil(t, createdItem)

	// Create an ItemSession linked to the BacklogItem with a specific session UUID.
	sessionUUID := uuid.New().String()
	isData := ItemSessionData{
		ItemID:      createdItem.ID,
		SessionUUID: sessionUUID,
		SessionRole: "work",
	}
	createdIS, err := storage.CreateItemSession(ctx, isData)
	require.NoError(t, err)
	require.NotNil(t, createdIS)

	// Create the BacklogLifecycleListener and call onSessionStarted.
	listener := NewBacklogLifecycleListener(storage)

	// Use a WaitGroup to synchronize with the goroutine spawned by onSessionStarted.
	var wg sync.WaitGroup
	done := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		listener.onSessionStarted(sessionUUID)
	}()
	go func() {
		wg.Wait()
		close(done)
	}()
	waitWithTimeout(t, done)

	// Verify that UpdateItemSessionStarted was called by checking StartedAt is set.
	repo := storage.repo.(*EntRepository)
	fetchedIS, err := repo.GetItemSession(ctx, createdIS.ID)
	require.NoError(t, err)
	require.NotNil(t, fetchedIS.StartedAt)
}

// TestBacklogLifecycleListener_OnSessionStarted_NotFound verifies that when a
// session UUID has no linked ItemSession, no error is logged or propagated.
func TestBacklogLifecycleListener_OnSessionStarted_NotFound(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	listener := NewBacklogLifecycleListener(storage)

	// Call onSessionStarted with a non-existent UUID. This should not panic or error.
	nonExistentUUID := uuid.New().String()
	done := make(chan struct{})
	go func() {
		defer close(done)
		listener.onSessionStarted(nonExistentUUID)
	}()
	waitWithTimeout(t, done)

	// If we reach here without panic, the test passes.
	// The method silently returns on ErrNotFound, so there's no observable state change.
}

// TestBacklogLifecycleListener_OnSessionExited_WorkSession_TransitionsToReview
// verifies that when a work session exits and item is in_progress, item transitions
// to review (when SkipReviewGate=false).
func TestBacklogLifecycleListener_OnSessionExited_WorkSession_TransitionsToReview(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Create a BacklogItem with status "in_progress" and SkipReviewGate=false.
	itemData := BacklogItemData{
		Title:              "Test Item",
		Description:        "A test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
		SkipReviewGate:     false,
	}
	createdItem, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)
	require.NotNil(t, createdItem)

	// Create an ItemSession linked to the BacklogItem with SessionRole="work".
	sessionUUID := uuid.New().String()
	isData := ItemSessionData{
		ItemID:      createdItem.ID,
		SessionUUID: sessionUUID,
		SessionRole: "work",
	}
	createdIS, err := storage.CreateItemSession(ctx, isData)
	require.NoError(t, err)
	require.NotNil(t, createdIS)

	// Create the BacklogLifecycleListener and call onSessionExited.
	listener := NewBacklogLifecycleListener(storage)
	done := make(chan struct{})
	go func() {
		defer close(done)
		listener.onSessionExited(sessionUUID)
	}()
	waitWithTimeout(t, done)

	// Verify that the item transitioned to review.
	fetchedItem, err := storage.GetBacklogItem(ctx, createdItem.ID)
	require.NoError(t, err)
	require.Equal(t, string(BacklogStatusReview), fetchedItem.Status)

	// Verify that the ItemSession has EndedAt set.
	repo := storage.repo.(*EntRepository)
	fetchedIS, err := repo.GetItemSession(ctx, createdIS.ID)
	require.NoError(t, err)
	require.NotNil(t, fetchedIS.EndedAt)
}

// fakeQueueDequeuer is a test double implementing QueueDequeuer, recording every
// call and signaling on a channel so async callers (onSessionExited invokes it
// in a goroutine) can be synchronized with in tests.
type fakeQueueDequeuer struct {
	called chan struct{}
}

func newFakeQueueDequeuer() *fakeQueueDequeuer {
	return &fakeQueueDequeuer{called: make(chan struct{}, 8)}
}

func (f *fakeQueueDequeuer) DequeueNextQueuedItems(ctx context.Context) error {
	f.called <- struct{}{}
	return nil
}

// TestBacklogLifecycleListener_OnSessionExited_WorkSession_TriggersDequeue
// verifies that once a work session exit frees a WIP slot (item leaves
// in_progress), onSessionExited invokes the wired QueueDequeuer immediately
// rather than waiting for the next ReconcileStuck tick.
func TestBacklogLifecycleListener_OnSessionExited_WorkSession_TriggersDequeue(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	createdItem, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "Test Item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
	})
	require.NoError(t, err)

	sessionUUID := uuid.New().String()
	_, err = storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      createdItem.ID,
		SessionUUID: sessionUUID,
		SessionRole: "work",
	})
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	dequeuer := newFakeQueueDequeuer()
	listener.SetDequeuer(dequeuer)

	done := make(chan struct{})
	go func() {
		defer close(done)
		listener.onSessionExited(sessionUUID)
	}()
	waitWithTimeout(t, done)

	select {
	case <-dequeuer.called:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for DequeueNextQueuedItems to be called via onSessionExited")
	}
}

// TestBacklogLifecycleListener_ReconcileStuck_TriggersDequeue verifies the
// safety-net path: the periodic ReconcileStuck sweep invokes the wired
// QueueDequeuer on every tick (not just onSessionExited), so a missed exit
// hook or a concurrency limit raised while items were queued still gets
// picked up.
func TestBacklogLifecycleListener_ReconcileStuck_TriggersDequeue(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	listener := NewBacklogLifecycleListener(storage)
	listener.SetEnabled(true)
	dequeuer := newFakeQueueDequeuer()
	listener.SetDequeuer(dequeuer)

	listener.ReconcileStuck(context.Background())

	select {
	case <-dequeuer.called:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for DequeueNextQueuedItems to be called via ReconcileStuck")
	}
}

// TestBacklogLifecycleListener_OnSessionExited_WorkSession_TransitionsToDone_WhenSkipReviewGate
// verifies that when SkipReviewGate=true, item transitions directly to done.
func TestBacklogLifecycleListener_OnSessionExited_WorkSession_TransitionsToDone_WhenSkipReviewGate(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Create a BacklogItem with status "in_progress" and SkipReviewGate=true.
	itemData := BacklogItemData{
		Title:              "Test Item",
		Description:        "A test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
		SkipReviewGate:     true,
	}
	createdItem, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)
	require.NotNil(t, createdItem)

	// Create an ItemSession linked to the BacklogItem with SessionRole="work".
	sessionUUID := uuid.New().String()
	isData := ItemSessionData{
		ItemID:      createdItem.ID,
		SessionUUID: sessionUUID,
		SessionRole: "work",
	}
	createdIS, err := storage.CreateItemSession(ctx, isData)
	require.NoError(t, err)
	require.NotNil(t, createdIS)

	// Create the BacklogLifecycleListener and call onSessionExited.
	listener := NewBacklogLifecycleListener(storage)
	done := make(chan struct{})
	go func() {
		defer close(done)
		listener.onSessionExited(sessionUUID)
	}()
	waitWithTimeout(t, done)

	// Verify that the item transitioned to done (not review).
	fetchedItem, err := storage.GetBacklogItem(ctx, createdItem.ID)
	require.NoError(t, err)
	require.Equal(t, string(BacklogStatusDone), fetchedItem.Status)

	// Verify that the ItemSession has EndedAt set.
	repo := storage.repo.(*EntRepository)
	fetchedIS, err := repo.GetItemSession(ctx, createdIS.ID)
	require.NoError(t, err)
	require.NotNil(t, fetchedIS.EndedAt)
}

// TestBacklogLifecycleListener_OnSessionExited_ReviewSession_NoTransition
// verifies that when a review/triage session exits (SessionRole != "work"),
// no transition happens (recursion guard).
func TestBacklogLifecycleListener_OnSessionExited_ReviewSession_NoTransition(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Create a BacklogItem with status "in_progress".
	itemData := BacklogItemData{
		Title:              "Test Item",
		Description:        "A test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
	}
	createdItem, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)
	require.NotNil(t, createdItem)

	// Create an ItemSession linked to the BacklogItem with SessionRole="review".
	sessionUUID := uuid.New().String()
	isData := ItemSessionData{
		ItemID:      createdItem.ID,
		SessionUUID: sessionUUID,
		SessionRole: "review",
	}
	createdIS, err := storage.CreateItemSession(ctx, isData)
	require.NoError(t, err)
	require.NotNil(t, createdIS)

	// Create the BacklogLifecycleListener and call onSessionExited.
	listener := NewBacklogLifecycleListener(storage)
	done := make(chan struct{})
	go func() {
		defer close(done)
		listener.onSessionExited(sessionUUID)
	}()
	waitWithTimeout(t, done)

	// Verify that the item status did NOT change (still in_progress).
	fetchedItem, err := storage.GetBacklogItem(ctx, createdItem.ID)
	require.NoError(t, err)
	require.Equal(t, string(BacklogStatusInProgress), fetchedItem.Status)

	// Verify that the ItemSession EndedAt IS set (exit is recorded for all roles).
	repo := storage.repo.(*EntRepository)
	fetchedIS, err := repo.GetItemSession(ctx, createdIS.ID)
	require.NoError(t, err)
	require.NotNil(t, fetchedIS.EndedAt, "review session should have EndedAt recorded when it exits")
}

// TestBacklogLifecycleListener_OnSessionExited_NotFound_NoError
// verifies that when session UUID has no ItemSession, no panic or error occurs.
func TestBacklogLifecycleListener_OnSessionExited_NotFound_NoError(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	listener := NewBacklogLifecycleListener(storage)

	// Call onSessionExited with a non-existent UUID. This should not panic or error.
	nonExistentUUID := uuid.New().String()
	done := make(chan struct{})
	go func() {
		defer close(done)
		listener.onSessionExited(nonExistentUUID)
	}()
	waitWithTimeout(t, done)

	// If we reach here without panic, the test passes.
}

// TestBacklogLifecycleListener_OnSessionExited_ItemNotInProgress_NoTransition
// verifies that if the item is not in in_progress status, no transition occurs
// (e.g., item is already in review or done).
func TestBacklogLifecycleListener_OnSessionExited_ItemNotInProgress_NoTransition(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Create a BacklogItem with status "review" (not in_progress).
	itemData := BacklogItemData{
		Title:              "Test Item",
		Description:        "A test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusReview),
	}
	createdItem, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)
	require.NotNil(t, createdItem)

	// Create an ItemSession linked to the BacklogItem with SessionRole="work".
	sessionUUID := uuid.New().String()
	isData := ItemSessionData{
		ItemID:      createdItem.ID,
		SessionUUID: sessionUUID,
		SessionRole: "work",
	}
	createdIS, err := storage.CreateItemSession(ctx, isData)
	require.NoError(t, err)
	require.NotNil(t, createdIS)

	// Create the BacklogLifecycleListener and call onSessionExited.
	listener := NewBacklogLifecycleListener(storage)
	done := make(chan struct{})
	go func() {
		defer close(done)
		listener.onSessionExited(sessionUUID)
	}()
	waitWithTimeout(t, done)

	// Verify that the item status did NOT change (still review).
	fetchedItem, err := storage.GetBacklogItem(ctx, createdItem.ID)
	require.NoError(t, err)
	require.Equal(t, string(BacklogStatusReview), fetchedItem.Status)

	// Verify that the ItemSession has EndedAt set (the exit was recorded).
	repo := storage.repo.(*EntRepository)
	fetchedIS, err := repo.GetItemSession(ctx, createdIS.ID)
	require.NoError(t, err)
	require.NotNil(t, fetchedIS.EndedAt)
}

// TestBacklogLifecycleListener_WireToInstance verifies that WireToInstance correctly
// registers a per-instance listener shim that fires on lifecycle events.
func TestBacklogLifecycleListener_WireToInstance(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	listener := NewBacklogLifecycleListener(storage)
	listener.SetEnabled(true)

	// Create a minimal Instance with a known UUID (without starting tmux).
	inst := &Instance{
		UUID: uuid.New().String(),
	}

	// Wire the listener to the instance.
	listener.WireToInstance(inst)

	// Verify a listener was registered by checking the slice length.
	inst.lifecycleListenersMu.Lock()
	count := len(inst.lifecycleListeners)
	inst.lifecycleListenersMu.Unlock()
	require.Equal(t, 1, count, "WireToInstance should register exactly one lifecycle listener")

	// Create a BacklogItem and ItemSession linked to inst.UUID so that
	// firing EventStarted updates the session's StartedAt.
	itemData := BacklogItemData{
		Title:              "WireToInstance test item",
		Description:        "Testing wire",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
	}
	createdItem, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	isData := ItemSessionData{
		ItemID:      createdItem.ID,
		SessionUUID: inst.UUID,
		SessionRole: "work",
	}
	createdIS, err := storage.CreateItemSession(ctx, isData)
	require.NoError(t, err)

	// Fire EventStarted through the registered shim. The shim dispatches to a goroutine.
	done := make(chan struct{})
	go func() {
		defer close(done)
		inst.fireLifecycleEvent(EventStarted, "")
	}()
	waitWithTimeout(t, done)

	// Allow the goroutine inside onSessionStarted to complete.
	// Since the shim spawns its own goroutine, we poll briefly.
	require.Eventually(t, func() bool {
		repo := storage.repo.(*EntRepository)
		fetchedIS, ferr := repo.GetItemSession(ctx, createdIS.ID)
		return ferr == nil && fetchedIS.StartedAt != nil
	}, 2*time.Second, 20*time.Millisecond, "EventStarted should trigger UpdateItemSessionStarted")
}

// TestBacklogLifecycleListener_WireToInstance_EventStopped_TransitionsToReview
// is the BUG-027 regression test: a session torn down via an explicit operator
// stop (Instance.Destroy(), as called by the stop_session MCP tool,
// SessionService.DeleteSession, and BacklogService.SessionStopper) must drive
// the same in_progress→review transition and ItemSession.EndedAt bookkeeping
// as a natural process exit — not silently strand the backlog item.
func TestBacklogLifecycleListener_WireToInstance_EventStopped_TransitionsToReview(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	listener := NewBacklogLifecycleListener(storage)
	listener.SetEnabled(true)

	inst := &Instance{UUID: uuid.New().String()}
	listener.WireToInstance(inst)

	itemData := BacklogItemData{
		Title:              "EventStopped test item",
		Description:        "Testing operator-stop reconciliation",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
		SkipReviewGate:     false,
	}
	createdItem, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	isData := ItemSessionData{
		ItemID:      createdItem.ID,
		SessionUUID: inst.UUID,
		SessionRole: "work",
	}
	createdIS, err := storage.CreateItemSession(ctx, isData)
	require.NoError(t, err)

	// Simulate Destroy()'s deferred fire — an operator-initiated stop, not a
	// natural exit — through the exact shim wired by WireToInstance.
	done := make(chan struct{})
	go func() {
		defer close(done)
		inst.fireLifecycleEvent(EventStopped, "operator-destroy")
	}()
	waitWithTimeout(t, done)

	require.Eventually(t, func() bool {
		fetchedItem, ferr := storage.GetBacklogItem(ctx, createdItem.ID)
		return ferr == nil && fetchedItem.Status == string(BacklogStatusReview)
	}, 2*time.Second, 20*time.Millisecond, "EventStopped should trigger the same in_progress->review transition as EventExited")

	repo := storage.repo.(*EntRepository)
	fetchedIS, err := repo.GetItemSession(ctx, createdIS.ID)
	require.NoError(t, err)
	require.NotNil(t, fetchedIS.EndedAt, "EventStopped should set ItemSession.EndedAt, same as a natural exit")
}

// TestBacklogLifecycleListener_NewBacklogLifecycleListener creates a listener
// without a spawner and verifies it's initialized correctly.
func TestBacklogLifecycleListener_NewBacklogLifecycleListener(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	listener := NewBacklogLifecycleListener(storage)
	require.NotNil(t, listener)
	require.Equal(t, storage, listener.storage)
	require.Nil(t, listener.sessionCreator)
}

// TestBacklogLifecycleListener_NewBacklogLifecycleListenerWithSpawner creates
// a listener with a spawner and verifies it's initialized correctly.
func TestBacklogLifecycleListener_NewBacklogLifecycleListenerWithSpawner(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	// Create a mock spawner.
	mockSpawner := &mockReviewGateSpawner{}

	listener := NewBacklogLifecycleListenerWithSpawner(storage, mockSpawner)
	require.NotNil(t, listener)
	require.Equal(t, storage, listener.storage)
	require.Equal(t, mockSpawner, listener.sessionCreator)
}

// mockReviewGateSpawner is a mock implementation of ReviewGateSpawner for testing.
// Guarded by mu since SpawnReviewSession can be invoked from a goroutine (e.g. via
// onSessionExited's bounded-semaphore fan-out).
type mockReviewGateSpawner struct {
	mu              sync.Mutex
	spawnCalled     bool
	callCount       int
	lastItem        *BacklogItemData
	lastItemSession string
	lastPrompt      string

	// err, when non-nil, is returned by SpawnReviewSession instead of a synthesized
	// Instance — used to test the spawner-error path.
	err error
	// instance, when non-nil, is returned by SpawnReviewSession on success instead of
	// a synthesized one with a random UUID — used to assert the returned Instance's
	// UUID is exactly what gets linked into the resulting ItemSession.
	instance *Instance
}

func (m *mockReviewGateSpawner) SpawnReviewSession(ctx context.Context, item *BacklogItemData, itemSessionID string, prompt string) (*Instance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.spawnCalled = true
	m.callCount++
	m.lastItem = item
	m.lastItemSession = itemSessionID
	m.lastPrompt = prompt
	if m.err != nil {
		return nil, m.err
	}
	if m.instance != nil {
		return m.instance, nil
	}
	return &Instance{UUID: uuid.New().String()}, nil
}

// getCallCount returns the number of times SpawnReviewSession has been called, under
// the mock's own lock — safe to read concurrently with in-flight calls.
func (m *mockReviewGateSpawner) getCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

// getLastPrompt returns the prompt passed to the most recent SpawnReviewSession call.
func (m *mockReviewGateSpawner) getLastPrompt() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastPrompt
}

// fakePRPendingChecker is a test double implementing prPendingChecker, used to
// inject canned IsPRMerged/GetPRStatus results without a live git worktree or
// authenticated gh CLI.
type fakePRPendingChecker struct {
	merged    bool
	mergedErr error
	status    *git.PRStatus
	statusErr error

	closeCalled  bool
	closedPR     int
	closeComment string
	closeErr     error

	// onIsPRMerged, if set, runs synchronously inside IsPRMerged before it
	// returns — lets a test simulate a concurrent status write landing exactly
	// between the caller's merge check and its own subsequent
	// TransitionBacklogItemStatus call (the precondition race these calls
	// guard against), deterministically rather than via a real goroutine race.
	onIsPRMerged func()
}

func (f *fakePRPendingChecker) IsPRMerged(prNumber int) (bool, error) {
	if f.onIsPRMerged != nil {
		f.onIsPRMerged()
	}
	return f.merged, f.mergedErr
}

func (f *fakePRPendingChecker) GetPRStatus(prNumber int) (*git.PRStatus, error) {
	return f.status, f.statusErr
}

func (f *fakePRPendingChecker) ClosePR(prNumber int, comment string) error {
	f.closeCalled = true
	f.closedPR = prNumber
	f.closeComment = comment
	return f.closeErr
}

// fakePRFixSpawner is a test double implementing PRFixSpawner, recording
// whether/how AutoReopenForPRFix was called. Same shape as mockReviewGateSpawner.
//
// err and onCall let a test simulate AutoReopenForPRFix's three real outcomes
// (BUG-040 regression coverage for ReconcilePRPending's closed-PR branch,
// which must only clear the item's PR fields once a reopen is CONFIRMED to
// have moved the item off pr_pending):
//   - success: onCall performs the real transition (e.g. via
//     storage.TransitionBacklogItemStatus to in_progress) and err is nil.
//   - no-op guard (active work session / rework cap): onCall is nil (or does
//     nothing) and err is nil — mirrors AutoReopenForPRFix returning nil
//     without transitioning anything.
//   - error: err is non-nil; onCall is typically nil.
type fakePRFixSpawner struct {
	spawnCalled    bool
	callCount      int
	lastFixContext string
	err            error
	onCall         func()
}

func (f *fakePRFixSpawner) AutoReopenForPRFix(ctx context.Context, itemID string, fixContext string) error {
	f.spawnCalled = true
	f.callCount++
	f.lastFixContext = fixContext
	if f.onCall != nil {
		f.onCall()
	}
	return f.err
}

// fakeReviewRespawner is a test double implementing ReviewRespawner. Calls are
// delivered on a buffered channel (not a plain bool/counter) because
// markAbandonedReview dispatches AutoRespawnReview asynchronously (bounded by
// reviewSem) — tests must synchronize on the channel rather than racing a
// direct field read.
type fakeReviewRespawner struct {
	calls chan string
}

func newFakeReviewRespawner() *fakeReviewRespawner {
	return &fakeReviewRespawner{calls: make(chan string, 8)}
}

func (f *fakeReviewRespawner) AutoRespawnReview(ctx context.Context, itemID string) error {
	f.calls <- itemID
	return nil
}

// TestBacklogLifecycleListener_IgnoresEventsWhenDisabled verifies that when the listener
// is disabled via SetEnabled(false), lifecycle events from an Instance are silently dropped
// and no storage side effects occur.
func TestBacklogLifecycleListener_IgnoresEventsWhenDisabled(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Create a BacklogItem in in_progress status.
	itemData := BacklogItemData{
		Title:              "Disabled gate test item",
		Description:        "Testing enabled gate",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
	}
	createdItem, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	// Create an ItemSession linked to the item.
	sessionUUID := uuid.New().String()
	isData := ItemSessionData{
		ItemID:      createdItem.ID,
		SessionUUID: sessionUUID,
		SessionRole: "work",
	}
	_, err = storage.CreateItemSession(ctx, isData)
	require.NoError(t, err)

	// Build a listener and wire it to a minimal Instance.
	listener := NewBacklogLifecycleListener(storage)
	listener.SetEnabled(false) // explicitly disabled

	inst := &Instance{UUID: sessionUUID}
	listener.WireToInstance(inst)

	// Fire EventExited — the gate should stop processing immediately.
	// Allow time for any goroutine that might have been started to settle.
	require.Eventually(t, func() bool {
		inst.fireLifecycleEvent(EventExited, "")
		// Check that the item was NOT transitioned.
		fetched, ferr := storage.GetBacklogItem(ctx, createdItem.ID)
		return ferr == nil && fetched.Status == string(BacklogStatusInProgress)
	}, 500*time.Millisecond, 20*time.Millisecond,
		"disabled listener should not transition item status")
}

// TestBacklogLifecycleListener_ProcessesEventsWhenEnabled verifies that when the listener
// is enabled via SetEnabled(true), lifecycle events ARE processed and storage is updated.
func TestBacklogLifecycleListener_ProcessesEventsWhenEnabled(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Create a BacklogItem in in_progress status.
	itemData := BacklogItemData{
		Title:              "Enabled gate test item",
		Description:        "Testing enabled gate",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
		SkipReviewGate:     true, // go straight to done to make assertion easy
	}
	createdItem, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	// Create an ItemSession linked to the item.
	sessionUUID := uuid.New().String()
	isData := ItemSessionData{
		ItemID:      createdItem.ID,
		SessionUUID: sessionUUID,
		SessionRole: "work",
	}
	_, err = storage.CreateItemSession(ctx, isData)
	require.NoError(t, err)

	// Build a listener, enable it, and wire to an Instance.
	listener := NewBacklogLifecycleListener(storage)
	listener.SetEnabled(true)

	inst := &Instance{UUID: sessionUUID}
	listener.WireToInstance(inst)

	// Fire EventExited — the listener must process it and transition the item.
	inst.fireLifecycleEvent(EventExited, "")

	require.Eventually(t, func() bool {
		fetched, ferr := storage.GetBacklogItem(ctx, createdItem.ID)
		return ferr == nil && fetched.Status == string(BacklogStatusDone)
	}, 2*time.Second, 20*time.Millisecond,
		"enabled listener should transition item from in_progress to done")
}

// TestCreateItemSessionWithVerdict_Atomic verifies that CreateItemSessionWithVerdict
// creates both ItemSession and ReviewVerdict atomically — both records must exist,
// and the verdict is linked to the session.
func TestCreateItemSessionWithVerdict_Atomic(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "Atomic Verdict Test",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
	})
	require.NoError(t, err)

	sessionUUID := "headless-review-" + uuid.New().String()
	is, err := storage.CreateItemSessionWithVerdict(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: SessionRoleReview,
	}, ReviewVerdictData{
		OverallOutcome: ReviewVerdictFail,
		Summary:        "Blocked by security check.",
	})
	require.NoError(t, err)
	assert.Equal(t, sessionUUID, is.SessionUUID)
	require.NotNil(t, is.ReviewVerdict, "ReviewVerdict must be linked to the ItemSession")
	assert.Equal(t, string(ReviewVerdictFail), is.ReviewVerdict.OverallOutcome)
	assert.Equal(t, "Blocked by security check.", is.ReviewVerdict.Summary)

	// Both records must be queryable from the same DB — verifies the commit succeeded.
	sessions, listErr := storage.ListItemSessions(ctx, item.ID)
	require.NoError(t, listErr)
	require.Len(t, sessions, 1)
	assert.Equal(t, sessionUUID, sessions[0].SessionUUID)
	require.NotNil(t, sessions[0].ReviewVerdict, "ReviewVerdict must be linked to the ItemSession")
	assert.Equal(t, string(ReviewVerdictFail), sessions[0].ReviewVerdict.OverallOutcome)
}

// newStuckReviewTestItem creates a review-status BacklogItem with a review
// ItemSession carrying the given verdict outcome. If endReviewSession is true,
// the review session's EndedAt is set (simulating the review session having
// exited) — the item then qualifies as "stuck" per FindStuckReviewItems as
// long as no other active session exists. If withActiveWorkSession is true, a
// second ItemSession (role=work, EndedAt nil) is attached, simulating rework
// still in progress — this should exclude the item from FindStuckReviewItems.
func newStuckReviewTestItem(t *testing.T, storage *Storage, outcome ReviewOutcome, endReviewSession, withActiveWorkSession bool) *BacklogItemData {
	t.Helper()
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "Stuck review test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusReview),
	})
	require.NoError(t, err)

	reviewIS, err := storage.CreateItemSessionWithVerdict(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "headless-review-" + uuid.New().String(),
		SessionRole: SessionRoleReview,
	}, ReviewVerdictData{
		OverallOutcome: outcome,
		Summary:        "no diff available",
	})
	require.NoError(t, err)

	if endReviewSession {
		require.NoError(t, storage.UpdateItemSessionEnded(ctx, reviewIS.ID, time.Now()))
	}

	if withActiveWorkSession {
		_, err := storage.CreateItemSession(ctx, ItemSessionData{
			ItemID:      item.ID,
			SessionUUID: uuid.New().String(),
			SessionRole: SessionRoleWork,
		})
		require.NoError(t, err)
	}

	return item
}

// TestFindStuckReviewItems_ReturnsAbandonedItem_ExcludesActiveAndGateless is a
// regression test for a live-data bug found via manual QA: an item whose
// AutoReopenAfterFailedReview spawn attempt failed and rolled the status back
// to "review" already has a review ItemSession on record, so
// FindReviewItemsWithoutGate never re-checks it and the item sits abandoned
// forever. FindStuckReviewItems must catch exactly that case while excluding
// items with an active session (review still running, or rework in progress)
// and items with no review session at all (that's FindReviewItemsWithoutGate's
// job, not this one's).
func TestFindStuckReviewItems_ReturnsAbandonedItem_ExcludesActiveAndGateless(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	abandoned := newStuckReviewTestItem(t, storage, ReviewVerdictUnverifiable, true, false)
	stillReviewing := newStuckReviewTestItem(t, storage, ReviewVerdictUnverifiable, false, false)
	reworkInProgress := newStuckReviewTestItem(t, storage, ReviewVerdictFail, true, true)

	// Gateless item: review status but no review ItemSession at all — belongs
	// to FindReviewItemsWithoutGate, must NOT show up here.
	gateless, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "Gateless review item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusReview),
	})
	require.NoError(t, err)

	er := storage.repo.(*EntRepository)
	stuck, err := er.FindStuckReviewItems(ctx)
	require.NoError(t, err)

	gotIDs := make([]string, 0, len(stuck))
	for _, item := range stuck {
		gotIDs = append(gotIDs, item.ID.String())
	}
	assert.Contains(t, gotIDs, abandoned.ID)
	assert.NotContains(t, gotIDs, stillReviewing.ID, "item with an active (unfinished) review session must be excluded")
	assert.NotContains(t, gotIDs, reworkInProgress.ID, "item with an active work session (rework in progress) must be excluded")
	assert.NotContains(t, gotIDs, gateless.ID, "item with no review session at all belongs to FindReviewItemsWithoutGate, not FindStuckReviewItems")
}

// TestReconcileStuckReviewItems_NotifiesOncePerItem verifies the ReconcileStuck
// safety net surfaces abandoned review items via a notification (fixing their
// total invisibility to both the human operator and every other reconciler),
// once the 15-minute grace (Story 2.1.3, abandonedReview pure fn) has
// elapsed, and that repeat ticks do not re-notify for the same item.
func TestReconcileStuckReviewItems_NotifiesOncePerItem(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item := newStuckReviewTestItem(t, storage, ReviewVerdictUnverifiable, true, false)

	listener := NewBacklogLifecycleListener(storage)
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	er := storage.repo.(*EntRepository)

	// First tick opens the row but must NOT notify yet — the item just
	// entered review (within the grace window).
	listener.reconcileStuckReviewItems(ctx, er)
	assert.Empty(t, notifier.calls, "must not notify before the 15-minute grace elapses")

	// Backdate the row past the grace so the next tick is notification-worthy.
	backdateStuckFirstDetected(t, er, item.ID, "abandoned_review", time.Now().Add(-20*time.Minute))

	listener.reconcileStuckReviewItems(ctx, er)
	assert.Equal(t, []string{"Review item needs attention"}, notifier.titles())
	require.Len(t, notifier.calls, 1)
	// The message body must interpolate the item's title, not just fire a generic
	// notification — this is the actionable content an operator needs to triage
	// the stuck item without digging further.
	assert.Contains(t, notifier.calls[0].Message, "Stuck review test item")

	// Third tick must not re-notify for the same item.
	listener.reconcileStuckReviewItems(ctx, er)
	assert.Len(t, notifier.calls, 1, "must not re-notify the same item on a repeat tick")

	// Item status itself must be untouched — this reconciler is detection-only.
	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusReview), fetched.Status)
}

// TestMarkAbandonedReview_AutoRespawnsReview_OncePastGrace is a regression test
// for the "detected and notified, but nothing ever respawns work" gap
// (docs/tasks/backlog-feature-improvement.md, 2026-07-17 update): 4 real backlog
// items sat stuck in review for days because markAbandonedReview only ever
// wrote the stuck row and notified — nothing re-triggered the review gate.
// AutoRespawnReview must fire exactly once a stuck-row occurrence, on the same
// 15-minute-grace / notify-once edge as the notification itself (not on every
// tick, and not before the grace elapses).
func TestMarkAbandonedReview_AutoRespawnsReview_OncePastGrace(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item := newStuckReviewTestItem(t, storage, ReviewVerdictUnverifiable, true, false)

	listener := NewBacklogLifecycleListener(storage)
	respawner := newFakeReviewRespawner()
	listener.SetReviewRespawner(respawner)

	er := storage.repo.(*EntRepository)

	// First tick: still within the 15-minute grace — must not respawn yet.
	listener.reconcileStuckReviewItems(ctx, er)
	select {
	case id := <-respawner.calls:
		t.Fatalf("must not respawn before the 15-minute grace elapses, got call for item=%s", id)
	case <-time.After(200 * time.Millisecond):
	}

	// Backdate the row past the grace so the next tick is respawn-worthy —
	// mirrors TestReconcileStuckReviewItems_NotifiesOncePerItem's identical setup
	// for the notification this respawn is meant to sit alongside.
	backdateStuckFirstDetected(t, er, item.ID, domain.StuckReasonAbandonedReview, time.Now().Add(-20*time.Minute))

	listener.reconcileStuckReviewItems(ctx, er)
	select {
	case id := <-respawner.calls:
		assert.Equal(t, item.ID, id)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for AutoRespawnReview to be dispatched")
	}

	// Third tick must not respawn a second time for the same stuck-row occurrence
	// (the notify-once gate — row.NotifiedAt already set — blocks re-entry).
	listener.reconcileStuckReviewItems(ctx, er)
	select {
	case id := <-respawner.calls:
		t.Fatalf("must not respawn a second time for the same occurrence, got call for item=%s", id)
	case <-time.After(200 * time.Millisecond):
	}

	// Detection-only for status: this reconciler must never mutate item status
	// itself — only the respawned review session (if any) may do that.
	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusReview), fetched.Status)
}

// TestMarkAbandonedReview_SkipsRespawn_WhenBouncingGateNotDue is a BUG-043
// regression test. Live trace (2026-07-23, three real backlog items) found
// that a respawned review's FAIL verdict only leads anywhere via
// handleReviewSessionExited's autoReopenWithBackoffGate, which is gated on
// the SEPARATE StuckReasonBouncing backoff clock — not the abandoned_review
// one markAbandonedReview itself checks. When bouncing's own gate is mid
// backoff (already tripped by earlier bounce cycles), every abandoned_review
// respawn keeps producing a correct-but-discarded verdict, silently burning
// through the 5-attempt cap for zero forward progress, until abandoned_review
// itself parks with a "use Reset to retry" notification that never mentions
// bouncing was the real blocker. markAbandonedReview must check whether
// bouncing is currently blocking a reopen BEFORE spending an abandoned_review
// attempt on a respawn that cannot possibly help.
func TestMarkAbandonedReview_SkipsRespawn_WhenBouncingGateNotDue(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item := newStuckReviewTestItem(t, storage, ReviewVerdictUnverifiable, true, false)

	listener := NewBacklogLifecycleListener(storage)
	respawner := newFakeReviewRespawner()
	listener.SetReviewRespawner(respawner)

	er := storage.repo.(*EntRepository)

	// Seed a "bouncing" stuck row for this item that already consumed an
	// attempt and is mid-backoff (next_remediation_at well in the future) —
	// mirrors the live DB state found on all three affected items (BUG-043):
	// a prior bounce cycle already tripped this gate independently of
	// abandoned_review's own schedule.
	_, err := er.MarkStuck(ctx, item.ID, domain.StuckReasonBouncing, BacklogStatusReview, "bounced previously")
	require.NoError(t, err)
	future := time.Now().Add(2 * time.Hour)
	_, err = er.RecordRemediationAttempt(ctx, item.ID, domain.StuckReasonBouncing, 1, &future)
	require.NoError(t, err)

	// First tick opens the abandoned_review row (still within grace).
	listener.reconcileStuckReviewItems(ctx, er)
	// Past grace: normally respawn-worthy.
	backdateStuckFirstDetected(t, er, item.ID, domain.StuckReasonAbandonedReview, time.Now().Add(-20*time.Minute))

	listener.reconcileStuckReviewItems(ctx, er)
	select {
	case id := <-respawner.calls:
		t.Fatalf("must not respawn while the bouncing reopen gate is not due — the resulting verdict could not be acted on, got call for item=%s", id)
	case <-time.After(300 * time.Millisecond):
	}

	// The abandoned_review attempt budget must be untouched — the whole point
	// is not to waste it on a foregone conclusion.
	rows, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	row, ok := findOpenStuckStateFor(rows, item.ID, domain.StuckReasonAbandonedReview)
	require.True(t, ok, "abandoned_review row must still be open")
	assert.Equal(t, int32(0), row.RemediationAttempts, "must not consume an abandoned_review attempt on a respawn blocked downstream by the bouncing gate")
}

// TestMarkAbandonedReview_NoRespawn_WhenNoReviewRespawnerConfigured verifies
// markAbandonedReview degrades gracefully (notification only, no panic) when no
// ReviewRespawner has been wired — the same nil-safe default every other
// injected spawner on this listener already follows.
func TestMarkAbandonedReview_NoRespawn_WhenNoReviewRespawnerConfigured(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item := newStuckReviewTestItem(t, storage, ReviewVerdictUnverifiable, true, false)

	listener := NewBacklogLifecycleListener(storage)
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)
	// Deliberately not calling SetReviewRespawner.

	er := storage.repo.(*EntRepository)
	listener.reconcileStuckReviewItems(ctx, er)
	backdateStuckFirstDetected(t, er, item.ID, domain.StuckReasonAbandonedReview, time.Now().Add(-20*time.Minute))
	listener.reconcileStuckReviewItems(ctx, er)

	assert.Equal(t, []string{"Review item needs attention"}, notifier.titles(), "notification must still fire with no respawner wired")
}

// newPRPendingTestItem creates a pr_pending BacklogItem with the given PR
// number/URL and a non-empty RepoPath (ReconcilePRPending skips items with an
// empty RepoPath). The repo path is a placeholder — the listener's PR-pending
// checker factory is overridden in these tests, so no real git/gh call is
// ever made against it.
//
// CreateBacklogItem does not persist PrURL/PrNumber (see ent_repository_backlog.go),
// so those fields are set via a follow-up UpdateBacklogItem call, mirroring how
// pushAndCreatePR itself stores them (backlog_lifecycle.go:500-506).
func newPRPendingTestItem(t *testing.T, storage *Storage, prNumber int) *BacklogItemData {
	t.Helper()
	ctx := context.Background()
	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "PR pending test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusPRPending),
		RepoPath:           "/tmp/fake-repo",
	})
	require.NoError(t, err)

	prURL := "https://github.com/TylerStaplerAtFanatics/stapler-squad/pull/152"
	updated, err := storage.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{
		PrURL:    &prURL,
		PrNumber: &prNumber,
	}, nil)
	require.NoError(t, err)
	return updated
}

// overridePRPendingChecker installs checker as listener's PR-pending-checker
// factory for the duration of the test (mirrors the timeNow seam override
// pattern used elsewhere in this package). No cleanup is needed since the
// factory lives on the listener instance, not a shared package var.
func overridePRPendingChecker(t *testing.T, listener *BacklogLifecycleListener, checker *fakePRPendingChecker) {
	t.Helper()
	listener.SetPRPendingCheckerFactory(func(repoPath string) prPendingChecker { return checker })
}

// redirectInfoLog swaps log.InfoLog for a logger writing to buf for the
// duration of the test and restores the original on cleanup. Equivalent to
// log.NewDummyLogger(buf, prefix) (log/log_test.go), reimplemented here
// because that helper lives in a _test.go file in package log and is not
// importable from other packages.
func redirectInfoLog(t *testing.T, buf *bytes.Buffer) {
	t.Helper()
	orig := log.InfoLog
	log.InfoLog = stdlog.New(buf, "INFO: ", 0)
	t.Cleanup(func() { log.InfoLog = orig })
}

// TestReconcilePRPending_SpawnsFixSession_WhenHasConflictsTrue_Alone verifies
// that HasConflicts=true alone (CI/reviews both false) is sufficient to spawn
// a fix session, and that the fix context carries the "## Merge conflict"
// section from FeedbackText.
func TestReconcilePRPending_SpawnsFixSession_WhenHasConflictsTrue_Alone(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	newPRPendingTestItem(t, storage, 152)

	listener := NewBacklogLifecycleListener(storage)
	overridePRPendingChecker(t, listener, &fakePRPendingChecker{
		merged: false,
		status: &git.PRStatus{
			HasConflicts: true,
			FeedbackText: "## Merge conflict\n...",
		},
	})
	fakeSpawner := &fakePRFixSpawner{}
	listener.SetPRFixSpawner(fakeSpawner)

	er := storage.repo.(*EntRepository)
	listener.ReconcilePRPending(context.Background(), er)

	assert.True(t, fakeSpawner.spawnCalled, "conflict-only PRStatus should trigger a fix-session spawn")
	assert.Contains(t, fakeSpawner.lastFixContext, "## Merge conflict")
}

// TestReconcilePRPending_LogsConflictTrue_WhenConflictTriggersSpawn verifies
// that the spawn log line records conflict=true when HasConflicts triggered it.
func TestReconcilePRPending_LogsConflictTrue_WhenConflictTriggersSpawn(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	newPRPendingTestItem(t, storage, 152)

	listener := NewBacklogLifecycleListener(storage)
	overridePRPendingChecker(t, listener, &fakePRPendingChecker{
		status: &git.PRStatus{
			HasConflicts: true,
			FeedbackText: "## Merge conflict\n...",
		},
	})
	listener.SetPRFixSpawner(&fakePRFixSpawner{})

	var buf bytes.Buffer
	redirectInfoLog(t, &buf)

	er := storage.repo.(*EntRepository)
	listener.ReconcilePRPending(context.Background(), er)

	assert.Contains(t, buf.String(), "conflict=true")
}

// TestReconcilePRPending_SpawnsFixSession_WhenCIFailingTrue is a regression
// test for the pre-existing CIFailing trigger, previously untested at the
// gate level. It also asserts the log line reports conflict=false so the
// extended log format doesn't spuriously report a conflict when only CI failed.
func TestReconcilePRPending_SpawnsFixSession_WhenCIFailingTrue(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	newPRPendingTestItem(t, storage, 152)

	listener := NewBacklogLifecycleListener(storage)
	overridePRPendingChecker(t, listener, &fakePRPendingChecker{
		status: &git.PRStatus{
			CIFailing:    true,
			FeedbackText: "## Failing CI checks\n- build FAILED\n",
		},
	})
	fakeSpawner := &fakePRFixSpawner{}
	listener.SetPRFixSpawner(fakeSpawner)

	var buf bytes.Buffer
	redirectInfoLog(t, &buf)

	er := storage.repo.(*EntRepository)
	listener.ReconcilePRPending(context.Background(), er)

	assert.True(t, fakeSpawner.spawnCalled, "CIFailing alone should trigger a fix-session spawn")
	assert.Contains(t, buf.String(), "CI=true")
	assert.Contains(t, buf.String(), "conflict=false")
}

// TestReconcilePRPending_DoesNotRespawnFixSession_When_StillCIFailingOnNextTick
// is the regression test for the MAJOR bug flagged in
// docs/tasks/backlog-feature-improvement.md's 2026-07-28 entry:
// ReconcilePRPending's CI-failing branch called AutoReopenForPRFix directly,
// on every reconciliation tick, with no remediation backoff gate — able to
// respawn a fix session every ~60s tick indefinitely for a PR that keeps
// failing CI, unlike every sibling remediation call site in this file. The
// first tick must still spawn (matching pre-fix behavior — RemediationDue is
// ungated until a stuck row exists), but an immediate second tick against
// the still-CI-failing PR must NOT spawn again: the just-recorded first
// attempt's next_remediation_at (30 minutes out, per
// remediationBackoffSchedule[0]) is still in the future.
func TestReconcilePRPending_DoesNotRespawnFixSession_When_StillCIFailingOnNextTick(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item := newPRPendingTestItem(t, storage, 152)

	listener := NewBacklogLifecycleListener(storage)
	overridePRPendingChecker(t, listener, &fakePRPendingChecker{
		status: &git.PRStatus{
			CIFailing:    true,
			FeedbackText: "## Failing CI checks\n- build FAILED\n",
		},
	})
	fakeSpawner := &fakePRFixSpawner{}
	listener.SetPRFixSpawner(fakeSpawner)

	er := storage.repo.(*EntRepository)
	listener.ReconcilePRPending(ctx, er)
	require.Equal(t, 1, fakeSpawner.callCount, "first tick (fresh row) must still spawn — RemediationDue is ungated until a stuck row exists")

	// Second tick, same still-CI-failing PR, no time elapsed: must be gated.
	listener.ReconcilePRPending(ctx, er)
	assert.Equal(t, 1, fakeSpawner.callCount, "a second tick against a still-CI-failing PR must not respawn faster than the backoff schedule allows")

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	row, ok := findOpenStuckStateFor(open, item.ID, domain.StuckReasonPRNeedsFix)
	require.True(t, ok, "a pr_needs_fix stuck row must be open while the PR keeps failing CI")
	assert.Equal(t, int32(1), row.RemediationAttempts, "the gated second tick must not consume a second remediation attempt")
}

// TestReconcilePRPending_RespawnsFixSession_When_BackoffElapses verifies the
// gate added for the bug above is a genuine backoff, not a permanent block:
// once next_remediation_at has passed, the next tick against a still-CI-
// failing PR must spawn again.
func TestReconcilePRPending_RespawnsFixSession_When_BackoffElapses(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item := newPRPendingTestItem(t, storage, 152)

	listener := NewBacklogLifecycleListener(storage)
	overridePRPendingChecker(t, listener, &fakePRPendingChecker{
		status: &git.PRStatus{
			CIFailing:    true,
			FeedbackText: "## Failing CI checks\n- build FAILED\n",
		},
	})
	fakeSpawner := &fakePRFixSpawner{}
	listener.SetPRFixSpawner(fakeSpawner)

	er := storage.repo.(*EntRepository)
	listener.ReconcilePRPending(ctx, er)
	require.Equal(t, 1, fakeSpawner.callCount)

	backdateNextRemediationAt(t, er, item.ID, domain.StuckReasonPRNeedsFix, time.Now().Add(-time.Second))

	listener.ReconcilePRPending(ctx, er)
	assert.Equal(t, 2, fakeSpawner.callCount, "once the backoff window has elapsed, the next tick must retry")
}

// TestReconcilePRPending_ClosedWithoutMerge_DoesNotRespawn_When_BackoffNotDue
// covers the sibling call site: the closed-without-merging branch shares the
// same remediatePRFixWithBackoffGate helper, so it must be gated identically.
// Also verifies the BUG-040 PR-field-clearing logic is skipped (not just the
// spawn) when the gate declines to attempt — clearing PrNumber/PrURL when
// nothing was actually attempted would reproduce BUG-040's exact dead end.
func TestReconcilePRPending_ClosedWithoutMerge_DoesNotRespawn_When_BackoffNotDue(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item := newPRPendingTestItem(t, storage, 173)

	listener := NewBacklogLifecycleListener(storage)
	overridePRPendingChecker(t, listener, &fakePRPendingChecker{
		merged: false,
		status: &git.PRStatus{IsClosed: true},
	})
	fakeSpawner := &fakePRFixSpawner{
		onCall: func() {
			_, transErr := storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusInProgress, nil, TriggeredBySystem)
			require.NoError(t, transErr)
		},
	}
	listener.SetPRFixSpawner(fakeSpawner)

	er := storage.repo.(*EntRepository)
	listener.ReconcilePRPending(ctx, er)
	require.Equal(t, 1, fakeSpawner.callCount, "first tick must still spawn and reopen the item")

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	require.Equal(t, string(BacklogStatusInProgress), fetched.Status, "first tick's reopen must have succeeded")

	// Put the item back at pr_pending with the same closed PR reference, as if
	// a fresh push landed the same closed PR number again — the realistic
	// repeat-tick scenario this gate protects against.
	_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusPRPending, nil, TriggeredBySystem)
	require.NoError(t, err)

	listener.ReconcilePRPending(ctx, er)
	assert.Equal(t, 1, fakeSpawner.callCount, "a second tick within the backoff window must not respawn again")
}

// TestReconcilePRPending_SpawnsFixSession_WhenHasBlockingReviewsTrue is a
// regression test for the pre-existing HasBlockingReviews trigger, previously
// untested at the gate level. It also asserts the log line reports
// conflict=false so the extended log format doesn't spuriously report a
// conflict when only a review blocked.
func TestReconcilePRPending_SpawnsFixSession_WhenHasBlockingReviewsTrue(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	newPRPendingTestItem(t, storage, 152)

	listener := NewBacklogLifecycleListener(storage)
	overridePRPendingChecker(t, listener, &fakePRPendingChecker{
		status: &git.PRStatus{
			HasBlockingReviews: true,
			FeedbackText:       "## Review: changes requested by @reviewer1\n",
		},
	})
	fakeSpawner := &fakePRFixSpawner{}
	listener.SetPRFixSpawner(fakeSpawner)

	var buf bytes.Buffer
	redirectInfoLog(t, &buf)

	er := storage.repo.(*EntRepository)
	listener.ReconcilePRPending(context.Background(), er)

	assert.True(t, fakeSpawner.spawnCalled, "HasBlockingReviews alone should trigger a fix-session spawn")
	assert.Contains(t, buf.String(), "reviews=true")
	assert.Contains(t, buf.String(), "conflict=false")
}

// TestReconcilePRPending_NoSpawn_WhenAllSignalsFalse verifies that a healthy
// PR (all three signals false) does not trigger a spawn and leaves the item
// in pr_pending — the extended 3-way gate must not over-trigger.
func TestReconcilePRPending_NoSpawn_WhenAllSignalsFalse(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	item := newPRPendingTestItem(t, storage, 152)

	listener := NewBacklogLifecycleListener(storage)
	overridePRPendingChecker(t, listener, &fakePRPendingChecker{
		status: &git.PRStatus{},
	})
	fakeSpawner := &fakePRFixSpawner{}
	listener.SetPRFixSpawner(fakeSpawner)

	er := storage.repo.(*EntRepository)
	listener.ReconcilePRPending(context.Background(), er)

	assert.False(t, fakeSpawner.spawnCalled, "healthy PR (all signals false) must not trigger a spawn")

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusPRPending), fetched.Status, "item status must remain pr_pending")
}

// TestReconcilePRPending_ClosedWithoutMerge_ClearsPRFieldsAndReopens verifies that
// a PR closed without merging (state=CLOSED, not caught by IsPRMerged since that
// only returns true for MERGED) does not stall forever as a "healthy open PR" —
// it must spawn a fix session and, once that reopen is CONFIRMED to have moved
// the item off pr_pending, clear the cached PrNumber/PrURL (so the next
// pushAndCreatePR creates a fresh PR instead of reusing the closed one).
//
// fakeSpawner.onCall performs the real transition AutoReopenForPRFix would
// perform on success (pr_pending -> in_progress), mirroring production
// behavior — this is what distinguishes "reopen succeeded" from BUG-040's
// no-op/error cases below, which must NOT clear the fields.
func TestReconcilePRPending_ClosedWithoutMerge_ClearsPRFieldsAndReopens(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item := newPRPendingTestItem(t, storage, 152)

	listener := NewBacklogLifecycleListener(storage)
	overridePRPendingChecker(t, listener, &fakePRPendingChecker{
		merged: false,
		status: &git.PRStatus{IsClosed: true},
	})
	fakeSpawner := &fakePRFixSpawner{
		onCall: func() {
			_, transErr := storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusInProgress, nil, TriggeredBySystem)
			require.NoError(t, transErr)
		},
	}
	listener.SetPRFixSpawner(fakeSpawner)

	er := storage.repo.(*EntRepository)
	listener.ReconcilePRPending(ctx, er)

	assert.True(t, fakeSpawner.spawnCalled, "a closed-without-merge PR must trigger a fix-session spawn")

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusInProgress), fetched.Status, "item must have actually left pr_pending")
	assert.Equal(t, 0, fetched.PrNumber, "PrNumber must be cleared so the next pushAndCreatePR creates a fresh PR")
	assert.Empty(t, fetched.PrURL, "PrURL must be cleared so the next pushAndCreatePR creates a fresh PR")
}

// TestReconcilePRPending_ClosedWithoutMerge_LeavesPRFieldsIntact_When_ReopenNoOps
// is the regression test for BUG-040: a live incident where an item ended up
// pr_pending with pr_number=0 and pr_url="" — a permanent dead end, since
// FindPRPendingItems' PrNumberGT(0) filter then structurally excludes the item
// from every future tick of ReconcilePRPending (and everything downstream of
// it), with nothing left to retry.
//
// Before the fix, this branch cleared PrURL/PrNumber unconditionally BEFORE
// calling AutoReopenForPRFix. AutoReopenForPRFix has legitimate no-op guard
// paths (an active work session already running, the rework cap) that return
// nil without transitioning the item off pr_pending — exactly what the fake
// spawner here simulates (no onCall, no err: the real AutoReopenForPRFix
// contract for those guards). After the fix, the fields must stay intact
// whenever the item is still observed in pr_pending after the call, so the
// item remains visible/retryable on the next tick instead of vanishing.
func TestReconcilePRPending_ClosedWithoutMerge_LeavesPRFieldsIntact_When_ReopenNoOps(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item := newPRPendingTestItem(t, storage, 173)

	listener := NewBacklogLifecycleListener(storage)
	overridePRPendingChecker(t, listener, &fakePRPendingChecker{
		merged: false,
		status: &git.PRStatus{IsClosed: true},
	})
	// No onCall, no err: simulates AutoReopenForPRFix's no-op guard paths
	// (hasActiveWorkSession / rework cap), which return nil without
	// transitioning the item off pr_pending.
	fakeSpawner := &fakePRFixSpawner{}
	listener.SetPRFixSpawner(fakeSpawner)

	er := storage.repo.(*EntRepository)
	listener.ReconcilePRPending(ctx, er)

	assert.True(t, fakeSpawner.spawnCalled, "AutoReopenForPRFix must still be attempted")

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusPRPending), fetched.Status, "item must remain pr_pending when the reopen no-ops")
	assert.Equal(t, 173, fetched.PrNumber, "PrNumber must NOT be cleared when the reopen never actually happened (BUG-040)")
	assert.Equal(t, item.PrURL, fetched.PrURL, "PrURL must NOT be cleared when the reopen never actually happened (BUG-040)")
}

// TestReconcilePRPending_ClosedWithoutMerge_LeavesPRFieldsIntact_When_ReopenErrors
// covers the sibling BUG-040 shape: AutoReopenForPRFix returns a genuine error
// (e.g. a storage failure) rather than a no-op. The fields must stay intact
// here too — an error means nothing was transitioned, so clearing the PR
// reference would produce the same dead end as the no-op case.
func TestReconcilePRPending_ClosedWithoutMerge_LeavesPRFieldsIntact_When_ReopenErrors(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item := newPRPendingTestItem(t, storage, 173)

	listener := NewBacklogLifecycleListener(storage)
	overridePRPendingChecker(t, listener, &fakePRPendingChecker{
		merged: false,
		status: &git.PRStatus{IsClosed: true},
	})
	fakeSpawner := &fakePRFixSpawner{err: errors.New("simulated spawn failure")}
	listener.SetPRFixSpawner(fakeSpawner)

	er := storage.repo.(*EntRepository)
	listener.ReconcilePRPending(ctx, er)

	assert.True(t, fakeSpawner.spawnCalled)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusPRPending), fetched.Status)
	assert.Equal(t, 173, fetched.PrNumber, "PrNumber must NOT be cleared when AutoReopenForPRFix errored (BUG-040)")
	assert.Equal(t, item.PrURL, fetched.PrURL, "PrURL must NOT be cleared when AutoReopenForPRFix errored (BUG-040)")
}

// TestReconcilePRPending_ClosedPR_ClosesAsSupersededInsteadOfReopening_When_LastCommitAlreadyOnMain
// is the regression test for BUG-036: a live incident where a work session
// closed its own PR directly (via `gh pr close`, bypassing this reconciler
// entirely) because the item's work had already shipped through another
// path — but ReconcilePRPending's "closed without merging" branch didn't
// carry the same closeIfSupersededByMain check its CI-failing/blocked/
// conflicting sibling branch already had (BUG-032), so it unconditionally
// spawned another wasted rework cycle instead of recognizing the item was
// already done. Mirrors TestReconcilePRPending_ClosesSupersededPR_When_LastCommitAlreadyOnMain
// but with prStatus.IsClosed=true (the "closed" branch) instead of
// CIFailing=true (the "still open" branch).
func TestReconcilePRPending_ClosedPR_ClosesAsSupersededInsteadOfReopening_When_LastCommitAlreadyOnMain(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	dir := t.TempDir()
	runGitTestCmd(t, dir, "init", "-b", "main")
	runGitTestCmd(t, dir, "config", "user.email", "test@example.com")
	runGitTestCmd(t, dir, "config", "user.name", "Test")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "shipped.txt"), []byte("already shipped\n"), 0o644))
	runGitTestCmd(t, dir, "add", "shipped.txt")
	runGitTestCmd(t, dir, "commit", "-m", "the fix, already on main via a different PR")
	shippedSHA := strings.TrimSpace(runGitTestCmd(t, dir, "rev-parse", "HEAD"))

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "Closed-PR superseded test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusPRPending),
		RepoPath:           dir,
	})
	require.NoError(t, err)
	prURL := "https://github.com/owner/repo/pull/173"
	prNumber := 173
	_, err = storage.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{PrURL: &prURL, PrNumber: &prNumber}, nil)
	require.NoError(t, err)

	workIS, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: uuid.New().String(),
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)
	require.NoError(t, storage.UpdateItemSessionGitActivity(ctx, workIS.ID, shippedSHA, "the fix", time.Now(), 1))

	listener := NewBacklogLifecycleListener(storage)
	checker := &fakePRPendingChecker{status: &git.PRStatus{IsClosed: true}}
	overridePRPendingChecker(t, listener, checker)
	fakeSpawner := &fakePRFixSpawner{}
	listener.SetPRFixSpawner(fakeSpawner)

	er := storage.repo.(*EntRepository)
	listener.ReconcilePRPending(ctx, er)

	assert.False(t, fakeSpawner.spawnCalled, "a closed PR whose work already shipped must not trigger another rework cycle")
	assert.True(t, checker.closeCalled, "the stale closed PR should still get an explanatory close comment")
	assert.Equal(t, 173, checker.closedPR)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusDone), fetched.Status, "item must transition straight to done once its commit is confirmed already on main")
	assert.Equal(t, 0, fetched.PrNumber, "PrNumber must be cleared")
	assert.Empty(t, fetched.PrURL, "PrURL must be cleared")
}

// TestReconcilePRPending_ClosesSupersededPR_When_LastCommitAlreadyOnMain is the
// regression test for BUG-032's live incident: an item's PR was CI-failing/
// conflicting purely because it had drifted stale behind an already-shipped
// fix (the real work landed on main via a different PR entirely) — not
// because its own code was wrong. Before the fix, ReconcilePRPending would
// spawn yet another rework+review cycle against an empty/irrelevant diff
// every time this happened. After the fix, when the item's last work
// session's commit is already an ancestor of main, the stale PR is closed as
// superseded and the item transitions straight to done — no fix-session spawn.
func TestReconcilePRPending_ClosesSupersededPR_When_LastCommitAlreadyOnMain(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	// A tiny real repo whose "main" branch already contains a commit —
	// standing in for the live incident where the item's actual fix had
	// already landed on main via a different PR.
	dir := t.TempDir()
	runGitTestCmd(t, dir, "init", "-b", "main")
	runGitTestCmd(t, dir, "config", "user.email", "test@example.com")
	runGitTestCmd(t, dir, "config", "user.name", "Test")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "shipped.txt"), []byte("already shipped\n"), 0o644))
	runGitTestCmd(t, dir, "add", "shipped.txt")
	runGitTestCmd(t, dir, "commit", "-m", "the fix, already on main via a different PR")
	shippedSHA := strings.TrimSpace(runGitTestCmd(t, dir, "rev-parse", "HEAD"))

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "Superseded PR test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusPRPending),
		RepoPath:           dir,
	})
	require.NoError(t, err)
	prURL := "https://github.com/owner/repo/pull/173"
	prNumber := 173
	_, err = storage.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{PrURL: &prURL, PrNumber: &prNumber}, nil)
	require.NoError(t, err)

	// A work session whose last commit is the one already on main — the same
	// shape as the live incident (this item's own branch's work already
	// shipped, but its stale PR still references it).
	workIS, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: uuid.New().String(),
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)
	require.NoError(t, storage.UpdateItemSessionGitActivity(ctx, workIS.ID, shippedSHA, "the fix", time.Now(), 1))

	listener := NewBacklogLifecycleListener(storage)
	checker := &fakePRPendingChecker{
		status: &git.PRStatus{
			CIFailing:    true,
			FeedbackText: "## Failing CI checks\n- build FAILED\n",
		},
	}
	overridePRPendingChecker(t, listener, checker)
	fakeSpawner := &fakePRFixSpawner{}
	listener.SetPRFixSpawner(fakeSpawner)

	er := storage.repo.(*EntRepository)
	listener.ReconcilePRPending(ctx, er)

	assert.False(t, fakeSpawner.spawnCalled, "a superseded PR must not trigger another fix-session spawn")
	assert.True(t, checker.closeCalled, "the stale PR must be closed")
	assert.Equal(t, 173, checker.closedPR)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusDone), fetched.Status, "item must transition straight to done once its commit is confirmed already on main")
	assert.Equal(t, 0, fetched.PrNumber, "PrNumber must be cleared")
	assert.Empty(t, fetched.PrURL, "PrURL must be cleared")
}

// TestReconcilePRPending_SpawnsFixSession_When_LastCommitNotOnMain verifies
// the new BUG-032 check doesn't over-trigger: a genuinely broken PR (its last
// commit is real work that simply hasn't reached main) must still go through
// the normal fix-session spawn path, not get treated as superseded.
func TestReconcilePRPending_SpawnsFixSession_When_LastCommitNotOnMain(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	dir := t.TempDir()
	runGitTestCmd(t, dir, "init", "-b", "main")
	runGitTestCmd(t, dir, "config", "user.email", "test@example.com")
	runGitTestCmd(t, dir, "config", "user.name", "Test")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "base.txt"), []byte("base\n"), 0o644))
	runGitTestCmd(t, dir, "add", "base.txt")
	runGitTestCmd(t, dir, "commit", "-m", "base commit")

	// Unmerged feature work — never reached main.
	runGitTestCmd(t, dir, "checkout", "-b", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.txt"), []byte("wip\n"), 0o644))
	runGitTestCmd(t, dir, "add", "feature.txt")
	runGitTestCmd(t, dir, "commit", "-m", "feature work, not yet on main")
	featureSHA := strings.TrimSpace(runGitTestCmd(t, dir, "rev-parse", "HEAD"))
	runGitTestCmd(t, dir, "checkout", "main")

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "Genuinely broken PR test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusPRPending),
		RepoPath:           dir,
	})
	require.NoError(t, err)
	prURL := "https://github.com/owner/repo/pull/174"
	prNumber := 174
	_, err = storage.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{PrURL: &prURL, PrNumber: &prNumber}, nil)
	require.NoError(t, err)

	workIS, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: uuid.New().String(),
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)
	require.NoError(t, storage.UpdateItemSessionGitActivity(ctx, workIS.ID, featureSHA, "feature work", time.Now(), 1))

	listener := NewBacklogLifecycleListener(storage)
	checker := &fakePRPendingChecker{
		status: &git.PRStatus{
			CIFailing:    true,
			FeedbackText: "## Failing CI checks\n- build FAILED\n",
		},
	}
	overridePRPendingChecker(t, listener, checker)
	fakeSpawner := &fakePRFixSpawner{}
	listener.SetPRFixSpawner(fakeSpawner)

	er := storage.repo.(*EntRepository)
	listener.ReconcilePRPending(ctx, er)

	assert.True(t, fakeSpawner.spawnCalled, "a genuinely broken PR (commit not on main) must still spawn a fix session")
	assert.False(t, checker.closeCalled, "a genuinely broken PR must not be closed as superseded")

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusPRPending), fetched.Status)
}

// TestBackfillMissingPRNumbers_ParsesNumberFromURL is a regression test for a
// live-data bug found via manual QA: a pr_pending item can have pr_url set but
// pr_number left at 0 (e.g. from a pre-migration row), making it permanently
// invisible to FindPRPendingItems' PrNumberGT(0) filter. Verifies the number is
// parsed out of the URL and persisted.
func TestBackfillMissingPRNumbers_ParsesNumberFromURL(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "Missing pr_number test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusPRPending),
		RepoPath:           "/tmp/fake-repo",
	})
	require.NoError(t, err)

	prURL := "https://github.com/tstapler/stapler-squad/pull/148"
	_, err = storage.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{PrURL: &prURL}, nil)
	require.NoError(t, err)

	er := storage.repo.(*EntRepository)

	// Before backfill: invisible to FindPRPendingItems despite being pr_pending with a URL.
	before, err := er.FindPRPendingItems(ctx)
	require.NoError(t, err)
	assert.Empty(t, before, "item with pr_number=0 must not be returned before backfill")

	n, err := er.BackfillMissingPRNumbers(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, 148, fetched.PrNumber)

	// After backfill: now visible to FindPRPendingItems.
	after, err := er.FindPRPendingItems(ctx)
	require.NoError(t, err)
	require.Len(t, after, 1)
	assert.Equal(t, item.ID, after[0].ID.String())
}

// fakePRCreator is a test double implementing prCreator, letting tests inject
// canned push/CreatePR results without a live git worktree or authenticated gh CLI.
type fakePRCreator struct {
	pushErr      error
	createErr    error
	createURL    string
	createNumber int
	autoMergeErr error
	pushCalled   bool
	createCalled bool
}

func (f *fakePRCreator) CommitChanges(commitMessage string) error { return nil }
func (f *fakePRCreator) PushBranch() error {
	f.pushCalled = true
	return f.pushErr
}
func (f *fakePRCreator) CreatePR(title, body string) (string, int, error) {
	f.createCalled = true
	return f.createURL, f.createNumber, f.createErr
}
func (f *fakePRCreator) EnablePRAutoMerge(prNumber int) error { return f.autoMergeErr }

// fakeOneShotShipRunnerCall records a single RunOneShotForSession invocation's
// arguments, so tests can assert shipViaAgentOrFallback called it with the
// right session UUID and prompt.
type fakeOneShotShipRunnerCall struct {
	SessionID string
	Prompt    string
}

// fakeOneShotShipRunner is a test double implementing OneShotShipRunner,
// letting tests inject a canned PR URL or error without a live headless pool
// or session.Instance registry.
type fakeOneShotShipRunner struct {
	calls []fakeOneShotShipRunnerCall
	prURL string
	err   error
}

func (f *fakeOneShotShipRunner) RunOneShotForSession(ctx context.Context, sessionID, prompt string, timeoutSeconds int32) (string, error) {
	f.calls = append(f.calls, fakeOneShotShipRunnerCall{SessionID: sessionID, Prompt: prompt})
	return f.prURL, f.err
}

// fakeNotifierCall records a single Notify invocation's title and message body, so
// tests can assert on interpolated message content (e.g. that a verdict/outcome
// actually reached the message), not just which notification fired.
type fakeNotifierCall struct {
	Title   string
	Message string
}

// fakeNotifier is a test double implementing Notifier, recording every call.
type fakeNotifier struct {
	calls []fakeNotifierCall // one per Notify call, in order
}

func (f *fakeNotifier) Notify(itemID, title, message string, notificationType, priority int32) {
	f.calls = append(f.calls, fakeNotifierCall{Title: title, Message: message})
}

// titles returns just the Title of every recorded call, in order — for tests (the
// majority) that only care which notification fired, not its message body.
func (f *fakeNotifier) titles() []string {
	titles := make([]string, len(f.calls))
	for i, c := range f.calls {
		titles[i] = c.Title
	}
	return titles
}

// newPushAndCreatePRTestFixture creates a review-status BacklogItem with a linked
// work ItemSession and a saved Instance backed by a git worktree, so
// GetWorktreeDataBySessionUUID (which pushAndCreatePR depends on) resolves.
// Returns the item and the ItemSessionSummary pushAndCreatePR expects as its
// second argument.
func newPushAndCreatePRTestFixture(t *testing.T, storage *Storage) (*BacklogItemData, ItemSessionSummary) {
	t.Helper()
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "Push and create PR test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusReview),
		RepoPath:           "/tmp/fake-repo",
	})
	require.NoError(t, err)

	sessionUUID := uuid.New().String()
	is, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)

	inst := newTestInstance("push-pr-test")
	inst.UUID = sessionUUID
	inst.gitManager.worktree = git.NewGitWorktreeFromStorage(
		"/tmp/fake-repo", "/tmp/fake-repo/../worktrees/push-pr-test", "push-pr-test", "backlog/push-pr-test", "abc123")
	require.NoError(t, storage.SaveInstances([]*Instance{inst}))

	return item, ItemSessionSummary{ID: is.ID, SessionUUID: sessionUUID, BacklogItemID: item.ID}
}

// TestPushAndCreatePR_PushFails_LeavesItemInReview_AndNotifies verifies that a
// failed git push does NOT transition the item to done — code is committed but
// never reached GitHub, so marking it done would silently discard that fact.
// The item must stay in review and a notification must be published.
func TestPushAndCreatePR_PushFails_LeavesItemInReview_AndNotifies(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	item, is := newPushAndCreatePRTestFixture(t, storage)

	listener := NewBacklogLifecycleListener(storage)
	fakeCreator := &fakePRCreator{pushErr: errors.New("push rejected: non-fast-forward")}
	listener.SetPRCreatorFactory(func(repoPath, worktreePath, sessionName, branchName, baseCommitSHA string) prCreator {
		return fakeCreator
	})
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	listener.pushAndCreatePR(context.Background(), item, is)

	assert.True(t, fakeCreator.pushCalled)
	assert.False(t, fakeCreator.createCalled, "CreatePR must not be attempted after a push failure")

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusReview), fetched.Status, "item must stay in review, not silently become done")
	assert.Contains(t, notifier.titles(), "PR creation failed")
}

// TestPushAndCreatePR_RepeatedPushFailure_DedupsToast verifies the fix for a
// runaway duplicate "PR creation failed" toast: when the same item's push keeps
// failing across repeated pushAndCreatePR calls (e.g. a non-fast-forward
// rejection retried every reconciliation tick), only the FIRST failure fires the
// ephemeral ERROR toast. Subsequent failures on the same still-open push_failed
// stuck row must not re-fire it — this is the notify-once dedup used by every
// other stuck reason (see markAbandonedReview).
func TestPushAndCreatePR_RepeatedPushFailure_DedupsToast(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	item, is := newPushAndCreatePRTestFixture(t, storage)

	listener := NewBacklogLifecycleListener(storage)
	fakeCreator := &fakePRCreator{pushErr: errors.New("push rejected: non-fast-forward")}
	listener.SetPRCreatorFactory(func(repoPath, worktreePath, sessionName, branchName, baseCommitSHA string) prCreator {
		return fakeCreator
	})
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	ctx := context.Background()
	listener.pushAndCreatePR(ctx, item, is)
	listener.pushAndCreatePR(ctx, item, is)
	listener.pushAndCreatePR(ctx, item, is)

	toastCount := 0
	for _, title := range notifier.titles() {
		if title == "PR creation failed" {
			toastCount++
		}
	}
	assert.Equal(t, 1, toastCount, "repeated failures on the same open stuck row must not re-fire the toast")
}

// TestPushAndCreatePR_CreatePRFails_LeavesItemInReview_AndNotifies verifies that a
// failed `gh pr create` call (push already succeeded) does NOT transition the item
// to done, for the same reason as the push-failure case above.
func TestPushAndCreatePR_CreatePRFails_LeavesItemInReview_AndNotifies(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	item, is := newPushAndCreatePRTestFixture(t, storage)

	listener := NewBacklogLifecycleListener(storage)
	fakeCreator := &fakePRCreator{createErr: errors.New("gh: authentication required")}
	listener.SetPRCreatorFactory(func(repoPath, worktreePath, sessionName, branchName, baseCommitSHA string) prCreator {
		return fakeCreator
	})
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	listener.pushAndCreatePR(context.Background(), item, is)

	assert.True(t, fakeCreator.pushCalled)
	assert.True(t, fakeCreator.createCalled)

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusReview), fetched.Status, "item must stay in review, not silently become done")
	assert.Contains(t, notifier.titles(), "PR creation failed")
}

// dbClosingPRCreator wraps a *fakePRCreator, closing the given EntRepository's
// underlying DB connection immediately after CreatePR returns — simulating a
// storage failure that arrives exactly between "PR created on GitHub" and
// "PR fields persisted on the item" (BUG-040 root cause #1). Storage-level
// helpers that shortcut straight to the concrete *EntRepository via a type
// assertion (e.g. GetWorktreeDataBySessionUUID, and MarkStuck reached through
// stayInReviewAndNotify) must keep working right up to this point — which
// rules out swapping storage.repo for a wrapper type, since that assertion
// would then fail for the entire call, not just the one write this test
// targets. Closing the real repository's connection at the right moment
// preserves its concrete type while still making the very next write fail.
type dbClosingPRCreator struct {
	*fakePRCreator
	repo *EntRepository
}

func (f *dbClosingPRCreator) CreatePR(title, body string) (string, int, error) {
	url, num, err := f.fakePRCreator.CreatePR(title, body)
	f.repo.Close()
	return url, num, err
}

// TestPushAndCreatePR_PRFieldsPersistFails_StaysInReview_AndNotifies is the
// regression test for BUG-040 root cause #1: pushAndCreatePR previously only
// logged a WARNING when the UpdateBacklogItem call caching a freshly created
// PR's PrURL/PrNumber failed, then proceeded unconditionally to
// resolveToPRPending anyway — landing the item in pr_pending with
// pr_number=0/pr_url="", a permanent dead end invisible to every downstream
// reconciler (FindPRPendingItems' PrNumberGT(0) filter structurally excludes
// it). After the fix, a persist failure here must be treated exactly like a
// push/PR-creation failure: the item stays in review, never silently
// entering pr_pending with no way to look the PR back up.
//
// The underlying DB connection is closed (via dbClosingPRCreator) right after
// CreatePR succeeds, so the PR-fields UpdateBacklogItem call — and every
// other DB call after it, including stayInReviewAndNotify's own MarkStuck —
// genuinely fails, exactly like a real transient storage outage would. That
// means the durable push_failed row (and the notification gated on it) also
// doesn't get written in this specific scenario — a real, pre-existing,
// narrower gap in stayInReviewAndNotify's own error handling, not something
// this fix introduces or is scoped to close. What this test asserts is the
// invariant BUG-040 is actually about: the item must never silently reach
// pr_pending with an empty PR reference.
func TestPushAndCreatePR_PRFieldsPersistFails_StaysInReview_AndNotifies(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "storage-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)
	dbPath := filepath.Join(tmpDir, fmt.Sprintf("test-%d.db", time.Now().UnixNano()))

	repo, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)
	storage, err := NewStorageWithRepository(repo)
	require.NoError(t, err)

	item, is := newPushAndCreatePRTestFixture(t, storage)

	listener := NewBacklogLifecycleListener(storage)
	creator := &dbClosingPRCreator{
		fakePRCreator: &fakePRCreator{createURL: "https://github.com/owner/repo/pull/999", createNumber: 999},
		repo:          repo,
	}
	listener.SetPRCreatorFactory(func(repoPath, worktreePath, sessionName, branchName, baseCommitSHA string) prCreator {
		return creator
	})

	listener.pushAndCreatePR(context.Background(), item, is)

	assert.True(t, creator.pushCalled)
	assert.True(t, creator.createCalled, "CreatePR must still be attempted — the failure is in caching its result")

	// Re-open a fresh connection to the same on-disk DB to inspect final
	// state — the original repo/client was closed above to force the
	// PR-fields persist to fail.
	repo2, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)
	defer repo2.Close()
	storage2, err := NewStorageWithRepository(repo2)
	require.NoError(t, err)

	fetched, err := storage2.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusReview), fetched.Status,
		"item must stay in review when the PR-fields persist fails (BUG-040) — must never silently enter pr_pending with pr_number=0")
	assert.Equal(t, 0, fetched.PrNumber, "PrNumber must remain unset since the persist failed")
	assert.Empty(t, fetched.PrURL, "PrURL must remain unset since the persist failed")
}

// TestPushAndCreatePR_AutoMergeFails_StillTransitionsButNotifies verifies the fix for
// the silent-auto-merge-fallback bug: when EnablePRAutoMerge fails (e.g. no branch
// protection configured), the item still transitions to pr_pending as normal (the PR
// itself was created successfully — ReconcilePRPending will still poll it), but the
// operator must be notified that nothing will initiate the merge automatically.
// Previously this only reached the log file.
func TestPushAndCreatePR_AutoMergeFails_StillTransitionsButNotifies(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	item, is := newPushAndCreatePRTestFixture(t, storage)

	listener := NewBacklogLifecycleListener(storage)
	fakeCreator := &fakePRCreator{
		createURL:    "https://github.com/TylerStaplerAtFanatics/stapler-squad/pull/321",
		createNumber: 321,
		autoMergeErr: errors.New("auto-merge is not enabled for this repository"),
	}
	listener.SetPRCreatorFactory(func(repoPath, worktreePath, sessionName, branchName, baseCommitSHA string) prCreator {
		return fakeCreator
	})
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	listener.pushAndCreatePR(context.Background(), item, is)

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusPRPending), fetched.Status, "the PR was created successfully — the item must still advance to pr_pending")
	assert.Contains(t, notifier.titles(), "Auto-merge not enabled", "operator must be told the PR needs a manual merge, not just a log line")
}

// TestPushAndCreatePR_ReusesExistingPR_WhenAlreadySet verifies the "PR already
// exists from a previous attempt" branch skips CreatePR and reuses the cached
// PrNumber/PrURL — a regression guard for the reuse-vs-recreate logic that
// TestReconcilePRPending's closed-PR handling depends on clearing correctly.
func TestPushAndCreatePR_ReusesExistingPR_WhenAlreadySet(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	item, is := newPushAndCreatePRTestFixture(t, storage)
	existingURL := "https://github.com/TylerStaplerAtFanatics/stapler-squad/pull/999"
	existingNum := 999
	updated, err := storage.UpdateBacklogItem(context.Background(), item.ID, BacklogItemUpdate{
		PrURL:    &existingURL,
		PrNumber: &existingNum,
	}, nil)
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	fakeCreator := &fakePRCreator{}
	listener.SetPRCreatorFactory(func(repoPath, worktreePath, sessionName, branchName, baseCommitSHA string) prCreator {
		return fakeCreator
	})

	listener.pushAndCreatePR(context.Background(), updated, is)

	assert.False(t, fakeCreator.createCalled, "must reuse the existing PR instead of creating a new one")

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusPRPending), fetched.Status)
	assert.Equal(t, existingNum, fetched.PrNumber)
}

// TestPushAndCreatePR_NoWorktree_FallsBackToDone verifies the one case where
// falling back directly to done is still correct: no worktree ever existed, so
// there is genuinely nothing to lose by marking the item done.
func TestPushAndCreatePR_NoWorktree_FallsBackToDone(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "No worktree test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusReview),
	})
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	listener.pushAndCreatePR(ctx, item, ItemSessionSummary{SessionUUID: uuid.New().String()})

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusDone), fetched.Status)
}

// ─── RecordPRCreatedOutOfBand ──────────────────────────────────────────────────
//
// Regression coverage for the status-desync bug behind PR #157's linked backlog
// item getting stuck at "review"/BOUNCING instead of "pr_pending": the Review
// Queue's manual "Create PR" button drives SessionService.RunOneShot, a path
// that creates a real PR but — unlike pushAndCreatePR — never touched the
// backlog item at all. See docs/tasks/backlog-feature-improvement.md's
// "second, compounding root cause" note and RecordPRCreatedOutOfBand's doc
// comment above for the full trace.

// TestRecordPRCreatedOutOfBand_TransitionsReviewToPRPending verifies the core
// fix: a PR created out-of-band (i.e. not via pushAndCreatePR) for a
// backlog-linked, in-review session moves the item to pr_pending with the PR
// fields populated, exactly like pushAndCreatePR itself would — this is what
// makes the item visible to ReconcilePRPending's FindPRPendingItems query
// again.
func TestRecordPRCreatedOutOfBand_TransitionsReviewToPRPending(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	item, is := newPushAndCreatePRTestFixture(t, storage)

	listener := NewBacklogLifecycleListener(storage)
	listener.SetEnabled(true)

	listener.RecordPRCreatedOutOfBand(context.Background(), is.SessionUUID,
		"https://github.com/tstapler/stapler-squad/pull/157", 157)

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusPRPending), fetched.Status,
		"item must leave review so ReconcilePRPending's FindPRPendingItems can find it")
	assert.Equal(t, 157, fetched.PrNumber)
	assert.Equal(t, "https://github.com/tstapler/stapler-squad/pull/157", fetched.PrURL)
}

// TestRecordPRCreatedOutOfBand_NoOp_WhenSessionNotBacklogLinked verifies the
// overwhelmingly common case — RunOneShot called for a session with no
// linked backlog item — is a silent no-op, not an error.
func TestRecordPRCreatedOutOfBand_NoOp_WhenSessionNotBacklogLinked(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	listener := NewBacklogLifecycleListener(storage)
	listener.SetEnabled(true)

	// Must not panic or block; there is nothing further to assert since no
	// backlog item exists to have been mutated.
	listener.RecordPRCreatedOutOfBand(context.Background(), uuid.New().String(),
		"https://github.com/tstapler/stapler-squad/pull/1", 1)
}

// TestRecordPRCreatedOutOfBand_NoOp_WhenItemNotInReview verifies the precondition
// guard: an item that isn't currently "review" (e.g. already pr_pending from a
// concurrent pushAndCreatePR, or in_progress) is left untouched rather than
// force-transitioned, so this out-of-band path can never fight the item's real
// state owner.
func TestRecordPRCreatedOutOfBand_NoOp_WhenItemNotInReview(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "Not in review test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
		RepoPath:           "/tmp/fake-repo",
	})
	require.NoError(t, err)

	sessionUUID := uuid.New().String()
	_, err = storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	listener.SetEnabled(true)

	listener.RecordPRCreatedOutOfBand(ctx, sessionUUID,
		"https://github.com/tstapler/stapler-squad/pull/1", 1)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusInProgress), fetched.Status,
		"must not transition an item that isn't in review")
	assert.Equal(t, 0, fetched.PrNumber, "must not stamp PR fields onto an item it declined to transition")
}

// TestRecordPRCreatedOutOfBand_NoOp_WhenListenerDisabled verifies the feature-flag
// gate: with the backlog automation feature off (the zero-value default —
// production only enables it via feature_controller.go's SetEnabled(true)),
// this manual-flow reconciliation must not fire either, matching every other
// listener entry point's behavior.
func TestRecordPRCreatedOutOfBand_NoOp_WhenListenerDisabled(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	item, is := newPushAndCreatePRTestFixture(t, storage)

	listener := NewBacklogLifecycleListener(storage)
	// Deliberately not calling SetEnabled(true).

	listener.RecordPRCreatedOutOfBand(context.Background(), is.SessionUUID,
		"https://github.com/tstapler/stapler-squad/pull/157", 157)

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusReview), fetched.Status, "disabled listener must not transition the item")
}

// TestRecordPRCreatedOutOfBand_ClearsAbandonedReviewStuckReason_WhenItemWasStuck
// verifies the resolveStuckLogged side effect actually clears a stale
// abandoned_review row, not just that it's called — code-review found the
// other four tests never left an item in a stuck state before calling
// RecordPRCreatedOutOfBand, so that clearing behavior (part of the method's
// own stated purpose, mirroring pushAndCreatePR's identical resolve calls)
// was never verified to actually work.
func TestRecordPRCreatedOutOfBand_ClearsAbandonedReviewStuckReason_WhenItemWasStuck(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	item, is := newPushAndCreatePRTestFixture(t, storage)

	applied, err := storage.MarkStuck(ctx, item.ID, domain.StuckReasonAbandonedReview, BacklogStatusReview, "review abandoned before manual PR creation")
	require.NoError(t, err)
	require.True(t, applied, "test setup: MarkStuck must actually open a row")

	listener := NewBacklogLifecycleListener(storage)
	listener.SetEnabled(true)

	listener.RecordPRCreatedOutOfBand(ctx, is.SessionUUID,
		"https://github.com/tstapler/stapler-squad/pull/157", 157)

	rows, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	_, stillOpen := findOpenStuckStateFor(rows, item.ID, domain.StuckReasonAbandonedReview)
	assert.False(t, stillOpen, "abandoned_review must be resolved once an out-of-band PR moves the item out of review")
}

// ─── PR-lifecycle status drift (self-heal + immediate recovery) ───────────────
//
// Live 2026-07-20 repro: two backlog items (c2ad7bf3-91bf-4d47-8654-
// 0f2f20869080, PR #251; 6700a3f2-8c0d-4a98-8bbd-39515d5391b1, PR #172) sat at
// status="review" with a real, open, cached PR reference that ReconcilePRPending
// could never see, because it anchors purely on status=="pr_pending". The tests
// below cover the two-part fix: (1) pushAndCreatePR/shipViaAgentOrFallback/
// RecordPRCreatedOutOfBand now attempt an immediate recovery when their own
// resolveToPRPending CAS loses a race after PR fields were already persisted,
// and (2) reconcileDriftedPRItems is the periodic self-heal backstop.

// TestPushAndCreatePR_StatusDriftedDuringRun_RecoversImmediately_WhenNoActiveSession
// reproduces the exact drift mechanism: pushAndCreatePR persists prNumber/prUrl
// unconditionally, then attempts a CAS transition to pr_pending that requires
// the item to still be "review". If some other concurrent, legitimate event
// (simulated here by forcing the item to "in_progress" mid-flight) wins that
// race, the item would previously be left stranded — real PR fields cached,
// status not pr_pending, invisible to ReconcilePRPending forever. With the fix,
// since nothing is actively working the item (no active session), it is
// recovered back to pr_pending immediately rather than waiting for the next
// reconcileDriftedPRItems tick.
func TestPushAndCreatePR_StatusDriftedDuringRun_RecoversImmediately_WhenNoActiveSession(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item, is := newPushAndCreatePRTestFixture(t, storage)

	// The work session has already ended by the time pushAndCreatePR runs — the
	// real precondition under which handleReviewSessionExited(PASS) calls it
	// (workEntry.EndedAt != nil), and also what makes this item eligible for
	// immediate recovery (no active session to defer to).
	require.NoError(t, storage.UpdateItemSessionEnded(ctx, is.ID, time.Now()))

	// Simulate the race: something else moves the item off "review" while
	// pushAndCreatePR's own network calls (push, create PR, enable auto-merge)
	// are still in flight, before its resolveToPRPending call runs.
	_, err := storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusInProgress, nil, TriggeredBySystem)
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	listener.SetEnabled(true)
	fakeCreator := &fakePRCreator{createURL: "https://github.com/tstapler/stapler-squad/pull/251", createNumber: 251}
	listener.SetPRCreatorFactory(func(repoPath, worktreePath, sessionName, branchName, baseCommitSHA string) prCreator {
		return fakeCreator
	})
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	listener.pushAndCreatePR(ctx, item, is)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusPRPending), fetched.Status,
		"item must be recovered to pr_pending immediately, not left stranded at in_progress with a live PR")
	assert.Equal(t, 251, fetched.PrNumber)
	assert.Equal(t, "https://github.com/tstapler/stapler-squad/pull/251", fetched.PrURL)
	assert.Contains(t, notifier.titles(), "Backlog item recovered from stuck state")
}

// TestPushAndCreatePR_StatusDriftedDuringRun_DefersToSelfHeal_WhenActiveSessionExists
// verifies the safety guard: if a new session is actively working the item by
// the time the CAS loses its race, immediate recovery must NOT steal the item
// out from under it — forcing status back to pr_pending here would fight a
// legitimate in-flight rework/fix session exactly the way BUG-026 warns
// against. The item is left at its drifted status; only the periodic
// reconcileDriftedPRItems sweep (itself guarded identically) may ever recover
// it, once that session ends.
func TestPushAndCreatePR_StatusDriftedDuringRun_DefersToSelfHeal_WhenActiveSessionExists(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item, is := newPushAndCreatePRTestFixture(t, storage)
	require.NoError(t, storage.UpdateItemSessionEnded(ctx, is.ID, time.Now()))

	_, err := storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusInProgress, nil, TriggeredBySystem)
	require.NoError(t, err)

	// A brand-new work session starts for the item — e.g. AutoReopenAfterFailedReview
	// legitimately reopened it for rework — and is still active (no EndedAt).
	_, err = storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: uuid.New().String(),
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	listener.SetEnabled(true)
	fakeCreator := &fakePRCreator{createURL: "https://github.com/tstapler/stapler-squad/pull/252", createNumber: 252}
	listener.SetPRCreatorFactory(func(repoPath, worktreePath, sessionName, branchName, baseCommitSHA string) prCreator {
		return fakeCreator
	})
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	listener.pushAndCreatePR(ctx, item, is)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusInProgress), fetched.Status,
		"must not force the item back to pr_pending while a new session is actively working it")
	assert.Equal(t, 252, fetched.PrNumber, "PR fields are still persisted even though the status transition lost its race")
	assert.NotContains(t, notifier.titles(), "Backlog item recovered from stuck state")
}

// TestReconcileDriftedPRItems_RecoversDriftedItemWithNoActiveSession is the
// direct regression test for the periodic self-heal detector: an item with a
// real, cached PR reference sitting at status="review" (matching the live
// 2026-07-20 repro) with no active session must be found and transitioned back
// to pr_pending.
func TestReconcileDriftedPRItems_RecoversDriftedItemWithNoActiveSession(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	prURL := "https://github.com/tstapler/stelekit/pull/251"
	prNumber := 251
	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "Drifted PR item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusReview),
		RepoPath:           "/tmp/fake-repo",
	})
	require.NoError(t, err)
	_, err = storage.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{
		PrURL:    &prURL,
		PrNumber: &prNumber,
	}, nil)
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	listener.SetEnabled(true)
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	listener.reconcileDriftedPRItems(ctx, er)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusPRPending), fetched.Status,
		"self-heal must anchor on the real PR reference and recover the item back to pr_pending")
	assert.Equal(t, prNumber, fetched.PrNumber)
	assert.Equal(t, prURL, fetched.PrURL)
	assert.Contains(t, notifier.titles(), "Backlog item recovered from stuck state")
}

// TestReconcileDriftedPRItems_DoesNotTouchItem_WhenActiveSessionExists verifies
// the safety guard directly against the detector: an item with a real PR
// cached at status="in_progress" — the exact shape AutoReopenForPRFix produces
// while a CI-fix session is legitimately in flight, still pushing new commits
// to the same PR — must be left completely alone while that session is active.
// Forcing it back to pr_pending mid-fix would reintroduce the pr_pending<->
// in_progress churn AutoReopenForPRFix's own hasActiveWorkSession guard was
// added to stop (see its doc comment).
func TestReconcileDriftedPRItems_DoesNotTouchItem_WhenActiveSessionExists(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	prURL := "https://github.com/tstapler/stapler-squad/pull/172"
	prNumber := 172
	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "Mid-fix PR item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
		RepoPath:           "/tmp/fake-repo",
	})
	require.NoError(t, err)
	_, err = storage.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{
		PrURL:    &prURL,
		PrNumber: &prNumber,
	}, nil)
	require.NoError(t, err)

	// An active (not-yet-ended) work session — AutoReopenForPRFix's fix session
	// still pushing to the same PR/branch.
	_, err = storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: uuid.New().String(),
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	listener.SetEnabled(true)
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	listener.reconcileDriftedPRItems(ctx, er)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusInProgress), fetched.Status,
		"must not touch an item with an active work session mid-fix")
	assert.Empty(t, notifier.calls, "must not notify about a recovery that never happened")
}

// TestReconcileDriftedPRItems_DoesNotTouchHealthyItem_WithNoPR verifies the
// query itself is scoped correctly: a "review"-status item with no PR yet
// (genuinely mid-review, not drifted) must never be matched or touched by the
// detector.
func TestReconcileDriftedPRItems_DoesNotTouchHealthyItem_WithNoPR(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "Healthy in-review item, no PR yet",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusReview),
		RepoPath:           "/tmp/fake-repo",
	})
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	listener.SetEnabled(true)
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	listener.reconcileDriftedPRItems(ctx, er)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusReview), fetched.Status, "must not touch a healthy item with no PR")
	assert.Empty(t, notifier.calls)
}

// TestFindDriftedPRItems_ExcludesPRPendingAndTerminalStatuses verifies the
// query's own status filter directly: items already in pr_pending (nothing to
// recover), done, or archived must never be returned, even though they may
// still carry PR fields.
func TestFindDriftedPRItems_ExcludesPRPendingAndTerminalStatuses(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	for _, status := range []BacklogStatus{BacklogStatusPRPending, BacklogStatusDone, BacklogStatusArchived} {
		prURL := "https://github.com/tstapler/stapler-squad/pull/900"
		prNumber := 900
		item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
			Title:              "Terminal/pr_pending item " + string(status),
			AcceptanceCriteria: `[]`,
			Priority:           1,
			Status:             string(status),
			RepoPath:           "/tmp/fake-repo",
		})
		require.NoError(t, err)
		_, err = storage.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{
			PrURL:    &prURL,
			PrNumber: &prNumber,
		}, nil)
		require.NoError(t, err)
	}

	items, err := er.FindDriftedPRItems(ctx)
	require.NoError(t, err)
	assert.Empty(t, items, "pr_pending/done/archived items must never be treated as drifted")
}

// ─── handleReviewSessionExited ────────────────────────────────────────────────
//
// Review now always happens in a real, hidden session.Instance rather than a
// synchronous in-process headless LLM call (see ReviewGateRunner.Run and
// server/services/session_service.go's SpawnReviewSession). The verdict — if
// any — is submitted via the submit_review_verdict MCP tool while that session
// runs, and handleReviewSessionExited is what processes it once the review
// session exits.

// newHandleReviewSessionExitedFixture creates a BacklogItem in "review" status
// with an ended work ItemSession (recorded via SaveInstances so it has a real
// worktree pushAndCreatePR can push) and a review ItemSession linking
// reviewSessionUUID to the item. If verdict is non-nil, it is saved onto the
// review ItemSession via SaveReviewVerdict before returning.
func newHandleReviewSessionExitedFixture(t *testing.T, storage *Storage, verdict *ReviewVerdictData) (item *BacklogItemData, reviewIS ItemSessionSummary, workSessionName string, workIS ItemSessionSummary) {
	t.Helper()
	ctx := context.Background()

	createdItem, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "Handle review session exited test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusReview),
		RepoPath:           "/tmp/fake-repo",
	})
	require.NoError(t, err)

	workSessionUUID := uuid.New().String()
	workSessionName = "handle-review-exited-work"
	createdWorkIS, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      createdItem.ID,
		SessionUUID: workSessionUUID,
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)
	workIS = createdWorkIS

	inst := newTestInstance(workSessionName)
	inst.UUID = workSessionUUID
	inst.gitManager.worktree = git.NewGitWorktreeFromStorage(
		"/tmp/fake-repo", "/tmp/fake-repo/../worktrees/"+workSessionName, workSessionName, "backlog/"+workSessionName, "abc123")
	require.NoError(t, storage.SaveInstances([]*Instance{inst}))

	reviewSessionUUID := uuid.New().String()
	createdReviewIS, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      createdItem.ID,
		SessionUUID: reviewSessionUUID,
		SessionRole: SessionRoleReview,
	})
	require.NoError(t, err)

	if verdict != nil {
		require.NoError(t, storage.SaveReviewVerdict(ctx, createdReviewIS.ID, *verdict))
	}

	reviewIS = ItemSessionSummary{
		ID:            createdReviewIS.ID,
		BacklogItemID: createdItem.ID,
		SessionUUID:   reviewSessionUUID,
		Role:          SessionRoleReview,
	}
	return createdItem, reviewIS, workSessionName, workIS
}

// TestHandleReviewSessionExited_Pass_EndedWorkSession_InvokesPushAndCreatePR
// verifies the pushAndCreatePR backstop when no OneShotShipRunner is wired
// (the shape of every constructor except production's SetOneShotShipRunner
// call): when the work session that earned the PASS verdict has already
// exited (EndedAt set — it crashed, was killed, or hit a turn cap before it
// could poll for the verdict and ship the PR itself via /backlog/ship),
// handleReviewSessionExited -> shipViaAgentOrFallback falls back to the
// mechanical push path directly, using the correct (most recent) work
// ItemSessionSummary — proven here by asserting the PR-creator factory is
// invoked with that work session's sessionName, and that the item ends up in
// pr_pending. See TestShipViaAgentOrFallback_EndedWorkSession_AttemptsOneShotShip_SkipsMechanicalPush
// below for the now-primary case (an OneShotShipRunner wired and succeeding),
// and the sibling test after this one for a still-live work session, where
// this backstop must NOT fire at all.
func TestHandleReviewSessionExited_Pass_EndedWorkSession_InvokesPushAndCreatePR(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item, reviewIS, workSessionName, workIS := newHandleReviewSessionExitedFixture(t, storage, &ReviewVerdictData{
		OverallOutcome: ReviewVerdictPass,
		PerCriterion:   `[]`,
		Summary:        "all good",
	})
	require.NoError(t, storage.UpdateItemSessionEnded(ctx, workIS.ID, time.Now()))

	listener := NewBacklogLifecycleListener(storage)
	var capturedSessionName string
	fakeCreator := &fakePRCreator{createURL: "https://github.com/tstapler/stapler-squad/pull/9", createNumber: 9}
	listener.SetPRCreatorFactory(func(repoPath, worktreePath, sessionName, branchName, baseCommitSHA string) prCreator {
		capturedSessionName = sessionName
		return fakeCreator
	})

	listener.handleReviewSessionExited(ctx, reviewIS, false)

	assert.True(t, fakeCreator.pushCalled, "PASS verdict with no live work session must fall back to pushAndCreatePR")
	assert.Equal(t, workSessionName, capturedSessionName, "pushAndCreatePR must be invoked with the work session's own worktree, not some other session's")

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusPRPending), fetched.Status)
}

// TestShipViaAgentOrFallback_EndedWorkSession_AttemptsOneShotShip_SkipsMechanicalPush
// verifies the primary new path this fix adds: when an OneShotShipRunner is
// wired and it successfully produces a PR URL, handleReviewSessionExited's
// PASS branch (ended work session) ships via the agent-driven one-shot
// /backlog/ship prompt instead of the mechanical pushAndCreatePR path, and
// the item transitions to pr_pending with the PR fields recorded.
func TestShipViaAgentOrFallback_EndedWorkSession_AttemptsOneShotShip_SkipsMechanicalPush(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item, reviewIS, _, workIS := newHandleReviewSessionExitedFixture(t, storage, &ReviewVerdictData{
		OverallOutcome: ReviewVerdictPass,
		PerCriterion:   `[]`,
		Summary:        "all good",
	})
	require.NoError(t, storage.UpdateItemSessionEnded(ctx, workIS.ID, time.Now()))

	listener := NewBacklogLifecycleListener(storage)
	fakeCreator := &fakePRCreator{createURL: "https://github.com/tstapler/stapler-squad/pull/1", createNumber: 1}
	listener.SetPRCreatorFactory(func(repoPath, worktreePath, sessionName, branchName, baseCommitSHA string) prCreator {
		return fakeCreator
	})
	runner := &fakeOneShotShipRunner{prURL: "https://github.com/tstapler/stapler-squad/pull/42"}
	listener.SetOneShotShipRunner(runner)

	listener.handleReviewSessionExited(ctx, reviewIS, false)

	require.Len(t, runner.calls, 1, "one-shot ship must be attempted exactly once")
	assert.Equal(t, workIS.SessionUUID, runner.calls[0].SessionID, "one-shot ship must run against the work session's own worktree")
	assert.Equal(t, agentShipPrompt, runner.calls[0].Prompt)
	assert.False(t, fakeCreator.pushCalled, "successful one-shot ship must skip the mechanical push path entirely")
	assert.False(t, fakeCreator.createCalled)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusPRPending), fetched.Status)
	assert.Equal(t, "https://github.com/tstapler/stapler-squad/pull/42", fetched.PrURL)
	assert.Equal(t, 42, fetched.PrNumber)
}

// TestShipViaAgentOrFallback_OneShotShipErrors_FallsBackToMechanicalPush
// verifies the documented "try the agent first, fall back to the mechanical
// path if that fails outright" policy: when the wired OneShotShipRunner
// returns an error (e.g. the session's Instance is no longer tracked live),
// shipViaAgentOrFallback still reaches pushAndCreatePR so the PR gets created
// one way or another rather than leaving the item stranded.
func TestShipViaAgentOrFallback_OneShotShipErrors_FallsBackToMechanicalPush(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item, reviewIS, _, workIS := newHandleReviewSessionExitedFixture(t, storage, &ReviewVerdictData{
		OverallOutcome: ReviewVerdictPass,
		PerCriterion:   `[]`,
		Summary:        "all good",
	})
	require.NoError(t, storage.UpdateItemSessionEnded(ctx, workIS.ID, time.Now()))

	listener := NewBacklogLifecycleListener(storage)
	fakeCreator := &fakePRCreator{createURL: "https://github.com/tstapler/stapler-squad/pull/9", createNumber: 9}
	listener.SetPRCreatorFactory(func(repoPath, worktreePath, sessionName, branchName, baseCommitSHA string) prCreator {
		return fakeCreator
	})
	runner := &fakeOneShotShipRunner{err: errors.New("session not found: instance no longer tracked live")}
	listener.SetOneShotShipRunner(runner)

	listener.handleReviewSessionExited(ctx, reviewIS, false)

	require.Len(t, runner.calls, 1, "one-shot ship must still be attempted before falling back")
	assert.True(t, fakeCreator.pushCalled, "a failed one-shot attempt must fall back to the mechanical push path")
	assert.True(t, fakeCreator.createCalled)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusPRPending), fetched.Status, "the mechanical fallback must still successfully ship the PR")
}

// TestShipViaAgentOrFallback_OneShotShipReturnsNoURL_FallsBackToMechanicalPush
// covers the case where RunOneShotForSession returns (nil error, empty URL) —
// the one-shot call ran but the agent's output never included a parseable PR
// link (see extractPRURL). This must be treated the same as an outright error,
// not silently accepted as "nothing to do".
func TestShipViaAgentOrFallback_OneShotShipReturnsNoURL_FallsBackToMechanicalPush(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	_, reviewIS, _, workIS := newHandleReviewSessionExitedFixture(t, storage, &ReviewVerdictData{
		OverallOutcome: ReviewVerdictPass,
		PerCriterion:   `[]`,
		Summary:        "all good",
	})
	require.NoError(t, storage.UpdateItemSessionEnded(ctx, workIS.ID, time.Now()))

	listener := NewBacklogLifecycleListener(storage)
	fakeCreator := &fakePRCreator{createURL: "https://github.com/tstapler/stapler-squad/pull/9", createNumber: 9}
	listener.SetPRCreatorFactory(func(repoPath, worktreePath, sessionName, branchName, baseCommitSHA string) prCreator {
		return fakeCreator
	})
	runner := &fakeOneShotShipRunner{prURL: ""}
	listener.SetOneShotShipRunner(runner)

	listener.handleReviewSessionExited(ctx, reviewIS, false)

	assert.True(t, fakeCreator.pushCalled, "an empty PR URL from a successful one-shot call must still fall back to the mechanical path")
}

// TestShipViaAgentOrFallback_NoWorktreeRecorded_StillFallsBackToDone verifies
// this fix does not regress pushAndCreatePR's pre-existing fallbackToDone
// behavior: when there is no worktree recorded for the work session at all
// (not even a storage row — the true "genuinely nothing to ship" case),
// shipViaAgentOrFallback's one-shot attempt fails fast and falls through to
// pushAndCreatePR, which still reaches the exact same done transition it
// always has. See TestReconcileUnprocessedReviewVerdicts_should_applyPassVerdict_When_ReviewSessionDiedButWorkSessionStillAlive
// for the forcePush crash-recovery variant of this same scenario.
func TestShipViaAgentOrFallback_NoWorktreeRecorded_StillFallsBackToDone(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "No worktree recorded test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusReview),
	})
	require.NoError(t, err)

	workSessionUUID := uuid.New().String()
	workIS, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: workSessionUUID,
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)
	require.NoError(t, storage.UpdateItemSessionEnded(ctx, workIS.ID, time.Now()))
	// Deliberately no storage.SaveInstances call — no Instance/worktree exists
	// for this session at all, matching newStuckReviewTestItem's fixture shape.

	reviewSessionUUID := uuid.New().String()
	reviewISData, err := storage.CreateItemSessionWithVerdict(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: reviewSessionUUID,
		SessionRole: SessionRoleReview,
	}, ReviewVerdictData{
		OverallOutcome: ReviewVerdictPass,
		PerCriterion:   `[]`,
		Summary:        "all good",
	})
	require.NoError(t, err)
	reviewIS := ItemSessionSummary{
		ID:            reviewISData.ID,
		BacklogItemID: item.ID,
		SessionUUID:   reviewSessionUUID,
		Role:          SessionRoleReview,
	}

	listener := NewBacklogLifecycleListener(storage)
	runner := &fakeOneShotShipRunner{err: errors.New("session not found: no worktree")}
	listener.SetOneShotShipRunner(runner)

	listener.handleReviewSessionExited(ctx, reviewIS, false)

	require.Len(t, runner.calls, 1)
	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusDone), fetched.Status, "genuinely nothing to ship must still resolve via pushAndCreatePR's pre-existing fallbackToDone, unchanged by this fix")
}

// TestShipViaAgentOrFallback_WorktreeGoneOnDisk_NotifiesOperator_DoesNotSilentlyDrop
// covers the "true last-resort" case this fix must not regress into a silent
// drop: a worktree row still exists in storage, but the agent-driven one-shot
// attempt fails (e.g. the session's Instance is no longer live) AND the
// mechanical fallback also fails (e.g. the worktree directory itself was
// cleaned up from disk — simulated here via a PushBranch error, the same
// failure shape pushAndCreatePR's own push-failure tests use). The PASS
// verdict must not be silently discarded: the item stays in review (never
// done) and an operator notification fires via the existing
// StuckReasonPushFailed / stayInReviewAndNotify machinery.
func TestShipViaAgentOrFallback_WorktreeGoneOnDisk_NotifiesOperator_DoesNotSilentlyDrop(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item, reviewIS, _, workIS := newHandleReviewSessionExitedFixture(t, storage, &ReviewVerdictData{
		OverallOutcome: ReviewVerdictPass,
		PerCriterion:   `[]`,
		Summary:        "all good",
	})
	require.NoError(t, storage.UpdateItemSessionEnded(ctx, workIS.ID, time.Now()))

	listener := NewBacklogLifecycleListener(storage)
	fakeCreator := &fakePRCreator{pushErr: errors.New("fatal: '/worktrees/handle-review-exited-work' does not exist")}
	listener.SetPRCreatorFactory(func(repoPath, worktreePath, sessionName, branchName, baseCommitSHA string) prCreator {
		return fakeCreator
	})
	runner := &fakeOneShotShipRunner{err: errors.New("session not found: instance no longer tracked live")}
	listener.SetOneShotShipRunner(runner)
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	listener.handleReviewSessionExited(ctx, reviewIS, false)

	require.Len(t, runner.calls, 1, "the agent-driven path must be attempted first")
	assert.True(t, fakeCreator.pushCalled, "the mechanical backstop must be attempted after the one-shot attempt fails")
	assert.False(t, fakeCreator.createCalled, "CreatePR must not be attempted after a push failure")

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusReview), fetched.Status, "PASS verdict must not be silently dropped — item stays in review, not done, when nothing could actually be shipped")
	assert.Contains(t, notifier.titles(), "PR creation failed", "operator must be notified rather than the verdict silently vanishing")
}

// TestHandleReviewSessionExited_Pass_LiveWorkSession_DoesNotInvokePushAndCreatePR
// verifies the new primary path: when the work session that earned the PASS
// verdict is still alive (EndedAt nil — it stays running and polls
// get_backlog_item/backlog status per taskProtocolBlock rules 8-9), the
// mechanical pushAndCreatePR path must NOT fire. The live agent is expected to
// discover the PASS verdict on its next poll and run /backlog/ship itself,
// which drives /github:pr-ship — see session/backlog_context.go and
// server/mcp/tools_backlog.go for the instruction text a live session reads.
// The item must stay in "review" (not pr_pending) so the agent-driven path has
// something to act on.
func TestHandleReviewSessionExited_Pass_LiveWorkSession_DoesNotInvokePushAndCreatePR(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item, reviewIS, _, _ := newHandleReviewSessionExitedFixture(t, storage, &ReviewVerdictData{
		OverallOutcome: ReviewVerdictPass,
		PerCriterion:   `[]`,
		Summary:        "all good",
	})

	listener := NewBacklogLifecycleListener(storage)
	fakeCreator := &fakePRCreator{createURL: "https://github.com/tstapler/stapler-squad/pull/9", createNumber: 9}
	listener.SetPRCreatorFactory(func(repoPath, worktreePath, sessionName, branchName, baseCommitSHA string) prCreator {
		return fakeCreator
	})

	listener.handleReviewSessionExited(ctx, reviewIS, false)

	assert.False(t, fakeCreator.pushCalled, "PASS verdict with a live work session must leave PR creation to the agent, not push mechanically")

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusReview), fetched.Status, "item must stay in review so the live agent's /backlog/ship has something to act on")
}

// TestHandleReviewSessionExited_Fail_InvokesAutoReopener verifies that a FAIL
// verdict (and, identically, PARTIAL/UNVERIFIABLE) triggers the auto-reopener
// instead of pushAndCreatePR.
func TestHandleReviewSessionExited_Fail_InvokesAutoReopener(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item, reviewIS, _, _ := newHandleReviewSessionExitedFixture(t, storage, &ReviewVerdictData{
		OverallOutcome: ReviewVerdictFail,
		PerCriterion:   `[]`,
		Summary:        "criteria not met",
	})

	listener := NewBacklogLifecycleListener(storage)
	reopener := newFakeAutoReopenSpawner()
	listener.SetAutoReopener(reopener)
	fakeCreator := &fakePRCreator{}
	listener.SetPRCreatorFactory(func(repoPath, worktreePath, sessionName, branchName, baseCommitSHA string) prCreator {
		return fakeCreator
	})

	listener.handleReviewSessionExited(ctx, reviewIS, false)

	select {
	case gotItemID := <-reopener.called:
		assert.Equal(t, item.ID, gotItemID)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for AutoReopenAfterFailedReview to be called")
	}
	assert.False(t, fakeCreator.pushCalled, "a FAIL verdict must never push/create a PR")
}

// TestHandleReviewSessionExited_NoVerdict_NotifiesAndInvokesAutoReopener verifies
// that a review session which exited without ever calling submit_review_verdict
// (crash, kill, ran out of turns) is treated like a failed review: the operator
// is notified and the auto-reopener is invoked.
func TestHandleReviewSessionExited_NoVerdict_NotifiesAndInvokesAutoReopener(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	_, reviewIS, _, _ := newHandleReviewSessionExitedFixture(t, storage, nil)

	listener := NewBacklogLifecycleListener(storage)
	reopener := newFakeAutoReopenSpawner()
	listener.SetAutoReopener(reopener)
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	listener.handleReviewSessionExited(ctx, reviewIS, false)

	select {
	case <-reopener.called:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for AutoReopenAfterFailedReview to be called")
	}
	assert.Contains(t, notifier.titles(), "Review session ended without a verdict")
}

// TestHandleReviewSessionExited_NoVerdict_NotifiesOnlyOnce_AcrossRepeatedSweepTicks
// is a BUG-046 regression test. Live evidence: item 12981e9d's "Review session
// ended without a verdict" notification reached occurrence_count 95 over ~94
// minutes (one per ~60s sweep tick). Root cause: reconcileUnprocessedReviewVerdicts
// re-detects the same dead review session on every tick because
// handleReviewSessionExited's no-verdict branch never transitions the item out
// of "review" when autoReopenWithBackoffGate's downstream "bouncing" gate is
// mid-backoff — so the sweep's own "already consumed" guard (which only
// catches "item left review and came back") never trips, and the identical
// dead SessionUUID gets reprocessed — same WARNING log, same notify() call —
// forever, until the gate finally opens.
//
// This reproduces the realistic timeline: tick 1 fires before any "bouncing"
// row exists (the first-ever detection of this failure shape, same as before
// this fix — RemediationDue's own ungated-until-first-detected default means
// the reopen genuinely gets attempted here too), then a "bouncing" row opens
// mid-backoff in between ticks (the shape reconcileBouncingItems produces once
// its own bounceThreshold trips, mirroring TestMarkAbandonedReview_SkipsRespawn_WhenBouncingGateNotDue's
// identical seed for the live DB state BUG-043 found on this same item), then
// tick 2 reprocesses the SAME dead SessionUUID with the gate now blocked.
// Pre-fix, tick 2 notifies again regardless of the gate; post-fix it must not.
func TestHandleReviewSessionExited_NoVerdict_NotifiesOnlyOnce_AcrossRepeatedSweepTicks(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	_, reviewIS, _, _ := newHandleReviewSessionExitedFixture(t, storage, nil)

	listener := NewBacklogLifecycleListener(storage)
	reopener := newFakeAutoReopenSpawner()
	listener.SetAutoReopener(reopener)
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	er := storage.repo.(*EntRepository)

	// Sweep tick 1 (forcePush=true, matching reconcileUnprocessedReviewVerdicts):
	// no "bouncing" row exists yet, so RemediationBlocked reports false (ungated
	// default) — this is a genuinely fresh detection and must notify. The reopen
	// is ungated too for the same reason, so AutoReopenAfterFailedReview fires.
	listener.handleReviewSessionExited(ctx, reviewIS, true)

	select {
	case <-reopener.called:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for AutoReopenAfterFailedReview on tick 1 (ungated — no bouncing row yet)")
	}
	assert.Equal(t, []string{"Review session ended without a verdict"}, notifier.titles(),
		"tick 1 is a fresh detection and must notify")

	// Between ticks: a "bouncing" stuck row opens mid-backoff — mirrors
	// reconcileBouncingItems tripping its own bounceThreshold independently,
	// the live DB state BUG-043 found on this exact item.
	_, err := er.MarkStuck(ctx, reviewIS.BacklogItemID, domain.StuckReasonBouncing, BacklogStatusReview, "bounced previously")
	require.NoError(t, err)
	future := time.Now().Add(2 * time.Hour)
	_, err = er.RecordRemediationAttempt(ctx, reviewIS.BacklogItemID, domain.StuckReasonBouncing, 1, &future)
	require.NoError(t, err)

	// Sweep tick 2: the item never left "review" (nothing transitioned it), so
	// reconcileUnprocessedReviewVerdicts' own guard doesn't skip it — the SAME
	// dead SessionUUID is reprocessed. The bouncing gate is now mid-backoff, so
	// the reopen correctly no-ops (autoReopenWithBackoffGate's own gating is
	// unchanged) — and, with this fix, must NOT notify a second time either.
	listener.handleReviewSessionExited(ctx, reviewIS, true)

	select {
	case id := <-reopener.called:
		t.Fatalf("bouncing gate is mid-backoff on tick 2 — AutoReopenAfterFailedReview must not be invoked, got call for item=%s", id)
	case <-time.After(300 * time.Millisecond):
	}

	assert.Equal(t, []string{"Review session ended without a verdict"}, notifier.titles(),
		"must not notify a second time for the same dead session once the bouncing gate is blocking (BUG-046)")
}

// TestBacklogLifecycleListener_OnSessionExited_ReviewSession_RoutesToHandleReviewSessionExited
// verifies that onSessionExited dispatches Role==SessionRoleReview to
// handleReviewSessionExited (proven by observing its FAIL-verdict side effect —
// the auto-reopener firing) rather than falling through to the work-session
// in_progress→review/done transition logic.
func TestBacklogLifecycleListener_OnSessionExited_ReviewSession_RoutesToHandleReviewSessionExited(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	_, reviewIS, _, _ := newHandleReviewSessionExitedFixture(t, storage, &ReviewVerdictData{
		OverallOutcome: ReviewVerdictFail,
		PerCriterion:   `[]`,
		Summary:        "criteria not met",
	})

	listener := NewBacklogLifecycleListener(storage)
	reopener := newFakeAutoReopenSpawner()
	listener.SetAutoReopener(reopener)

	done := make(chan struct{})
	go func() {
		defer close(done)
		listener.onSessionExited(reviewIS.SessionUUID)
	}()
	waitWithTimeout(t, done)

	select {
	case gotItemID := <-reopener.called:
		assert.Equal(t, reviewIS.BacklogItemID, gotItemID)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for AutoReopenAfterFailedReview to be called via onSessionExited routing")
	}
}

// ─── SetSessionCreator / getSessionCreator wiring ─────────────────────────────

// TestBacklogLifecycleListener_SetSessionCreator_WiresPostConstruction verifies
// that SetSessionCreator can wire (or rewire) the review-gate spawner after
// construction, and that getSessionCreator observes the latest value — needed
// because production wiring (server/dependencies.go) constructs this listener
// before SessionService exists.
func TestBacklogLifecycleListener_SetSessionCreator_WiresPostConstruction(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	listener := NewBacklogLifecycleListener(storage)
	require.Nil(t, listener.getSessionCreator())

	spawner := &mockReviewGateSpawner{}
	listener.SetSessionCreator(spawner)
	assert.Equal(t, ReviewGateSpawner(spawner), listener.getSessionCreator())
}

// TestBacklogLifecycleListener_HeadlessPoolAlone_NoLongerTriggersReviewGateSpawn
// verifies that a headless pool configured with no session creator does NOT
// cause onSessionExited to spawn a review gate anymore — review-gate spawning
// now keys exclusively off getSessionCreator(), since the headless in-process
// review path has been removed in favor of always spawning a real session.
func TestBacklogLifecycleListener_HeadlessPoolAlone_NoLongerTriggersReviewGateSpawn(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	createdItem, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "Headless pool alone test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
		RepoPath:           "/tmp/fake-repo",
	})
	require.NoError(t, err)

	sessionUUID := uuid.New().String()
	_, err = storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      createdItem.ID,
		SessionUUID: sessionUUID,
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)

	// NewBacklogLifecycleListenerWithPool wires a headless pool but no session
	// creator — the pool is still used elsewhere (PR description drafting), but
	// must no longer be treated as "a review mechanism is configured".
	listener := NewBacklogLifecycleListenerWithPool(storage, nil, nil)

	done := make(chan struct{})
	go func() {
		defer close(done)
		listener.onSessionExited(sessionUUID)
	}()
	waitWithTimeout(t, done)

	// The item still transitions to review (that part of onSessionExited is
	// unconditional), but no review ItemSession should ever be created since the
	// gate never spawns.
	require.Eventually(t, func() bool {
		fetched, ferr := storage.GetBacklogItem(ctx, createdItem.ID)
		return ferr == nil && fetched.Status == string(BacklogStatusReview)
	}, 2*time.Second, 20*time.Millisecond)

	sessions, err := storage.ListItemSessions(ctx, createdItem.ID)
	require.NoError(t, err)
	for _, s := range sessions {
		assert.NotEqual(t, SessionRoleReview, s.Role, "no review ItemSession should be created when only a headless pool (no session creator) is configured")
	}
}

// --- Story 3.3.1: CaptureShipSnapshot ---

// runGitTestCmd runs `git <args...>` in dir, failing the test on error. Test-only
// helper — CaptureShipSnapshot itself never shells out (it calls
// git.FileStatsBetween, which is go-git-based per
// .claude/rules/prefer-go-git-over-subshells.md); this just builds fixture repo
// data for FileStatsBetween to read.
func runGitTestCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	fullArgs := append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", fullArgs...) //nolint:norawexec // test helper, blocking CombinedOutput, no zombie risk
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, out)
	return string(out)
}

// setupShipSnapshotTestRepo creates a minimal two-commit git repository on disk
// and returns its path plus the base and head commit SHAs, so
// git.FileStatsBetween(repoPath, baseSHA, headSHA) has real diff data to compute
// stats from. The second commit adds one new file (feature.txt), giving
// FileStatsBetween exactly one FileStat entry to report.
func setupShipSnapshotTestRepo(t *testing.T) (repoPath, baseSHA, headSHA string) {
	t.Helper()
	dir := t.TempDir()
	runGitTestCmd(t, dir, "init")
	runGitTestCmd(t, dir, "config", "user.email", "test@example.com")
	runGitTestCmd(t, dir, "config", "user.name", "Test")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "base.txt"), []byte("base\n"), 0o644))
	runGitTestCmd(t, dir, "add", "base.txt")
	runGitTestCmd(t, dir, "commit", "-m", "base commit")
	baseSHA = strings.TrimSpace(runGitTestCmd(t, dir, "rev-parse", "HEAD"))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.txt"), []byte("line1\nline2\nline3\n"), 0o644))
	runGitTestCmd(t, dir, "add", "feature.txt")
	runGitTestCmd(t, dir, "commit", "-m", "feature commit")
	headSHA = strings.TrimSpace(runGitTestCmd(t, dir, "rev-parse", "HEAD"))

	return dir, baseSHA, headSHA
}

// newShipSnapshotTestItem creates a pr_pending BacklogItem with repoPath and a
// PR number set, mirroring newPRPendingTestItem but parameterized on repoPath
// so CaptureShipSnapshot tests can point it at a real fixture git repo (unlike
// newPRPendingTestItem's placeholder "/tmp/fake-repo", which is fine for tests
// that never reach FileStatsBetween but not for these).
func newShipSnapshotTestItem(t *testing.T, storage *Storage, repoPath string, prNumber int) *BacklogItemData {
	t.Helper()
	ctx := context.Background()
	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "Ship snapshot test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusPRPending),
		RepoPath:           repoPath,
	})
	require.NoError(t, err)

	prURL := "https://github.com/TylerStaplerAtFanatics/stapler-squad/pull/9001"
	updated, err := storage.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{
		PrURL:    &prURL,
		PrNumber: &prNumber,
	}, nil)
	require.NoError(t, err)
	return updated
}

// TestCaptureShipSnapshot_ShouldWriteAllSixFieldsBeforeDoneTransition_WhenBothGroupsSucceed
// verifies the happy path (Task 3.3.1a/b/c): when both the GitHub-data group
// (from prStatus) and the file-stats group (from FileStatsBetween) succeed,
// CaptureShipSnapshot writes all 6 durable snapshot fields via a single
// UpdateBacklogItem call, with ShippedSnapshotCaptureFailed left false.
// "BeforeDoneTransition" ordering itself is covered directly by
// TestReconcilePRPending_ShouldCallCaptureShipSnapshotBeforeTransitionToDone_WhenPRMerged
// below; this test locks in CaptureShipSnapshot's own field-writing contract.
func TestCaptureShipSnapshot_ShouldWriteAllSixFieldsBeforeDoneTransition_WhenBothGroupsSucceed(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	repoPath, baseSHA, headSHA := setupShipSnapshotTestRepo(t)
	item := newShipSnapshotTestItem(t, storage, repoPath, 9001)

	prStatus := &git.PRStatus{ApprovedCount: 2, ChangesRequestedCount: 0, CIFailing: false}
	lastWork := &ItemSessionSummary{SessionUUID: uuid.New().String(), LastCommitSha: headSHA}
	wt := &GitWorktreeData{RepoPath: repoPath, BaseCommitSHA: baseSHA}

	err := CaptureShipSnapshot(ctx, storage, item, prStatus, lastWork, wt)
	require.NoError(t, err, "CaptureShipSnapshot must never return an error")

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)

	assert.Equal(t, "success", fetched.ShippedCheckConclusion)
	assert.Equal(t, 2, fetched.ShippedApprovedCount)
	assert.Equal(t, 0, fetched.ShippedChangesReqCount)
	require.NotNil(t, fetched.ShippedSnapshotAt, "ShippedSnapshotAt must be set when at least one group succeeds")
	assert.False(t, fetched.ShippedSnapshotCaptureFailed, "both groups succeeded — capture-failed must be false")
	require.NotEmpty(t, fetched.ShippedFileStats)

	var stats []git.FileStat
	require.NoError(t, json.Unmarshal([]byte(fetched.ShippedFileStats), &stats))
	require.Len(t, stats, 1, "only feature.txt changed between base and head")
	assert.Equal(t, "feature.txt", stats[0].Path)
}

// TestCaptureShipSnapshot_ShouldPreserveSuccessfulGithubGroup_WhenFileStatsBetweenFailsIndependently
// is a table test covering Task 3.3.1e's three independent-failure rows: (1)
// FileStatsBetween fails but prStatus maps successfully — the GitHub fields must
// survive; (2) the mirror case, prStatus == nil but FileStatsBetween succeeds —
// the file-stats field must survive; (3) both groups fail — the done transition
// (exercised separately by the integration test) is still never blocked, and
// ShippedSnapshotCaptureFailed is set with no field ever holding the string
// "failed".
func TestCaptureShipSnapshot_ShouldPreserveSuccessfulGithubGroup_WhenFileStatsBetweenFailsIndependently(t *testing.T) {
	repoPath, baseSHA, headSHA := setupShipSnapshotTestRepo(t)
	const badSHA = "0000000000000000000000000000000000000000"

	tests := []struct {
		name                 string
		prStatus             *git.PRStatus
		baseSHA              string
		wantGithubWritten    bool
		wantFileStatsWritten bool
	}{
		{
			name:                 "FileStatsBetween fails, github group succeeds",
			prStatus:             &git.PRStatus{ApprovedCount: 3, ChangesRequestedCount: 1, CIFailing: true},
			baseSHA:              badSHA,
			wantGithubWritten:    true,
			wantFileStatsWritten: false,
		},
		{
			name:                 "prStatus nil, file-stats group succeeds",
			prStatus:             nil,
			baseSHA:              baseSHA,
			wantGithubWritten:    false,
			wantFileStatsWritten: true,
		},
		{
			name:                 "both groups fail",
			prStatus:             nil,
			baseSHA:              badSHA,
			wantGithubWritten:    false,
			wantFileStatsWritten: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage, cleanup := createTestStorage(t)
			defer cleanup()
			ctx := context.Background()

			item := newShipSnapshotTestItem(t, storage, repoPath, 9002)
			lastWork := &ItemSessionSummary{SessionUUID: uuid.New().String(), LastCommitSha: headSHA}
			wt := &GitWorktreeData{RepoPath: repoPath, BaseCommitSHA: tt.baseSHA}

			err := CaptureShipSnapshot(ctx, storage, item, tt.prStatus, lastWork, wt)
			require.NoError(t, err, "CaptureShipSnapshot must never return an error, even when both groups fail")

			fetched, ferr := storage.GetBacklogItem(ctx, item.ID)
			require.NoError(t, ferr)

			if tt.wantGithubWritten {
				assert.Equal(t, "failure", fetched.ShippedCheckConclusion)
				assert.Equal(t, tt.prStatus.ApprovedCount, fetched.ShippedApprovedCount)
				assert.Equal(t, tt.prStatus.ChangesRequestedCount, fetched.ShippedChangesReqCount)
			} else {
				assert.Empty(t, fetched.ShippedCheckConclusion, "github group failed — ShippedCheckConclusion must stay unset")
				assert.Zero(t, fetched.ShippedApprovedCount)
				assert.Zero(t, fetched.ShippedChangesReqCount)
			}
			// Never a sentinel string — reserved for genuine CI-conclusion values.
			assert.NotEqual(t, "failed", fetched.ShippedCheckConclusion)

			if tt.wantFileStatsWritten {
				require.NotEmpty(t, fetched.ShippedFileStats)
				var stats []git.FileStat
				require.NoError(t, json.Unmarshal([]byte(fetched.ShippedFileStats), &stats))
				require.Len(t, stats, 1)
			} else {
				assert.Empty(t, fetched.ShippedFileStats, "file-stats group failed — ShippedFileStats must stay unset")
			}

			// This test's every row has at least one group fail, so
			// ShippedSnapshotCaptureFailed must always be true here.
			assert.True(t, fetched.ShippedSnapshotCaptureFailed)

			// ShippedSnapshotAt is set whenever at least one group succeeded
			// (Task 3.3.1c); only the both-fail row has neither succeed.
			if tt.wantGithubWritten || tt.wantFileStatsWritten {
				assert.NotNil(t, fetched.ShippedSnapshotAt, "at least one group succeeded — ShippedSnapshotAt must be set")
			}
		})
	}
}

// TestReconcilePRPending_ShouldCallCaptureShipSnapshotBeforeTransitionToDone_WhenPRMerged
// is the integration test: a real ent-backed Storage, a seeded pr_pending item,
// and a merged PR faked at the existing prPendingChecker seam
// (SetPRPendingCheckerFactory). After ReconcilePRPending returns,
// BacklogItem.ShippedSnapshotAt must be non-nil and Status must be
// BacklogStatusDone — asserted structurally (no time.Sleep/polling), since
// CaptureShipSnapshot runs synchronously on the same goroutine, strictly before
// the TransitionBacklogItemStatus call in the same `if merged` block.
func TestReconcilePRPending_ShouldCallCaptureShipSnapshotBeforeTransitionToDone_WhenPRMerged(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item := newPRPendingTestItem(t, storage, 9003)

	listener := NewBacklogLifecycleListener(storage)
	overridePRPendingChecker(t, listener, &fakePRPendingChecker{
		merged: true,
		status: &git.PRStatus{ApprovedCount: 1, ChangesRequestedCount: 0, CIFailing: false},
	})

	er := storage.repo.(*EntRepository)
	listener.ReconcilePRPending(ctx, er)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusDone), fetched.Status, "PR merged — item must reach done")
	require.NotNil(t, fetched.ShippedSnapshotAt, "CaptureShipSnapshot must have run and captured at least the GitHub group before the done transition")
	assert.Equal(t, "success", fetched.ShippedCheckConclusion)
	assert.Equal(t, 1, fetched.ShippedApprovedCount)
}

// TestReconcilePRPending_CleansUpBacklogScaffolding_WhenPRMerged is the call-site
// regression test for wiring CleanupBacklogContextFile/CleanupSlashCommands into
// production: previously these functions had zero call sites. Once an item's
// last work session's worktree is known and its PR merges, ReconcilePRPending
// must remove the leftover .backlog-context.md and .claude/commands/backlog/
// scaffolding from that worktree — ship.md's "must still exist" constraint
// (see CleanupSlashCommands' doc comment) no longer applies once the PR is
// merged and the item has reached done.
func TestReconcilePRPending_CleansUpBacklogScaffolding_WhenPRMerged(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item := newPRPendingTestItem(t, storage, 9010)

	worktreePath := t.TempDir()
	contextPath := filepath.Join(worktreePath, ".backlog-context.md")
	require.NoError(t, os.WriteFile(contextPath, []byte("stale context"), 0o644))
	cmdDir := filepath.Join(worktreePath, backlogCommandsDir)
	require.NoError(t, os.MkdirAll(cmdDir, 0o755))
	statusPath := filepath.Join(cmdDir, "status.md")
	require.NoError(t, os.WriteFile(statusPath, []byte("stale status"), 0o644))

	inst := newTestInstance("pr-pending-worktree")
	inst.UUID = uuid.New().String()
	inst.gitManager.worktree = git.NewGitWorktreeFromStorage("/repo", worktreePath, "pr-pending-worktree", "backlog/some-item", "abc123")
	require.NoError(t, storage.SaveInstances([]*Instance{inst}))

	_, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: inst.UUID,
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	overridePRPendingChecker(t, listener, &fakePRPendingChecker{
		merged: true,
		status: &git.PRStatus{ApprovedCount: 1},
	})

	er := storage.repo.(*EntRepository)
	listener.ReconcilePRPending(ctx, er)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	require.Equal(t, string(BacklogStatusDone), fetched.Status, "PR merged — item must reach done")

	_, statErr := os.Stat(contextPath)
	assert.True(t, os.IsNotExist(statErr), ".backlog-context.md must be cleaned up once the item reaches done")
	_, statErr = os.Stat(statusPath)
	assert.True(t, os.IsNotExist(statErr), "slash command files must be cleaned up once the item reaches done")
}

// newOrphanedAgentPRTestItem creates a review-status item with pr_number=0
// and an ENDED work session whose worktree branch is branchName — the exact
// shape reconcileOrphanedAgentPRs (Epic 3.2's reconciliation backstop)
// targets: an agent shipped a PR out-of-band and then exited/crashed before
// ever calling report_pr_created (Epic 3.1) to report it back.
func newOrphanedAgentPRTestItem(t *testing.T, storage *Storage, branchName string) *BacklogItemData {
	t.Helper()
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "Orphaned agent PR test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusReview),
		RepoPath:           "/tmp/orphaned-agent-pr-test-repo",
	})
	require.NoError(t, err)

	inst := newTestInstance("orphaned-agent-pr-worktree")
	inst.UUID = uuid.New().String()
	inst.gitManager.worktree = git.NewGitWorktreeFromStorage("/tmp/orphaned-agent-pr-test-repo", "/tmp/orphaned-agent-pr-test-repo/../wt", "orphaned-agent-pr-worktree", branchName, "abc123")
	require.NoError(t, storage.SaveInstances([]*Instance{inst}))

	is, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: inst.UUID,
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)
	// No live session — the whole point of this backstop is that the agent
	// that shipped the PR has already exited/crashed.
	require.NoError(t, storage.UpdateItemSessionEnded(ctx, is.ID, time.Now()))

	return item
}

// TestReconcileOrphanedAgentPRs_should_LinkPR_When_ReviewStatusNoLiveSessionPRExists
// is the Epic 3.2 happy-path regression test: a review-status item with no
// live session and no PR reference recorded, but a real open GitHub PR for
// its branch, must be linked and transitioned to pr_pending — the exact
// backstop for an agent that shipped via /backlog:ship but crashed before
// calling report_pr_created.
func TestReconcileOrphanedAgentPRs_should_LinkPR_When_ReviewStatusNoLiveSessionPRExists(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item := newOrphanedAgentPRTestItem(t, storage, "backlog/orphan-item")

	listener := NewBacklogLifecycleListener(storage)
	listener.SetOrphanedPRFinder(func(_ context.Context, repoPath, branch string) (*github.PRInfo, error) {
		assert.Equal(t, "/tmp/orphaned-agent-pr-test-repo", repoPath)
		assert.Equal(t, "backlog/orphan-item", branch)
		return &github.PRInfo{
			Number:  77,
			HTMLURL: "https://github.com/tstapler/stapler-squad/pull/77",
			State:   "open",
			HeadRef: branch,
		}, nil
	})

	listener.reconcileOrphanedAgentPRs(ctx, er)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusPRPending), fetched.Status)
	assert.Equal(t, 77, fetched.PrNumber)
	assert.Equal(t, "https://github.com/tstapler/stapler-squad/pull/77", fetched.PrURL)
}

// TestReconcileOrphanedAgentPRs_should_NoOp_When_NoMatchingPR verifies the
// detector leaves the item untouched when GitHub genuinely has no PR for the
// item's branch yet — the common, expected case on every tick until the
// agent (or a human) actually ships.
func TestReconcileOrphanedAgentPRs_should_NoOp_When_NoMatchingPR(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item := newOrphanedAgentPRTestItem(t, storage, "backlog/no-pr-yet")

	listener := NewBacklogLifecycleListener(storage)
	listener.SetOrphanedPRFinder(func(context.Context, string, string) (*github.PRInfo, error) {
		return nil, github.ErrNoPR
	})

	listener.reconcileOrphanedAgentPRs(ctx, er)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusReview), fetched.Status, "no matching PR — item must stay in review")
	assert.Equal(t, 0, fetched.PrNumber)
}
