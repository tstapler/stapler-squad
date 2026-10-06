package services

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgevents "github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
)

// This file covers Epic 7.2 (`MergeDatabase` Live-Leak Check) from
// project_plans/backlog-event-driven-updates/implementation/plan.md.
//
// Task 7.2.1a's trace (read mergeSessions in database_service.go, the actual
// bulk-copy implementation behind DatabaseService.MergeDatabase) found that
// the plan's two anticipated outcomes ("loops the 9 hooked repository
// methods" vs. "already-safe bulk-insert path") both undersold how safe this
// path actually is: mergeSessions's raw SQL only INSERT OR IGNOREs into
// main.sessions, main.worktrees, main.diff_stats, main.tags,
// main.claude_sessions, and main.session_tags — it never references
// backlog_items (or item_sessions / backlog_status_events /
// backlog_stuck_states / backlog_progress_notes) at all. Backlog items are
// simply outside MergeDatabase's data model entirely, so there is no code
// path by which a merge could invoke the 9 hooked EntRepository methods (the
// only place session.ItemChangePublisher.PublishItemChanged is ever called)
// — no bypass/suppression parameter is needed per Task 7.2.1b's "else" branch.
//
// The three tests below are the confirming tests validation.md maps to R16,
// adapted to assert what the code actually does rather than the assumed
// bypass-flag mechanism:
//   - notPublishLiveEvents: zero BacklogItemEvents fire during a merge, even
//     with a live publisher wired on the destination and backlog items
//     present in the source.
//   - stillPersistCopiedItems: the real (session-scoped) data the merge does
//     copy still lands correctly, unaffected by backlog items/publisher
//     wiring being present alongside it.
//   - makeCopiedItemsVisibleOnNextSnapshot: a fresh read of the destination's
//     backlog items after merge (simulating a client reconnect) sees only the
//     destination's own pre-existing items — none of the source workspace's
//     items leak in, duplicated or otherwise.

// newTempEntRepoForMergeTest creates a temporary ent-backed *session.EntRepository
// (full schema migration applied) and returns it alongside its bare file path
// (the same path mergeSessions expects) and a cleanup func. Mirrors
// session_test package's newTestEntRepositoryForEvents helper, reimplemented
// here because that helper lives in an external test package this file can't
// import from.
func newTempEntRepoForMergeTest(t *testing.T) (*session.EntRepository, string, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, fmt.Sprintf("merge-test-%d.db", time.Now().UnixNano()))

	repo, err := session.NewEntRepository(session.WithDatabasePath(dbPath))
	require.NoError(t, err)

	cleanup := func() {
		repo.Close()
		os.Remove(dbPath)
		os.Remove(dbPath + "-wal")
		os.Remove(dbPath + "-shm")
	}
	return repo, dbPath, cleanup
}

// seedBacklogItems creates n minimal backlog items in repo and returns their titles.
func seedBacklogItems(t *testing.T, repo *session.EntRepository, ctx context.Context, n int, titlePrefix string) []string {
	t.Helper()
	titles := make([]string, 0, n)
	for i := 0; i < n; i++ {
		title := fmt.Sprintf("%s-%d", titlePrefix, i)
		_, err := repo.CreateBacklogItem(ctx, session.BacklogItemData{
			Title:  title,
			Status: string(session.BacklogStatusIdea),
		})
		require.NoError(t, err)
		titles = append(titles, title)
	}
	return titles
}

// countRows returns COUNT(*) for the given table in the sqlite file at dbPath.
func countRows(t *testing.T, dbPath, table string) int {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer db.Close()

	var count int
	require.NoError(t, db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&count))
	return count
}

// TestMergeDatabase_should_notPublishLiveEvents_When_BulkCopyingBacklogItems
// is the R16 happy-path confirming test (Task 7.2.1a/b): wire the
// destination workspace with a live itemChangePublisher, seed the source
// workspace with 5 backlog items, run the merge, and assert zero
// BacklogItemEvents are observed. This also asserts the *why*: the
// destination's backlog_items count is unchanged post-merge, because
// mergeSessions never touches that table.
func TestMergeDatabase_should_notPublishLiveEvents_When_BulkCopyingBacklogItems(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	sourceRepo, sourcePath, sourceCleanup := newTempEntRepoForMergeTest(t)
	defer sourceCleanup()
	seedBacklogItems(t, sourceRepo, ctx, 5, "source-item")
	sourceRepo.Close()

	destRepo, destPath, destCleanup := newTempEntRepoForMergeTest(t)
	defer destCleanup()

	bus := pkgevents.NewEventBus(10)
	defer bus.Close()
	destRepo.SetItemChangePublisher(&BacklogItemEventPublisher{Bus: bus})

	sub, subID := bus.Subscribe(ctx)
	defer bus.Unsubscribe(subID)

	destItemsBefore := countRows(t, destPath, "backlog_items")

	// Close the destination repo's own ent connection before running the raw
	// merge SQL so mergeSessions's plain sql.Open connection is the only
	// writer against destPath (avoids ent's serialized single-conn pool
	// contending with the merge's own connection/ATTACH).
	destRepo.Close()

	imported, skipped, err := mergeSessions(ctx, destPath, sourcePath)
	require.NoError(t, err)
	assert.Equal(t, 0, imported, "no sessions exist in either workspace, so nothing should be imported")
	assert.Equal(t, 0, skipped)

	select {
	case ev := <-sub:
		t.Fatalf("expected zero BacklogItemEvents during MergeDatabase, got: %+v", ev)
	case <-time.After(150 * time.Millisecond):
		// Expected: no event arrives.
	}

	destItemsAfter := countRows(t, destPath, "backlog_items")
	assert.Equal(t, destItemsBefore, destItemsAfter, "MergeDatabase must not modify the destination's backlog_items table")
}

