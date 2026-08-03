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
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestToProtoStuckReason_should_mapToUnspecified_When_UnknownString verifies
// the proto reason mapping never panics on an unrecognized DB string and maps
// it to STUCK_REASON_UNSPECIFIED, while all six known domain.StuckReason
// constants map to their matching proto enum value.
func TestToProtoStuckReason_should_mapToUnspecified_When_UnknownString(t *testing.T) {
	t.Run("unknown reason maps to unspecified", func(t *testing.T) {
		got := toProtoStuckReason(domain.StuckReason("banana"))
		assert.Equal(t, sessionv1.StuckReason_STUCK_REASON_UNSPECIFIED, got)
	})

	t.Run("empty reason maps to unspecified", func(t *testing.T) {
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
	}
	for _, c := range cases {
		t.Run(string(c.reason), func(t *testing.T) {
			assert.Equal(t, c.want, toProtoStuckReason(c.reason))
			// Round-trip: the inverse mapping must recover the same domain reason.
			assert.Equal(t, c.reason, fromProtoStuckReason(c.want))
		})
	}
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

// TestSnoozeStuckItem_should_setSnoozedUntilAndOmitFromList_When_Called
// verifies SnoozeStuckItem sets snoozed_until on the matching open row and
// that the next ListStuckBacklogItems call omits it.
func TestSnoozeStuckItem_should_setSnoozedUntilAndOmitFromList_When_Called(t *testing.T) {
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

// TestTriggerRemediationNow_should_reject_When_NoOpenStuckRow verifies the
// operator "Retry now" RPC fails clearly when there is nothing to remediate.
func TestTriggerRemediationNow_should_reject_When_NoOpenStuckRow(t *testing.T) {
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
	svc := NewBacklogService(nil, nil, nil, nil, nil, nil)
	for _, reason := range domain.AllStuckReasons {
		wired := svc.remediationActionByReason(reason) != nil
		explicitlyUnwired := reasonsWithoutAutomatedRemediation[reason]
		assert.True(t, wired != explicitlyUnwired,
			"StuckReason %q must be either wired in remediationActionByReason or listed in reasonsWithoutAutomatedRemediation, not both or neither", reason)
	}
}
