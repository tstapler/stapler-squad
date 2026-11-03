package services

import (
	"testing"
	"time"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// spawnedPRPendingItem returns an item with one live work session, parked in pr_pending
// with a PR reference and notes, the state a deleted/dead work session leaves behind.
func spawnedPRPendingItem(t *testing.T, svc *BacklogService, storage *session.Storage) string {
	t.Helper()
	itemID := spawnReadyItemForDriftHookTest(t, svc, "pr pending restart item")
	_, err := svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: itemID}))
	require.NoError(t, err)

	notes, prURL := "keep these notes", "https://github.com/o/r/pull/7"
	_, err = storage.UpdateBacklogItem(t.Context(), itemID, session.BacklogItemUpdate{Notes: &notes, PrURL: &prURL}, nil)
	require.NoError(t, err)
	_, err = storage.Rollback(t.Context(), itemID, session.BacklogStatusPRPending,
		&session.BacklogItemPrecondition{ExpectedStatus: string(session.BacklogStatusInProgress)}, session.TriggeredBySystem)
	require.NoError(t, err)
	return itemID
}

func TestSpawnSessionFromItem_should_RestartPRPendingItem_When_ForceTrue(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)
	stopper := &mockSessionStopper{}
	svc.SetSessionStopper(stopper)
	itemID := spawnedPRPendingItem(t, svc, storage)
	require.Len(t, creator.calls, 1)

	_, err := svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: itemID, Force: true}))
	require.NoError(t, err)

	assert.Len(t, creator.calls, 2, "force restart must spawn a new work session")
	assert.Len(t, stopper.stoppedUUIDs, 1, "the previous live work session must be stopped")
	item, err := storage.GetBacklogItem(t.Context(), itemID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusInProgress), item.Status)
	assert.Equal(t, "keep these notes", item.Notes)
	assert.Equal(t, "https://github.com/o/r/pull/7", item.PrURL)
}

func TestSpawnSessionFromItem_should_RejectPRPendingItem_When_ForceFalse(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)
	itemID := spawnedPRPendingItem(t, svc, storage)

	_, err := svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: itemID}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.Len(t, creator.calls, 1, "no new session without force")
}

func TestForceResetItem_should_StopSessionsAndMoveToInProgress_When_PRPending(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, &mockSessionCreator{}, nil, nil, nil, nil)
	stopper := &mockSessionStopper{}
	svc.SetSessionStopper(stopper)
	itemID := spawnedPRPendingItem(t, svc, storage)
	seedOpenStuckRow(t, storage, itemID, domain.StuckReasonReworkCap, time.Now(), "cap")
	seedOpenStuckRow(t, storage, itemID, domain.StuckReasonPRNeedsFix, time.Now(), "fix")
	seedOpenStuckRow(t, storage, itemID, domain.StuckReasonMultipleReasons, time.Now(), "multi")

	item, err := storage.GetBacklogItem(t.Context(), itemID)
	require.NoError(t, err)
	updated, err := svc.forceResetItem(t.Context(), item, session.TriggeredByUser)
	require.NoError(t, err)

	assert.Equal(t, string(session.BacklogStatusInProgress), updated.Status)
	assert.Len(t, stopper.stoppedUUIDs, 1)
	open, err := storage.FindOpenStuckStates(t.Context())
	require.NoError(t, err)
	assert.Empty(t, open, "pr_pending-only stuck rows must clear on force reset")
}

func TestTriggerRemediationNow_should_ListConstituentReasons_When_MultipleReasons(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{Title: "multi", Status: string(session.BacklogStatusPRPending)})
	require.NoError(t, err)
	seedOpenStuckRow(t, storage, item.ID, domain.StuckReasonPRNeedsFix, time.Now(), "x")
	seedOpenStuckRow(t, storage, item.ID, domain.StuckReasonMultipleReasons, time.Now(), "y")

	_, err = svc.TriggerRemediationNow(t.Context(), connect.NewRequest(&sessionv1.TriggerRemediationNowRequest{
		ItemId: item.ID,
		Reason: sessionv1.StuckReason_STUCK_REASON_MULTIPLE_REASONS,
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.Contains(t, err.Error(), string(domain.StuckReasonPRNeedsFix))
}