// TestMergeDatabase_should_stillPersistCopiedItems_When_EventPublishIsSuppressed
// is R16's error/regression-guard test (Task 7.2.1b). The plan anticipated
// this test would confirm a bypass flag doesn't also suppress the real data
// copy; Task 7.2.1a's trace found no bypass flag is needed (backlog items are
// never in scope), so this test instead confirms the adjacent property that
// matters: the merge's actual (session-scoped) data copy is unaffected by a
// live backlog-item publisher being wired on the destination, and backlog
// items present in both workspaces are left untouched by that copy.
func TestMergeDatabase_should_stillPersistCopiedItems_When_EventPublishIsSuppressed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	sourceRepo, sourcePath, sourceCleanup := newTempEntRepoForMergeTest(t)
	defer sourceCleanup()
	seedBacklogItems(t, sourceRepo, ctx, 5, "source-item")
	require.NoError(t, sourceRepo.Create(ctx, session.InstanceData{
		Title:     "merged-session-1",
		Path:      "/tmp/merge-test-session-1",
		Program:   "claude",
		Status:    session.Stopped,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}))
	sourceRepo.Close()

	destRepo, destPath, destCleanup := newTempEntRepoForMergeTest(t)
	defer destCleanup()

	bus := pkgevents.NewEventBus(10)
	defer bus.Close()
	destRepo.SetItemChangePublisher(&BacklogItemEventPublisher{Bus: bus})
	destItemsBefore := countRows(t, destPath, "backlog_items")
	destRepo.Close()

	imported, skipped, err := mergeSessions(ctx, destPath, sourcePath)
	require.NoError(t, err)
	assert.Equal(t, 1, imported, "the one session in the source workspace should be copied")
	assert.Equal(t, 0, skipped)

	sessionCount := countRows(t, destPath, "sessions")
	assert.Equal(t, 1, sessionCount, "the copied session must actually be persisted in the destination")

	destItemsAfter := countRows(t, destPath, "backlog_items")
	assert.Equal(t, destItemsBefore, destItemsAfter, "backlog items must remain untouched even though a real session was copied")
}

// TestMergeDatabase_should_makeCopiedItemsVisibleOnNextSnapshot_When_ClientReconnectsAfterMerge
// is R16's integration test (Task 7.2.1b). Reframed for the actual scope of
// MergeDatabase: since backlog items are never copied, a client re-listing
// backlog items against the destination workspace after a merge (simulating
// a WatchBacklogItems reconnect/fresh-snapshot) must see only the
// destination's own pre-existing items — none of the source workspace's
// items appear, confirming the merge doesn't leak or duplicate backlog data
// into the destination's backlog view.
func TestMergeDatabase_should_makeCopiedItemsVisibleOnNextSnapshot_When_ClientReconnectsAfterMerge(t *testing.T) {
	ctx := context.Background()

	sourceRepo, sourcePath, sourceCleanup := newTempEntRepoForMergeTest(t)
	defer sourceCleanup()
	sourceTitles := seedBacklogItems(t, sourceRepo, ctx, 5, "source-item")
	require.NoError(t, sourceRepo.Create(ctx, session.InstanceData{
		Title:     "merged-session-2",
		Path:      "/tmp/merge-test-session-2",
		Program:   "claude",
		Status:    session.Stopped,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}))
	sourceRepo.Close()

	destRepo, destPath, destCleanup := newTempEntRepoForMergeTest(t)
	defer destCleanup()
	destTitles := seedBacklogItems(t, destRepo, ctx, 2, "dest-own-item")
	destRepo.Close()

	imported, _, err := mergeSessions(ctx, destPath, sourcePath)
	require.NoError(t, err)
	assert.Equal(t, 1, imported)

	// Simulate a client reconnecting after the merge: open a fresh repository
	// handle against the destination and list backlog items, as
	// WatchBacklogItems's initial snapshot would.
	reconnected, err := session.NewEntRepository(session.WithDatabasePath(destPath))
	require.NoError(t, err)
	defer reconnected.Close()

	items, err := reconnected.ListBacklogItems(ctx, session.BacklogItemFilter{})
	require.NoError(t, err)

	gotTitles := make(map[string]bool, len(items))
	for _, item := range items {
		gotTitles[item.Title] = true
	}

	assert.Len(t, items, len(destTitles), "only the destination's own backlog items should be visible after merge")
	for _, title := range destTitles {
		assert.True(t, gotTitles[title], "destination's own item %q must still be visible", title)
	}
	for _, title := range sourceTitles {
		assert.False(t, gotTitles[title], "source workspace item %q must not leak into the destination", title)
	}
}
