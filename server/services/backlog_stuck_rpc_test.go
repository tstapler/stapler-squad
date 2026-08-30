package services

// backlog_stuck_rpc_test.go — tests for the stuck-backlog-item read RPC
// surface added in Phase 3 (Epic 3.1) of the backlog-stuck-item-visibility
// feature: ListStuckBacklogItems, SnoozeStuckItem, and the proto <-> domain
// StuckReason mapping helpers in backlog_service_stuck.go.

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestToProtoStuckReason_should_mapToUnspecified_When_UnknownString verifies
// the proto reason mapping never panics on an unrecognized DB string and maps
// it to STUCK_REASON_UNSPECIFIED, while all six known domain.StuckReason
// constants map to their matching proto enum value.
func TestToProtoStuckReason_should_mapToUnspecified_When_UnknownString(t *testing.T) {
	t.Parallel()
	t.Run("unknown reason maps to unspecified", func(t *testing.T) {
		t.Parallel()
		got := toProtoStuckReason(domain.StuckReason("banana"))
		assert.Equal(t, sessionv1.StuckReason_STUCK_REASON_UNSPECIFIED, got)
	})

	t.Run("empty reason maps to unspecified", func(t *testing.T) {
		t.Parallel()
		got := toProtoStuckReason(domain.StuckReason(""))
		assert.Equal(t, sessionv1.StuckReason_STUCK_REASON_UNSPECIFIED, got)
	})

	cases := []struct {
		reason domain.StuckReason
		want   sessionv1.StuckReason
	}{
		{domain.StuckReasonPRReadyUnmerged, sessionv1.StuckReason_STUCK_REASON_PR_READY_UNMERGED},
		{domain.StuckReasonReworkCap, sessionv1.StuckReason_STUCK_REASON_REWORK_CAP},
		{domain.StuckReasonAbandonedReview, sessionv1.StuckReason_STUCK_REASON_ABANDONED_REVIEW},
		{domain.StuckReasonStaleWork, sessionv1.StuckReason_STUCK_REASON_STALE_WORK},
		{domain.StuckReasonBouncing, sessionv1.StuckReason_STUCK_REASON_BOUNCING},
		{domain.StuckReasonPushFailed, sessionv1.StuckReason_STUCK_REASON_PUSH_FAILED},
		{domain.StuckReasonPRPendingNoPR, sessionv1.StuckReason_STUCK_REASON_PR_PENDING_NO_PR},
		{domain.StuckReasonReworkBlockedStale, sessionv1.StuckReason_STUCK_REASON_REWORK_BLOCKED_STALE},
		{domain.StuckReasonPRNeedsFix, sessionv1.StuckReason_STUCK_REASON_PR_NEEDS_FIX},
		{domain.StuckReasonRespawnBlockedActive, sessionv1.StuckReason_STUCK_REASON_RESPAWN_BLOCKED_ACTIVE},
		{domain.StuckReasonLikelyFlaky, sessionv1.StuckReason_STUCK_REASON_LIKELY_FLAKY},
	}
	for _, c := range cases {
		t.Run(string(c.reason), func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, c.want, toProtoStuckReason(c.reason))
			// Round-trip: the inverse mapping must recover the same domain reason.
			assert.Equal(t, c.reason, fromProtoStuckReason(c.want))
		})
	}
}

// TestToProtoStuckReason_should_ReturnMultipleReasons_When_DomainStuckReasonMultipleReasons
// verifies the new synthetic aggregate reason (backlog-bounce-escalation,
// Epic 1.1) maps to its dedicated proto enum value rather than falling
// through to STUCK_REASON_UNSPECIFIED.
func TestToProtoStuckReason_should_ReturnMultipleReasons_When_DomainStuckReasonMultipleReasons(t *testing.T) {
	t.Parallel()
	got := toProtoStuckReason(domain.StuckReasonMultipleReasons)
	assert.Equal(t, sessionv1.StuckReason_STUCK_REASON_MULTIPLE_REASONS, got)
}

// TestFromProtoStuckReason_should_ReturnBounceCapExhausted_When_ProtoBounceCapExhausted
// verifies the inverse mapping for the new synthetic aggregate reason
// (backlog-bounce-escalation, Epic 1.1) recovers the correct domain constant.
func TestFromProtoStuckReason_should_ReturnBounceCapExhausted_When_ProtoBounceCapExhausted(t *testing.T) {
	t.Parallel()
	got := fromProtoStuckReason(sessionv1.StuckReason_STUCK_REASON_BOUNCE_CAP_EXHAUSTED)
	assert.Equal(t, domain.StuckReasonBounceCapExhausted, got)
}

