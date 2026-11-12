package session

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

func itemInStatus(t *testing.T, f *itemPRFixture, status BacklogStatus) *backlogdto.BacklogItemData {
	t.Helper()
	_, err := f.repo.Client.BacklogItem.UpdateOneID(f.itemEnt().ID).SetStatus(string(status)).Save(context.Background())
	require.NoError(t, err)
	item, err := f.storage.GetBacklogItem(context.Background(), f.itemID)
	require.NoError(t, err)
	return item
}

func prAssoc(n int, src AttachSource) PRAssociation {
	return PRAssociation{URL: "https://github.com/acme/app/pull/" + strconv.Itoa(n), Number: n, Summary: "pr", Source: src}
}

func itemLinkNumbers(t *testing.T, f *itemPRFixture) []int {
	t.Helper()
	views, err := f.repo.ListByItem(context.Background(), f.itemID)
	require.NoError(t, err)
	out := make([]int, 0, len(views))
	for _, v := range views {
		out = append(out, v.Ref.Number())
	}
	return out
}

// Item-direct link writes are unconditional: with every scope off a first report still
// leaves the link and the state row for the refresher, the gate and the sweeper.
func TestSetPRAndTransition_ShouldWriteItemDirectLinkWithPRFields_WhenFirstReportScopesOff(t *testing.T) {
	t.Parallel()
	f := newItemPRFixture(t, 0)
	item := itemInStatus(t, f, BacklogStatusReview)

	require.NoError(t, f.storage.setPRAndTransition(context.Background(), item, prAssoc(22, AttachSourceReportPRCreated)))

	got, err := f.storage.GetBacklogItem(context.Background(), f.itemID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusPRPending), got.Status)
	assert.Equal(t, 22, got.PrNumber)
	assert.Equal(t, []int{22}, itemLinkNumbers(t, f))
	views, _ := f.repo.ListByItem(context.Background(), f.itemID)
	assert.Equal(t, AttachSourceReportPRCreated, views[0].AttachSource)
	assert.Equal(t, PRStateOpen, views[0].State, "the refresher polls it from here")
}

func TestSetPRAndTransition_ShouldDropReplacedItemLinkWithoutTombstone_WhenCorrectionPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newItemPRFixture(t, 21)
	f.link(ItemPROwner(f.itemID), 21, PRStateOpen)
	item := itemInStatus(t, f, BacklogStatusPRPending)
	pr := prAssoc(22, AttachSourceReportPRCreated)
	pr.Guard = &backlogdto.PRReassignmentGuard{OverrideReason: "polluted branch", NewPRAuthorVerified: true}

	require.NoError(t, f.storage.setPRAndTransition(ctx, item, pr))

	assert.Equal(t, []int{22}, itemLinkNumbers(t, f), "the replaced #21 link is gone, #22 is linked")
	tombstoned, err := f.repo.IsTombstoned(ctx, ItemPROwner(f.itemID), testRef(21))
	require.NoError(t, err)
	assert.False(t, tombstoned, "a correction is not a detach")
}

func TestSetPRAndTransition_ShouldAttachFollowupAndKeepPending_WhenAdditive(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newItemPRFixture(t, 21)
	f.link(ItemPROwner(f.itemID), 21, PRStateMerged)
	item := itemInStatus(t, f, BacklogStatusPRPending)
	pr := prAssoc(22, AttachSourceReportPRCreated)
	pr.Additive = true

	require.NoError(t, f.storage.setPRAndTransition(ctx, item, pr), "no override_reason and no guard")

	got, err := f.storage.GetBacklogItem(ctx, f.itemID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusPRPending), got.Status, "status unchanged")
	assert.Equal(t, 22, got.PrNumber, "the mirror follows the derived active PR (open #22 outranks merged #21)")
	assert.ElementsMatch(t, []int{21, 22}, itemLinkNumbers(t, f))
	assert.Equal(t, ItemVerdictOpen, f.verdict())
	views, _ := f.repo.ListByItem(ctx, f.itemID)
	for _, v := range views {
		if v.Ref.Number() == 22 {
			assert.Equal(t, PRRoleFollowup, v.Role)
		}
	}
}

