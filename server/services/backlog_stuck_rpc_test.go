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