// seedOpenStuckRow creates a backlog item and inserts an open BacklogStuckState
// row directly via the ent client (bypassing MarkStuck/its status precondition,
// since these RPC-level tests only need a row to exist, not the reconciler's
// write path — that path is covered by session/ent_repository_backlog_stuck_test.go).
func seedOpenStuckRow(t *testing.T, storage *session.Storage, itemID string, reason domain.StuckReason, firstDetectedAt time.Time, stuckContext string) {
	t.Helper()
	client := storage.GetEntClient()
	require.NotNil(t, client)

	parsedID, err := uuid.Parse(itemID)
	require.NoError(t, err)

	now := time.Now()
	err = client.BacklogStuckState.Create().
		SetItemID(parsedID).
		SetReason(string(reason)).
		SetFirstDetectedAt(firstDetectedAt).
		SetLastCheckedAt(now).
		SetContext(stuckContext).
		Exec(context.Background())
	require.NoError(t, err)
}

// TestListStuckBacklogItems_should_returnMappedItems_When_OpenRowsExist
// verifies the handler maps an open BacklogStuckState row (joined with its
// parent item) to a StuckBacklogItem with the correct reason enum, PR
// context, and duration.
func TestListStuckBacklogItems_should_returnMappedItems_When_OpenRowsExist(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	ctx := t.Context()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "PR #148 ready to merge",
		Status: string(session.BacklogStatusPRPending),
	})
	require.NoError(t, err)

	prNumber := 148
	prURL := "https://github.com/example/repo/pull/148"
	_, err = storage.UpdateBacklogItem(ctx, item.ID, session.BacklogItemUpdate{
		PrNumber: &prNumber,
		PrURL:    &prURL,
	}, nil)
	require.NoError(t, err)

	threeDaysAgo := time.Now().Add(-3 * 24 * time.Hour)
	seedOpenStuckRow(t, storage, item.ID, domain.StuckReasonPRReadyUnmerged, threeDaysAgo, "PR #148 green & mergeable 3d")

	resp, err := svc.ListStuckBacklogItems(ctx, connect.NewRequest(&sessionv1.ListStuckBacklogItemsRequest{}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Items, 1)

	got := resp.Msg.Items[0]
	assert.Equal(t, item.ID, got.ItemId)
	assert.Equal(t, sessionv1.StuckReason_STUCK_REASON_PR_READY_UNMERGED, got.Reason)
	assert.Equal(t, int32(148), got.PrNumber)
	assert.Equal(t, prURL, got.PrUrl)
	assert.Equal(t, "PR #148 green & mergeable 3d", got.Context)
	require.NotNil(t, got.FirstDetectedAt)
	assert.WithinDuration(t, threeDaysAgo, got.FirstDetectedAt.AsTime(), 5*time.Second)
	// allow_auto_merge is a Phase 4 concern — this handler must leave it unset.
	assert.Nil(t, got.AllowAutoMerge)
}

// TestListStuckBacklogItems_should_PopulatePlanArtifactsPath verifies
// plan_artifacts_path round-trips end to end — from the parent item's
// column, through the FindOpenStuckStates join, through
// stuckBacklogItemToProto — so the frontend's hasPlan gate
// (StuckItemDetail.tsx) has real data to check instead of trusting `reason`
// alone (research/pitfalls.md #1).
func TestListStuckBacklogItems_should_PopulatePlanArtifactsPath(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	ctx := t.Context()

	itemWithPlan, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "queued item with a plan",
		Status: string(session.BacklogStatusQueued),
	})
	require.NoError(t, err)
	planPath := "project_plans/queued-item/plan.md"
	_, err = storage.UpdateBacklogItem(ctx, itemWithPlan.ID, session.BacklogItemUpdate{
		PlanArtifactsPath: &planPath,
	}, nil)
	require.NoError(t, err)
	seedOpenStuckRow(t, storage, itemWithPlan.ID, domain.StuckReasonPlanNotApproved, time.Now(), "has a plan")

	itemWithoutPlan, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "queued item with no plan yet",
		Status: string(session.BacklogStatusQueued),
	})
	require.NoError(t, err)
	seedOpenStuckRow(t, storage, itemWithoutPlan.ID, domain.StuckReasonPlanNotApproved, time.Now(), "no plan yet")

	resp, err := svc.ListStuckBacklogItems(ctx, connect.NewRequest(&sessionv1.ListStuckBacklogItemsRequest{}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Items, 2)

	byItemID := make(map[string]*sessionv1.StuckBacklogItem, len(resp.Msg.Items))
	for _, it := range resp.Msg.Items {
		byItemID[it.ItemId] = it
	}

	assert.Equal(t, planPath, byItemID[itemWithPlan.ID].PlanArtifactsPath)
	assert.Empty(t, byItemID[itemWithoutPlan.ID].PlanArtifactsPath)
}

