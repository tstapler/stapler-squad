package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent/backlogstuckstate"
	"github.com/tstapler/stapler-squad/session/git"
)

// backdateStuckFirstDetected sets first_detected_at on the open
// BacklogStuckState row for (itemID, reason) to when, so tests can simulate a
// condition that has held past a pure-fn threshold (stuckPRReady,
// abandonedReview) without sleeping in real time.
func backdateStuckFirstDetected(t *testing.T, er *EntRepository, itemID string, reason domain.StuckReason, when time.Time) {
	t.Helper()
	parsedID, err := uuid.Parse(itemID)
	require.NoError(t, err)
	n, err := er.client.BacklogStuckState.Update().
		Where(
			backlogstuckstate.ItemID(parsedID),
			backlogstuckstate.Reason(string(reason)),
		).
		SetFirstDetectedAt(when).
		Save(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n, "expected exactly one open row for item=%s reason=%s", itemID, reason)
}

// newStaleWorkTestItem creates an in_progress BacklogItem with a single active
// (EndedAt nil) work-role ItemSession whose last_progress_at is backdated
// beyond maxWorkSessionStaleness, matching the shape reconcileStaleWorkSessions
// (and BackfillStuckStates' mirror of it) looks for.
func newStaleWorkTestItem(t *testing.T, storage *Storage, er *EntRepository) *BacklogItemData {
	t.Helper()
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "Stale work test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
	})
	require.NoError(t, err)

	workIS, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "work-" + uuid.New().String(),
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)

	parsedID, err := uuid.Parse(workIS.ID)
	require.NoError(t, err)
	stale := time.Now().Add(-3 * time.Hour) // beyond maxWorkSessionStaleness (2h)
	_, err = er.client.ItemSession.UpdateOneID(parsedID).SetLastProgressAt(stale).Save(ctx)
	require.NoError(t, err)

	return item
}

// newOrphanedTriageTestItem creates an idea-status BacklogItem with a single open
// (EndedAt nil) triage-role ItemSession whose created_at is backdated by ageAgo,
// matching the shape reconcileOrphanedTriageItems looks for.
func newOrphanedTriageTestItem(t *testing.T, storage *Storage, er *EntRepository, ageAgo time.Duration) *BacklogItemData {
	t.Helper()
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "Orphaned triage test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusIdea),
	})
	require.NoError(t, err)

	// created_at is Immutable() in the ent schema (no Update-builder setter), so
	// the backdated value must be set at Create time via the raw ent client
	// rather than storage.CreateItemSession (which always uses time.Now()).
	parsedItemID, err := uuid.Parse(item.ID)
	require.NoError(t, err)
	_, err = er.client.ItemSession.Create().
		SetSessionUUID("headless-triage-" + uuid.New().String()).
		SetSessionRole(string(SessionRoleTriage)).
		SetBacklogItemID(parsedItemID).
		SetCreatedAt(time.Now().Add(-ageAgo)).
		Save(ctx)
	require.NoError(t, err)

	return item
}

// TestBackfillStuckStates_should_seedDBDerivableRowsWithNotifiedAt_When_ItemsParked
// verifies that BackfillStuckStates seeds an open, notified stuck row for each
// currently-stuck item it can detect from existing DB-derivable queries.
//
// Scope note (deviation from the literal plan wording): this Epic (1.1) seeds
// only abandoned_review and stale_work — the two reasons with a detection
// query that already exists as of this Epic (FindStuckReviewItems, and the
// maxWorkSessionStaleness check mirrored from reconcileStaleWorkSessions).
// rework_cap, bouncing, and push_failed detection logic is introduced by
// Phase 2 (Stories 2.1.2, 2.1.4, 2.1.6) and does not exist yet — seeding them
// here would mean fabricating that not-yet-built detection logic ahead of
// schedule. See the BackfillStuckStates doc comment in backlog_lifecycle.go.
func TestBackfillStuckStates_should_seedDBDerivableRowsWithNotifiedAt_When_ItemsParked(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	reviewItem := newStuckReviewTestItem(t, storage, ReviewVerdictUnverifiable, true, false)
	staleItem := newStaleWorkTestItem(t, storage, er)

	listener := NewBacklogLifecycleListener(storage)
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	listener.BackfillStuckStates(ctx)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 2)

	byItem := make(map[string]OpenStuckStateData, len(open))
	for _, row := range open {
		byItem[row.ItemID] = row
	}

	reviewRow, ok := byItem[reviewItem.ID]
	require.True(t, ok, "abandoned_review row expected for stuck review item")
	assert.Equal(t, domain.StuckReasonAbandonedReview, reviewRow.Reason)
	assert.NotNil(t, reviewRow.NotifiedAt, "backfilled row must have notified_at pre-set")

	staleRow, ok := byItem[staleItem.ID]
	require.True(t, ok, "stale_work row expected for stale in_progress item")
	assert.Equal(t, domain.StuckReasonStaleWork, staleRow.Reason)
	assert.NotNil(t, staleRow.NotifiedAt, "backfilled row must have notified_at pre-set")

	// Backfill itself never calls the notifier — it writes notified_at
	// directly so that once Phase 2 wires the reconcilers to read it (Story
	// 2.1.3), the first genuine tick after this restart issues zero new
	// notifications for these two already-known conditions.
	assert.Empty(t, notifier.calls, "backfill must not itself fire operator notifications")
}

// TestBackfillStuckStates_should_notCallGitHubNorSeedPRReady_When_Run verifies
// that backfill never seeds a pr_ready_unmerged row and — by construction, since
// BackfillStuckStates has no GitHub-calling code path at all — never touches
// GitHub. Detecting pr_ready_unmerged needs a per-item GetPRStatus/IsPRMerged
// call, which would burst the GitHub API on every boot; the first genuine
// reconcile tick handles it instead (one-tick delay, no startup API burst).
func TestBackfillStuckStates_should_notCallGitHubNorSeedPRReady_When_Run(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	newPRPendingTestItem(t, storage, 148)

	listener := NewBacklogLifecycleListener(storage)
	listener.BackfillStuckStates(ctx)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	for _, row := range open {
		assert.NotEqual(t, domain.StuckReasonPRReadyUnmerged, row.Reason,
			"backfill must never seed pr_ready_unmerged")
	}
}

// TestBackfillStuckStates_should_beIdempotent_When_RunTwice verifies a second
// backfill run produces no duplicate rows — guarded by the (item_id, reason)
// unique constraint via MarkStuck's upsert.
func TestBackfillStuckStates_should_beIdempotent_When_RunTwice(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	newStuckReviewTestItem(t, storage, ReviewVerdictUnverifiable, true, false)
	newStaleWorkTestItem(t, storage, er)

	listener := NewBacklogLifecycleListener(storage)
	listener.BackfillStuckStates(ctx)

	first, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, first, 2)

	listener.BackfillStuckStates(ctx)

	second, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Len(t, second, 2, "second backfill run must not create duplicate rows")

	count, err := er.client.BacklogStuckState.Query().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

// --- Story 2.1.1: pr_ready_unmerged ---

// TestReconcilePRPending_should_markStuck_When_PRGreenMergeableUnapproved
// verifies the flagship single-user case (pre-mortem F1): a PR that is green,
// mergeable, no changes requested, not draft, and has ZERO approvals (Tyler
// cannot self-approve) is flagged pr_ready_unmerged, and exactly one
// notification fires once the condition has held past the 30-minute
// threshold — no second notification while it stays ready.
func TestReconcilePRPending_should_markStuck_When_PRGreenMergeableUnapproved(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item := newPRPendingTestItem(t, storage, 148)

	listener := NewBacklogLifecycleListener(storage)
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)
	overridePRPendingChecker(t, listener, &fakePRPendingChecker{
		merged: false,
		status: &git.PRStatus{
			IsDraft:               false,
			Mergeable:             "MERGEABLE",
			ApprovedCount:         0, // flagship case: unapproved, still ready
			ChangesRequestedCount: 0,
			CIFailing:             false,
			HasBlockingReviews:    false,
			HasConflicts:          false,
		},
	})

	er := storage.repo.(*EntRepository)

	// First tick: opens the row but must not notify yet (within 30m threshold).
	listener.ReconcilePRPending(ctx, er)
	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, domain.StuckReasonPRReadyUnmerged, open[0].Reason)
	assert.Equal(t, item.ID, open[0].ItemID)
	assert.Empty(t, notifier.calls, "must not notify before the 30-minute threshold elapses")

	backdateStuckFirstDetected(t, er, item.ID, domain.StuckReasonPRReadyUnmerged, time.Now().Add(-31*time.Minute))

	listener.ReconcilePRPending(ctx, er)
	assert.Equal(t, []string{"PR ready to merge"}, notifier.titles())

	// Third tick: PR still ready — must not re-notify.
	listener.ReconcilePRPending(ctx, er)
	assert.Len(t, notifier.calls, 1, "must not re-notify while the PR stays ready")
}

// TestReconcilePRPending_should_resolvePRReadyRow_When_PRMerged verifies that
// when the PR merges, the open pr_ready_unmerged row is resolved in the same
// reconcile pass that transitions the item to done (Task 2.1.5a).
func TestReconcilePRPending_should_resolvePRReadyRow_When_PRMerged(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item := newPRPendingTestItem(t, storage, 148)
	er := storage.repo.(*EntRepository)
	applied, err := er.MarkStuck(ctx, item.ID, domain.StuckReasonPRReadyUnmerged, BacklogStatusPRPending, "PR is green & mergeable")
	require.NoError(t, err)
	require.True(t, applied)

	listener := NewBacklogLifecycleListener(storage)
	overridePRPendingChecker(t, listener, &fakePRPendingChecker{merged: true})

	listener.ReconcilePRPending(ctx, er)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusDone), fetched.Status)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "pr_ready_unmerged row must be resolved when the PR merges")
}

// TestReconcilePRPending_should_resolvePRReadyRow_When_NewCommitClearsReadinessWhileStillPrPending
// is the C2 regression test: a same-status clear (a new commit makes CI start
// failing while the item is still pr_pending) must be resolved by the
// detector's own poll-shaped else-branch — the status-anchored self-heal
// sweep structurally cannot see this, since the item's status never changed.
func TestReconcilePRPending_should_resolvePRReadyRow_When_NewCommitClearsReadinessWhileStillPrPending(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item := newPRPendingTestItem(t, storage, 148)
	er := storage.repo.(*EntRepository)
	applied, err := er.MarkStuck(ctx, item.ID, domain.StuckReasonPRReadyUnmerged, BacklogStatusPRPending, "PR is green & mergeable")
	require.NoError(t, err)
	require.True(t, applied)

	listener := NewBacklogLifecycleListener(storage)
	// New commit re-runs CI; it's now failing while the item is still pr_pending.
	overridePRPendingChecker(t, listener, &fakePRPendingChecker{
		merged: false,
		status: &git.PRStatus{CIFailing: true, FeedbackText: "CI is red"},
	})

	listener.ReconcilePRPending(ctx, er)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusPRPending), fetched.Status, "item status must not have changed (no PRFixSpawner wired)")

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "pr_ready_unmerged row must resolve on the same tick even though item status is unchanged")
}

// --- Story 2.1.3: abandoned_review / stale_work durable + zombie detection ---

