package backlogsvc

import (
	"context"
	"fmt"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

func associatePR(t *testing.T, svc *BacklogService, itemID string, number int32) (*sessionv1.UpdateBacklogItemResponse, error) {
	t.Helper()
	url := fmt.Sprintf("https://github.com/tstapler/stapler-squad/pull/%d", number)
	resp, err := svc.UpdateBacklogItem(context.Background(), connect.NewRequest(&sessionv1.UpdateBacklogItemRequest{
		ItemId: itemID, PrUrl: &url, PrNumber: &number}))
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

func TestUpdateBacklogItem_ShouldReportEachPRAssociationOutcome_WhenTrackingByURL(t *testing.T) {
	t.Parallel()
	storage := svctest.CreateTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	ctx := context.Background()
	create := func(status session.BacklogStatus) string {
		item, err := storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "pr wire", Status: string(status)})
		require.NoError(t, err)
		return item.ID
	}

	t.Run("review links and moves to pr_pending", func(t *testing.T) {
		t.Parallel()
		got, err := associatePR(t, svc, create(session.BacklogStatusReview), 320)
		require.NoError(t, err)
		assert.Equal(t, sessionv1.PrAssociationResult_PR_ASSOCIATION_RESULT_LINKED_AND_MOVED, got.PrAssociation)
		assert.Equal(t, string(session.BacklogStatusPRPending), got.Item.Status)
		assert.EqualValues(t, 320, got.Item.PrNumber)
	})

	t.Run("pr_pending on the same PR is already tracked and writes nothing", func(t *testing.T) {
		t.Parallel()
		id := create(session.BacklogStatusReview)
		_, err := associatePR(t, svc, id, 321)
		require.NoError(t, err)
		got, err := associatePR(t, svc, id, 321)
		require.NoError(t, err)
		assert.Equal(t, sessionv1.PrAssociationResult_PR_ASSOCIATION_RESULT_ALREADY_TRACKED, got.PrAssociation)
		assert.Equal(t, string(session.BacklogStatusPRPending), got.Item.Status)
	})

	t.Run("pr_pending on a different PR is refused, not reassigned", func(t *testing.T) {
		t.Parallel()
		id := create(session.BacklogStatusReview)
		_, err := associatePR(t, svc, id, 322)
		require.NoError(t, err)
		_, err = associatePR(t, svc, id, 323)
		assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
		final, gerr := storage.GetBacklogItem(ctx, id)
		require.NoError(t, gerr)
		assert.Equal(t, 322, final.PrNumber, "the tracked PR is untouched")
	})

	t.Run("an item outside review/pr_pending is refused and unchanged", func(t *testing.T) {
		t.Parallel()
		id := create(session.BacklogStatusInProgress)
		_, err := associatePR(t, svc, id, 324)
		assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
		final, gerr := storage.GetBacklogItem(ctx, id)
		require.NoError(t, gerr)
		assert.Equal(t, string(session.BacklogStatusInProgress), final.Status)
		assert.Zero(t, final.PrNumber, "no PR fields were written")
	})

	t.Run("a call without PR fields reports unspecified", func(t *testing.T) {
		t.Parallel()
		id := create(session.BacklogStatusReview)
		resp, err := svc.UpdateBacklogItem(ctx, connect.NewRequest(&sessionv1.UpdateBacklogItemRequest{ItemId: id, Title: "renamed"}))
		require.NoError(t, err)
		assert.Equal(t, sessionv1.PrAssociationResult_PR_ASSOCIATION_RESULT_UNSPECIFIED, resp.Msg.PrAssociation)
	})
}

func seedItemPRs(t *testing.T, storage *session.Storage, itemID string, merged, open int) {
	t.Helper()
	ctx := context.Background()
	n := 100
	attach := func(state domain.PRState) {
		n++
		ref, err := domain.NewPRRef("", "acme", "app", n)
		require.NoError(t, err)
		_, err = storage.AttachItemPR(ctx, itemID, ref, session.PRLink{Role: domain.PRRolePrimary, Source: domain.AttachSourceManual,
			Initial: session.PRInitial{State: state, Terminal: state.Terminal()}})
		require.NoError(t, err)
	}
	for range merged {
		attach(domain.PRStateMerged)
	}
	for range open {
		attach(domain.PRStateOpen)
	}
}

func TestListStuckBacklogItems_ShouldCountMergedPRsOfTheItemSet_WhenMultiPRBacklogScopeOn(t *testing.T) {
	t.Parallel()
	storage := svctest.CreateTestStorage(t)
	ctx := context.Background()
	item, err := storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "stuck multi", Status: string(session.BacklogStatusPRPending)})
	require.NoError(t, err)
	seedItemPRs(t, storage, item.ID, 2, 1)
	applied, err := storage.MarkStuck(ctx, item.ID, domain.StuckReasonAutonomousStuck, session.BacklogStatusPRPending, "ctx")
	require.NoError(t, err)
	require.True(t, applied)

	list := func(scopeOn bool) *sessionv1.StuckBacklogItem {
		svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
		svc.cleanup.liveEnabled = func() bool { return scopeOn }
		resp, lerr := svc.ListStuckBacklogItems(ctx, connect.NewRequest(&sessionv1.ListStuckBacklogItemsRequest{}))
		require.NoError(t, lerr)
		require.Len(t, resp.Msg.Items, 1)
		return resp.Msg.Items[0]
	}

	on := list(true)
	assert.EqualValues(t, 3, on.PrCount)
	assert.EqualValues(t, 2, on.PrMergedCount)

	off := list(false)
	assert.Zero(t, off.PrCount, "scope off: counts are not computed")
	assert.Zero(t, off.PrMergedCount)
}

func TestGetBacklogItem_ShouldFlagItemDirectPRs_WhenSetHoldsItemAndSessionLinks(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	ctx := context.Background()
	item, err := fix.storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "mixed", Status: string(session.BacklogStatusPRPending)})
	require.NoError(t, err)
	seedItemPRs(t, fix.storage, item.ID, 0, 1) // #101, item-direct

	svc := NewBacklogService(fix.storage, nil, nil, nil, nil, nil)
	svc.cleanup.liveEnabled = func() bool { return true }
	resp, err := svc.GetBacklogItem(ctx, connect.NewRequest(&sessionv1.GetBacklogItemRequest{ItemId: item.ID}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Item.PullRequests, 1)
	assert.True(t, resp.Msg.Item.PullRequests[0].ItemDirect)
}