// TestSnoozeStuckItem_should_setSnoozedUntilAndOmitFromList_When_Called
// verifies SnoozeStuckItem sets snoozed_until on the matching open row and
// that the next ListStuckBacklogItems call omits it.
func TestSnoozeStuckItem_should_setSnoozedUntilAndOmitFromList_When_Called(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	ctx := t.Context()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "rework-capped item",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)

	seedOpenStuckRow(t, storage, item.ID, domain.StuckReasonReworkCap, time.Now(), "rework cap hit")

	// Confirm it's visible before snoozing.
	before, err := svc.ListStuckBacklogItems(ctx, connect.NewRequest(&sessionv1.ListStuckBacklogItemsRequest{}))
	require.NoError(t, err)
	require.Len(t, before.Msg.Items, 1)

	tomorrow := time.Now().Add(24 * time.Hour)
	snoozeResp, err := svc.SnoozeStuckItem(ctx, connect.NewRequest(&sessionv1.SnoozeStuckItemRequest{
		ItemId: item.ID,
		Reason: sessionv1.StuckReason_STUCK_REASON_REWORK_CAP,
		Until:  timestamppb.New(tomorrow),
	}))
	require.NoError(t, err)
	assert.True(t, snoozeResp.Msg.Applied)

	after, err := svc.ListStuckBacklogItems(ctx, connect.NewRequest(&sessionv1.ListStuckBacklogItemsRequest{}))
	require.NoError(t, err)
	assert.Empty(t, after.Msg.Items, "snoozed row must be omitted from the active list")
}

