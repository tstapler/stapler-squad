package session

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent"
)

type itemPRFixture struct {
	t       *testing.T
	storage *Storage
	repo    *EntRepository
	itemID  string
}

func newItemPRFixture(t *testing.T, legacyNumber int) *itemPRFixture {
	t.Helper()
	storage, cleanup := createTestStorage(t)
	t.Cleanup(cleanup)
	item, err := storage.CreateBacklogItem(context.Background(), backlogdto.BacklogItemData{
		Title: "item prs", AcceptanceCriteria: `[]`, Priority: 1,
		Status: string(domain.BacklogStatusPRPending), RepoPath: "/tmp/fake-repo",
	})
	require.NoError(t, err)
	f := &itemPRFixture{t: t, storage: storage, repo: storage.Repo, itemID: item.ID}
	if legacyNumber > 0 {
		url := "https://github.com/acme/app/pull/" + strconv.Itoa(legacyNumber)
		_, err = storage.UpdateBacklogItem(context.Background(), item.ID, backlogdto.BacklogItemUpdate{PrURL: &url, PrNumber: &legacyNumber}, nil)
		require.NoError(t, err)
	}
	return f
}

func testRef(n int) domain.PRRef {
	ref, err := domain.NewPRRef("github.com", "acme", "app", n)
	if err != nil {
		panic(err)
	}
	return ref
}

func (f *itemPRFixture) session(role string) string {
	f.t.Helper()
	id := uuid.New().String()
	_, err := f.storage.CreateItemSession(context.Background(), backlogdto.ItemSessionData{ItemID: f.itemID, SessionUUID: id, SessionRole: role})
	require.NoError(f.t, err)
	return id
}

func (f *itemPRFixture) link(owner PROwner, n int, state domain.PRState) {
	f.t.Helper()
	res, err := f.repo.UpsertSessionPR(context.Background(), owner, testRef(n), domain.PRLink{
		Role: domain.PRRolePrimary, Source: domain.AttachSourceManual, Initial: domain.PRInitial{State: state, Terminal: state.Terminal()},
	})
	require.NoError(f.t, err)
	require.False(f.t, res.Tombstoned)
}

func (f *itemPRFixture) detach(owner PROwner, n int) {
	f.t.Helper()
	_, err := f.repo.DeleteSessionPR(context.Background(), owner, testRef(n))
	require.NoError(f.t, err)
}

func (f *itemPRFixture) itemEnt() *ent.BacklogItem {
	f.t.Helper()
	id, err := uuid.Parse(f.itemID)
	require.NoError(f.t, err)
	it, err := f.repo.Client.BacklogItem.Get(context.Background(), id)
	require.NoError(f.t, err)
	return it
}

func (f *itemPRFixture) numbers() []int {
	f.t.Helper()
	prs, err := f.repo.ItemPRs(context.Background(), f.itemEnt())
	require.NoError(f.t, err)
	out := make([]int, 0, len(prs))
	for _, v := range prs {
		out = append(out, v.Ref.Number())
	}
	return out
}

func (f *itemPRFixture) verdict() ItemVerdict {
	f.t.Helper()
	prs, err := f.repo.ItemPRs(context.Background(), f.itemEnt())
	require.NoError(f.t, err)
	return ItemPRVerdict(prs)
}

func TestItemPRs_ShouldSynthesizeUnknownPR_WhenLegacyNumberUnrepresented(t *testing.T) {
	t.Parallel()
	f := newItemPRFixture(t, 21)
	prs, err := f.repo.ItemPRs(context.Background(), f.itemEnt())
	require.NoError(t, err)
	require.Len(t, prs, 1)
	assert.Equal(t, 21, prs[0].Ref.Number())
	assert.Equal(t, domain.PRStateUnknown, prs[0].State)
	assert.False(t, prs[0].Terminal())
	assert.Equal(t, ItemVerdictOpen, ItemPRVerdict(prs), "an unmigrated item is never read as all terminal")
}

func TestItemPRs_ShouldNotSynthesize_WhenNumberRepresentedOnlyBySupersededRound(t *testing.T) {
	t.Parallel()
	f := newItemPRFixture(t, 30)
	s1 := f.session(SessionRoleWork)
	f.link(SessionPROwner(s1), 30, domain.PRStateOpen)
	s2 := f.session(SessionRoleWork)
	f.link(SessionPROwner(s2), 31, domain.PRStateOpen)
	assert.Equal(t, []int{31}, f.numbers(), "stale mirror value is not unmigrated")
}

