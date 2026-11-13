package backlog

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

// IT-002: in_progress → review via onSessionExited (lifecycle listener)
// Tests that when a work session exits, the item transitions from in_progress to review,
// and the ItemSession records the exit time.
func TestBacklogIntegration_IT002_InProgressToReviewViaSessionExit(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// 1. Create item (status "in_progress"), create ItemSession (role "work")
	itemData := backlogdto.BacklogItemData{
		Title:              "Refactor database",
		Description:        "Improve query performance",
		AcceptanceCriteria: `[{"index":0,"text":"Queries run in <100ms","status":"pending"}]`,
		Priority:           2,
		Status:             string(domain.BacklogStatusInProgress),
		SkipReviewGate:     false,
	}
	createdItem, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	sessionUUID := uuid.New().String()
	isData := backlogdto.ItemSessionData{
		ItemID:      createdItem.ID,
		SessionUUID: sessionUUID,
		SessionRole: "work",
	}
	createdIS, err := storage.CreateItemSession(ctx, isData)
	require.NoError(t, err)

	// 2. Create BacklogLifecycleListener backed by real storage
	listener := NewBacklogLifecycleListener(storage)

	// 3. Call onSessionExited — the status transition and EndedAt bookkeeping
	// happen synchronously before onSessionExited spawns any goroutine (see
	// its doc comment), so no wait is needed before reading them back.
	listener.onSessionExited(sessionUUID)

	// 4. Reload item, assert status == "review"
	fetchedItem, err := storage.GetBacklogItem(ctx, createdItem.ID)
	require.NoError(t, err)
	require.Equal(t, string(domain.BacklogStatusReview), fetchedItem.Status)

	// 6. Reload ItemSession, assert ended_at is set
	repo := storage.Repo
	fetchedIS, err := repo.GetItemSession(ctx, createdIS.ID)
	require.NoError(t, err)
	require.NotNil(t, fetchedIS.EndedAt, "ItemSession should have EndedAt set")
}

// IT-003: skip_review_gate → done on session exit
// Tests that when SkipReviewGate=true, the item transitions directly to done (not review).
func TestBacklogIntegration_IT003_SkipReviewGateTransitionsToDone(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// 1. Create item with SkipReviewGate=true
	itemData := backlogdto.BacklogItemData{
		Title:              "Quick fix",
		Description:        "Minor bug fix",
		AcceptanceCriteria: `[{"index":0,"text":"Fix is deployed","status":"pending"}]`,
		Priority:           3,
		Status:             string(domain.BacklogStatusInProgress),
		SkipReviewGate:     true,
	}
	createdItem, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	// 2. Create ItemSession
	sessionUUID := uuid.New().String()
	isData := backlogdto.ItemSessionData{
		ItemID:      createdItem.ID,
		SessionUUID: sessionUUID,
		SessionRole: "work",
	}
	createdIS, err := storage.CreateItemSession(ctx, isData)
	require.NoError(t, err)

	// 3. Create listener and call onSessionExited — synchronous, see IT-002.
	listener := NewBacklogLifecycleListener(storage)
	listener.onSessionExited(sessionUUID)

	// 4. Verify item transitions to done (not review)
	fetchedItem, err := storage.GetBacklogItem(ctx, createdItem.ID)
	require.NoError(t, err)
	require.Equal(t, string(domain.BacklogStatusDone), fetchedItem.Status, "should transition to done when SkipReviewGate=true")

	// 5. Verify ItemSession.EndedAt is set
	repo := storage.Repo
	fetchedIS, err := repo.GetItemSession(ctx, createdIS.ID)
	require.NoError(t, err)
	require.NotNil(t, fetchedIS.EndedAt)
}