// TestSnoozeStuckItem_should_rejectInvalidArguments_When_ReasonOrItemMissing
// covers the handler's input validation guards.
func TestSnoozeStuckItem_should_rejectInvalidArguments_When_ReasonOrItemMissing(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	ctx := t.Context()

	_, err := svc.SnoozeStuckItem(ctx, connect.NewRequest(&sessionv1.SnoozeStuckItemRequest{
		Reason: sessionv1.StuckReason_STUCK_REASON_REWORK_CAP,
		Until:  timestamppb.New(time.Now().Add(time.Hour)),
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

	_, err = svc.SnoozeStuckItem(ctx, connect.NewRequest(&sessionv1.SnoozeStuckItemRequest{
		ItemId: uuid.NewString(),
		Reason: sessionv1.StuckReason_STUCK_REASON_UNSPECIFIED,
		Until:  timestamppb.New(time.Now().Add(time.Hour)),
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// TestResetStuckRemediation_should_clearCountersAndSurfaceInList_When_RowIsOpen
// verifies the RPC handler resets remediation_attempts/next_remediation_at on
// the matching open row and the reset is visible via ListStuckBacklogItems.
func TestResetStuckRemediation_should_clearCountersAndSurfaceInList_When_RowIsOpen(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	ctx := t.Context()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "bouncing item",
		Status: string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)

	seedOpenStuckRow(t, storage, item.ID, domain.StuckReasonBouncing, time.Now(), "bouncing x3")
	applied, err := storage.RecordRemediationAttempt(ctx, item.ID, domain.StuckReasonBouncing, session.MaxRemediationAttempts, nil)
	require.NoError(t, err)
	require.True(t, applied)

	resp, err := svc.ResetStuckRemediation(ctx, connect.NewRequest(&sessionv1.ResetStuckRemediationRequest{
		ItemId: item.ID,
		Reason: sessionv1.StuckReason_STUCK_REASON_BOUNCING,
	}))
	require.NoError(t, err)
	assert.True(t, resp.Msg.Applied)

	list, err := svc.ListStuckBacklogItems(ctx, connect.NewRequest(&sessionv1.ListStuckBacklogItemsRequest{}))
	require.NoError(t, err)
	require.Len(t, list.Msg.Items, 1)
	assert.Equal(t, int32(0), list.Msg.Items[0].RemediationAttempts)
	assert.Nil(t, list.Msg.Items[0].NextRemediationAt)
}

// TestResetStuckRemediation_should_rejectInvalidArguments_When_ReasonOrItemMissing
// mirrors SnoozeStuckItem's input validation test.
func TestResetStuckRemediation_should_rejectInvalidArguments_When_ReasonOrItemMissing(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	ctx := t.Context()

	_, err := svc.ResetStuckRemediation(ctx, connect.NewRequest(&sessionv1.ResetStuckRemediationRequest{
		Reason: sessionv1.StuckReason_STUCK_REASON_BOUNCING,
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

	_, err = svc.ResetStuckRemediation(ctx, connect.NewRequest(&sessionv1.ResetStuckRemediationRequest{
		ItemId: uuid.NewString(),
		Reason: sessionv1.StuckReason_STUCK_REASON_UNSPECIFIED,
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// TestBulkResetStuckRemediation_should_defaultToOnlyParked_When_FlagNotExplicitlySet
// verifies the RPC layer's only_parked default (true) when the caller omits
// only_parked_explicitly_set — a parked row is reset, a mid-backoff
// (not-yet-parked) row is left alone.
func TestBulkResetStuckRemediation_should_defaultToOnlyParked_When_FlagNotExplicitlySet(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	ctx := t.Context()

	parked, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "parked", Status: string(session.BacklogStatusInProgress)})
	require.NoError(t, err)
	seedOpenStuckRow(t, storage, parked.ID, domain.StuckReasonBouncing, time.Now(), "bouncing")
	_, err = storage.RecordRemediationAttempt(ctx, parked.ID, domain.StuckReasonBouncing, session.MaxRemediationAttempts, nil)
	require.NoError(t, err)

	midBackoff, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "mid-backoff", Status: string(session.BacklogStatusReview)})
	require.NoError(t, err)
	seedOpenStuckRow(t, storage, midBackoff.ID, domain.StuckReasonAbandonedReview, time.Now(), "abandoned")
	next := time.Now().Add(2 * time.Hour)
	_, err = storage.RecordRemediationAttempt(ctx, midBackoff.ID, domain.StuckReasonAbandonedReview, 2, &next)
	require.NoError(t, err)

	resp, err := svc.BulkResetStuckRemediation(ctx, connect.NewRequest(&sessionv1.BulkResetStuckRemediationRequest{}))
	require.NoError(t, err)
	assert.Equal(t, int32(1), resp.Msg.ResetCount)

	list, err := svc.ListStuckBacklogItems(ctx, connect.NewRequest(&sessionv1.ListStuckBacklogItemsRequest{}))
	require.NoError(t, err)
	require.Len(t, list.Msg.Items, 2)
	for _, item := range list.Msg.Items {
		if item.ItemId == parked.ID {
			assert.Equal(t, int32(0), item.RemediationAttempts)
		} else {
			assert.Equal(t, int32(2), item.RemediationAttempts)
		}
	}
}

// TestBulkResetStuckRemediation_should_resetEveryOpenRow_When_OnlyParkedExplicitlyFalse
// verifies only_parked_explicitly_set=true with only_parked=false performs a
// full reset regardless of attempt count.
func TestBulkResetStuckRemediation_should_resetEveryOpenRow_When_OnlyParkedExplicitlyFalse(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	ctx := t.Context()

	midBackoff, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "mid-backoff", Status: string(session.BacklogStatusReview)})
	require.NoError(t, err)
	seedOpenStuckRow(t, storage, midBackoff.ID, domain.StuckReasonAbandonedReview, time.Now(), "abandoned")
	next := time.Now().Add(2 * time.Hour)
	_, err = storage.RecordRemediationAttempt(ctx, midBackoff.ID, domain.StuckReasonAbandonedReview, 2, &next)
	require.NoError(t, err)

	resp, err := svc.BulkResetStuckRemediation(ctx, connect.NewRequest(&sessionv1.BulkResetStuckRemediationRequest{
		OnlyParked:              false,
		OnlyParkedExplicitlySet: true,
	}))
	require.NoError(t, err)
	assert.Equal(t, int32(1), resp.Msg.ResetCount)
}

// TestBulkResetStuckRemediation_should_onlyResetMatchingReason_When_ReasonFilterSet
// verifies a reason-scoped sweep (AC 2 of the reason-scoped-sweep feature)
// resets only parked rows matching the requested reason, leaving a different
// reason's parked row untouched — the whole point of scoping the "recover
// items parked by a now-fixed bucket" sweep to just that bucket rather than
// resetting every parked row regardless of cause.
func TestBulkResetStuckRemediation_should_onlyResetMatchingReason_When_ReasonFilterSet(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	ctx := t.Context()

	bouncing, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "bouncing", Status: string(session.BacklogStatusReview)})
	require.NoError(t, err)
	seedOpenStuckRow(t, storage, bouncing.ID, domain.StuckReasonBouncing, time.Now(), "bouncing")
	_, err = storage.RecordRemediationAttempt(ctx, bouncing.ID, domain.StuckReasonBouncing, session.MaxRemediationAttempts, nil)
	require.NoError(t, err)

	staleWork, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "stale-work", Status: string(session.BacklogStatusInProgress)})
	require.NoError(t, err)
	seedOpenStuckRow(t, storage, staleWork.ID, domain.StuckReasonStaleWork, time.Now(), "stale")
	_, err = storage.RecordRemediationAttempt(ctx, staleWork.ID, domain.StuckReasonStaleWork, session.MaxRemediationAttempts, nil)
	require.NoError(t, err)

	resp, err := svc.BulkResetStuckRemediation(ctx, connect.NewRequest(&sessionv1.BulkResetStuckRemediationRequest{
		Reason: sessionv1.StuckReason_STUCK_REASON_BOUNCING,
	}))
	require.NoError(t, err)
	assert.Equal(t, int32(1), resp.Msg.ResetCount, "only the bouncing row should be reset")

	list, err := svc.ListStuckBacklogItems(ctx, connect.NewRequest(&sessionv1.ListStuckBacklogItemsRequest{}))
	require.NoError(t, err)
	require.Len(t, list.Msg.Items, 2)
	for _, item := range list.Msg.Items {
		switch item.ItemId {
		case bouncing.ID:
			assert.Equal(t, int32(0), item.RemediationAttempts, "reason-matched row must be reset")
		case staleWork.ID:
			assert.Equal(t, int32(session.MaxRemediationAttempts), item.RemediationAttempts, "different-reason row must be untouched")
		default:
			t.Fatalf("unexpected item %s", item.ItemId)
		}
	}
}

// TestBulkResetStuckRemediation_should_publishNotification_When_RowsReset
// verifies AC 3 of the reason-scoped-sweep feature: a bulk reset that
// actually changes rows fires a visible operator notification naming the
// reason and the reset count, mirroring the existing justParked one-time-notify
// pattern (session/backlog_lifecycle.go's notify) instead of mutating
// silently — the exact "notify-once-then-silent" gap this feature exists to
// close on the *reset* side.
func TestBulkResetStuckRemediation_should_publishNotification_When_RowsReset(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	bus := events.NewEventBus(4)
	svc.SetEventBus(bus)
	ctx := t.Context()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "bouncing", Status: string(session.BacklogStatusReview)})
	require.NoError(t, err)
	seedOpenStuckRow(t, storage, item.ID, domain.StuckReasonBouncing, time.Now(), "bouncing")
	_, err = storage.RecordRemediationAttempt(ctx, item.ID, domain.StuckReasonBouncing, session.MaxRemediationAttempts, nil)
	require.NoError(t, err)

	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch, _ := bus.Subscribe(subCtx)

	resp, err := svc.BulkResetStuckRemediation(ctx, connect.NewRequest(&sessionv1.BulkResetStuckRemediationRequest{
		Reason: sessionv1.StuckReason_STUCK_REASON_BOUNCING,
	}))
	require.NoError(t, err)
	require.Equal(t, int32(1), resp.Msg.ResetCount)

	select {
	case ev := <-ch:
		assert.Equal(t, events.EventNotification, ev.Type)
		assert.Contains(t, ev.NotificationMessage, string(domain.StuckReasonBouncing))
		assert.Contains(t, ev.NotificationMessage, "1 parked item")
		assert.Equal(t, string(domain.StuckReasonBouncing), ev.NotificationMetadata["reason"])
		assert.Equal(t, "1", ev.NotificationMetadata["reset_count"])
		assert.NotEmpty(t, ev.SessionID, "sessionID must not be empty — see notifyBulkResetParked's doc comment for the coalescing-collision bug this avoids")
		assert.Equal(t, ev.SessionID, ev.NotificationMetadata["item_id"], "item_id must mirror sessionID so eventToRecord doesn't fall back SessionName to the raw synthetic key")
	case <-time.After(2 * time.Second):
		t.Fatal("expected a notification event after a non-empty bulk reset")
	}
}

// TestBulkResetStuckRemediation_should_useDistinctSessionIDsPerReason_When_NotifyingDifferentScopes
// covers the notification-history-collision bug an earlier draft of
// notifyBulkResetParked reintroduced (caught in review): NotificationHistoryStore
// coalesces unread notifications by (sessionID, notificationType) with no time
// bound (server/notifications/store.go's findUnreadDuplicate), and
// EventBusNotifier.Notify's doc comment (server/services/backlog_notifier.go)
// documents the exact prior incident this class of bug caused — two different
// items'/scopes' same-type notifications sharing sessionID="" silently
// clobbered each other in the persisted history. This test proves
// notifyBulkResetParked's fix: two different-reason resets, and a reason-scoped
// reset vs. the global (unscoped) reset, always produce distinct SessionIDs, so
// they can never collapse into the same coalescing bucket
// (TestCoalescing_DifferentBacklogItemsSurviveWithinWindow in
// server/notifications/subscriber_test.go covers the general
// distinct-sessionID-survives-coalescing mechanism this relies on).
func TestBulkResetStuckRemediation_should_useDistinctSessionIDsPerReason_When_NotifyingDifferentScopes(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	bus := events.NewEventBus(8)
	svc.SetEventBus(bus)
	ctx := t.Context()

	bouncing, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "bouncing", Status: string(session.BacklogStatusReview)})
	require.NoError(t, err)
	seedOpenStuckRow(t, storage, bouncing.ID, domain.StuckReasonBouncing, time.Now(), "bouncing")
	_, err = storage.RecordRemediationAttempt(ctx, bouncing.ID, domain.StuckReasonBouncing, session.MaxRemediationAttempts, nil)
	require.NoError(t, err)

	staleWork, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "stale-work", Status: string(session.BacklogStatusInProgress)})
	require.NoError(t, err)
	seedOpenStuckRow(t, storage, staleWork.ID, domain.StuckReasonStaleWork, time.Now(), "stale")
	_, err = storage.RecordRemediationAttempt(ctx, staleWork.ID, domain.StuckReasonStaleWork, session.MaxRemediationAttempts, nil)
	require.NoError(t, err)

	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch, _ := bus.Subscribe(subCtx)

	_, err = svc.BulkResetStuckRemediation(ctx, connect.NewRequest(&sessionv1.BulkResetStuckRemediationRequest{
		Reason: sessionv1.StuckReason_STUCK_REASON_BOUNCING,
	}))
	require.NoError(t, err)
	_, err = svc.BulkResetStuckRemediation(ctx, connect.NewRequest(&sessionv1.BulkResetStuckRemediationRequest{
		Reason: sessionv1.StuckReason_STUCK_REASON_STALE_WORK,
	}))
	require.NoError(t, err)

	var sessionIDs []string
	for i := 0; i < 2; i++ {
		select {
		case ev := <-ch:
			sessionIDs = append(sessionIDs, ev.SessionID)
		case <-time.After(2 * time.Second):
			t.Fatalf("expected 2 notification events, got %d", i)
		}
	}
	require.Len(t, sessionIDs, 2)
	assert.NotEqual(t, sessionIDs[0], sessionIDs[1], "two different-reason resets must not share a coalescing sessionID")
}