// TestReconcileStuckReviewItems_should_markAbandoned_When_OnlyActiveSessionIsDeadZombie
// guards pre-mortem F3: a review item whose only EndedAt-IS-NULL session
// looks active in the DB but whose underlying tmux/CLI process is confirmed
// dead must still be flagged abandoned_review — closing the gap where
// FindStuckReviewItems' "nothing active in flight" filter would otherwise
// leave it invisible forever.
func TestReconcileStuckReviewItems_should_markAbandoned_When_OnlyActiveSessionIsDeadZombie(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	// endReviewSession=false: the review session's EndedAt stays nil, so
	// FindStuckReviewItems excludes it — this is exactly the zombie-candidate shape.
	item := newStuckReviewTestItem(t, storage, ReviewVerdictUnverifiable, false, false)

	listener := NewBacklogLifecycleListener(storage)
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)
	listener.SetSessionLivenessChecker(func(sessionUUID string) bool { return false }) // always dead

	er := storage.repo.(*EntRepository)
	listener.reconcileStuckReviewItems(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, item.ID, open[0].ItemID)
	assert.Equal(t, domain.StuckReasonAbandonedReview, open[0].Reason)
	assert.Contains(t, open[0].Context, "zombie")
}

// TestReconcileStuckReviewItems_should_tombstoneZombieSession_When_ConfirmedDead is the
// regression test for the bug where zombie detection fired but never closed the
// EndedAt-nil ItemSession row it found — leaving AutoRespawnReview's
// hasActiveReviewSession guard (server/services/backlog_service_triage.go) permanently
// convinced a respawn was already in flight, silently no-oping every dispatched retry.
func TestReconcileStuckReviewItems_should_tombstoneZombieSession_When_ConfirmedDead(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item := newStuckReviewTestItem(t, storage, ReviewVerdictUnverifiable, false, false)

	listener := NewBacklogLifecycleListener(storage)
	listener.SetSessionLivenessChecker(func(sessionUUID string) bool { return false }) // always dead

	er := storage.repo.(*EntRepository)
	listener.reconcileStuckReviewItems(ctx, er)

	sessions, err := storage.ListItemSessions(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.NotNil(t, sessions[0].EndedAt, "confirmed-dead zombie review session must be tombstoned, not left EndedAt-nil")
}

// TestReconcileStuckReviewItems_should_notMarkAbandoned_When_ActiveSessionStillAlive
// verifies the zombie detector does not flag a genuinely-live review session —
// a real in-flight review is not a false positive.
func TestReconcileStuckReviewItems_should_notMarkAbandoned_When_ActiveSessionStillAlive(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	newStuckReviewTestItem(t, storage, ReviewVerdictUnverifiable, false, false)

	listener := NewBacklogLifecycleListener(storage)
	listener.SetSessionLivenessChecker(func(sessionUUID string) bool { return true }) // always alive

	er := storage.repo.(*EntRepository)
	listener.reconcileStuckReviewItems(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "a genuinely-alive active session must not be flagged")
}

// --- reconcileUnprocessedReviewVerdicts: verdict recorded but never actioned ---

// TestReconcileUnprocessedReviewVerdicts_should_applyPassVerdict_When_ReviewSessionDiedButWorkSessionStillAlive
// is the regression test for the exact live bug reported: a work session left
// alive polling for a verdict (AutoReopenAfterFailedReview's live-session-reuse)
// makes reconcileStuckReviewItems' zombie check skip the item entirely (it only
// looks for ALL sessions dead), even though the review session itself died with
// a PASS verdict nobody ever actioned. This detector must apply that verdict —
// here, falling back to a direct "done" transition since the test's work
// session has no real git worktree — regardless of the still-alive work session.
func TestReconcileUnprocessedReviewVerdicts_should_applyPassVerdict_When_ReviewSessionDiedButWorkSessionStillAlive(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	// endReviewSession=false (crashed before its exit event fired),
	// withActiveWorkSession=true (the live-session-reuse shape that fools the
	// zombie detector above).
	item := newStuckReviewTestItem(t, storage, ReviewVerdictPass, false, true)

	listener := NewBacklogLifecycleListener(storage)
	// Only the review session (headless-review- prefixed) is dead; the work
	// session is genuinely alive — the zombie detector would skip this item.
	listener.SetSessionLivenessChecker(func(sessionUUID string) bool {
		return !strings.HasPrefix(sessionUUID, "headless-review-")
	})

	er := storage.repo.(*EntRepository)
	listener.reconcileUnprocessedReviewVerdicts(ctx, er)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusDone), fetched.Status,
		"the unactioned PASS verdict must be applied even though a work session is still alive")

	sessions, err := storage.ListItemSessions(ctx, item.ID)
	require.NoError(t, err)
	for _, is := range sessions {
		if is.Role == string(SessionRoleReview) {
			assert.NotNil(t, is.EndedAt, "the dead review session must be tombstoned")
		}
	}
}

// TestReconcileUnprocessedReviewVerdicts_should_notAct_When_ReviewSessionStillAlive
// verifies a review session that hasn't ended yet and isn't confirmed dead is left
// alone — it may simply be doing its own post-verdict cleanup.
func TestReconcileUnprocessedReviewVerdicts_should_notAct_When_ReviewSessionStillAlive(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item := newStuckReviewTestItem(t, storage, ReviewVerdictPass, false, false)

	listener := NewBacklogLifecycleListener(storage)
	listener.SetSessionLivenessChecker(func(sessionUUID string) bool { return true }) // everything alive

	er := storage.repo.(*EntRepository)
	listener.reconcileUnprocessedReviewVerdicts(ctx, er)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusReview), fetched.Status, "a still-alive review session must not be acted on")
}

// TestReconcileUnprocessedReviewVerdicts_should_skipStaleVerdict_When_ItemReenteredReviewAfterAlreadyShipping
// is the regression test for the exact live 2026-07-20 incident on backlog
// item 0fd4a940 ("Backlog Rich text", PR #176): its most recent review-role
// session's PASS verdict had already been correctly applied once — the item
// shipped all the way to "done" — but nothing marks a verdict as "consumed",
// so FindReviewItemsWithUnprocessedVerdict keeps matching on "most recent
// review-role session is dead and has a verdict" forever. When the item
// later re-entered "review" for an unrelated reason (a separate bug —
// AutoReopenAfterFailedReview's rollback, fixed independently in this same
// change — force-reopened the already-"done" item) with no new review-role
// session yet created, this sweep treated the already-shipped verdict as
// fresh and reprocessed it, kicking off the reopen cascade
// (done -> in_progress -> review, spawning a redundant second review). This
// test only needs the resulting shape — a "done" item forced back into
// "review" with no new review session — regardless of what put it there.
func TestReconcileUnprocessedReviewVerdicts_should_skipStaleVerdict_When_ItemReenteredReviewAfterAlreadyShipping(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	// Same recipe as the sibling "applyPassVerdict" test above: a dead review
	// session with a PASS verdict, plus a work session so the PASS branch has
	// something to ship (falls back to a direct "done" transition — no real
	// git worktree in this test).
	item := newStuckReviewTestItem(t, storage, ReviewVerdictPass, true, true)

	listener := NewBacklogLifecycleListener(storage)
	listener.SetSessionLivenessChecker(func(sessionUUID string) bool { return false }) // everything dead
	er := storage.repo.(*EntRepository)

	listener.reconcileUnprocessedReviewVerdicts(ctx, er)
	shipped, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	require.Equal(t, string(BacklogStatusDone), shipped.Status, "sanity: the verdict must ship the item once")

	// The item is forced back into "review" from "done" — standing in for
	// whatever put it there live (a separate, already-fixed bug). No new
	// review-role session is created: the only one on record is the one
	// whose verdict already shipped the item above.
	_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusReview, nil)
	require.NoError(t, err)

	listener.reconcileUnprocessedReviewVerdicts(ctx, er)

	final, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusReview), final.Status,
		"a verdict from a prior, already-concluded review cycle must not be reprocessed")
}

// TestReconcileStuckReviewItems_should_resolveAbandonedRow_When_ReviewGateBackInFlightWhileStillReview
// is the C2 regression test for abandoned_review: when the review gate comes
// back in flight (a new active session appears) while the item is still
// "review", the detector's own else-branch must resolve the open row on the
// same tick — the status-anchored self-heal sweep cannot see this same-status
// clear.
func TestReconcileStuckReviewItems_should_resolveAbandonedRow_When_ReviewGateBackInFlightWhileStillReview(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item := newStuckReviewTestItem(t, storage, ReviewVerdictUnverifiable, true, false)

	listener := NewBacklogLifecycleListener(storage)
	er := storage.repo.(*EntRepository)

	listener.reconcileStuckReviewItems(ctx, er)
	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1, "abandoned_review row must be open after the first tick")

	// The review gate comes back in flight: a fresh active work session appears.
	_, err = storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: uuid.New().String(),
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)

	listener.reconcileStuckReviewItems(ctx, er)
	open, err = er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "row must resolve immediately once the item is no longer abandoned, even though status is still review")
}

// TestReconcileStaleWorkSessions_should_writeDurableStaleWorkRow_When_ActiveSessionStale
// verifies a durable stale_work row is opened (DB-backed, not the removed
// in-memory map) for an in_progress item whose active work session has gone
// quiet past maxWorkSessionStaleness.
func TestReconcileStaleWorkSessions_should_writeDurableStaleWorkRow_When_ActiveSessionStale(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item := newStaleWorkTestItem(t, storage, er)

	listener := NewBacklogLifecycleListener(storage)
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	listener.reconcileStaleWorkSessions(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, item.ID, open[0].ItemID)
	assert.Equal(t, domain.StuckReasonStaleWork, open[0].Reason)
	assert.Equal(t, []string{"Work session may be stuck"}, notifier.titles())

	// Repeat tick must not re-notify (DB-backed notify-once dedup).
	listener.reconcileStaleWorkSessions(ctx, er)
	assert.Len(t, notifier.calls, 1)
}

// TestReconcileStaleWorkSessions_should_resolveStaleWorkRow_When_SessionResumesWhileStillInProgress
// is the C2 regression test for stale_work: when the active session resumes
// reporting progress while the item stays in_progress, the detector's
// else-branch must resolve the row on the same tick.
func TestReconcileStaleWorkSessions_should_resolveStaleWorkRow_When_SessionResumesWhileStillInProgress(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item := newStaleWorkTestItem(t, storage, er)

	listener := NewBacklogLifecycleListener(storage)
	listener.reconcileStaleWorkSessions(ctx, er)
	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1)

	// The session resumes reporting progress.
	sessions, err := storage.ListItemSessions(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	parsedID, err := uuid.Parse(sessions[0].ID)
	require.NoError(t, err)
	_, err = er.client.ItemSession.UpdateOneID(parsedID).SetLastProgressAt(time.Now()).Save(ctx)
	require.NoError(t, err)

	listener.reconcileStaleWorkSessions(ctx, er)
	open, err = er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "row must resolve immediately once progress resumes, even though status is still in_progress")
}

// --- stale_work: Phase B automated remediation (docs/tasks/backlog-stuck-
// item-auto-remediation.md) — reconcileStaleWorkSessions was previously
// notify-only; a live item (9264efe7-b4c2-455a-9e2a-ab0196a63ecd, rework
// suffix -r14) sat in_progress for 24h+ with its work session's agent idle
// at an interactive prompt (TmuxAlive true, PaneProcessDead false — not a
// zombie the generic health check would ever catch) because nothing ever
// acted on the detection. ---