func TestItemPRs_ShouldNotSynthesize_WhenAnyTombstoneCoversNumber(t *testing.T) {
	t.Parallel()
	t.Run("session tombstone", func(t *testing.T) {
		f := newItemPRFixture(t, 30)
		s := f.session(SessionRoleWork)
		f.link(SessionPROwner(s), 30, domain.PRStateOpen)
		f.detach(SessionPROwner(s), 30)
		assert.Empty(t, f.numbers())
	})
	t.Run("item tombstone", func(t *testing.T) {
		f := newItemPRFixture(t, 30)
		f.detach(ItemPROwner(f.itemID), 30)
		assert.Empty(t, f.numbers())
	})
}

// countingSource counts ListBySessionUUIDs calls: the round query is one, the
// all-rounds fallback query would be a second.
type countingSource struct {
	itemPRSource
	sessionLinkCalls int
}

func (c *countingSource) ListBySessionUUIDs(ctx context.Context, uuids []string) (map[string][]domain.PRView, error) {
	c.sessionLinkCalls++
	return c.itemPRSource.ListBySessionUUIDs(ctx, uuids)
}

func TestItemPRs_ShouldShortCircuitFallback_WhenNumberAlreadyInSet(t *testing.T) {
	t.Parallel()
	f := newItemPRFixture(t, 21)
	s := f.session(SessionRoleWork)
	f.link(SessionPROwner(s), 21, domain.PRStateOpen)
	src := &countingSource{itemPRSource: f.repo}
	it := f.itemEnt()
	prs, err := deriveItemPRs(context.Background(), src, f.itemID, it.PrURL, it.PrNumber)
	require.NoError(t, err)
	require.Len(t, prs, 1)
	assert.Equal(t, 1, src.sessionLinkCalls, "no all-rounds link query when pr_number is already in the set")
}

func TestItemPRs_ShouldReturnOnePR_WhenSameStateViaItemAndSessionLink(t *testing.T) {
	t.Parallel()
	f := newItemPRFixture(t, 0)
	s := f.session(SessionRoleWork)
	f.link(ItemPROwner(f.itemID), 21, domain.PRStateOpen)
	f.link(SessionPROwner(s), 21, domain.PRStateOpen)
	assert.Equal(t, []int{21}, f.numbers())
}

func TestItemPRs_ShouldDropPR_WhenOnlySessionLinkDetachedOrSessionDeleted(t *testing.T) {
	t.Parallel()
	f := newItemPRFixture(t, 0)
	s := f.session(SessionRoleWork)
	f.link(SessionPROwner(s), 30, domain.PRStateOpen)
	require.Equal(t, []int{30}, f.numbers())
	f.detach(SessionPROwner(s), 30)
	assert.Empty(t, f.numbers())
}

func TestItemPRs_ShouldUseLatestWorkRoundOnly_WhenReopened(t *testing.T) {
	t.Parallel()
	f := newItemPRFixture(t, 0)
	s1 := f.session(SessionRoleWork)
	f.link(SessionPROwner(s1), 30, domain.PRStateOpen)
	s2 := f.session(SessionRoleWork)
	f.link(SessionPROwner(s2), 31, domain.PRStateMerged)
	assert.Equal(t, []int{31}, f.numbers())
	assert.Equal(t, ItemVerdictDone, f.verdict(), "verdict is Done regardless of #30")
	byS1, err := f.repo.ListBySessionUUIDs(context.Background(), []string{s1})
	require.NoError(t, err)
	assert.Len(t, byS1[s1], 1, "#30 stays linked to its own session")
}

func TestItemPRs_ShouldNotSupersede_WhenLaterReviewSession(t *testing.T) {
	t.Parallel()
	f := newItemPRFixture(t, 0)
	w1 := f.session(SessionRoleWork)
	f.link(SessionPROwner(w1), 30, domain.PRStateOpen)
	f.session(SessionRoleReview)
	assert.Equal(t, []int{30}, f.numbers())
}

func TestCurrentRoundSession_ShouldBreakCreatedAtTiesByLargerID(t *testing.T) {
	t.Parallel()
	now := time.Now()
	work := []backlogdto.ItemSessionSummary{
		{ID: "a", SessionUUID: "ua", CreatedAt: now},
		{ID: "b", SessionUUID: "ub", CreatedAt: now},
	}
	links := map[string][]domain.PRView{"ua": {{}}, "ub": {{}}}
	got, skipped := currentRoundSession(work, links, nil)
	assert.Equal(t, "ub", got)
	assert.False(t, skipped)
}

func TestItemPRs_ShouldNotResurrectEarlierRound_WhenLatestRoundLinkDetached(t *testing.T) {
	t.Parallel()
	f := newItemPRFixture(t, 0)
	s1 := f.session(SessionRoleWork)
	f.link(SessionPROwner(s1), 30, domain.PRStateMerged)
	s2 := f.session(SessionRoleWork)
	f.link(SessionPROwner(s2), 31, domain.PRStateOpen)
	f.detach(SessionPROwner(s2), 31)
	assert.Empty(t, f.numbers())
	assert.Equal(t, ItemVerdictNoPRs, f.verdict())
}

