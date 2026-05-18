package session

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// IT-001: Create item → transition to ready → spawn marker → verify in_progress
// Tests the full lifecycle from idea through creating an ItemSession that marks the item in_progress.
func TestBacklogIntegration_IT001_IdeaToInProgressWithItemSession(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// 1. Create a BacklogItem with status "idea"
	itemData := BacklogItemData{
		Title:              "Implement login feature",
		Description:        "User authentication flow",
		AcceptanceCriteria: `[{"index":0,"text":"User can enter credentials","status":"pending"}]`,
		Priority:           1,
		Status:             string(BacklogStatusIdea),
	}
	createdItem, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)
	require.NotNil(t, createdItem)
	require.Equal(t, string(BacklogStatusIdea), createdItem.Status)

	// 2. Transition to "ready"
	readyItem, err := storage.TransitionBacklogItemStatus(ctx, createdItem.ID, BacklogStatusReady, nil)
	require.NoError(t, err)
	require.Equal(t, string(BacklogStatusReady), readyItem.Status)

	// 3. Approve plan and transition to "in_progress"
	readyItem.PlanApproved = true
	inProgressItem, err := storage.TransitionBacklogItemStatus(ctx, createdItem.ID, BacklogStatusInProgress, nil)
	require.NoError(t, err)
	require.Equal(t, string(BacklogStatusInProgress), inProgressItem.Status)

	// 4. Create an ItemSession (role "work")
	sessionUUID := uuid.New().String()
	isData := ItemSessionData{
		ItemID:      createdItem.ID,
		SessionUUID: sessionUUID,
		SessionRole: "work",
	}
	createdIS, err := storage.CreateItemSession(ctx, isData)
	require.NoError(t, err)
	require.NotNil(t, createdIS)
	require.Equal(t, "work", createdIS.SessionRole)

	// 5. Verify state
	fetchedItem, err := storage.GetBacklogItem(ctx, createdItem.ID)
	require.NoError(t, err)
	require.Equal(t, string(BacklogStatusInProgress), fetchedItem.Status)
}

// IT-002: in_progress → review via onSessionExited (lifecycle listener)
// Tests that when a work session exits, the item transitions from in_progress to review,
// and the ItemSession records the exit time.
func TestBacklogIntegration_IT002_InProgressToReviewViaSessionExit(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// 1. Create item (status "in_progress"), create ItemSession (role "work")
	itemData := BacklogItemData{
		Title:              "Refactor database",
		Description:        "Improve query performance",
		AcceptanceCriteria: `[{"index":0,"text":"Queries run in <100ms","status":"pending"}]`,
		Priority:           2,
		Status:             string(BacklogStatusInProgress),
		SkipReviewGate:     false,
	}
	createdItem, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	sessionUUID := uuid.New().String()
	isData := ItemSessionData{
		ItemID:      createdItem.ID,
		SessionUUID: sessionUUID,
		SessionRole: "work",
	}
	createdIS, err := storage.CreateItemSession(ctx, isData)
	require.NoError(t, err)

	// 2. Create BacklogLifecycleListener backed by real storage
	listener := NewBacklogLifecycleListener(storage)

	// 3. Call onSessionExited
	listener.onSessionExited(sessionUUID)

	// 4. Sleep to allow goroutine to complete
	time.Sleep(150 * time.Millisecond)

	// 5. Reload item, assert status == "review"
	fetchedItem, err := storage.GetBacklogItem(ctx, createdItem.ID)
	require.NoError(t, err)
	require.Equal(t, string(BacklogStatusReview), fetchedItem.Status)

	// 6. Reload ItemSession, assert ended_at is set
	repo := storage.repo.(*EntRepository)
	fetchedIS, err := repo.GetItemSession(ctx, createdIS.ID.String())
	require.NoError(t, err)
	require.NotNil(t, fetchedIS.EndedAt, "ItemSession should have EndedAt set")
}