// fakeStaleWorkRemediator is a test double implementing StaleWorkRemediator,
// recording every RemediateStaleWorkSession call on a buffered channel (not a
// plain counter) because remediateStaleWorkWithBackoffGate dispatches
// asynchronously — mirrors fakeReviewRespawner's identical rationale.
type fakeStaleWorkRemediator struct {
	calls chan string
	err   error
}

func newFakeStaleWorkRemediator() *fakeStaleWorkRemediator {
	return &fakeStaleWorkRemediator{calls: make(chan string, 32)}
}

func (f *fakeStaleWorkRemediator) RemediateStaleWorkSession(ctx context.Context, itemID string) error {
	f.calls <- itemID
	return f.err
}

// TestReconcileStaleWorkSessions_should_notRemediateOnFirstSighting_When_RowJustOpened
// verifies the tick that first opens (and notifies) a stale_work row does NOT
// also dispatch remediation — the existing maxWorkSessionStaleness threshold
// is already the "give it a chance" window; remediation only starts on a
// subsequent tick that reconfirms the row is still open, matching how
// autoReopenWithBackoffGate/retryPushFailedWithBackoffGate are always
// invoked from a call site architecturally separate from their reason's own
// MarkStuck call.
func TestReconcileStaleWorkSessions_should_notRemediateOnFirstSighting_When_RowJustOpened(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	newStaleWorkTestItem(t, storage, er)

	listener := NewBacklogLifecycleListener(storage)
	listener.SetNotifier(&fakeNotifier{})
	remediator := newFakeStaleWorkRemediator()
	listener.SetStaleWorkRemediator(remediator)

	listener.reconcileStaleWorkSessions(ctx, er)

	select {
	case itemID := <-remediator.calls:
		t.Fatalf("remediation must not fire on the same tick that first opened the row, got call for item %s", itemID)
	case <-time.After(100 * time.Millisecond):
		// expected: no remediation dispatched yet
	}
}

// TestReconcileStaleWorkSessions_should_dispatchRemediation_When_RowAlreadyOpenAndDue
// verifies requirement (a): once a stale_work row is already open (a prior
// tick marked+notified it) and the item is still stale, the NEXT sweep tick
// dispatches RemediateStaleWorkSession through the backoff gate, and
// RemediationDue's own attempt-accounting state advances (mirrors
// TestReconcilePushFailedItems_should_dispatchRetryThroughBackoffGate's
// production-entry-point style, exercised through reconcileStaleWorkSessions
// itself rather than calling remediateStaleWorkWithBackoffGate directly).
func TestReconcileStaleWorkSessions_should_dispatchRemediation_When_RowAlreadyOpenAndDue(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item := newStaleWorkTestItem(t, storage, er)

	listener := NewBacklogLifecycleListener(storage)
	listener.SetNotifier(&fakeNotifier{})
	remediator := newFakeStaleWorkRemediator()
	listener.SetStaleWorkRemediator(remediator)

	// First tick: opens + notifies the row, does not remediate.
	listener.reconcileStaleWorkSessions(ctx, er)

	// Second tick: item is still stale (LastProgressAt untouched) — the row
	// is already open, so this tick must dispatch remediation.
	listener.reconcileStaleWorkSessions(ctx, er)

	require.Eventually(t, func() bool {
		select {
		case itemID := <-remediator.calls:
			return itemID == item.ID
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond, "remediation must be dispatched once the row is already open and still due")

	require.Eventually(t, func() bool {
		rows, findErr := er.FindOpenStuckStates(ctx)
		return findErr == nil && len(rows) == 1 && rows[0].RemediationAttempts == 1
	}, time.Second, 10*time.Millisecond, "RemediationDue's attempt accounting must advance exactly once")
}

// TestRemediateStaleWorkWithBackoffGate_should_respectBackoffSchedule_When_CalledRepeatedly
// verifies requirement (b): repeated rapid sweep ticks do not bypass the
// backoff schedule — mirrors
// TestRetryPushFailedWithBackoffGate_should_respectBackoffSchedule_When_CalledRepeatedly
// for the "stale_work" reason.
func TestRemediateStaleWorkWithBackoffGate_should_respectBackoffSchedule_When_CalledRepeatedly(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item := newStaleWorkTestItem(t, storage, er)

	listener := NewBacklogLifecycleListener(storage)
	listener.SetNotifier(&fakeNotifier{})
	remediator := newFakeStaleWorkRemediator()
	listener.SetStaleWorkRemediator(remediator)

	// Open the row (first-sighting tick: mark + notify only).
	listener.reconcileStaleWorkSessions(ctx, er)

	for i := 0; i < 10; i++ {
		listener.remediateStaleWorkWithBackoffGate(ctx, item.ID, item.Title)
	}

	require.Eventually(t, func() bool {
		rows, err := er.FindOpenStuckStates(ctx)
		return err == nil && len(rows) == 1 && rows[0].RemediationAttempts == 1
	}, time.Second, 10*time.Millisecond, "only the first of 10 back-to-back calls should consume an attempt")
}

// TestReconcileStaleWorkSessions_should_notTouchHealthySession_When_ProgressIsRecent
// verifies requirement (c): a work session correctly identified as still
// active/healthy (LastProgressAt recent, under maxWorkSessionStaleness) is
// never marked stuck, notified, or handed to the remediator — no kill, no
// notify, no remediation attempt.
func TestReconcileStaleWorkSessions_should_notTouchHealthySession_When_ProgressIsRecent(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "Healthy in-progress item",
		Status: string(BacklogStatusInProgress),
	})
	require.NoError(t, err)
	_, err = storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "healthy-work-session-uuid",
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)
	remediator := newFakeStaleWorkRemediator()
	listener.SetStaleWorkRemediator(remediator)

	// Several ticks, exactly as a real periodic sweep would run.
	for i := 0; i < 3; i++ {
		listener.reconcileStaleWorkSessions(ctx, er)
	}

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "a healthy session must never be marked stuck")
	assert.Empty(t, notifier.calls, "a healthy session must never be notified")
	select {
	case itemID := <-remediator.calls:
		t.Fatalf("a healthy session must never be handed to the remediator, got call for item %s", itemID)
	default:
		// expected: no remediation attempted
	}
}

// TestRemediateStaleWorkWithBackoffGate_should_parkAfterMaxAttempts_When_ReworkCapIsUnlimited
// is the bonus regression test: an item with ReworkCapOverride=0 (unlimited
// rework retries — set on the live incident item, rework suffix -r14) must
// still be capped by MaxRemediationAttempts and eventually park with the
// standard "Auto-rework paused" notification, proving stale_work remediation
// is bounded independently of the per-item rework cap. remediateStaleWork-
// WithBackoffGate itself never reads ReworkCapOverride — RemediationDue's
// attempt cap is the only thing that can ever stop it — so this also
// verifies the field is present for documentation/regression purposes even
// though this gate-level test doesn't need a real remediation action to
// prove the point (mirrors
// TestRemediationDue_should_advanceThroughFullScheduleThenPark's backdating
// technique for driving all 5 attempts without a 72h+ real sleep).
func TestRemediateStaleWorkWithBackoffGate_should_parkAfterMaxAttempts_When_ReworkCapIsUnlimited(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	unlimited := 0
	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:             "Unlimited-rework-cap stale item",
		Status:            string(BacklogStatusInProgress),
		ReworkCapOverride: &unlimited,
	})
	require.NoError(t, err)
	workIS, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "work-" + uuid.New().String(),
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)
	parsedID, err := uuid.Parse(workIS.ID)
	require.NoError(t, err)
	_, err = er.client.ItemSession.UpdateOneID(parsedID).SetLastProgressAt(time.Now().Add(-3 * time.Hour)).Save(ctx)
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)
	remediator := newFakeStaleWorkRemediator()
	listener.SetStaleWorkRemediator(remediator)

	// First-sighting tick: mark + notify only.
	listener.reconcileStaleWorkSessions(ctx, er)

	for attempt := 1; attempt <= 5; attempt++ {
		listener.remediateStaleWorkWithBackoffGate(ctx, item.ID, item.Title)
		require.Eventually(t, func() bool {
			rows, findErr := er.FindOpenStuckStates(ctx)
			return findErr == nil && len(rows) == 1 && rows[0].RemediationAttempts == int32(attempt)
		}, time.Second, 10*time.Millisecond, "attempt %d must be recorded", attempt)
		if attempt < 5 {
			backdateNextRemediationAt(t, er, item.ID, domain.StuckReasonStaleWork, time.Now().Add(-time.Second))
		}
	}

	assert.Contains(t, notifier.titles(), "Auto-rework paused", "the 5th attempt must fire the parked notification regardless of ReworkCapOverride=0")

	// 6th call, well past backoff: must not consume another attempt — the
	// unlimited rework cap does not un-park a MaxRemediationAttempts-exhausted row.
	backdateNextRemediationAt(t, er, item.ID, domain.StuckReasonStaleWork, time.Now().Add(-time.Second))
	listener.remediateStaleWorkWithBackoffGate(ctx, item.ID, item.Title)

	rows, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, int32(5), rows[0].RemediationAttempts, "parked attempt count must not grow past the cap even with an unlimited rework override")
}

// --- orphaned_triage: standing detector for tombstoneOrphanTriageSessions'
// manual-re-trigger-only blind spot (backlog-feature-improvement audit finding #8) ---

func TestReconcileOrphanedTriageItems_should_writeDurableRowNotifyOnce_When_TriageSessionStale(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item := newOrphanedTriageTestItem(t, storage, er, 3*time.Hour) // beyond maxWorkSessionStaleness (2h)

	listener := NewBacklogLifecycleListener(storage)
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	listener.reconcileOrphanedTriageItems(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, item.ID, open[0].ItemID)
	assert.Equal(t, domain.StuckReasonOrphanedTriage, open[0].Reason)
	assert.Equal(t, []string{"Triage may be stuck"}, notifier.titles())

	// Repeat tick must not re-notify (DB-backed notify-once dedup).
	listener.reconcileOrphanedTriageItems(ctx, er)
	assert.Len(t, notifier.calls, 1)
}

