package session

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/ent"
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
	fetchedIS, err := repo.GetItemSession(ctx, createdIS.ID.String())
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
	fetchedIS, err := repo.GetItemSession(ctx, createdIS.ID.String())
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
	fetchedIS, err := repo.GetItemSession(ctx, createdIS.ID.String())
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

	// Verify that the ItemSession EndedAt was NOT set (review sessions are guarded).
	repo := storage.repo.(*EntRepository)
	fetchedIS, err := repo.GetItemSession(ctx, createdIS.ID.String())
	require.NoError(t, err)
	require.Nil(t, fetchedIS.EndedAt, "review session should not have EndedAt set (recursion guard)")
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
	fetchedIS, err := repo.GetItemSession(ctx, createdIS.ID.String())
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
		fetchedIS, ferr := repo.GetItemSession(ctx, createdIS.ID.String())
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
type mockReviewGateSpawner struct {
	spawnCalled bool
	lastItem    *ent.BacklogItem
}

func (m *mockReviewGateSpawner) SpawnReviewSession(ctx context.Context, item *ent.BacklogItem, itemSessionID string, prompt string) (*Instance, error) {
	m.spawnCalled = true
	m.lastItem = item
	return &Instance{}, nil
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