func TestItemPRs_ShouldKeepLivePR_WhenNewerWorkSessionHasNoLinkYet(t *testing.T) {
	t.Parallel()
	f := newItemPRFixture(t, 0)
	s1 := f.session(SessionRoleWork)
	f.link(SessionPROwner(s1), 30, domain.PRStateOpen)
	s2 := f.session(SessionRoleWork)
	assert.Equal(t, []int{30}, f.numbers())
	assert.Equal(t, ItemVerdictOpen, f.verdict())
	f.link(SessionPROwner(s2), 31, domain.PRStateOpen)
	assert.Equal(t, []int{31}, f.numbers(), "#30 leaves once the new session links")
}

func TestItemPRs_ShouldKeepItemDirectPR_WhenRound2Merges(t *testing.T) {
	t.Parallel()
	f := newItemPRFixture(t, 0)
	f.link(ItemPROwner(f.itemID), 30, domain.PRStateOpen)
	s2 := f.session(SessionRoleWork)
	f.link(SessionPROwner(s2), 31, domain.PRStateMerged)
	assert.ElementsMatch(t, []int{30, 31}, f.numbers())
	assert.Equal(t, ItemVerdictOpen, f.verdict())
	f.detach(ItemPROwner(f.itemID), 30)
	assert.Equal(t, ItemVerdictDone, f.verdict())
}

func TestItemPRs_ShouldKeepSharedPR_WhenOneOfTwoRoundsDetaches(t *testing.T) {
	t.Parallel()
	f := newItemPRFixture(t, 0)
	s1 := f.session(SessionRoleWork)
	f.link(SessionPROwner(s1), 30, domain.PRStateOpen)
	s2 := f.session(SessionRoleWork)
	f.link(SessionPROwner(s2), 30, domain.PRStateOpen)
	f.detach(SessionPROwner(s1), 30)
	assert.Equal(t, []int{30}, f.numbers())
	assert.Equal(t, ItemVerdictOpen, f.verdict())

	f.link(ItemPROwner(f.itemID), 30, domain.PRStateOpen)
	f.detach(SessionPROwner(s2), 30)
	assert.Equal(t, []int{30}, f.numbers(), "item-direct link still holds it")

	f.detach(ItemPROwner(f.itemID), 30)
	assert.Empty(t, f.numbers(), "item tombstone removes it")
}

func TestItemPRs_ShouldIgnoreItemTombstoneOverSessionLink_WhenBothHoldIt(t *testing.T) {
	t.Parallel()
	f := newItemPRFixture(t, 0)
	s := f.session(SessionRoleWork)
	f.link(ItemPROwner(f.itemID), 30, domain.PRStateUnknown)
	f.link(SessionPROwner(s), 30, domain.PRStateUnknown)
	f.detach(ItemPROwner(f.itemID), 30)
	assert.Empty(t, f.numbers())
	links, err := f.repo.ListBySessionUUIDs(context.Background(), []string{s})
	require.NoError(t, err)
	assert.Len(t, links[s], 1, "the session link remains")
}

func TestItemPRs_ShouldIgnoreUnrelatedSession_WhenManualAttachToOtherSession(t *testing.T) {
	t.Parallel()
	f := newItemPRFixture(t, 0)
	f.link(ItemPROwner(f.itemID), 21, domain.PRStateOpen)
	f.link(SessionPROwner(uuid.New().String()), 50, domain.PRStateOpen)
	assert.Equal(t, []int{21}, f.numbers())
}

func TestItemPRs_ShouldStillReturnItemDirect_WhenSessionLinkBroken(t *testing.T) {
	t.Parallel()
	f := newItemPRFixture(t, 0)
	f.link(ItemPROwner(f.itemID), 21, domain.PRStateOpen)
	assert.Equal(t, []int{21}, f.numbers(), "no ItemSession rows at all")
}

func TestRoundMembership_ShouldCreateNoItemDirectLink_WhenBackfillRunsOnSessionDerivedPR(t *testing.T) {
	t.Parallel()
	f := newItemPRFixture(t, 30)
	s := f.session(SessionRoleWork)
	f.link(SessionPROwner(s), 30, domain.PRStateOpen)
	for run := 0; run < 2; run++ {
		_, err := f.repo.runPRBackfill(context.Background(), nil)
		require.NoError(t, err)
		direct, err := f.repo.ListByItem(context.Background(), f.itemID)
		require.NoError(t, err)
		assert.Empty(t, direct, "run %d: a session-derived PR gets no item-direct link", run)
	}
	f.link(SessionPROwner(f.session(SessionRoleWork)), 31, domain.PRStateOpen)
	assert.Equal(t, []int{31}, f.numbers(), "#30 leaves the set when the next round links #31")
}