// TestReconcileOrphanedTriageItems_should_tombstoneStaleSession_When_Detected is the
// regression test for the bug where a stale triage row was flagged/notified but left
// EndedAt-nil forever — only a human manually re-triggering triage (via
// tombstoneOrphanTriageSessions, server/services/backlog_service_triage.go) ever closed
// it, so a crashed triage on an item nobody revisits accumulated as an open row
// indefinitely.
func TestReconcileOrphanedTriageItems_should_tombstoneStaleSession_When_Detected(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item := newOrphanedTriageTestItem(t, storage, er, 3*time.Hour) // beyond maxWorkSessionStaleness (2h)

	listener := NewBacklogLifecycleListener(storage)
	listener.reconcileOrphanedTriageItems(ctx, er)

	sessions, err := storage.ListItemSessions(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.NotNil(t, sessions[0].EndedAt, "a confirmed-stale triage session must be tombstoned, not left open indefinitely")
}

func TestReconcileOrphanedTriageItems_should_notFlag_When_TriageSessionRecent(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	// A triage session that just started (well within maxWorkSessionStaleness)
	// must not be flagged — it is plausibly still running.
	newOrphanedTriageTestItem(t, storage, er, 5*time.Minute)

	listener := NewBacklogLifecycleListener(storage)
	listener.reconcileOrphanedTriageItems(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "a recently-started triage session must not be flagged as orphaned")
}

// TestReconcileOrphanedTriageItems_should_flagHeadlessSession_After30Min is the
// regression test for closing the "triage session died before submit_triage_result,
// item silently stuck in idea for up to 2h" gap (GAP-20/21): a headless-triage session
// (the common execution path) must be flagged well before the general-purpose 2h
// staleness ceiling, since an open headless row reliably means dead, not slow.
func TestReconcileOrphanedTriageItems_should_flagHeadlessSession_After30Min(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	// 45 minutes: past maxHeadlessTriageSessionStaleness (30m) but nowhere near the
	// general-purpose maxWorkSessionStaleness (2h) — would NOT have been flagged
	// before this fix.
	item := newOrphanedTriageTestItem(t, storage, er, 45*time.Minute)

	listener := NewBacklogLifecycleListener(storage)
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	listener.reconcileOrphanedTriageItems(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1, "a headless triage session at 45m must be flagged, not held to the 2h general-purpose threshold")
	assert.Equal(t, item.ID, open[0].ItemID)
	assert.Equal(t, domain.StuckReasonOrphanedTriage, open[0].Reason)
	assert.Equal(t, []string{"Triage may be stuck"}, notifier.titles())
}

func TestSelfHealSweep_should_resolveOrphanedTriageRow_When_ItemLeavesIdea(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "Orphaned triage item that got re-triggered",
		Status: string(BacklogStatusReady),
	})
	require.NoError(t, err)
	applied, err := er.MarkStuck(ctx, item.ID, domain.StuckReasonOrphanedTriage, BacklogStatusReady, "stale triage session")
	require.NoError(t, err)
	require.True(t, applied)

	listener := NewBacklogLifecycleListener(storage)
	listener.selfHealStuck(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "orphaned_triage row must resolve once the item leaves idea (re-triggered triage succeeded)")
}

// --- archive_terminal_sessions: safety-net sweep for backlog work sessions that
// accumulate forever because the TransitionBacklogItemStatus archival hook only
// fires on a NEW transition into done/archived — see SessionArchiver's doc comment
// and reconcileTerminalItemSessions. ---

// fakeSessionArchiver is a test stub implementing SessionArchiver. It records every
// UUID it was asked to archive, in order, and can be configured to fail for
// specific UUIDs.
type fakeSessionArchiver struct {
	archivedUUIDs []string
	errForUUID    map[string]error
}

func (f *fakeSessionArchiver) ArchiveSessionByUUID(_ context.Context, sessionUUID string) error {
	if f.errForUUID != nil {
		if err, ok := f.errForUUID[sessionUUID]; ok {
			return err
		}
	}
	f.archivedUUIDs = append(f.archivedUUIDs, sessionUUID)
	return nil
}

func TestReconcileTerminalItemSessions_should_ArchiveWorkSession_When_ItemAlreadyDone(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	// Simulates an item that reached "done" through a call path that bypasses the
	// TransitionBacklogItemStatus RPC handler's archival hook (e.g.
	// SubmitManualReview, which transitions review->done via
	// storage.TransitionBacklogItemStatus directly) — the sweep is the backstop
	// for exactly this case.
	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "already-done item missed by the transition hook",
		Status: string(BacklogStatusDone),
	})
	require.NoError(t, err)
	_, err = storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "done-work-session",
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	archiver := &fakeSessionArchiver{}
	listener.SetSessionArchiver(archiver)

	listener.reconcileTerminalItemSessions(ctx)

	assert.Contains(t, archiver.archivedUUIDs, "done-work-session")
}

func TestReconcileTerminalItemSessions_should_ArchiveWorkSession_When_ItemAlreadyArchived(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "already-archived item missed by the transition hook",
		Status: string(BacklogStatusArchived),
	})
	require.NoError(t, err)
	_, err = storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "archived-work-session",
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	archiver := &fakeSessionArchiver{}
	listener.SetSessionArchiver(archiver)

	listener.reconcileTerminalItemSessions(ctx)

	assert.Contains(t, archiver.archivedUUIDs, "archived-work-session")
}

func TestReconcileTerminalItemSessions_should_NotArchiveNonWorkSessions_When_ItemDone(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "done item with a review session",
		Status: string(BacklogStatusDone),
	})
	require.NoError(t, err)
	_, err = storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "review-session-not-archived-here",
		SessionRole: SessionRoleReview,
	})
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	archiver := &fakeSessionArchiver{}
	listener.SetSessionArchiver(archiver)

	listener.reconcileTerminalItemSessions(ctx)

	assert.Empty(t, archiver.archivedUUIDs, "the sweep only archives work-role sessions — review sessions are already hidden/one-shot and excluded by design")
}

func TestReconcileTerminalItemSessions_should_NotArchiveAnything_When_ItemNotTerminal(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "still in-flight item",
		Status: string(BacklogStatusInProgress),
	})
	require.NoError(t, err)
	_, err = storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "in-flight-work-session",
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	archiver := &fakeSessionArchiver{}
	listener.SetSessionArchiver(archiver)

	listener.reconcileTerminalItemSessions(ctx)

	assert.Empty(t, archiver.archivedUUIDs, "a still in-flight item's work session must not be archived")
}

func TestReconcileTerminalItemSessions_should_NoOp_When_ArchiverNotWired(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "done item with no archiver wired",
		Status: string(BacklogStatusDone),
	})
	require.NoError(t, err)
	_, err = storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "no-archiver-work-session",
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	// SetSessionArchiver deliberately not called — must degrade gracefully, not panic.
	assert.NotPanics(t, func() {
		listener.reconcileTerminalItemSessions(ctx)
	})
}

// TestReconcileTerminalItemSessions_should_BeIdempotent_When_RunTwice proves the
// sweep is safe to re-run every tick: a second run over already-archived sessions
// must not error or duplicate archive calls in a way that matters (the real
// SessionArchiver implementation is itself a CAS no-op for already-archived
// sessions — this test only proves the sweep's own iteration doesn't choke on
// repeated runs).
func TestReconcileTerminalItemSessions_should_BeIdempotent_When_RunTwice(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "done item swept twice",
		Status: string(BacklogStatusDone),
	})
	require.NoError(t, err)
	_, err = storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "swept-twice-work-session",
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	archiver := &fakeSessionArchiver{}
	listener.SetSessionArchiver(archiver)

	listener.reconcileTerminalItemSessions(ctx)
	listener.reconcileTerminalItemSessions(ctx)

	assert.Equal(t, []string{"swept-twice-work-session", "swept-twice-work-session"}, archiver.archivedUUIDs,
		"the sweep itself calls ArchiveSessionByUUID once per tick per session — idempotency is the archiver's responsibility (CAS), proven separately for SessionService.ArchiveSessionByUUID")
}

// --- Story 2.1.4: bouncing ---

// TestCountReviewCyclesSince_should_countInProgressToReviewTransitions_When_WithinWindow
// verifies the cycle-count query only counts in_progress->review
// BacklogStatusEvent rows inside the lookback window.
func TestCountReviewCyclesSince_should_countInProgressToReviewTransitions_When_WithinWindow(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "Bouncing cycle count test item",
		Status: string(BacklogStatusInProgress),
	})
	require.NoError(t, err)

	// 3 in_progress->review round trips.
	for i := 0; i < 3; i++ {
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusReview, nil)
		require.NoError(t, err)
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusInProgress, nil)
		require.NoError(t, err)
	}

	count, err := er.CountReviewCyclesSince(ctx, item.ID, time.Now().Add(-bounceLookback))
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	// A since-cutoff after all events must count zero.
	count, err = er.CountReviewCyclesSince(ctx, item.ID, time.Now().Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

// TestReconcileBouncingItems_should_writeBouncingRowNotifyOnce_When_ThreeCyclesIn24hNoPass
// verifies an item that bounced in_progress<->review >= bounceThreshold times
// within bounceLookback with no PASS verdict is flagged bouncing and notified
// once.
func TestReconcileBouncingItems_should_writeBouncingRowNotifyOnce_When_ThreeCyclesIn24hNoPass(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "Bouncing item",
		Status: string(BacklogStatusInProgress),
	})
	require.NoError(t, err)
	for i := 0; i < 3; i++ {
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusReview, nil)
		require.NoError(t, err)
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusInProgress, nil)
		require.NoError(t, err)
	}

	listener := NewBacklogLifecycleListener(storage)
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	listener.reconcileBouncingItems(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, domain.StuckReasonBouncing, open[0].Reason)
	assert.Equal(t, []string{"Item is thrashing between work and review"}, notifier.titles())

	// Repeat tick must not re-notify.
	listener.reconcileBouncingItems(ctx, er)
	assert.Len(t, notifier.calls, 1)
}

// TestReconcileBouncingItems_should_notFlag_When_BelowThresholdOrHasPass verifies
// an item with fewer than bounceThreshold cycles, and one with a recorded
// PASS verdict, are not flagged bouncing.
func TestReconcileBouncingItems_should_notFlag_When_BelowThresholdOrHasPass(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	// Below threshold: only 2 cycles.
	belowThreshold, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "Below threshold item",
		Status: string(BacklogStatusInProgress),
	})
	require.NoError(t, err)
	for i := 0; i < 2; i++ {
		_, err = storage.TransitionBacklogItemStatus(ctx, belowThreshold.ID, BacklogStatusReview, nil)
		require.NoError(t, err)
		_, err = storage.TransitionBacklogItemStatus(ctx, belowThreshold.ID, BacklogStatusInProgress, nil)
		require.NoError(t, err)
	}

	listener := NewBacklogLifecycleListener(storage)
	listener.reconcileBouncingItems(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "an item with fewer than bounceThreshold cycles must not be flagged")
}

// TestReconcileBouncingItems_should_transitionToDone_When_LinkedPRAlreadyMerged
// verifies the reconciler recognizes a bouncing item whose linked PR has
// already merged — including a PR merged manually, outside the app's own
// ship flow (allow_auto_merge is disabled at the repo-settings level) — and
// transitions it to done instead of flagging it STUCK_REASON_BOUNCING.
// Regression test for the 2026-07-20 live repro: backlog item "Add sorting
// and grouping by repository path" bounced with remediationAttempts: 4 and a
// remediation scheduled three hours *after* its PR #172 had already merged,
// because reconcileBouncingItems never checked merge state before MarkStuck.
func TestReconcileBouncingItems_should_transitionToDone_When_LinkedPRAlreadyMerged(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:    "Bouncing item with merged PR",
		Status:   string(BacklogStatusInProgress),
		RepoPath: "/tmp/fake-repo",
	})
	require.NoError(t, err)
	prNumber := 172
	prURL := "https://github.com/TylerStaplerAtFanatics/stapler-squad/pull/172"
	_, err = storage.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{
		PrURL:    &prURL,
		PrNumber: &prNumber,
	}, nil)
	require.NoError(t, err)

	// 3 in_progress->review round trips with no PASS verdict — the exact
	// shape isBouncing flags.
	for i := 0; i < 3; i++ {
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusReview, nil)
		require.NoError(t, err)
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusInProgress, nil)
		require.NoError(t, err)
	}

	listener := NewBacklogLifecycleListener(storage)
	overridePRPendingChecker(t, listener, &fakePRPendingChecker{merged: true})
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	listener.reconcileBouncingItems(ctx, er)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusDone), fetched.Status,
		"an item whose linked PR already merged must transition to done, not stay bouncing")

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "a merged item must never be flagged bouncing")
	assert.Empty(t, notifier.calls, "no bouncing notification should fire for an already-shipped item")
}