// TestBulkResetStuckRemediation_should_notPublishNotification_When_NothingReset
// verifies the notification only fires when the sweep actually changed
// something — a no-op bulk reset (nothing parked) must not still surface a
// "reset 0 items" notification.
func TestBulkResetStuckRemediation_should_notPublishNotification_When_NothingReset(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	bus := events.NewEventBus(4)
	svc.SetEventBus(bus)
	ctx := t.Context()

	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch, _ := bus.Subscribe(subCtx)

	resp, err := svc.BulkResetStuckRemediation(ctx, connect.NewRequest(&sessionv1.BulkResetStuckRemediationRequest{}))
	require.NoError(t, err)
	require.Equal(t, int32(0), resp.Msg.ResetCount)

	select {
	case ev := <-ch:
		t.Fatalf("expected no notification event for a no-op reset, got %+v", ev)
	case <-time.After(200 * time.Millisecond):
		// expected: nothing published
	}
}

// TestBulkResetStuckRemediation_should_resolveToLastWriterDeterministically_When_RacingAConcurrentColdRetryWrite
// covers AC 5 of the reason-scoped-sweep feature: a reason-scoped bulk reset
// racing a concurrent cold-retry heartbeat write (BUG-083, PR #572) on the
// SAME row. This test intentionally does NOT use goroutines: the ent SQLite
// connection is opened with SetMaxOpenConns(1) (session/ent_repository.go),
// so database/sql fully serializes both writers' Exec calls regardless of
// goroutine scheduling — a goroutine-based version of this test cannot
// exercise real interleaving and would pass identically whether the
// underlying logic is correct or not (caught in review of an earlier draft).
// Instead this proves the actual claim directly: both writers issue a
// single-column literal UPDATE (never read-modify-write), so whichever call
// runs LAST always wins outright — exercised here in both orders — and the
// row is never torn/partial. This is the documented last-write-wins outcome
// (research/pitfalls.md §3), not silent corruption.
func TestBulkResetStuckRemediation_should_resolveToLastWriterDeterministically_When_RacingAConcurrentColdRetryWrite(t *testing.T) {
	t.Parallel()

	t.Run("cold-retry re-park runs last: re-park wins", func(t *testing.T) {
		t.Parallel()
		storage := createTestStorage(t)
		ctx := t.Context()
		reason := domain.StuckReasonBouncing

		item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "racing item", Status: string(session.BacklogStatusReview)})
		require.NoError(t, err)
		seedOpenStuckRow(t, storage, item.ID, reason, time.Now(), "bouncing")
		_, err = storage.RecordRemediationAttempt(ctx, item.ID, reason, session.MaxRemediationAttempts, nil)
		require.NoError(t, err)

		_, err = storage.BulkResetStuckRemediation(ctx, &reason, true)
		require.NoError(t, err)
		// Simulates a cold-retry heartbeat tick landing right after the sweep.
		_, err = storage.RecordRemediationAttempt(ctx, item.ID, reason, session.MaxRemediationAttempts, nil)
		require.NoError(t, err)

		rows, err := storage.FindOpenStuckStates(ctx)
		require.NoError(t, err)
		require.Len(t, rows, 1, "row must still exist exactly once — no torn/duplicate write")
		assert.Equal(t, int32(session.MaxRemediationAttempts), rows[0].RemediationAttempts,
			"the write that ran last (re-park) must win outright, not be partially applied")
	})

	t.Run("bulk reset runs last: reset wins", func(t *testing.T) {
		t.Parallel()
		storage := createTestStorage(t)
		ctx := t.Context()
		reason := domain.StuckReasonBouncing

		item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "racing item", Status: string(session.BacklogStatusReview)})
		require.NoError(t, err)
		seedOpenStuckRow(t, storage, item.ID, reason, time.Now(), "bouncing")
		_, err = storage.RecordRemediationAttempt(ctx, item.ID, reason, session.MaxRemediationAttempts, nil)
		require.NoError(t, err)

		// Simulates a cold-retry heartbeat tick landing right before the sweep.
		_, err = storage.RecordRemediationAttempt(ctx, item.ID, reason, session.MaxRemediationAttempts, nil)
		require.NoError(t, err)
		_, err = storage.BulkResetStuckRemediation(ctx, &reason, true)
		require.NoError(t, err)

		rows, err := storage.FindOpenStuckStates(ctx)
		require.NoError(t, err)
		require.Len(t, rows, 1, "row must still exist exactly once — no torn/duplicate write")
		assert.Equal(t, int32(0), rows[0].RemediationAttempts,
			"the write that ran last (reset) must win outright, not be partially applied")
	})
}

