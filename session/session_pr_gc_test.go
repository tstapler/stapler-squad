package session

import (
	"context"
	"database/sql"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

// backdate rewrites an immutable timestamp column directly, since ent exposes no
// setter for created_at / attached_at; where is a SQL predicate over args.
func backdate(t *testing.T, repo *EntRepository, table, column, where string, to time.Time, args ...any) {
	t.Helper()
	var res sql.Result
	query := "UPDATE " + table + " SET " + column + " = ? WHERE " + where
	require.NoError(t, repo.driver.Exec(context.Background(), query, append([]any{to}, args...), &res))
}

func createLiveSession(t *testing.T, repo *EntRepository, title, uuid string) {
	t.Helper()
	data := createTestSession(title)
	data.UUID = uuid
	require.NoError(t, repo.Create(context.Background(), data))
}

func createLiveItem(t *testing.T, repo *EntRepository, title string) string {
	t.Helper()
	item, err := repo.CreateBacklogItem(context.Background(), backlogdto.BacklogItemData{Title: title})
	require.NoError(t, err)
	return item.ID
}

func TestDeleteSession_ShouldRemoveItsLinksAndTombstones_WhenSessionDeleted(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	ctx := context.Background()
	createLiveSession(t, repo, "gc-del-1", "uuid-del-1")
	createLiveSession(t, repo, "gc-del-2", "uuid-del-2")
	itemID := createLiveItem(t, repo, "gc-del-item")
	ref := mustPRRef(t, "", "acme", "app", 12)
	other := mustPRRef(t, "", "acme", "app", 13)

	upsertLink(t, repo, SessionPROwner("uuid-del-1"), ref, domain.AttachSourceManual)
	upsertLink(t, repo, SessionPROwner("uuid-del-2"), ref, domain.AttachSourceManual)
	upsertLink(t, repo, ItemPROwner(itemID), ref, domain.AttachSourceReportPRCreated)
	_, err := repo.DeleteSessionPR(ctx, SessionPROwner("uuid-del-1"), other)
	require.NoError(t, err)

	require.NoError(t, repo.Delete(ctx, "gc-del-1"))

	views, err := repo.ListBySessionUUIDs(ctx, []string{"uuid-del-1", "uuid-del-2"})
	require.NoError(t, err)
	assert.Empty(t, views["uuid-del-1"], "the deleted session keeps no link")
	assert.Len(t, views["uuid-del-2"], 1, "another session's link survives")
	itemViews, err := repo.ListByItem(ctx, itemID)
	require.NoError(t, err)
	assert.Len(t, itemViews, 1, "an item link survives a session delete")
	tombs, err := repo.ListSessionTombstonesByUUIDs(ctx, []string{"uuid-del-1"})
	require.NoError(t, err)
	assert.Empty(t, tombs, "the deleted session keeps no tombstone")
}

func TestDeleteBacklogItem_ShouldRemoveItsLinksAndTombstones_WhenItemDeleted(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	ctx := context.Background()
	createLiveSession(t, repo, "gc-item-sess", "uuid-item-sess")
	itemID := createLiveItem(t, repo, "gc-item-delete")
	ref := mustPRRef(t, "", "acme", "app", 12)
	upsertLink(t, repo, ItemPROwner(itemID), ref, domain.AttachSourceManual)
	upsertLink(t, repo, SessionPROwner("uuid-item-sess"), ref, domain.AttachSourceManual)
	_, err := repo.DeleteSessionPR(ctx, ItemPROwner(itemID), mustPRRef(t, "", "acme", "app", 14))
	require.NoError(t, err)

	require.NoError(t, repo.DeleteBacklogItem(ctx, itemID))

	itemViews, err := repo.ListByItem(ctx, itemID)
	require.NoError(t, err)
	assert.Empty(t, itemViews)
	tombs, err := repo.ListItemTombstones(ctx, []string{itemID})
	require.NoError(t, err)
	assert.Empty(t, tombs)
	views, err := repo.ListBySessionUUIDs(ctx, []string{"uuid-item-sess"})
	require.NoError(t, err)
	assert.Len(t, views["uuid-item-sess"], 1, "the session's own link survives an item delete")
}

func TestSessionPRLinkGC_ShouldCollectOnlyOrphansOlderThanGrace(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	ctx := context.Background()
	createLiveSession(t, repo, "gc-live", "uuid-live")
	liveItem := createLiveItem(t, repo, "gc-live-item")
	now := time.Now()
	old := now.Add(-48 * time.Hour)

	ref := func(n int) domain.PRRef { return mustPRRef(t, "", "acme", "app", n) }
	upsertLink(t, repo, SessionPROwner("uuid-live"), ref(1), domain.AttachSourceManual)       // owner exists
	upsertLink(t, repo, ItemPROwner(liveItem), ref(2), domain.AttachSourceManual)             // owner exists
	upsertLink(t, repo, SessionPROwner("uuid-gone-old"), ref(3), domain.AttachSourceBackfill) // orphan, old
	upsertLink(t, repo, SessionPROwner("uuid-gone-new"), ref(4), domain.AttachSourceBackfill) // orphan, young
	upsertLink(t, repo, ItemPROwner("11111111-1111-1111-1111-111111111111"), ref(5), domain.AttachSourceJules)
	_, err := repo.DeleteSessionPR(ctx, SessionPROwner("uuid-gone-old"), ref(6)) // orphan tombstone, old
	require.NoError(t, err)
	_, err = repo.DeleteSessionPR(ctx, SessionPROwner("uuid-gone-new"), ref(7)) // orphan tombstone, young
	require.NoError(t, err)
	_, err = repo.DeleteSessionPR(ctx, SessionPROwner("uuid-live"), ref(8)) // live-owner tombstone, old
	require.NoError(t, err)

	// Age everything except the "young" orphans.
	backdate(t, repo, "session_prs", "attached_at", "session_uuid <> ?", old, "uuid-gone-new")
	backdate(t, repo, "session_pr_tombstones", "created_at", "session_uuid <> ?", old, "uuid-gone-new")
	// State 4 belongs to the young orphan; keep its row young.
	backdate(t, repo, "pull_request_states", "created_at", "number <> ? AND ? = ?", old, 4, 1, 1)

	res, err := repo.RunSessionPRLinkGC(ctx, now)
	require.NoError(t, err)

	assert.Equal(t, 2, res.Links, "old orphan session link and old orphan item link")
	assert.Equal(t, 1, res.Tombstones, "only the old tombstone whose session is gone")
	assert.Equal(t, 2, res.States, "states 3 and 5 lose their last link and are old (tombstone-only refs have no state row)")
	assert.Equal(t, 3, countStates(t, repo), "states 1, 2 and 4 remain")
	assert.Equal(t, 3, countLinks(t, repo), "live session link, live item link, young orphan link")
	live, err := repo.ListBySessionUUIDs(ctx, []string{"uuid-live"})
	require.NoError(t, err)
	assert.Len(t, live["uuid-live"], 1)
	itemLinks, err := repo.ListByItem(ctx, liveItem)
	require.NoError(t, err)
	assert.Len(t, itemLinks, 1, "an item link whose item exists is never collected")
}

func TestSessionPRLinkGC_ShouldKeepStateRow_WhenEnsureStateAndLinkInterleaveWithGC(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	ctx := context.Background()
	now := time.Now()
	ref := mustPRRef(t, "", "acme", "app", 21)

	// An attach has run EnsureState (young row, no link yet) when the GC fires.
	_, err := repo.EnsureState(ctx, ref, PRInitial{})
	require.NoError(t, err)
	res, err := repo.RunSessionPRLinkGC(ctx, now)
	require.NoError(t, err)
	assert.Equal(t, 0, res.States, "a state younger than the grace is never collected")

	// The link insert that follows finds its state row intact.
	got := upsertLink(t, repo, SessionPROwner("uuid-attach"), ref, domain.AttachSourceManual)
	assert.True(t, got.Created)
	assert.Equal(t, 1, countStates(t, repo))

	// An OLD state row that gained a link in the meantime is not collected either.
	backdate(t, repo, "pull_request_states", "created_at", "? = ?", now.Add(-72*time.Hour), 1, 1)
	res, err = repo.RunSessionPRLinkGC(ctx, now)
	require.NoError(t, err)
	assert.Equal(t, 0, res.States, "a state with a link survives regardless of age")
}

func TestSessionPRLinkGC_ShouldCollectOnlyWhenFlagOn_AndStopOnCancel(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	upsertLink(t, repo, SessionPROwner("uuid-gone"), mustPRRef(t, "", "acme", "app", 1), domain.AttachSourceBackfill)
	backdate(t, repo, "session_prs", "attached_at", "session_uuid = ?", time.Now().Add(-48*time.Hour), "uuid-gone")

	var on atomic.Bool
	gc := NewSessionPRLinkGC(repo, time.Hour)
	gc.enabled = on.Load

	gc.runOnce(context.Background())
	assert.Equal(t, 1, countLinks(t, repo), "flag off: nothing collected")

	on.Store(true)
	ctx, cancel := context.WithCancel(context.Background())
	gc.Start(ctx)
	gc.Start(ctx) // second call is a no-op
	require.True(t, gc.Started())
	require.Eventually(t, func() bool { return countLinks(t, repo) == 0 }, 5*time.Second, 10*time.Millisecond)
	cancel()
	gc.Wait()
}