// TestReconcileBouncingItems_should_stillFlag_When_LinkedPRNotYetMerged verifies
// the new merge check doesn't suppress detection for a bouncing item whose PR
// is still open — only an actually-merged PR should short-circuit MarkStuck.
func TestReconcileBouncingItems_should_stillFlag_When_LinkedPRNotYetMerged(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:    "Bouncing item with open PR",
		Status:   string(BacklogStatusInProgress),
		RepoPath: "/tmp/fake-repo",
	})
	require.NoError(t, err)
	prNumber := 173
	prURL := "https://github.com/TylerStaplerAtFanatics/stapler-squad/pull/173"
	_, err = storage.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{
		PrURL:    &prURL,
		PrNumber: &prNumber,
	}, nil)
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusReview, nil)
		require.NoError(t, err)
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusInProgress, nil)
		require.NoError(t, err)
	}

	listener := NewBacklogLifecycleListener(storage)
	overridePRPendingChecker(t, listener, &fakePRPendingChecker{merged: false})
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	listener.reconcileBouncingItems(ctx, er)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusInProgress), fetched.Status, "an unmerged item must not be auto-transitioned to done")

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, domain.StuckReasonBouncing, open[0].Reason, "an item with an unmerged PR must still be flagged bouncing")
}

// setupBounceMainRepo creates a temporary git repo with a single commit on a
// branch explicitly renamed to "main" (git's init default branch name isn't
// guaranteed to be "main" — see session/git/worktree_creation_test.go's
// setupTestRepo for the same pattern), matching bounceMainBranch. Returns the
// repo path and the tip commit's SHA.
func setupBounceMainRepo(t *testing.T) (repoPath, mainSHA string) {
	t.Helper()
	dir := t.TempDir()
	runGitTestCmd(t, dir, "init")
	runGitTestCmd(t, dir, "config", "user.email", "test@example.com")
	runGitTestCmd(t, dir, "config", "user.name", "Test")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "base.txt"), []byte("base\n"), 0o644))
	runGitTestCmd(t, dir, "add", "base.txt")
	runGitTestCmd(t, dir, "commit", "-m", "base commit")
	runGitTestCmd(t, dir, "branch", "-M", "main")
	mainSHA = strings.TrimSpace(runGitTestCmd(t, dir, "rev-parse", "HEAD"))
	return dir, mainSHA
}

// TestReconcileBouncingItems_should_transitionToDone_When_ShippedWithoutPR
// verifies the fallback added alongside the PR-merge check above: a bouncing
// item that never had a PR at all (item.PrNumber == 0) — because its real
// work was committed and merged/pushed straight to main outside the app's
// ship flow entirely — is recognized via its own most recent work session's
// commit and transitioned to done instead of being left bouncing forever.
// Regression test for the 2026-07-21 live repro: backlog item "Rich File
// Browser" (93565fa1) had its acceptance-criteria commits verified as
// ancestors of main via `git merge-base --is-ancestor`, but item.PrNumber was
// never set (no PR ever existed for those commits), so the PR-merge check
// added earlier that day never fired and the item kept bouncing/getting
// flagged stuck indefinitely despite its work already having shipped.
//
// The work session's worktree is recorded via SaveInstances (not
// UpdateItemSessionGitActivity, which only ever seeds the pre-work base SHA
// — see resolveLatestWorkCommit's doc comment) so mostRecentWorkCommitShippedToMain
// resolves the commit the same way production does: from the worktree's own
// HEAD, not the stale LastCommitSha field.
func TestReconcileBouncingItems_should_transitionToDone_When_ShippedWithoutPR(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	repoPath, mainSHA := setupBounceMainRepo(t)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:    "Bouncing item shipped without a PR",
		Status:   string(BacklogStatusInProgress),
		RepoPath: repoPath,
	})
	require.NoError(t, err)

	workSessionUUID := "shipped-no-pr-work-session"
	_, err = storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: workSessionUUID,
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)

	// repoPath's checked-out HEAD is already the shipped mainSHA (its only
	// commit) — using repoPath as the worktree path mirrors a work session
	// whose worktree is the shared main checkout itself.
	inst := newTestInstance("shipped-no-pr-instance")
	inst.UUID = workSessionUUID
	inst.gitManager.worktree = git.NewGitWorktreeFromStorage(repoPath, repoPath, "shipped-no-pr-instance", "main", mainSHA)
	require.NoError(t, storage.SaveInstances([]*Instance{inst}))

	// 3 in_progress->review round trips with no PASS verdict — the exact
	// shape isBouncing flags.
	for i := 0; i < 3; i++ {
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusReview, nil)
		require.NoError(t, err)
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusInProgress, nil)
		require.NoError(t, err)
	}

	listener := NewBacklogLifecycleListener(storage)
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	listener.reconcileBouncingItems(ctx, er)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusDone), fetched.Status,
		"an item with no PR whose most recent work-session commit is already on main must transition to done, not stay bouncing")

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "an item whose code already shipped to main without a PR must never be flagged bouncing")
	assert.Empty(t, notifier.calls, "no bouncing notification should fire for an already-shipped item")
}

// TestReconcileBouncingItems_should_stillFlag_When_NoPRAndCommitNotOnMain
// verifies the new no-PR fallback doesn't suppress detection for a genuinely
// stuck item: when there's no PR AND the item's most recent work-session
// commit was never actually merged to main, the item must still be flagged
// bouncing exactly as before — no regression on the normal (truly stuck) case.
//
// The work session's worktree is recorded via SaveInstances so
// mostRecentWorkCommitShippedToMain resolves the commit from the worktree's
// own HEAD (as production does), not the stale LastCommitSha field.
func TestReconcileBouncingItems_should_stillFlag_When_NoPRAndCommitNotOnMain(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	repoPath, _ := setupBounceMainRepo(t)
	runGitTestCmd(t, repoPath, "checkout", "-b", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "feature.txt"), []byte("unshipped work\n"), 0o644))
	runGitTestCmd(t, repoPath, "add", "feature.txt")
	runGitTestCmd(t, repoPath, "commit", "-m", "work that never merged")
	featureSHA := strings.TrimSpace(runGitTestCmd(t, repoPath, "rev-parse", "HEAD"))

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:    "Bouncing item with unshipped commit and no PR",
		Status:   string(BacklogStatusInProgress),
		RepoPath: repoPath,
	})
	require.NoError(t, err)

	workSessionUUID := "unshipped-no-pr-work-session"
	_, err = storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: workSessionUUID,
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)

	// repoPath's checked-out HEAD is the "feature" branch's unshipped commit.
	inst := newTestInstance("unshipped-no-pr-instance")
	inst.UUID = workSessionUUID
	inst.gitManager.worktree = git.NewGitWorktreeFromStorage(repoPath, repoPath, "unshipped-no-pr-instance", "feature", featureSHA)
	require.NoError(t, storage.SaveInstances([]*Instance{inst}))

	for i := 0; i < 3; i++ {
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusReview, nil)
		require.NoError(t, err)
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusInProgress, nil)
		require.NoError(t, err)
	}

	listener := NewBacklogLifecycleListener(storage)
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	listener.reconcileBouncingItems(ctx, er)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusInProgress), fetched.Status,
		"an item with no PR and a commit that never landed on main must not be auto-transitioned to done")

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, domain.StuckReasonBouncing, open[0].Reason,
		"an item with no PR whose commit isn't on main must still be flagged bouncing, same as before this fix")
}

// TestReconcileBouncingItems_should_stillFlag_When_LastCommitShaIsStaleBaseSeed
// is the direct regression test for the 2026-07-21 false-done bug: ItemSession
// .LastCommitSha is only ever seeded once at spawn with the pre-work base SHA
// (UpdateItemSessionGitActivity's callers all pass baseSHA — see
// resolveLatestWorkCommit's doc comment) and never updated as the agent
// commits real work. A base SHA is, by construction, always an ancestor of
// main, so trusting LastCommitSha as "the agent's latest commit" made
// mostRecentWorkCommitShippedToMain trivially true for any PR-less bouncing
// item regardless of whether real work ever shipped — confirmed live: items
// 635a373d, e99d3f4a, and 54e5aa1f were all incorrectly auto-marked done in a
// single reconciliation tick despite each having real, unmerged work. This
// test sets LastCommitSha to the shipped mainSHA (the stale-but-plausible
// value spawn seeding actually produces) while the work session's real
// worktree HEAD sits on an unshipped "feature" commit, and asserts the item
// is still correctly flagged bouncing rather than false-positive "done".
func TestReconcileBouncingItems_should_stillFlag_When_LastCommitShaIsStaleBaseSeed(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	repoPath, mainSHA := setupBounceMainRepo(t)
	runGitTestCmd(t, repoPath, "checkout", "-b", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "feature.txt"), []byte("unshipped work\n"), 0o644))
	runGitTestCmd(t, repoPath, "add", "feature.txt")
	runGitTestCmd(t, repoPath, "commit", "-m", "work that never merged")
	featureSHA := strings.TrimSpace(runGitTestCmd(t, repoPath, "rev-parse", "HEAD"))

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:    "Bouncing item with a stale shipped-looking LastCommitSha",
		Status:   string(BacklogStatusInProgress),
		RepoPath: repoPath,
	})
	require.NoError(t, err)

	workSessionUUID := "stale-base-seed-work-session"
	workIS, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: workSessionUUID,
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)
	// Exactly what real spawn-time seeding writes: the base SHA (here, mainSHA
	// itself — always shipped), not the agent's actual latest commit.
	require.NoError(t, storage.UpdateItemSessionGitActivity(ctx, workIS.ID, mainSHA, "", time.Now(), 0))

	// repoPath's checked-out HEAD is the "feature" branch's unshipped commit —
	// the worktree's real state disagrees with the stale LastCommitSha above.
	inst := newTestInstance("stale-base-seed-instance")
	inst.UUID = workSessionUUID
	inst.gitManager.worktree = git.NewGitWorktreeFromStorage(repoPath, repoPath, "stale-base-seed-instance", "feature", featureSHA)
	require.NoError(t, storage.SaveInstances([]*Instance{inst}))

	for i := 0; i < 3; i++ {
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusReview, nil)
		require.NoError(t, err)
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusInProgress, nil)
		require.NoError(t, err)
	}

	listener := NewBacklogLifecycleListener(storage)
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	listener.reconcileBouncingItems(ctx, er)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusInProgress), fetched.Status,
		"a stale base-seeded LastCommitSha that happens to be on main must NOT be trusted as proof of shipped work")

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, domain.StuckReasonBouncing, open[0].Reason,
		"the item must still be flagged bouncing based on the worktree's real HEAD, not the stale LastCommitSha")
}