// TestTriggerRemediationNow_should_reject_When_NoOpenStuckRow verifies the
// operator "Retry now" RPC fails clearly when there is nothing to remediate.
func TestTriggerRemediationNow_should_reject_When_NoOpenStuckRow(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	ctx := t.Context()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "not stuck", Status: string(session.BacklogStatusInProgress)})
	require.NoError(t, err)

	_, err = svc.TriggerRemediationNow(ctx, connect.NewRequest(&sessionv1.TriggerRemediationNowRequest{
		ItemId: item.ID,
		Reason: sessionv1.StuckReason_STUCK_REASON_BOUNCING,
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
}

// TestTriggerRemediationNow_should_reject_When_AlreadyParked verifies a
// parked row (remediation_attempts at cap) is not silently un-parked by a
// manual trigger — the operator must call ResetStuckRemediation first.
func TestTriggerRemediationNow_should_reject_When_AlreadyParked(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	ctx := t.Context()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "parked", Status: string(session.BacklogStatusInProgress)})
	require.NoError(t, err)
	seedOpenStuckRow(t, storage, item.ID, domain.StuckReasonBouncing, time.Now(), "bouncing")
	_, err = storage.RecordRemediationAttempt(ctx, item.ID, domain.StuckReasonBouncing, session.MaxRemediationAttempts, nil)
	require.NoError(t, err)

	_, err = svc.TriggerRemediationNow(ctx, connect.NewRequest(&sessionv1.TriggerRemediationNowRequest{
		ItemId: item.ID,
		Reason: sessionv1.StuckReason_STUCK_REASON_BOUNCING,
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))

	list, err := svc.ListStuckBacklogItems(ctx, connect.NewRequest(&sessionv1.ListStuckBacklogItemsRequest{}))
	require.NoError(t, err)
	require.Len(t, list.Msg.Items, 1)
	assert.Equal(t, session.MaxRemediationAttempts, list.Msg.Items[0].RemediationAttempts, "a rejected manual trigger must not change the stored attempt count")
}

// TestTriggerRemediationNow_should_reject_When_ReasonHasNoPhaseAAction verifies
// a Phase B reason (no wired remediation action yet) is rejected with
// Unimplemented rather than silently doing nothing.
func TestTriggerRemediationNow_should_reject_When_ReasonHasNoPhaseAAction(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	ctx := t.Context()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "push failed", Status: string(session.BacklogStatusReview)})
	require.NoError(t, err)
	seedOpenStuckRow(t, storage, item.ID, domain.StuckReasonPushFailed, time.Now(), "push rejected")

	_, err = svc.TriggerRemediationNow(ctx, connect.NewRequest(&sessionv1.TriggerRemediationNowRequest{
		ItemId: item.ID,
		Reason: sessionv1.StuckReason_STUCK_REASON_PUSH_FAILED,
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
}

// TestTriggerRemediationNow_should_succeedAndConsumeAnAttempt_When_ActionRuns
// verifies the happy path: the wrapped action runs (here, AutoReopenAfterFailedReview
// no-ops early because the item already has an active work session — a
// legitimate, error-free outcome) and the attempt is recorded exactly like a
// normal dispatcher-triggered one.
func TestTriggerRemediationNow_should_succeedAndConsumeAnAttempt_When_ActionRuns(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	ctx := t.Context()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "bouncing", Status: string(session.BacklogStatusReview)})
	require.NoError(t, err)
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "work-session-uuid",
		SessionRole: string(session.SessionRoleWork),
	})
	require.NoError(t, err)
	seedOpenStuckRow(t, storage, item.ID, domain.StuckReasonBouncing, time.Now(), "bouncing")

	resp, err := svc.TriggerRemediationNow(ctx, connect.NewRequest(&sessionv1.TriggerRemediationNowRequest{
		ItemId: item.ID,
		Reason: sessionv1.StuckReason_STUCK_REASON_BOUNCING,
	}))
	require.NoError(t, err)
	assert.True(t, resp.Msg.Triggered)

	list, err := svc.ListStuckBacklogItems(ctx, connect.NewRequest(&sessionv1.ListStuckBacklogItemsRequest{}))
	require.NoError(t, err)
	require.Len(t, list.Msg.Items, 1)
	assert.Equal(t, int32(1), list.Msg.Items[0].RemediationAttempts)
}