func (f *itemPRFixture) mirror() int {
	f.t.Helper()
	return f.itemEnt().PrNumber
}

func TestItemMirror_ShouldFollowSet_WhenSetChanges(t *testing.T) {
	t.Parallel()
	f := newItemPRFixture(t, 21)
	ctx := context.Background()
	f.link(ItemPROwner(f.itemID), 21, domain.PRStateMerged)
	f.link(ItemPROwner(f.itemID), 22, domain.PRStateOpen)

	changed, err := f.repo.RecomputeItemPRMirror(ctx, f.itemID)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, 22, f.mirror())
	assert.Equal(t, "https://github.com/acme/app/pull/22", f.itemEnt().PrURL, "url synthesized when the state row has none")

	pending, err := f.repo.FindPRPendingItems(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 1, "FindPRPendingItems (PrNumberGT(0)) still returns the item")

	changed, err = f.repo.RecomputeItemPRMirror(ctx, f.itemID)
	require.NoError(t, err)
	assert.False(t, changed, "idempotent when already in sync")
}

func TestItemMirror_ShouldNeverZeroStoredNumber_WhenSetEmpty(t *testing.T) {
	t.Parallel()
	f := newItemPRFixture(t, 30)
	ctx := context.Background()
	s := f.session(SessionRoleWork)
	f.link(SessionPROwner(s), 30, domain.PRStateOpen)
	f.detach(SessionPROwner(s), 30)
	require.Empty(t, f.numbers())

	changed, err := f.repo.RecomputeItemPRMirror(ctx, f.itemID)
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Equal(t, 30, f.mirror(), "an empty set leaves the stored number untouched")
}

func TestItemMirror_ShouldLoseToConcurrentWriter_WhenStoredNumberChangedUnderneath(t *testing.T) {
	t.Parallel()
	f := newItemPRFixture(t, 21)
	ctx := context.Background()
	f.link(ItemPROwner(f.itemID), 22, domain.PRStateOpen)
	stale := f.itemEnt()
	n := 99
	_, err := f.storage.UpdateBacklogItem(ctx, f.itemID, backlogdto.BacklogItemUpdate{PrNumber: &n}, nil)
	require.NoError(t, err)

	changed, err := f.repo.recomputeItemPRMirror(ctx, stale)
	require.NoError(t, err)
	assert.False(t, changed, "compare-and-set on the number read; the other writer wins")
	assert.Equal(t, 99, f.mirror())
}

func TestItemMirror_ShouldFollowCurrentRound_WhenLaterWorkSessionLinks(t *testing.T) {
	t.Parallel()
	f := newItemPRFixture(t, 30)
	ctx := context.Background()
	s1 := f.session(SessionRoleWork)
	f.link(SessionPROwner(s1), 30, domain.PRStateOpen)
	for run := 0; run < 2; run++ {
		_, err := f.repo.runPRBackfill(ctx, nil)
		require.NoError(t, err)
	}
	f.link(SessionPROwner(f.session(SessionRoleWork)), 31, domain.PRStateOpen)
	_, err := f.repo.RecomputeItemPRMirror(ctx, f.itemID)
	require.NoError(t, err)
	assert.Equal(t, 31, f.mirror())
}

func TestAttachItemPRLink_ShouldLinkOnceAndHonorItemTombstone(t *testing.T) {
	t.Parallel()
	f := newItemPRFixture(t, 0)
	ctx := context.Background()
	url31 := "https://github.com/acme/app/pull/31"
	url32 := "https://github.com/acme/app/pull/32"

	for i := 0; i < 2; i++ { // second pass is a re-poll
		for _, u := range []string{url31, url32} {
			ok, err := f.storage.AttachItemPRLink(ctx, f.itemID, u, domain.AttachSourceJules)
			require.NoError(t, err)
			require.True(t, ok)
		}
	}
	views, err := f.repo.ListByItem(ctx, f.itemID)
	require.NoError(t, err)
	require.Len(t, views, 2)
	for _, v := range views {
		assert.Equal(t, domain.AttachSourceJules, v.AttachSource)
	}
	roles := []domain.PRRole{views[0].Role, views[1].Role}
	assert.ElementsMatch(t, []domain.PRRole{domain.PRRolePrimary, domain.PRRoleFollowup}, roles)

	f.detach(ItemPROwner(f.itemID), 32)
	ok, err := f.storage.AttachItemPRLink(ctx, f.itemID, url32, domain.AttachSourceJules)
	require.NoError(t, err)
	assert.False(t, ok, "a re-poll cannot undo a detach")
	assert.Equal(t, []int{31}, f.numbers())
}