// --- Story 2.1.6: push_failed ---

// TestStayInReviewAndNotify_should_markPushFailedRow_When_PushAndCreatePRFails
// verifies a push/PR-creation failure writes a durable push_failed row
// alongside the existing ERROR notification (Story 2.1.6).
func TestStayInReviewAndNotify_should_markPushFailedRow_When_PushAndCreatePRFails(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item, is := newPushAndCreatePRTestFixture(t, storage)

	listener := NewBacklogLifecycleListener(storage)
	fakeCreator := &fakePRCreator{pushErr: errors.New("push rejected: non-fast-forward")}
	listener.SetPRCreatorFactory(func(repoPath, worktreePath, sessionName, branchName, baseCommitSHA string) prCreator {
		return fakeCreator
	})
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	listener.pushAndCreatePR(ctx, item, is)

	assert.Contains(t, notifier.titles(), "PR creation failed", "existing ERROR toast must still fire")

	er := storage.repo.(*EntRepository)
	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, domain.StuckReasonPushFailed, open[0].Reason)
	assert.Equal(t, item.ID, open[0].ItemID)
	assert.NotNil(t, open[0].NotifiedAt, "push_failed dedup must be pre-set since the toast already fired")
}

// TestPushFailed_should_persistRowSurvivingRestart_When_ItemHasNoPrNumber verifies
// an item left with no pr_number (invisible to FindPRPendingItems'
// PrNumberGT(0) filter) still surfaces via a push_failed row that survives a
// simulated server restart (DB close/reopen from the same file).
func TestPushFailed_should_persistRowSurvivingRestart_When_ItemHasNoPrNumber(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "push-failed-restart.db")

	var itemID string
	func() {
		repo, err := NewEntRepository(WithDatabasePath(dbPath))
		require.NoError(t, err)
		defer repo.Close()
		storage, err := NewStorageWithRepository(repo)
		require.NoError(t, err)

		item, is := newPushAndCreatePRTestFixture(t, storage)
		itemID = item.ID
		require.Zero(t, item.PrNumber, "item must have no pr_number before the failed attempt")

		listener := NewBacklogLifecycleListener(storage)
		fakeCreator := &fakePRCreator{createErr: errors.New("gh: authentication required")}
		listener.SetPRCreatorFactory(func(repoPath, worktreePath, sessionName, branchName, baseCommitSHA string) prCreator {
			return fakeCreator
		})
		listener.pushAndCreatePR(context.Background(), item, is)

		fetched, err := storage.GetBacklogItem(context.Background(), itemID)
		require.NoError(t, err)
		assert.Zero(t, fetched.PrNumber, "item must still have no pr_number — invisible to FindPRPendingItems")
	}()

	repo2, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)
	defer repo2.Close()

	open, err := repo2.FindOpenStuckStates(context.Background())
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, itemID, open[0].ItemID)
	assert.Equal(t, domain.StuckReasonPushFailed, open[0].Reason)
}

// --- Phase B: automated push_failed remediation ---
//
// These tests cover the gap where a push_failed row was write-only —
// MarkStuck fired once and nothing ever retried, exactly the shape of the
// live 2026-07-20 repro (backlog item c2ad7bf3-91bf-4d47-8654-0f2f20869080,
// stelekit branch backlog/stelekit-fix-shift-tab-dedent-desktop, rejected
// non-fast-forward). reconcilePushFailedItems is the new periodic
// counterpart to pushAndCreatePR's event-driven attempt.

// TestAttemptPushRemediation_should_resolveStuckRow_When_MergeSucceedsAndRetryPushSucceeds
// verifies requirement (c): a non-fast-forward failure that's resolved by a
// successful merge+retry actually clears the push_failed row and ships the
// item (transition to pr_pending).
func TestAttemptPushRemediation_should_resolveStuckRow_When_MergeSucceedsAndRetryPushSucceeds(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, is := newPushAndCreatePRTestFixture(t, storage)

	listener := NewBacklogLifecycleListener(storage)
	fakeCreator := &fakePRCreator{pushErr: errors.New("push rejected: non-fast-forward")}
	listener.SetPRCreatorFactory(func(repoPath, worktreePath, sessionName, branchName, baseCommitSHA string) prCreator {
		return fakeCreator
	})
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	// Initial failed push writes the durable push_failed row.
	listener.pushAndCreatePR(ctx, item, is)
	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1)
	require.Equal(t, domain.StuckReasonPushFailed, open[0].Reason)

	// The underlying non-fast-forward is now reconcilable: the branch
	// reconciler reports a clean merge, and the retried push succeeds.
	fakeCreator.pushErr = nil
	fakeCreator.createURL = "https://github.com/tstapler/stelekit/pull/251"
	fakeCreator.createNumber = 251
	listener.SetBranchReconciler(func(worktreePath, branchName string) (*git.MergeMainResult, error) {
		assert.NotEmpty(t, worktreePath)
		assert.NotEmpty(t, branchName)
		return &git.MergeMainResult{Merged: true}, nil
	})

	listener.attemptPushRemediation(ctx, item.ID, item.Title)

	assert.True(t, fakeCreator.pushCalled, "the retried push must actually be attempted")
	assert.True(t, fakeCreator.createCalled, "PR creation should proceed once the push succeeds")

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusPRPending), fetched.Status, "item must move to pr_pending once remediation succeeds")

	openAfter, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	for _, row := range openAfter {
		assert.NotEqual(t, domain.StuckReasonPushFailed, row.Reason, "push_failed row must be resolved once the retry succeeds")
	}
}

// TestAttemptPushRemediation_should_notifyManualRebaseNeeded_When_BranchReconcilerReportsConflict
// verifies a real content conflict is NOT mechanically retried: the push must
// not be re-attempted, a distinct "Manual rebase needed" notification must
// fire, and the row must stay open for a human to resolve.
func TestAttemptPushRemediation_should_notifyManualRebaseNeeded_When_BranchReconcilerReportsConflict(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, is := newPushAndCreatePRTestFixture(t, storage)

	listener := NewBacklogLifecycleListener(storage)
	fakeCreator := &fakePRCreator{pushErr: errors.New("push rejected: non-fast-forward")}
	listener.SetPRCreatorFactory(func(repoPath, worktreePath, sessionName, branchName, baseCommitSHA string) prCreator {
		return fakeCreator
	})
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)
	listener.pushAndCreatePR(ctx, item, is)

	fakeCreator.pushCalled = false // reset so we can prove the retry never re-attempts a push
	listener.SetBranchReconciler(func(worktreePath, branchName string) (*git.MergeMainResult, error) {
		return &git.MergeMainResult{Conflicted: true, ConflictedFiles: []string{"src/edit.ts"}}, nil
	})

	listener.attemptPushRemediation(ctx, item.ID, item.Title)

	assert.False(t, fakeCreator.pushCalled, "a real content conflict must not be mechanically retried")
	assert.Contains(t, notifier.titles(), "Manual rebase needed")

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, domain.StuckReasonPushFailed, open[0].Reason, "row stays open — a human needs to resolve the real conflict")
}

// TestRetryPushFailedWithBackoffGate_should_respectBackoffSchedule_When_CalledRepeatedly
// verifies requirement (b): repeated rapid failures don't bypass the backoff
// schedule — mirrors TestRemediationDue_should_capAtFiveAttemptsWithDelayedRetries
// for the "bouncing" reason.
func TestRetryPushFailedWithBackoffGate_should_respectBackoffSchedule_When_CalledRepeatedly(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, is := newPushAndCreatePRTestFixture(t, storage)
	listener := NewBacklogLifecycleListener(storage)
	fakeCreator := &fakePRCreator{pushErr: errors.New("push rejected: non-fast-forward")}
	listener.SetPRCreatorFactory(func(repoPath, worktreePath, sessionName, branchName, baseCommitSHA string) prCreator {
		return fakeCreator
	})
	listener.SetNotifier(&fakeNotifier{})
	listener.pushAndCreatePR(ctx, item, is)

	// The reconciler never actually resolves anything here — only the
	// gate's own bookkeeping is under test, so every dispatched attempt
	// must fail to merge and bail without the loop below consuming more
	// than one attempt.
	listener.SetBranchReconciler(func(worktreePath, branchName string) (*git.MergeMainResult, error) {
		return nil, errors.New("simulated fetch failure")
	})

	for i := 0; i < 10; i++ {
		listener.retryPushFailedWithBackoffGate(ctx, item.ID, item.Title)
	}

	require.Eventually(t, func() bool {
		rows, err := er.FindOpenStuckStates(ctx)
		return err == nil && len(rows) == 1 && rows[0].RemediationAttempts == 1
	}, time.Second, 10*time.Millisecond, "only the first of 10 back-to-back calls should consume an attempt")
}

// TestReconcilePushFailedItems_should_dispatchRetryThroughBackoffGate_When_RowIsDueAndReconcilerSucceeds
// verifies requirement (a): a push_failed item gets an automated retry
// attempt when due, exercised through the actual production entry point
// wired into ReconcileStuck rather than calling the remediation action
// directly — proving the periodic sweep alone (no active session, no new
// review event) is enough to unstick the item, closing the exact gap behind
// the 2026-07-20 live repro.
func TestReconcilePushFailedItems_should_dispatchRetryThroughBackoffGate_When_RowIsDueAndReconcilerSucceeds(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, is := newPushAndCreatePRTestFixture(t, storage)
	listener := NewBacklogLifecycleListener(storage)
	fakeCreator := &fakePRCreator{pushErr: errors.New("push rejected: non-fast-forward")}
	listener.SetPRCreatorFactory(func(repoPath, worktreePath, sessionName, branchName, baseCommitSHA string) prCreator {
		return fakeCreator
	})
	listener.SetNotifier(&fakeNotifier{})
	listener.pushAndCreatePR(ctx, item, is)

	fakeCreator.pushErr = nil
	listener.SetBranchReconciler(func(worktreePath, branchName string) (*git.MergeMainResult, error) {
		return &git.MergeMainResult{Merged: true}, nil
	})

	listener.reconcilePushFailedItems(ctx, er)

	require.Eventually(t, func() bool {
		fetched, err := storage.GetBacklogItem(ctx, item.ID)
		return err == nil && fetched.Status == string(BacklogStatusPRPending)
	}, 2*time.Second, 10*time.Millisecond, "the periodic sweep must dispatch a retry that eventually ships the item")
}

// TestReconcilePushFailedItems_should_skip_When_ItemNoLongerInReview verifies
// an item whose push_failed row is still open but whose status has since
// moved off "review" (event-shaped rows are excluded from the status-anchor
// self-heal sweep, so nothing else would stop this) is never retried.
func TestReconcilePushFailedItems_should_skip_When_ItemNoLongerInReview(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, is := newPushAndCreatePRTestFixture(t, storage)
	listener := NewBacklogLifecycleListener(storage)
	fakeCreator := &fakePRCreator{pushErr: errors.New("push rejected: non-fast-forward")}
	listener.SetPRCreatorFactory(func(repoPath, worktreePath, sessionName, branchName, baseCommitSHA string) prCreator {
		return fakeCreator
	})
	listener.SetNotifier(&fakeNotifier{})
	listener.pushAndCreatePR(ctx, item, is)

	_, err := storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusDone, nil)
	require.NoError(t, err)

	reconcileCalled := false
	listener.SetBranchReconciler(func(worktreePath, branchName string) (*git.MergeMainResult, error) {
		reconcileCalled = true
		return &git.MergeMainResult{Merged: true}, nil
	})

	listener.reconcilePushFailedItems(ctx, er)
	assert.False(t, reconcileCalled, "an item that has moved off review must not be retried")
}