// reasonsWithoutAutomatedRemediation lists every domain.StuckReason that
// intentionally has no wired remediationActionByReason case — these require a
// human decision (e.g. plan_not_approved) or don't yet have a safe automated
// fix. A reason must be listed here explicitly, or wired to an action: it may
// not silently do neither.
var reasonsWithoutAutomatedRemediation = map[domain.StuckReason]bool{
	domain.StuckReasonPRReadyUnmerged: true,
	domain.StuckReasonReworkCap:       true,
	domain.StuckReasonPushFailed:      true,
	domain.StuckReasonSpawnFailed:     true,
	domain.StuckReasonPlanNotApproved: true,
	// StuckReasonReworkBlockedStale: deliberately notify + durably mark +
	// resolve-when-recovered only (plan.md Story 2.1.1) — no automated
	// remediation action, per requirements.md's explicit out-of-scope item C
	// (auto-escalation past a grace period that detaches bookkeeping and
	// proceeds without killing the still-running session needs its own
	// design, not built here).
	domain.StuckReasonReworkBlockedStale: true,
	// StuckReasonRespawnBlockedActive: deliberately notify + durably mark +
	// resolve-once-the-guard-passes only, mirroring StuckReasonReworkBlockedStale
	// above — a single reason spans three different triggering statuses/
	// functions (AutoRespawnAutonomousWork/in_progress, AutoReopenForPRFix/
	// pr_pending, AutoRespawnReview/review), so there is no single unambiguous
	// "retry now" action to wire, and re-invoking any of the three while the
	// blocking session is still active would just re-mark the same row —
	// exactly what the next reconcile tick already does for free.
	domain.StuckReasonRespawnBlockedActive: true,
	// StuckReasonLikelyFlaky: purely informational (plan.md option (c)) — a
	// behavioral hint that the review outcome may be non-deterministic, not a
	// condition with a "retry now" fix. There is nothing to remediate: the
	// item's normal reopen/park flow already proceeds unaffected by this row
	// (see notifyLikelyFlaky's doc comment in backlog_service_triage.go).
	domain.StuckReasonLikelyFlaky: true,
	// StuckReasonBlockedByDependency: purely informational — there is no
	// "retry now" action because dequeue eligibility is derived automatically
	// from the blocker's status (DequeueNextQueuedItems re-checks it on every
	// pass; see criterion 3 in project_plans/backlog-item-dependencies). Once
	// the blocking item reaches its resolved state, this reason simply stops
	// firing on the next reconcile tick — nothing to trigger manually.
	domain.StuckReasonBlockedByDependency: true,
	// StuckReasonMultipleReasons and StuckReasonBounceCapExhausted
	// (backlog-bounce-escalation, Epic 1.1) are synthetic, aggregate signals
	// derived from other open stuck reasons/the remediation-attempt cap, not
	// independently actionable conditions — there is no "retry now" action
	// that makes sense for an aggregate row, so both are deliberately
	// notify + durably mark + resolve-when-the-underlying-condition-clears
	// only, same shape as StuckReasonReworkBlockedStale above.
	domain.StuckReasonMultipleReasons:    true,
	domain.StuckReasonBounceCapExhausted: true,
	// StuckReasonSteerFailed: same gap as StuckReasonRespawnBlockedActive
	// above — known pre-existing, not a regression (see plan.md's Epic 4.3
	// goal note and remediationActionByReason's doc comment). Wiring
	// automated remediation for a failed steer is out of scope for
	// pr-fix-steering.
	domain.StuckReasonSteerFailed: true,
}

