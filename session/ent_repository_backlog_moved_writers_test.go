package session

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/backlogstore"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/instance"
)

type publishedChange struct {
	item   backlogdto.BacklogItemData
	change backlogdto.BacklogItemChange
}

// captureChanges records every publish; the returned func snapshots them.
func captureChanges(repo *EntRepository) func() []publishedChange {
	var mu sync.Mutex
	var got []publishedChange
	repo.SetItemChangePublisher(hookPublisher{func(i *backlogdto.BacklogItemData, c backlogdto.BacklogItemChange) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, publishedChange{item: *i, change: c})
	}})
	return func() []publishedChange {
		mu.Lock()
		defer mu.Unlock()
		return append([]publishedChange(nil), got...)
	}
}

func newMovedWriterItem(t *testing.T, repo *EntRepository) (itemID, sessionID string) {
	t.Helper()
	ctx := context.Background()
	item, err := repo.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "moved writer item", Status: string(domain.BacklogStatusIdea)})
	require.NoError(t, err)
	is, err := repo.CreateItemSession(ctx, backlogdto.ItemSessionData{ItemID: item.ID, SessionUUID: "moved-" + item.ID, SessionRole: instance.SessionRoleTriage})
	require.NoError(t, err)
	return item.ID, is.ID
}

func TestUpdateBacklogItem_should_PersistAndPublishEachField_When_ItIsTheOnlyOneSet(t *testing.T) {
	t.Parallel()
	one, three, seven := 1, 3, 7
	cases := []struct {
		field  string
		update backlogdto.BacklogItemUpdate
		check  func(t *testing.T, got *backlogdto.BacklogItemData)
	}{
		{"labels", backlogdto.BacklogItemUpdate{Labels: &[]string{"a", "b"}}, func(t *testing.T, g *backlogdto.BacklogItemData) { assert.Equal(t, []string{"a", "b"}, g.Labels) }},
		{"externalUrl", backlogdto.BacklogItemUpdate{ExternalURL: ptrTo("https://x.test/1")}, func(t *testing.T, g *backlogdto.BacklogItemData) { assert.Equal(t, "https://x.test/1", g.ExternalURL) }},
		{"prUrl", backlogdto.BacklogItemUpdate{PrURL: ptrTo("https://github.com/o/r/pull/5")}, func(t *testing.T, g *backlogdto.BacklogItemData) {
			assert.Equal(t, "https://github.com/o/r/pull/5", g.PrURL)
		}},
		{"prNumber", backlogdto.BacklogItemUpdate{PrNumber: &seven}, func(t *testing.T, g *backlogdto.BacklogItemData) { assert.Equal(t, 7, g.PrNumber) }},
		{"shippedApprovedCount", backlogdto.BacklogItemUpdate{ShippedApprovedCount: &three}, func(t *testing.T, g *backlogdto.BacklogItemData) { assert.Equal(t, 3, g.ShippedApprovedCount) }},
		{"reworkCapOverride", backlogdto.BacklogItemUpdate{ReworkCapOverride: &one}, func(t *testing.T, g *backlogdto.BacklogItemData) {
			require.NotNil(t, g.ReworkCapOverride)
			assert.Equal(t, 1, *g.ReworkCapOverride)
		}},
		{"triggeredByChainDepth", backlogdto.BacklogItemUpdate{TriggeredByChainDepth: &three}, func(t *testing.T, g *backlogdto.BacklogItemData) { assert.Equal(t, 3, g.TriggeredByChainDepth) }},
		{"chainFired", backlogdto.BacklogItemUpdate{ChainFired: ptrTo(true)}, func(t *testing.T, g *backlogdto.BacklogItemData) { assert.True(t, g.ChainFired) }},
		{"priority", backlogdto.BacklogItemUpdate{Priority: &three}, func(t *testing.T, g *backlogdto.BacklogItemData) { assert.Equal(t, 3, g.Priority) }},
		{"title", backlogdto.BacklogItemUpdate{Title: ptrTo("renamed")}, func(t *testing.T, g *backlogdto.BacklogItemData) { assert.Equal(t, "renamed", g.Title) }},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			t.Parallel()
			repo := NewTestEntRepository(t)
			ctx := context.Background()
			item, err := repo.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "t", Status: string(domain.BacklogStatusIdea)})
			require.NoError(t, err)
			published := captureChanges(repo)

			_, err = repo.UpdateBacklogItem(ctx, item.ID, tc.update, nil)
			require.NoError(t, err)

			got, err := repo.GetBacklogItem(ctx, item.ID)
			require.NoError(t, err)
			tc.check(t, got)
			require.Len(t, published(), 1)
			assert.Equal(t, backlogdto.ChangeItemUpdated, published()[0].change.Kind)
			assert.Equal(t, []string{tc.field}, published()[0].change.UpdatedFields)
		})
	}
}