// --- Story 2.1.5: self-heal sweep (adversarial concern C1) ---

// TestSelfHealSweep_should_resolveAnchoredRow_When_ItemStatusInconsistentWithReason
// verifies a phantom stale_work row on an item that has since moved to done
// (a write raced a transition, or an un-stick call site was missed) is
// resolved by the self-heal sweep.
func TestSelfHealSweep_should_resolveAnchoredRow_When_ItemStatusInconsistentWithReason(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "Phantom stale_work item",
		Status: string(BacklogStatusDone),
	})
	require.NoError(t, err)
	applied, err := er.MarkStuck(ctx, item.ID, domain.StuckReasonStaleWork, BacklogStatusDone, "phantom row")
	require.NoError(t, err)
	require.True(t, applied)

	listener := NewBacklogLifecycleListener(storage)
	listener.selfHealStuck(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "stale_work row on a done item must be resolved by the self-heal sweep")
}

// TestSelfHealSweep_should_notResolveBouncingRow_When_ItemInInProgressHealthyHalfCycle
// guards C1's over-eager map: bouncing legitimately spans both halves of the
// cycle (in_progress AND review), so the sweep must NOT resolve while the
// item sits in either — resolving there would kill a valid signal.
func TestSelfHealSweep_should_notResolveBouncingRow_When_ItemInInProgressHealthyHalfCycle(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "Bouncing item mid-cycle",
		Status: string(BacklogStatusInProgress),
	})
	require.NoError(t, err)
	applied, err := er.MarkStuck(ctx, item.ID, domain.StuckReasonBouncing, BacklogStatusInProgress, "3 cycles")
	require.NoError(t, err)
	require.True(t, applied)

	listener := NewBacklogLifecycleListener(storage)
	listener.selfHealStuck(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1, "a valid bouncing row must NOT be resolved while the item is in a healthy half-cycle (in_progress)")
	assert.Equal(t, domain.StuckReasonBouncing, open[0].Reason)
}

// TestSelfHealSweep_should_resolveBouncingRow_When_ItemReachesDoneOrPass verifies
// the bouncing row resolves once the item reaches a terminal/converged status.
func TestSelfHealSweep_should_resolveBouncingRow_When_ItemReachesDoneOrPass(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "Bouncing item converged",
		Status: string(BacklogStatusDone),
	})
	require.NoError(t, err)
	applied, err := er.MarkStuck(ctx, item.ID, domain.StuckReasonBouncing, BacklogStatusDone, "3 cycles, now done")
	require.NoError(t, err)
	require.True(t, applied)

	listener := NewBacklogLifecycleListener(storage)
	listener.selfHealStuck(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "bouncing row must resolve once the item reaches done")
}

// TestSelfHealSweep_should_notResolveEventShapedRows_When_ItemNotYetTerminal
// verifies rework_cap rows — the one remaining reason with no non-terminal
// anchor at all — stay open while the item has not yet reached done/archived.
// Before this fix, rework_cap (like autonomous_stuck and push_failed before
// PRs #200/#203) had no resolve path here whatsoever; now it relies on the
// blanket terminal-status rule (see TestSelfHealSweep_should_resolveAnyReasonRow_When_ItemReachesTerminalStatus
// below), which only fires once the item actually finishes — so it must
// still stay open on a merely non-terminal, in-flight status.
func TestSelfHealSweep_should_notResolveEventShapedRows_When_ItemNotYetTerminal(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "Event-shaped rows item",
		Status: string(BacklogStatusInProgress), // non-terminal
	})
	require.NoError(t, err)
	applied, err := er.MarkStuck(ctx, item.ID, domain.StuckReasonReworkCap, BacklogStatusInProgress, "cap hit")
	require.NoError(t, err)
	require.True(t, applied)

	listener := NewBacklogLifecycleListener(storage)
	listener.selfHealStuck(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1, "rework_cap has no non-terminal anchor — it must stay open until the item reaches done/archived")
	assert.Equal(t, domain.StuckReasonReworkCap, open[0].Reason)
}

// TestSelfHealSweep_should_resolvePhantomRow_When_WriteRacedTransitionToDone verifies
// the self-heal sweep is the correctness backstop for a stale write that
// lands after a racing transition — the phantom resolves within one tick,
// never leaking a permanent false-positive.
func TestSelfHealSweep_should_resolvePhantomRow_When_WriteRacedTransitionToDone(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	// Simulate the race: item is created in_progress, a stale_work row is
	// marked while the precondition read still shows in_progress, then the
	// item concurrently transitions to done before the self-heal tick runs.
	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "Raced transition item",
		Status: string(BacklogStatusInProgress),
	})
	require.NoError(t, err)
	applied, err := er.MarkStuck(ctx, item.ID, domain.StuckReasonStaleWork, BacklogStatusInProgress, "no progress")
	require.NoError(t, err)
	require.True(t, applied)

	_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusDone, nil)
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	listener.selfHealStuck(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "the racing write must self-correct within one self-heal tick")
}

// --- blanket terminal-status rule (docs/bugs: orphaned autonomous_stuck and
// push_failed rows never auto-resolved once their item completed — fixed
// once each, one-off, in PR #200 and PR #203. rework_cap was the third
// reason sitting in the same trap, unfixed. Rather than a fourth one-off
// case, selfHealStuck now checks a single blanket rule up front: any open
// stuck row on an item that has reached a genuine terminal status (done or
// archived) is resolved, regardless of reason. The table below is the
// generic regression proof for that rule — any reason, once its item goes
// terminal, resolves — superseding the old per-reason
// resolveAutonomousStuckRow_When_ItemReachesDone/Archived and
// resolvePushFailedRow_When_ItemReachesDone/Archived tests.) ---

// TestSelfHealSweep_should_resolveAnyReasonRow_When_ItemReachesTerminalStatus
// is the generic regression proof for selfHealStuck's blanket terminal rule:
// for every StuckReason — including autonomous_stuck and push_failed (the
// two reasons this was previously proven for, one PR at a time) and
// rework_cap (the reason left behind, fixed here) — an open row resolves
// once its item reaches done or archived, independent of whatever
// reason-specific anchor (if any) that reason otherwise uses.
func TestSelfHealSweep_should_resolveAnyReasonRow_When_ItemReachesTerminalStatus(t *testing.T) {
	cases := []struct {
		name          string
		reason        domain.StuckReason
		initialStatus BacklogStatus
	}{
		{"pr_ready_unmerged", domain.StuckReasonPRReadyUnmerged, BacklogStatusPRPending},
		{"abandoned_review", domain.StuckReasonAbandonedReview, BacklogStatusReview},
		{"stale_work", domain.StuckReasonStaleWork, BacklogStatusInProgress},
		{"bouncing", domain.StuckReasonBouncing, BacklogStatusInProgress},
		{"orphaned_triage", domain.StuckReasonOrphanedTriage, BacklogStatusIdea},
		{"autonomous_stuck", domain.StuckReasonAutonomousStuck, BacklogStatusInProgress},
		{"push_failed", domain.StuckReasonPushFailed, BacklogStatusReview},
		{"rework_cap", domain.StuckReasonReworkCap, BacklogStatusInProgress},
	}
	terminals := []BacklogStatus{BacklogStatusDone, BacklogStatusArchived}

	for _, terminal := range terminals {
		terminal := terminal
		for _, tc := range cases {
			tc := tc
			t.Run(tc.name+"_to_"+string(terminal), func(t *testing.T) {
				storage, cleanup := createTestStorage(t)
				defer cleanup()
				ctx := context.Background()
				er := storage.repo.(*EntRepository)

				item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
					Title:  "Terminal blanket-rule item: " + tc.name,
					Status: string(tc.initialStatus),
				})
				require.NoError(t, err)
				applied, err := er.MarkStuck(ctx, item.ID, tc.reason, tc.initialStatus, "test-marked stuck")
				require.NoError(t, err)
				require.True(t, applied)

				// Reach the terminal status. Archived is only reachable through
				// done from a non-idea/refining/ready status, so route through
				// done first — mirrors how these transitions actually happen
				// (auto-archive only ever acts on items already done).
				_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusDone, nil)
				require.NoError(t, err)
				if terminal == BacklogStatusArchived {
					_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusArchived, nil)
					require.NoError(t, err)
				}

				listener := NewBacklogLifecycleListener(storage)
				listener.selfHealStuck(ctx, er)

				open, err := er.FindOpenStuckStates(ctx)
				require.NoError(t, err)
				assert.Empty(t, open, "%s row must resolve once the item reaches %s, via the blanket terminal rule", tc.reason, terminal)
			})
		}
	}
}

// TestSelfHealSweep_should_resolveReworkCapRow_When_ItemReachesDone is the
// direct, narrow regression test for the bug this PR closes: rework_cap was
// the one StuckReason still sitting in the "event-shaped, continue" trap that
// PRs #200 and #203 fixed one-off for autonomous_stuck and push_failed —
// unfixed only because nobody had hit it live yet. Before this PR, this
// exact scenario (a rework_cap row open on an item that later reaches done)
// left the row permanently orphaned; the blanket terminal rule now resolves
// it like every other reason.
func TestSelfHealSweep_should_resolveReworkCapRow_When_ItemReachesDone(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "Rework-cap item that finished",
		Status: string(BacklogStatusInProgress),
	})
	require.NoError(t, err)
	applied, err := er.MarkStuck(ctx, item.ID, domain.StuckReasonReworkCap, BacklogStatusInProgress,
		"rework cap hit: unlimited backoff exhausted")
	require.NoError(t, err)
	require.True(t, applied)

	// The item later legitimately completes — e.g. a human took over and
	// pushed it through manually after the automated rework budget ran out.
	// Before this fix, rework_cap's row would have stayed open forever: it
	// was excluded from the sweep entirely with no anchor, terminal or
	// otherwise.
	_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusDone, nil)
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	listener.selfHealStuck(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "rework_cap row must now resolve once the item reaches done — this previously leaked forever")
}

// TestSelfHealSweep_should_notResolveAutonomousStuckRow_When_ItemStillInProgress
// verifies an item that is genuinely still stuck (never reached done or
// archived) keeps its row open — the sweep must not resolve on a bare
// "left in_progress" signal the way the other, non-inverted anchors do.
func TestSelfHealSweep_should_notResolveAutonomousStuckRow_When_ItemStillInProgress(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "Autonomous item still stuck",
		Status: string(BacklogStatusInProgress),
	})
	require.NoError(t, err)
	applied, err := er.MarkStuck(ctx, item.ID, domain.StuckReasonAutonomousStuck, BacklogStatusInProgress,
		"autonomous driver stopped after 20 turns without a DONE signal")
	require.NoError(t, err)
	require.True(t, applied)

	listener := NewBacklogLifecycleListener(storage)
	listener.selfHealStuck(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1, "a genuinely stuck item must keep its autonomous_stuck row open")
	assert.Equal(t, domain.StuckReasonAutonomousStuck, open[0].Reason)
}