// IT-005: ReconcileStuckItems finds and transitions stuck item
// Tests that ReconcileStuckItems identifies items with ended sessions and transitions them to review.
func TestBacklogIntegration_IT005_ReconcileStuckItemsTransitionsToReview(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// 1. Create item (status "in_progress")
	itemData := backlogdto.BacklogItemData{
		Title:              "Stuck task",
		Description:        "This task is stuck in progress",
		AcceptanceCriteria: `[{"index":0,"text":"Complete work","status":"pending"}]`,
		Priority:           1,
		Status:             string(domain.BacklogStatusInProgress),
		SkipReviewGate:     false,
	}
	createdItem, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	// 2. Create ItemSession (role "work")
	sessionUUID := uuid.New().String()
	isData := backlogdto.ItemSessionData{
		ItemID:      createdItem.ID,
		SessionUUID: sessionUUID,
		SessionRole: "work",
	}
	createdIS, err := storage.CreateItemSession(ctx, isData)
	require.NoError(t, err)

	// 3. Manually end the ItemSession (simulate abnormal exit)
	pastTime := time.Now().Add(-5 * time.Minute)
	err = storage.UpdateItemSessionEnded(ctx, createdIS.ID, pastTime)
	require.NoError(t, err)

	// 4. Call ReconcileStuckItems via listener
	listener := NewBacklogLifecycleListener(storage)
	listener.SetEnabled(true)
	listener.ReconcileStuck(ctx)

	// 5. Reload item, assert status == "review" and notes contain "[auto]"
	fetchedItem, err := storage.GetBacklogItem(ctx, createdItem.ID)
	require.NoError(t, err)
	require.Equal(t, string(domain.BacklogStatusReview), fetchedItem.Status)

	// Notes must contain the exact auto-reconciliation marker written by ReconcileStuckItems.
	require.Contains(t, fetchedItem.Notes, "[auto]", "ReconcileStuckItems should set notes with [auto] marker")
	require.Contains(t, fetchedItem.Notes, "review", "ReconcileStuckItems notes should mention transition to review")

	// 6. A BacklogStatusEvent audit row must be recorded for the reconcile transition.
	require.Len(t, fetchedItem.StatusEvents, 1)
	require.Equal(t, string(domain.BacklogStatusInProgress), fetchedItem.StatusEvents[0].FromStatus)
	require.Equal(t, string(domain.BacklogStatusReview), fetchedItem.StatusEvents[0].ToStatus)
	require.Equal(t, session.TriggeredBySystem, fetchedItem.StatusEvents[0].TriggeredBy)
}

// IT-006: Review session exit does NOT transition item (recursion guard)
// Tests that when a review session exits, the item status does NOT change (preventing infinite loops).
func TestBacklogIntegration_IT006_ReviewSessionExitDoesNotTransition(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// 1. Create item (status "review")
	itemData := backlogdto.BacklogItemData{
		Title:              "Under review",
		Description:        "Item awaiting review",
		AcceptanceCriteria: `[{"index":0,"text":"Code reviewed","status":"pending"}]`,
		Priority:           1,
		Status:             string(domain.BacklogStatusReview),
	}
	createdItem, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	// 2. Create ItemSession with role "review"
	sessionUUID := uuid.New().String()
	isData := backlogdto.ItemSessionData{
		ItemID:      createdItem.ID,
		SessionUUID: sessionUUID,
		SessionRole: "review",
	}
	createdIS, err := storage.CreateItemSession(ctx, isData)
	require.NoError(t, err)

	// 3. Call onSessionExited — synchronous, see IT-002.
	listener := NewBacklogLifecycleListener(storage)
	listener.onSessionExited(sessionUUID)

	// 4. Verify item status did NOT change (still "review")
	fetchedItem, err := storage.GetBacklogItem(ctx, createdItem.ID)
	require.NoError(t, err)
	require.Equal(t, string(domain.BacklogStatusReview), fetchedItem.Status, "review session exit should not transition item")

	// 5. Verify ItemSession.EndedAt IS set (exit is recorded for all roles)
	repo := storage.Repo
	fetchedIS, err := repo.GetItemSession(ctx, createdIS.ID)
	require.NoError(t, err)
	require.NotNil(t, fetchedIS.EndedAt, "review session exit should set EndedAt")
}