func ptrTo[T any](v T) *T { return &v }

// The publisher sees the committed row: reading through the single-connection pool from inside
// the callback would block (or see the old title) if the publish ran before the commit.
func TestUpdateBacklogItem_should_PublishAfterCommit(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	item, err := repo.CreateBacklogItem(context.Background(), backlogdto.BacklogItemData{Title: "old", Status: string(domain.BacklogStatusIdea)})
	require.NoError(t, err)

	var seen string
	repo.SetItemChangePublisher(hookPublisher{func(*backlogdto.BacklogItemData, backlogdto.BacklogItemChange) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if got, gerr := repo.GetBacklogItem(ctx, item.ID); gerr == nil {
			seen = got.Title
		}
	}})
	_, err = repo.UpdateBacklogItem(context.Background(), item.ID, backlogdto.BacklogItemUpdate{Title: ptrTo("new")}, nil)
	require.NoError(t, err)
	assert.Equal(t, "new", seen)
}

func TestDeleteBacklogItem_should_CascadeVerdictsAndSessionsAndPublishRemovedSnapshot(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	itemID, sessionID := newMovedWriterItem(t, repo)
	require.NoError(t, repo.SaveReviewVerdict(ctx, sessionID, backlogdto.ReviewVerdictData{OverallOutcome: domain.ReviewOutcome("pass"), Summary: "ok"}))
	published := captureChanges(repo)

	require.NoError(t, repo.DeleteBacklogItem(ctx, itemID))

	assert.Zero(t, repo.Client.ReviewVerdict.Query().CountX(ctx), "verdict rows must go with the item")
	assert.Zero(t, repo.Client.ItemSession.Query().CountX(ctx), "item sessions must go with the item")
	assert.Equal(t, 1, repo.Client.DeletedItemSessionCost.Query().CountX(ctx), "the cost ledger keeps one row per deleted session")
	require.Len(t, published(), 1)
	got := published()[0]
	assert.Equal(t, backlogdto.ChangeItemRemoved, got.change.Kind)
	assert.Equal(t, itemID, got.item.ID)
	assert.Len(t, got.item.ItemSessions, 1, "the snapshot is taken before the sessions are deleted")
}

func TestSaveReviewVerdict_should_UpdateInPlaceAndPublish_When_VerdictAlreadyExists(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	_, sessionID := newMovedWriterItem(t, repo)
	published := captureChanges(repo)

	require.NoError(t, repo.SaveReviewVerdict(ctx, sessionID, backlogdto.ReviewVerdictData{OverallOutcome: domain.ReviewOutcome("fail"), Summary: "first"}))
	require.NoError(t, repo.SaveReviewVerdict(ctx, sessionID, backlogdto.ReviewVerdictData{OverallOutcome: domain.ReviewOutcome("pass"), Summary: "second"}))

	rows := repo.Client.ReviewVerdict.Query().AllX(ctx)
	require.Len(t, rows, 1, "the second save must update, not insert")
	assert.Equal(t, "pass", rows[0].OverallOutcome)
	assert.Equal(t, "second", rows[0].Summary)
	assert.Len(t, published(), 2, "each save publishes once")
}

func TestUpdateItemSessionDiagnoseResult_should_PersistAndPublishDiagnoseResultField(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	itemID, sessionID := newMovedWriterItem(t, repo)
	published := captureChanges(repo)

	require.NoError(t, repo.UpdateItemSessionDiagnoseResult(ctx, sessionID, backlogdto.DiagnoseResultData{Outcome: "NOTE_ONLY", Summary: "s"}))

	sessions, err := repo.ListItemSessions(ctx, itemID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	d, ok := backlogstore.ParseDiagnoseResult(sessions[0].DiagnoseResult)
	require.True(t, ok)
	assert.Equal(t, "NOTE_ONLY", d.Outcome)
	require.Len(t, published(), 1)
	assert.Equal(t, []string{"diagnoseResult"}, published()[0].change.UpdatedFields)
	assert.Equal(t, itemID, published()[0].item.ID)
}

func TestTouchItemSessionProgress_should_SetLastProgressAtAndPublishItemSessions(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	itemID, sessionID := newMovedWriterItem(t, repo)
	published := captureChanges(repo)
	at := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)

	require.NoError(t, repo.TouchItemSessionProgress(ctx, sessionID, at))

	sessions, err := repo.ListItemSessions(ctx, itemID)
	require.NoError(t, err)
	require.NotNil(t, sessions[0].LastProgressAt)
	assert.True(t, sessions[0].LastProgressAt.Equal(at))
	require.Len(t, published(), 1)
	assert.Equal(t, []string{"itemSessions"}, published()[0].change.UpdatedFields)
}