func TestSetPRAndTransition_ShouldRefuseAdditive_WhenOperatorDetachedThatPR(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newItemPRFixture(t, 21)
	f.link(ItemPROwner(f.itemID), 22, PRStateOpen)
	f.detach(ItemPROwner(f.itemID), 22)
	item := itemInStatus(t, f, BacklogStatusPRPending)
	pr := prAssoc(22, AttachSourceReportPRCreated)
	pr.Additive = true

	err := f.storage.setPRAndTransition(ctx, item, pr)

	require.ErrorIs(t, err, backlogdto.ErrPRDetached)
	assert.Empty(t, itemLinkNumbers(t, f))
}

func TestSetPRAndTransition_ShouldKeepLegacyReassignRules_WhenNotAdditive(t *testing.T) {
	t.Parallel()
	f := newItemPRFixture(t, 21)
	item := itemInStatus(t, f, BacklogStatusPRPending)
	err := f.storage.setPRAndTransition(context.Background(), item, prAssoc(22, AttachSourceReportPRCreated))
	require.ErrorIs(t, err, backlogdto.ErrPRReassignmentNotAllowed, "flag off: a different PR on pr_pending still needs the guard")
}

func TestSetItemPRFieldsWithLink_ShouldWriteFieldsAndLinkAtomically_AndRollBackBoth(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newItemPRFixture(t, 0)
	f.repo.afterPRLinkHook = func() error { return errors.New("forced link failure") }

	err := f.storage.SetItemPRFieldsWithLink(ctx, f.itemID, "https://github.com/acme/app/pull/30", 30, AttachSourcePRCreationService)
	require.Error(t, err)
	got, _ := f.storage.GetBacklogItem(ctx, f.itemID)
	assert.Zero(t, got.PrNumber, "the fields were rolled back with the link")
	assert.Empty(t, itemLinkNumbers(t, f))

	f.repo.afterPRLinkHook = nil
	require.NoError(t, f.storage.SetItemPRFieldsWithLink(ctx, f.itemID, "https://github.com/acme/app/pull/30", 30, AttachSourcePRCreationService))
	got, _ = f.storage.GetBacklogItem(ctx, f.itemID)
	assert.Equal(t, 30, got.PrNumber)
	assert.Equal(t, []int{30}, itemLinkNumbers(t, f))
}

// BUG-058 narrowed: a forced failure of the link write leaves neither pr_number nor
// the link, and the item stays in review; a CAS loss leaves no link either.
func TestPushAndCreatePR_ShouldWriteFieldsAndLinkAtomically_AndKeepReviewOnFailure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newItemPRFixture(t, 0)
	item := itemInStatus(t, f, BacklogStatusReview)
	f.repo.afterPRLinkHook = func() error { return errors.New("forced link failure") }

	err := f.storage.setPRAndTransition(ctx, item, prAssoc(22, AttachSourcePRCreationService))
	require.Error(t, err)

	got, gerr := f.storage.GetBacklogItem(ctx, f.itemID)
	require.NoError(t, gerr)
	assert.Equal(t, string(BacklogStatusReview), got.Status, "the item stays in review")
	assert.Zero(t, got.PrNumber, "pr_number was rolled back with the link")
	assert.Empty(t, itemLinkNumbers(t, f))

	f.repo.afterPRLinkHook = nil
	stale := *item
	stale.UpdatedAt = stale.UpdatedAt.Add(-1)
	require.Error(t, f.storage.setPRAndTransition(ctx, &stale, prAssoc(23, AttachSourcePRCreationService)), "a lost CAS writes nothing")
	assert.Empty(t, itemLinkNumbers(t, f))
}