// TestRemediationActionByReason_should_beDecidedForEveryStuckReason_When_NewReasonIsAdded
// is an exhaustiveness guard: every domain.AllStuckReasons entry must either be
// wired in remediationActionByReason or be explicitly listed in
// reasonsWithoutAutomatedRemediation. Without this test, adding a new
// StuckReason (a detector) with no corresponding remediation decision compiles
// fine and silently falls through remediationActionByReason's default case —
// exactly what happened with StuckReasonPRPendingNoPR: BUG-040 added the
// detector but TriggerRemediationNow returned Unimplemented for it until this
// gap was found in a live audit and closed. golangci-lint's `exhaustive`
// linter would catch this at compile-adjacent time, but this repo scopes
// `exhaustive` out of server/ entirely (see .golangci.yml) because most
// switches there use iota types with intentional defaults — this test is the
// narrowly-scoped substitute for this one specific reason/action contract.
func TestRemediationActionByReason_should_beDecidedForEveryStuckReason_When_NewReasonIsAdded(t *testing.T) {
	t.Parallel()
	svc := NewBacklogService(nil, nil, nil, nil, nil, nil)
	for _, reason := range domain.AllStuckReasons {
		wired := svc.remediationActionByReason(reason) != nil
		explicitlyUnwired := reasonsWithoutAutomatedRemediation[reason]
		assert.True(t, wired != explicitlyUnwired,
			"StuckReason %q must be either wired in remediationActionByReason or listed in reasonsWithoutAutomatedRemediation, not both or neither", reason)
	}
}