func TestWorkflowSuggestions_should_PersistAndPublishWorkflowSuggestionsField(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	itemID, _ := newMovedWriterItem(t, repo)
	published := captureChanges(repo)

	sugID, err := repo.AppendWorkflowSuggestion(ctx, itemID, backlogdto.WorkflowSuggestionData{Recommendation: "split it"})
	require.NoError(t, err)
	require.NoError(t, repo.DismissWorkflowSuggestion(ctx, itemID, sugID))

	got, err := repo.GetBacklogItem(ctx, itemID)
	require.NoError(t, err)
	list := backlogstore.ParseWorkflowSuggestions(got.WorkflowSuggestions)
	require.Len(t, list, 1)
	assert.True(t, list[0].Dismissed)
	require.Len(t, published(), 2)
	for _, p := range published() {
		assert.Equal(t, []string{"workflowSuggestions"}, p.change.UpdatedFields)
	}
}

func TestWaiveAcCriterion_should_PublishItemUpdated_When_CriterionIsWaived(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	ac, err := domain.SerializeAcCriteria([]domain.AcCriterion{{Index: 0, Text: "needs proof", Status: domain.AcStatusPending}})
	require.NoError(t, err)
	item, err := repo.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "w", Status: string(domain.BacklogStatusInProgress), AcceptanceCriteria: ac})
	require.NoError(t, err)
	published := captureChanges(repo)

	require.NoError(t, repo.WaiveAcCriterion(ctx, item.ID, 0, "owner says so"))

	require.Len(t, published(), 1)
	assert.Equal(t, backlogdto.ChangeItemUpdated, published()[0].change.Kind)
	assert.Equal(t, item.ID, published()[0].item.ID)
}

func TestCreateItemSession_should_PublishItemSessionsUpdate_And_AttachSessionToPublishedItem(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	item, err := repo.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "p", Status: string(domain.BacklogStatusIdea)})
	require.NoError(t, err)
	published := captureChanges(repo)

	is, err := repo.CreateItemSession(ctx, backlogdto.ItemSessionData{ItemID: item.ID, SessionUUID: "pub-" + item.ID, SessionRole: instance.SessionRoleTriage})
	require.NoError(t, err)

	require.Len(t, published(), 1)
	got := published()[0]
	assert.Equal(t, item.ID, got.item.ID)
	require.Len(t, got.item.ItemSessions, 1, "the published item carries its sessions")
	assert.Equal(t, is.ID, got.item.ItemSessions[0].ID)
}

// Every constructor path binds the backlog store to its own guidance repo, so a flag stamped on a
// read or a publish reflects that repository's data and never a sibling's.
func TestBacklogStore_should_StampPendingGuidance_When_ConstructedByAnyPath(t *testing.T) {
	t.Parallel()
	seeded := NewTestEntRepository(t)
	for name, repo := range map[string]*EntRepository{
		"seeded":      seeded,
		"from-client": NewEntRepositoryFromClient(NewTestEntRepository(t).GetEntClient()),
	} {
		ctx := context.Background()
		itemID := createTestBacklogItemForGuidance(t, repo)
		_, err := repo.CreateGuidanceRequest(ctx, domain.CreateGuidanceRequestInput{
			Scope: domain.RequestScopeBacklogItem, ItemID: &itemID, QuestionText: "Merge?", QuestionType: domain.QuestionTypeYesNo,
		})
		require.NoError(t, err, name)

		got, err := repo.GetBacklogItem(ctx, itemID.String())
		require.NoError(t, err, name)
		assert.True(t, got.HasPendingGuidanceRequests, name)
		list, err := repo.ListBacklogItems(ctx, backlogdto.BacklogItemFilter{})
		require.NoError(t, err, name)
		require.Len(t, list, 1, name)
		assert.True(t, list[0].HasPendingGuidanceRequests, name)
		sums, err := repo.ListBacklogItemSummaries(ctx, backlogdto.BacklogItemFilter{})
		require.NoError(t, err, name)
		require.Len(t, sums, 1, name)
		assert.True(t, sums[0].HasPendingGuidanceRequests, name)
	}
}

// A repository over a counting client must query guidance through its own client: a port bound to
// the original repo's guidance store would leave the counted driver blind to the lookup.
func TestBacklogStore_should_QueryGuidanceThroughItsOwnClient_When_BuiltOverCountingClient(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	itemID := createTestBacklogItemForGuidance(t, repo)

	n := countStatementsMatching(t, repo, func(q string) bool { return strings.Contains(q, "guidance_requests") }, func(counted *EntRepository) {
		_, err := counted.GetBacklogItem(context.Background(), itemID.String())
		require.NoError(t, err)
	})
	assert.Positive(t, n, "the counted repo must run its own pending-guidance query")
}