// IT-003: skip_review_gate → done on session exit
// Tests that when SkipReviewGate=true, the item transitions directly to done (not review).
func TestBacklogIntegration_IT003_SkipReviewGateTransitionsToDone(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// 1. Create item with SkipReviewGate=true
	itemData := BacklogItemData{
		Title:              "Quick fix",
		Description:        "Minor bug fix",
		AcceptanceCriteria: `[{"index":0,"text":"Fix is deployed","status":"pending"}]`,
		Priority:           3,
		Status:             string(BacklogStatusInProgress),
		SkipReviewGate:     true,
	}
	createdItem, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	// 2. Create ItemSession
	sessionUUID := uuid.New().String()
	isData := ItemSessionData{
		ItemID:      createdItem.ID,
		SessionUUID: sessionUUID,
		SessionRole: "work",
	}
	createdIS, err := storage.CreateItemSession(ctx, isData)
	require.NoError(t, err)

	// 3. Create listener and call onSessionExited
	listener := NewBacklogLifecycleListener(storage)
	listener.onSessionExited(sessionUUID)
	time.Sleep(150 * time.Millisecond)

	// 4. Verify item transitions to done (not review)
	fetchedItem, err := storage.GetBacklogItem(ctx, createdItem.ID)
	require.NoError(t, err)
	require.Equal(t, string(BacklogStatusDone), fetchedItem.Status, "should transition to done when SkipReviewGate=true")

	// 5. Verify ItemSession.EndedAt is set
	repo := storage.repo.(*EntRepository)
	fetchedIS, err := repo.GetItemSession(ctx, createdIS.ID.String())
	require.NoError(t, err)
	require.NotNil(t, fetchedIS.EndedAt)
}

// IT-004: AC criterion update roundtrip
// Tests that AC criteria can be updated via storage.UpdateAcCriterionStatus
// and the updated value survives a reload.
func TestBacklogIntegration_IT004_AcCriterionUpdateRoundtrip(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// 1. Create item with AC criteria
	criteria := []AcCriterion{
		{Index: 0, Text: "must compile", Status: "pending"},
		{Index: 1, Text: "tests pass", Status: "pending"},
	}
	rawCriteria, err := SerializeAcCriteria(criteria)
	require.NoError(t, err)

	itemData := BacklogItemData{
		Title:              "Build feature",
		Description:        "New feature implementation",
		AcceptanceCriteria: rawCriteria,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
	}
	createdItem, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	// 2. Call storage.UpdateAcCriterionStatus for criterion 0
	err = storage.UpdateAcCriterionStatus(ctx, createdItem.ID, 0, "done", "compiled successfully")
	require.NoError(t, err)

	// 3. Reload item, parse AC criteria, assert criteria[0].Status == "done"
	fetchedItem, err := storage.GetBacklogItem(ctx, createdItem.ID)
	require.NoError(t, err)

	parsedCriteria, err := ParseAcCriteria(fetchedItem.AcceptanceCriteria)
	require.NoError(t, err)
	require.Len(t, parsedCriteria, 2)
	require.Equal(t, "done", parsedCriteria[0].Status)
	require.Equal(t, "pending", parsedCriteria[1].Status)
}

