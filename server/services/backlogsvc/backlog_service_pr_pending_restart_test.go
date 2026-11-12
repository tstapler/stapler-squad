package backlogsvc

import (
	"testing"
	"time"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"

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
	_, err = storage.UpdateBacklogItem(t.Context(), itemID, backlogdto.BacklogItemUpdate{Notes: &notes, PrURL: &prURL}, nil)
	require.NoError(t, err)
	_, err = storage.Rollback(t.Context(), itemID, session.BacklogStatusPRPending,
		&backlogdto.BacklogItemPrecondition{ExpectedStatus: string(session.BacklogStatusInProgress)}, session.TriggeredBySystem)
	require.NoError(t, err)
	return itemID
}

func TestSpawnSessionFromItem_should_RestartPRPendingItem_When_ForceTrue(t *testing.T) {
	t.Parallel()
	storage := svctest.CreateTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)
	stopper := &svctest.MockSessionStopper{}
	svc.SetSessionStopper(stopper)
	itemID := spawnedPRPendingItem(t, svc, storage)
	require.Len(t, creator.calls, 1)
	first, err := storage.ListItemSessions(t.Context(), itemID)
	require.NoError(t, err)
	require.Len(t, first, 1)

	_, err = svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: itemID, Force: true}))
	require.NoError(t, err)

	assert.Len(t, creator.calls, 2, "force restart must spawn a new work session")
	assert.Equal(t, []string{first[0].SessionUUID}, stopper.StoppedUUIDs, "the previous live work session must be stopped")
	item, err := storage.GetBacklogItem(t.Context(), itemID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusInProgress), item.Status)
	assert.Equal(t, "keep these notes", item.Notes)
	assert.Equal(t, "https://github.com/o/r/pull/7", item.PrURL)
}

func TestSpawnSessionFromItem_should_RestartPRPendingItem_When_WorkSessionAlreadyEnded(t *testing.T) {
	t.Parallel()
	storage := svctest.CreateTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)
	stopper := &svctest.MockSessionStopper{}
	svc.SetSessionStopper(stopper)
	itemID := spawnedPRPendingItem(t, svc, storage)
	sessions, err := storage.ListItemSessions(t.Context(), itemID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.NoError(t, storage.UpdateItemSessionEnded(t.Context(), sessions[0].ID, time.Now()))

	_, err = svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: itemID, Force: true}))
	require.NoError(t, err)

	assert.Len(t, creator.calls, 2, "a gone work session must not block the restart")
	assert.Empty(t, stopper.StoppedUUIDs, "nothing live to stop")
	item, err := storage.GetBacklogItem(t.Context(), itemID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusInProgress), item.Status)
}

func TestSpawnSessionFromItem_should_RejectPRPendingItem_When_ForceFalse(t *testing.T) {
	t.Parallel()
	storage := svctest.CreateTestStorage(t)
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
	storage := svctest.CreateTestStorage(t)
	svc := NewBacklogService(storage, &mockSessionCreator{}, nil, nil, nil, nil)
	stopper := &svctest.MockSessionStopper{}
	svc.SetSessionStopper(stopper)
	itemID := spawnedPRPendingItem(t, svc, storage)
	svctest.SeedOpenStuckRow(t, storage, itemID, domain.StuckReasonReworkCap, time.Now(), "cap")
	svctest.SeedOpenStuckRow(t, storage, itemID, domain.StuckReasonPRNeedsFix, time.Now(), "fix")
	svctest.SeedOpenStuckRow(t, storage, itemID, domain.StuckReasonMultipleReasons, time.Now(), "multi")
	svctest.SeedOpenStuckRow(t, storage, itemID, domain.StuckReasonSteerFailed, time.Now(), "steer")
	svctest.SeedOpenStuckRow(t, storage, itemID, domain.StuckReasonRespawnBlockedActive, time.Now(), "blocked")
	svctest.SeedOpenStuckRow(t, storage, itemID, domain.StuckReasonBouncing, time.Now(), "out of scope")
	other, err := storage.CreateBacklogItem(t.Context(), backlogdto.BacklogItemData{Title: "other", Status: string(session.BacklogStatusPRPending)})
	require.NoError(t, err)
	svctest.SeedOpenStuckRow(t, storage, other.ID, domain.StuckReasonPRNeedsFix, time.Now(), "other item")

	item, err := storage.GetBacklogItem(t.Context(), itemID)
	require.NoError(t, err)
	updated, err := svc.forceResetItem(t.Context(), item, session.TriggeredByUser)
	require.NoError(t, err)

	assert.Equal(t, string(session.BacklogStatusInProgress), updated.Status)
	assert.Len(t, stopper.StoppedUUIDs, 1)
	open, err := storage.FindOpenStuckStates(t.Context())
	require.NoError(t, err)
	type key struct {
		itemID string
		reason domain.StuckReason
	}
	remaining := make([]key, 0, len(open))
	for _, row := range open {
		remaining = append(remaining, key{row.ItemID, row.Reason})
	}
	assert.ElementsMatch(t, []key{
		{itemID, domain.StuckReasonBouncing},
		{other.ID, domain.StuckReasonPRNeedsFix},
	}, remaining, "only this item's pr_pending-scoped rows clear on force reset")
}