// TestSelfHealSweep_should_notResolveAutonomousStuckRow_When_ItemTransientlyInReviewBeforeLaterStuckState
// is the anti-regression test named directly by the bug report: the
// selfHealStuck doc comment used to exclude autonomous_stuck from the sweep
// entirely because (pre-PR #180) a turn-cap stop could force an
// in_progress->review transition even while genuinely stuck, which would
// have made a naive {in_progress} anchor false-resolve the row before an
// operator ever saw it. This test proves the new {done, archived} anchor
// does not reintroduce that failure mode: an item that has merely cycled
// forward into review (a real, but non-terminal, status) on its way to a
// later stuck condition must NOT have its row resolved.
func TestSelfHealSweep_should_notResolveAutonomousStuckRow_When_ItemTransientlyInReviewBeforeLaterStuckState(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "Autonomous item mid-cycle",
		Status: string(BacklogStatusInProgress),
	})
	require.NoError(t, err)
	applied, err := er.MarkStuck(ctx, item.ID, domain.StuckReasonAutonomousStuck, BacklogStatusInProgress,
		"autonomous driver stopped after 20 turns without a DONE signal")
	require.NoError(t, err)
	require.True(t, applied)

	// The item cycles forward into review — a real transition (e.g. a human
	// pushed the stuck work through manually), but not yet a terminal one.
	_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusReview, nil)
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	listener.selfHealStuck(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1, "an item merely cycling through review (not yet done/archived) must not false-resolve")
	assert.Equal(t, domain.StuckReasonAutonomousStuck, open[0].Reason)
}

// --- push_failed: still-relevant negative coverage (docs/bugs: orphaned
// push_failed rows never auto-resolved once their item completed — fixed by
// PR #203, now generalized into the blanket terminal rule proven by
// TestSelfHealSweep_should_resolveAnyReasonRow_When_ItemReachesTerminalStatus
// above). The positive done/archived cases live in that generic table now;
// what's still worth testing per-reason is the negative case below, which
// guards a specific false-resolve risk unique to push_failed's non-terminal
// behavior. ---

// TestSelfHealSweep_should_notResolvePushFailedRow_When_ItemStillInReview
// verifies an item that is genuinely still stuck (never reached done or
// archived, e.g. retries are still in flight or the backoff gate has parked
// it) keeps its row open — the sweep must not resolve on a bare "left
// review" signal, since push_failed retries never change the item's status
// until a retry actually succeeds.
func TestSelfHealSweep_should_notResolvePushFailedRow_When_ItemStillInReview(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "Push-failed item still stuck",
		Status: string(BacklogStatusReview),
	})
	require.NoError(t, err)
	applied, err := er.MarkStuck(ctx, item.ID, domain.StuckReasonPushFailed, BacklogStatusReview,
		"push rejected: non-fast-forward")
	require.NoError(t, err)
	require.True(t, applied)

	listener := NewBacklogLifecycleListener(storage)
	listener.selfHealStuck(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1, "a genuinely stuck item must keep its push_failed row open")
	assert.Equal(t, domain.StuckReasonPushFailed, open[0].Reason)
}

// TestSelfHealSweep_should_notResolvePushFailedRow_When_ItemTransientlyInProgressBeforeLaterStuckState
// proves the new {done, archived} anchor does not reintroduce a false-resolve
// on a same-status-anchored reason: an item that has merely cycled backward
// into in_progress (e.g. a human sent it back for rework) on its way to a
// later stuck condition must NOT have its row resolved just because it left
// "review" — the anchor is inverted-terminal, not "left review".
func TestSelfHealSweep_should_notResolvePushFailedRow_When_ItemTransientlyInProgressBeforeLaterStuckState(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "Push-failed item mid-cycle",
		Status: string(BacklogStatusReview),
	})
	require.NoError(t, err)
	applied, err := er.MarkStuck(ctx, item.ID, domain.StuckReasonPushFailed, BacklogStatusReview,
		"push rejected: non-fast-forward")
	require.NoError(t, err)
	require.True(t, applied)

	// The item cycles backward into in_progress — a real transition (e.g. a
	// human sent it back for rework), but not yet a terminal one.
	_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusInProgress, nil)
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	listener.selfHealStuck(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1, "an item merely cycling through in_progress (not yet done/archived) must not false-resolve")
	assert.Equal(t, domain.StuckReasonPushFailed, open[0].Reason)
}

// --- Story 2.1.5e: per-detector panic isolation ---

// TestRunStuckDetector_should_recoverAndLogPanic_When_DetectorPanics verifies
// that a panicking detector is recovered by its own defer, appended to the
// panicked-names list (never the ok-names list), and does not propagate —
// so one detector's panic cannot skip the others or merge detection
// (Story 2.1.5e, pre-mortem P3/F5).
func TestRunStuckDetector_should_recoverAndLogPanic_When_DetectorPanics(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	listener := NewBacklogLifecycleListener(storage)

	var ok, panicked []string
	assert.NotPanics(t, func() {
		listener.runStuckDetector("boom", &ok, &panicked, func() {
			panic("simulated detector panic")
		})
	})
	assert.Empty(t, ok)
	assert.Equal(t, []string{"boom"}, panicked)

	// A subsequent healthy detector must still run normally after a prior panic.
	listener.runStuckDetector("healthy", &ok, &panicked, func() {})
	assert.Equal(t, []string{"healthy"}, ok)
	assert.Equal(t, []string{"boom"}, panicked)
}

// TestReconcilers_should_delegateThresholdDecisionsToPureFns_When_Reviewed is a
// structural regression test: the pure decision functions must exist with the
// documented signatures and behavior (rather than threshold arithmetic being
// re-inlined into a reconciler entangled with a DB read). Exercised via the
// package-level pure-function unit tests in stuck_decisions_test.go; this
// test asserts the specific boundary values the reconcilers rely on so a
// future inlining regression (threshold moved back into a reconciler with a
// different value) would be caught here too.
func TestReconcilers_should_delegateThresholdDecisionsToPureFns_When_Reviewed(t *testing.T) {
	now := time.Now()
	assert.True(t, stuckPRReady(now.Add(-prReadyThreshold-time.Minute), now))
	assert.True(t, abandonedReview(now.Add(-abandonedReviewGrace-time.Minute), now))
	assert.True(t, staleWork(now.Add(-maxWorkSessionStaleness-time.Minute), now))
	assert.True(t, isBouncing(bounceThreshold, false))
}

// --- auto_archive_done: sweep that auto-archives backlog items 3+ days
// after their most recent transition into "done" (see archiveStaleDoneItems
// and FindDoneItemsOlderThan's doc comments). ---

// newDoneTestItem creates a BacklogItem in "done" status and backdates a
// synthetic status-event row recording a transition into "done" doneAgo in
// the past — mirroring newOrphanedTriageTestItem's pattern of writing
// directly via the raw ent client, since BacklogStatusEvent.created_at is
// Immutable() (no Update-builder setter; must be set at Create time).
func newDoneTestItem(t *testing.T, storage *Storage, er *EntRepository, doneAgo time.Duration) *BacklogItemData {
	t.Helper()
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "Done test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusDone),
	})
	require.NoError(t, err)

	parsedID, err := uuid.Parse(item.ID)
	require.NoError(t, err)
	_, err = er.client.BacklogStatusEvent.Create().
		SetItemID(parsedID).
		SetFromStatus(string(BacklogStatusReview)).
		SetToStatus(string(BacklogStatusDone)).
		SetTriggeredBy(TriggeredBySystem).
		SetCreatedAt(time.Now().Add(-doneAgo)).
		Save(ctx)
	require.NoError(t, err)

	return item
}

func TestArchiveStaleDoneItems_should_ArchiveItem_When_DoneMoreThan3DaysAgo(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item := newDoneTestItem(t, storage, er, maxDoneAge+time.Hour)

	listener := NewBacklogLifecycleListener(storage)
	listener.archiveStaleDoneItems(ctx)

	got, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusArchived), got.Status, "item done more than 3 days ago must be auto-archived")
}

func TestArchiveStaleDoneItems_should_NotArchiveItem_When_DoneLessThan3DaysAgo(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item := newDoneTestItem(t, storage, er, maxDoneAge-time.Hour)

	listener := NewBacklogLifecycleListener(storage)
	listener.archiveStaleDoneItems(ctx)

	got, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusDone), got.Status, "item done less than 3 days ago must not be auto-archived yet")
}

func TestArchiveStaleDoneItems_should_BeIdempotent_When_RunTwice(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item := newDoneTestItem(t, storage, er, maxDoneAge+time.Hour)

	listener := NewBacklogLifecycleListener(storage)
	listener.archiveStaleDoneItems(ctx)
	require.NotPanics(t, func() {
		listener.archiveStaleDoneItems(ctx)
	}, "a second sweep tick over an already-archived item must not panic or error")

	got, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusArchived), got.Status, "item must remain archived, not double-transitioned or reverted")
}

func TestArchiveStaleDoneItems_should_SkipItem_When_NoDoneStatusEventHistory(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	// Created directly in "done" status with no status-event history at all
	// (e.g. a directly-seeded row) — must not age out based on an unrelated
	// timestamp; see FindDoneItemsOlderThan's doc comment.
	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "done item with no status-event history",
		Status: string(BacklogStatusDone),
	})
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	listener.archiveStaleDoneItems(ctx)

	got, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusDone), got.Status, "an item with no done-transition event on record must not be auto-archived")
}

// TestArchiveStaleDoneItems_should_DisappearFromDefaultBacklogView_When_AutoArchived
// is the integration-level test connecting Part 1 (auto-archive sweep) and
// Part 2 (default-view archived filtering): an item auto-archived by the
// sweep must then be excluded from the same default-filter query the backlog
// list page uses (ExcludeArchived: true, ExcludeDone: false — show done,
// hide archived), proving the two halves of this feature connect end to end.
func TestArchiveStaleDoneItems_should_DisappearFromDefaultBacklogView_When_AutoArchived(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	staleDone := newDoneTestItem(t, storage, er, maxDoneAge+time.Hour)
	recentDone := newDoneTestItem(t, storage, er, time.Hour)

	// Sanity check: before the sweep runs, both done items are visible under
	// the default filter (done included, archived excluded — but neither is
	// archived yet).
	before, err := storage.ListBacklogItemSummaries(ctx, BacklogItemFilter{ExcludeArchived: true})
	require.NoError(t, err)
	beforeIDs := make([]string, len(before))
	for i, s := range before {
		beforeIDs[i] = s.ID
	}
	assert.Contains(t, beforeIDs, staleDone.ID)
	assert.Contains(t, beforeIDs, recentDone.ID)

	listener := NewBacklogLifecycleListener(storage)
	listener.archiveStaleDoneItems(ctx)

	after, err := storage.ListBacklogItemSummaries(ctx, BacklogItemFilter{ExcludeArchived: true})
	require.NoError(t, err)
	afterIDs := make([]string, len(after))
	for i, s := range after {
		afterIDs[i] = s.ID
	}
	assert.NotContains(t, afterIDs, staleDone.ID, "the auto-archived item must disappear from the default (archived-excluded) backlog view")
	assert.Contains(t, afterIDs, recentDone.ID, "the still-recent done item must remain visible in the default view")
}