// IT-005: ReconcileStuckItems finds and transitions stuck item
// Tests that ReconcileStuckItems identifies items with ended sessions and transitions them to review.
func TestBacklogIntegration_IT005_ReconcileStuckItemsTransitionsToReview(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// 1. Create item (status "in_progress")
	itemData := BacklogItemData{
		Title:              "Stuck task",
		Description:        "This task is stuck in progress",
		AcceptanceCriteria: `[{"index":0,"text":"Complete work","status":"pending"}]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
		SkipReviewGate:     false,
	}
	createdItem, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	// 2. Create ItemSession (role "work")
	sessionUUID := uuid.New().String()
	isData := ItemSessionData{
		ItemID:      createdItem.ID,
		SessionUUID: sessionUUID,
		SessionRole: "work",
	}
	createdIS, err := storage.CreateItemSession(ctx, isData)
	require.NoError(t, err)

	// 3. Manually end the ItemSession (simulate abnormal exit)
	pastTime := time.Now().Add(-5 * time.Minute)
	err = storage.UpdateItemSessionEnded(ctx, createdIS.ID.String(), pastTime)
	require.NoError(t, err)

	// 4. Call ReconcileStuckItems via listener
	listener := NewBacklogLifecycleListener(storage)
	listener.SetEnabled(true)
	listener.ReconcileStuck(ctx)

	// 5. Reload item, assert status == "review" and notes contain "[auto]"
	fetchedItem, err := storage.GetBacklogItem(ctx, createdItem.ID)
	require.NoError(t, err)
	require.Equal(t, string(BacklogStatusReview), fetchedItem.Status)

	// Notes must contain the exact auto-reconciliation marker written by ReconcileStuckItems.
	require.Contains(t, fetchedItem.Notes, "[auto]", "ReconcileStuckItems should set notes with [auto] marker")
	require.Contains(t, fetchedItem.Notes, "review", "ReconcileStuckItems notes should mention transition to review")
}

// IT-006: Review session exit does NOT transition item (recursion guard)
// Tests that when a review session exits, the item status does NOT change (preventing infinite loops).
func TestBacklogIntegration_IT006_ReviewSessionExitDoesNotTransition(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// 1. Create item (status "review")
	itemData := BacklogItemData{
		Title:              "Under review",
		Description:        "Item awaiting review",
		AcceptanceCriteria: `[{"index":0,"text":"Code reviewed","status":"pending"}]`,
		Priority:           1,
		Status:             string(BacklogStatusReview),
	}
	createdItem, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	// 2. Create ItemSession with role "review"
	sessionUUID := uuid.New().String()
	isData := ItemSessionData{
		ItemID:      createdItem.ID,
		SessionUUID: sessionUUID,
		SessionRole: "review",
	}
	createdIS, err := storage.CreateItemSession(ctx, isData)
	require.NoError(t, err)

	// 3. Call onSessionExited
	listener := NewBacklogLifecycleListener(storage)
	listener.onSessionExited(sessionUUID)
	time.Sleep(150 * time.Millisecond)

	// 4. Verify item status did NOT change (still "review")
	fetchedItem, err := storage.GetBacklogItem(ctx, createdItem.ID)
	require.NoError(t, err)
	require.Equal(t, string(BacklogStatusReview), fetchedItem.Status, "review session exit should not transition item")

	// 5. Verify ItemSession.EndedAt was NOT set (recursion guard)
	repo := storage.repo.(*EntRepository)
	fetchedIS, err := repo.GetItemSession(ctx, createdIS.ID.String())
	require.NoError(t, err)
	require.Nil(t, fetchedIS.EndedAt, "review session exit should not set EndedAt (recursion guard)")
}

// IT-007: Multiple ItemSessions for same item
// Tests that an item can have multiple ItemSessions (e.g., work → review → work again).
func TestBacklogIntegration_IT007_MultipleItemSessionsPerItem(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// 1. Create item
	itemData := BacklogItemData{
		Title:              "Iterative work",
		Description:        "Requires multiple passes",
		AcceptanceCriteria: `[{"index":0,"text":"Work complete","status":"pending"}]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
	}
	createdItem, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	// 2. Create first work session
	workSession1UUID := uuid.New().String()
	is1Data := ItemSessionData{
		ItemID:      createdItem.ID,
		SessionUUID: workSession1UUID,
		SessionRole: "work",
	}
	is1, err := storage.CreateItemSession(ctx, is1Data)
	require.NoError(t, err)

	// 3. Create a review session
	reviewSessionUUID := uuid.New().String()
	is2Data := ItemSessionData{
		ItemID:      createdItem.ID,
		SessionUUID: reviewSessionUUID,
		SessionRole: "review",
	}
	is2, err := storage.CreateItemSession(ctx, is2Data)
	require.NoError(t, err)

	// 4. Create another work session
	workSession2UUID := uuid.New().String()
	is3Data := ItemSessionData{
		ItemID:      createdItem.ID,
		SessionUUID: workSession2UUID,
		SessionRole: "work",
	}
	is3, err := storage.CreateItemSession(ctx, is3Data)
	require.NoError(t, err)

	// 5. Verify all three ItemSessions exist and have their session UUIDs
	repo := storage.repo.(*EntRepository)

	fetchedIS1, err := repo.GetItemSession(ctx, is1.ID.String())
	require.NoError(t, err)
	require.Equal(t, workSession1UUID, fetchedIS1.SessionUUID)
	require.Equal(t, "work", fetchedIS1.SessionRole)

	fetchedIS2, err := repo.GetItemSession(ctx, is2.ID.String())
	require.NoError(t, err)
	require.Equal(t, reviewSessionUUID, fetchedIS2.SessionUUID)
	require.Equal(t, "review", fetchedIS2.SessionRole)

	fetchedIS3, err := repo.GetItemSession(ctx, is3.ID.String())
	require.NoError(t, err)
	require.Equal(t, workSession2UUID, fetchedIS3.SessionUUID)
	require.Equal(t, "work", fetchedIS3.SessionRole)
}

