package session

import (
	"bytes"
	"context"
	"errors"
	stdlog "log"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
}

func (f *fakePRPendingChecker) IsPRMerged(prNumber int) (bool, error) {
	return f.merged, f.mergedErr
}

func (f *fakePRPendingChecker) GetPRStatus(prNumber int) (*git.PRStatus, error) {
	return f.status, f.statusErr
}

// fakePRFixSpawner is a test double implementing PRFixSpawner, recording
// whether/how AutoReopenForPRFix was called. Same shape as mockReviewGateSpawner.
type fakePRFixSpawner struct {
	spawnCalled    bool
	lastFixContext string
}

func (f *fakePRFixSpawner) AutoReopenForPRFix(ctx context.Context, itemID string, fixContext string) error {
	f.spawnCalled = true
	f.lastFixContext = fixContext
	return nil
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
// it must clear the cached PrNumber/PrURL (so the next pushAndCreatePR creates a
// fresh PR instead of reusing the closed one) and spawn a fix session.
func TestReconcilePRPending_ClosedWithoutMerge_ClearsPRFieldsAndReopens(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	item := newPRPendingTestItem(t, storage, 152)

	listener := NewBacklogLifecycleListener(storage)
	overridePRPendingChecker(t, listener, &fakePRPendingChecker{
		merged: false,
		status: &git.PRStatus{IsClosed: true},
	})
	fakeSpawner := &fakePRFixSpawner{}
	listener.SetPRFixSpawner(fakeSpawner)

	er := storage.repo.(*EntRepository)
	listener.ReconcilePRPending(context.Background(), er)

	assert.True(t, fakeSpawner.spawnCalled, "a closed-without-merge PR must trigger a fix-session spawn")

	fetched, err := storage.GetBacklogItem(context.Background(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, fetched.PrNumber, "PrNumber must be cleared so the next pushAndCreatePR creates a fresh PR")
	assert.Empty(t, fetched.PrURL, "PrURL must be cleared so the next pushAndCreatePR creates a fresh PR")
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
func newHandleReviewSessionExitedFixture(t *testing.T, storage *Storage, verdict *ReviewVerdictData) (item *BacklogItemData, reviewIS ItemSessionSummary, workSessionName string) {
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
	_, err = storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      createdItem.ID,
		SessionUUID: workSessionUUID,
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)

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
	return createdItem, reviewIS, workSessionName
}

// TestHandleReviewSessionExited_Pass_InvokesPushAndCreatePR verifies that a PASS
// verdict drives pushAndCreatePR using the correct (most recent) work
// ItemSessionSummary — proven here by asserting the PR-creator factory is
// invoked with that work session's sessionName, and that the item ends up in
// pr_pending.
func TestHandleReviewSessionExited_Pass_InvokesPushAndCreatePR(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item, reviewIS, workSessionName := newHandleReviewSessionExitedFixture(t, storage, &ReviewVerdictData{
		OverallOutcome: ReviewVerdictPass,
		PerCriterion:   `[]`,
		Summary:        "all good",
	})

	listener := NewBacklogLifecycleListener(storage)
	var capturedSessionName string
	fakeCreator := &fakePRCreator{createURL: "https://github.com/tstapler/stapler-squad/pull/9", createNumber: 9}
	listener.SetPRCreatorFactory(func(repoPath, worktreePath, sessionName, branchName, baseCommitSHA string) prCreator {
		capturedSessionName = sessionName
		return fakeCreator
	})

	listener.handleReviewSessionExited(ctx, reviewIS)

	assert.True(t, fakeCreator.pushCalled, "PASS verdict must drive a push via pushAndCreatePR")
	assert.Equal(t, workSessionName, capturedSessionName, "pushAndCreatePR must be invoked with the work session's own worktree, not some other session's")

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusPRPending), fetched.Status)
}

// TestHandleReviewSessionExited_Fail_InvokesAutoReopener verifies that a FAIL
// verdict (and, identically, PARTIAL/UNVERIFIABLE) triggers the auto-reopener
// instead of pushAndCreatePR.
func TestHandleReviewSessionExited_Fail_InvokesAutoReopener(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item, reviewIS, _ := newHandleReviewSessionExitedFixture(t, storage, &ReviewVerdictData{
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

	listener.handleReviewSessionExited(ctx, reviewIS)

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

	_, reviewIS, _ := newHandleReviewSessionExitedFixture(t, storage, nil)

	listener := NewBacklogLifecycleListener(storage)
	reopener := newFakeAutoReopenSpawner()
	listener.SetAutoReopener(reopener)
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	listener.handleReviewSessionExited(ctx, reviewIS)

	select {
	case <-reopener.called:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for AutoReopenAfterFailedReview to be called")
	}
	assert.Contains(t, notifier.titles(), "Review session ended without a verdict")
}

// TestBacklogLifecycleListener_OnSessionExited_ReviewSession_RoutesToHandleReviewSessionExited
// verifies that onSessionExited dispatches Role==SessionRoleReview to
// handleReviewSessionExited (proven by observing its FAIL-verdict side effect —
// the auto-reopener firing) rather than falling through to the work-session
// in_progress→review/done transition logic.
func TestBacklogLifecycleListener_OnSessionExited_ReviewSession_RoutesToHandleReviewSessionExited(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	_, reviewIS, _ := newHandleReviewSessionExitedFixture(t, storage, &ReviewVerdictData{
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