func TestTriggerRemediationNow_should_ExplainMultipleReasons_When_Aggregate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		seed         []domain.StuckReason
		wantContains []string
		wantMissing  []string
		wantNoneOpen bool
	}{
		{
			name:         "lists constituents and excludes other items' rows",
			seed:         []domain.StuckReason{domain.StuckReasonPRNeedsFix, domain.StuckReasonMultipleReasons},
			wantContains: []string{string(domain.StuckReasonPRNeedsFix)},
			wantMissing:  []string{string(domain.StuckReasonPushFailed)},
		},
		{
			name:         "reports no constituents when only the aggregate is open",
			seed:         []domain.StuckReason{domain.StuckReasonMultipleReasons},
			wantContains: []string{"no constituent reasons are open"},
			wantNoneOpen: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			storage := svctest.CreateTestStorage(t)
			svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
			item, err := storage.CreateBacklogItem(t.Context(), backlogdto.BacklogItemData{Title: "multi", Status: string(session.BacklogStatusPRPending)})
			require.NoError(t, err)
			other, err := storage.CreateBacklogItem(t.Context(), backlogdto.BacklogItemData{Title: "other", Status: string(session.BacklogStatusPRPending)})
			require.NoError(t, err)
			for _, reason := range tt.seed {
				svctest.SeedOpenStuckRow(t, storage, item.ID, reason, time.Now(), "x")
			}
			svctest.SeedOpenStuckRow(t, storage, other.ID, domain.StuckReasonPushFailed, time.Now(), "other item")

			_, err = svc.TriggerRemediationNow(t.Context(), connect.NewRequest(&sessionv1.TriggerRemediationNowRequest{
				ItemId: item.ID,
				Reason: sessionv1.StuckReason_STUCK_REASON_MULTIPLE_REASONS,
			}))

			require.Error(t, err)
			assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
			for _, want := range tt.wantContains {
				assert.Contains(t, err.Error(), want)
			}
			for _, miss := range tt.wantMissing {
				assert.NotContains(t, err.Error(), miss)
			}
			if tt.wantNoneOpen {
				assert.NotContains(t, err.Error(), "[")
			}
		})
	}
}

func TestSpawnSessionFromItem_should_RefuseForceRestart_When_JulesSessionLive(t *testing.T) {
	t.Parallel()
	storage := svctest.CreateTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)
	itemID := spawnedPRPendingItem(t, svc, storage)
	_, err := storage.CreateItemSession(t.Context(), backlogdto.ItemSessionData{
		ItemID: itemID, SessionUUID: "jules-sessions/force-guard", SessionRole: session.SessionRoleJulesWork,
	})
	require.NoError(t, err)

	_, err = svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: itemID, Force: true}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.Len(t, creator.calls, 1, "no new session may spawn")
	item, err := storage.GetBacklogItem(t.Context(), itemID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusPRPending), item.Status, "item must not be half-reset")
}