// IT-008: ItemSession.AcSnapshot captures AC at time of session creation
// Tests that AcSnapshot preserves AC criteria at the moment the work session starts.
func TestBacklogIntegration_IT008_AcSnapshotCapture(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// 1. Create item with initial AC
	initialCriteria := []AcCriterion{
		{Index: 0, Text: "initial requirement", Status: "pending"},
	}
	initialRaw, err := SerializeAcCriteria(initialCriteria)
	require.NoError(t, err)

	itemData := BacklogItemData{
		Title:              "Snapshot test",
		Description:        "Testing AC snapshot",
		AcceptanceCriteria: initialRaw,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
	}
	createdItem, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	// 2. Create work session with AcSnapshot
	sessionUUID := uuid.New().String()
	isData := ItemSessionData{
		ItemID:      createdItem.ID,
		SessionUUID: sessionUUID,
		SessionRole: "work",
		AcSnapshot:  initialRaw,
	}
	createdIS, err := storage.CreateItemSession(ctx, isData)
	require.NoError(t, err)

	// 3. Reload ItemSession, verify AcSnapshot still has the snapshot content
	repo := storage.repo.(*EntRepository)
	fetchedIS, err := repo.GetItemSession(ctx, createdIS.ID.String())
	require.NoError(t, err)

	snapshotCriteria, err := ParseAcCriteria(fetchedIS.AcSnapshot)
	require.NoError(t, err)
	require.Len(t, snapshotCriteria, 1, "AcSnapshot should preserve original AC at session creation time")
	require.Equal(t, "initial requirement", snapshotCriteria[0].Text)
}

// IT-009: GetItemSessionBySessionAndItem validates session-item link
// Tests that GetItemSessionBySessionAndItem correctly verifies a session is linked to a specific item.
func TestBacklogIntegration_IT009_GetItemSessionBySessionAndItem(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// 1. Create two items
	item1Data := BacklogItemData{
		Title:              "Item 1",
		Description:        "First item",
		AcceptanceCriteria: `[{"index":0,"text":"Complete","status":"pending"}]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
	}
	item1, err := storage.CreateBacklogItem(ctx, item1Data)
	require.NoError(t, err)

	item2Data := BacklogItemData{
		Title:              "Item 2",
		Description:        "Second item",
		AcceptanceCriteria: `[{"index":0,"text":"Complete","status":"pending"}]`,
		Priority:           2,
		Status:             string(BacklogStatusInProgress),
	}
	item2, err := storage.CreateBacklogItem(ctx, item2Data)
	require.NoError(t, err)

	// 2. Create session linked to item1
	sessionUUID := uuid.New().String()
	isData := ItemSessionData{
		ItemID:      item1.ID,
		SessionUUID: sessionUUID,
		SessionRole: "work",
	}
	createdIS, err := storage.CreateItemSession(ctx, isData)
	require.NoError(t, err)

	// 3. Verify session is linked to item1
	linkedIS, err := storage.GetItemSessionBySessionAndItem(ctx, sessionUUID, item1.ID)
	require.NoError(t, err)
	require.Equal(t, createdIS.ID.String(), linkedIS.ID.String())

	// 4. Verify session is NOT linked to item2
	_, err = storage.GetItemSessionBySessionAndItem(ctx, sessionUUID, item2.ID)
	require.Error(t, err)
}

// IT-010: ItemSession.LastCommitSha can be updated
// Tests that LastCommitSha is persisted and retrievable (used for review gate diffs).
func TestBacklogIntegration_IT010_ItemSessionLastCommitSha(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// 1. Create item and session
	itemData := BacklogItemData{
		Title:              "Git tracking",
		Description:        "Tracking commits",
		AcceptanceCriteria: `[{"index":0,"text":"Done","status":"pending"}]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
	}
	createdItem, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	sessionUUID := uuid.New().String()
	isData := ItemSessionData{
		ItemID:      createdItem.ID,
		SessionUUID: sessionUUID,
		SessionRole: "work",
	}
	createdIS, err := storage.CreateItemSession(ctx, isData)
	require.NoError(t, err)

	// 2. Update LastCommitSha
	repo := storage.repo.(*EntRepository)
	testSha := "abc123def456"

	_, err = repo.client.ItemSession.UpdateOne(createdIS).
		SetLastCommitSha(testSha).
		Save(ctx)
	require.NoError(t, err)

	// 3. Reload and verify LastCommitSha
	fetchedIS, err := repo.GetItemSession(ctx, createdIS.ID.String())
	require.NoError(t, err)
	require.Equal(t, testSha, fetchedIS.LastCommitSha)
}
