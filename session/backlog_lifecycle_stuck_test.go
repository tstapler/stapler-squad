package session

import (
	"context"
	"errors"
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

// TestSelfHealSweep_should_notResolveEventShapedRows_When_StatusVaries verifies
// rework_cap and push_failed rows — written with expectedStatus=<current> at
// the event site, no fixed anchor — are excluded from the status sweep
// entirely and rely only on their explicit event-site ResolveStuck calls.
func TestSelfHealSweep_should_notResolveEventShapedRows_When_StatusVaries(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "Event-shaped rows item",
		Status: string(BacklogStatusDone), // arbitrary/unrelated status
	})
	require.NoError(t, err)
	applied, err := er.MarkStuck(ctx, item.ID, domain.StuckReasonReworkCap, BacklogStatusDone, "cap hit")
	require.NoError(t, err)
	require.True(t, applied)
	applied, err = er.MarkStuck(ctx, item.ID, domain.StuckReasonPushFailed, BacklogStatusDone, "push failed")
	require.NoError(t, err)
	require.True(t, applied)

	listener := NewBacklogLifecycleListener(storage)
	listener.selfHealStuck(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 2, "event-shaped rows must never be touched by the status sweep")
	reasons := map[domain.StuckReason]bool{}
	for _, row := range open {
		reasons[row.Reason] = true
	}
	assert.True(t, reasons[domain.StuckReasonReworkCap])
	assert.True(t, reasons[domain.StuckReasonPushFailed])
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
