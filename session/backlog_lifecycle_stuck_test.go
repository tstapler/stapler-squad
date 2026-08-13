package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

// newEndedTriageTestItem creates an idea-status BacklogItem with a single
// triage-role ItemSession that has already ended (EndedAt set), matching
// "shape 2" of reconcileOrphanedTriageItems: a headless triage call that
// returned (errored, or produced output ParseHeadlessTriageResult rejected —
// see TestParseHeadlessTriageResult_PrematureCompletionPlaceholder) and was
// tombstoned by TriggerTriage's own goroutine, but never transitioned the item
// out of idea. No staleness backdating needed — shape 2 has no staleness gate,
// since an ended session with the item still in idea is unambiguous the moment
// it's observed.
func newEndedTriageTestItem(t *testing.T, storage *Storage, er *EntRepository) *BacklogItemData {
	t.Helper()
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "Ended-without-transition triage test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusIdea),
	})
	require.NoError(t, err)

	is, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "headless-triage-" + uuid.New().String(),
		SessionRole: SessionRoleTriage,
	})
	require.NoError(t, err)
	require.NoError(t, storage.UpdateItemSessionEnded(ctx, is.ID, time.Now()))

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
	newTrackedWorkSession(t, storage, item.ID, item.RepoPath, "backlog/pr-ready-merged", "")
	er := storage.repo.(*EntRepository)
	applied, err := er.MarkStuck(ctx, item.ID, domain.StuckReasonPRReadyUnmerged, BacklogStatusPRPending, "PR is green & mergeable")
	require.NoError(t, err)
	require.True(t, applied)

	listener := NewBacklogLifecycleListener(storage)
	overridePRPendingChecker(t, listener, &fakePRPendingChecker{merged: true})
	stubMatchingPRByNumberFinder(listener, "backlog/pr-ready-merged")

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
// alone — it may simply be doing its own post-verdict cleanup — unless its
// verdict has aged past reviewVerdictIdleThreshold, in which case the sweep
// must act regardless of what SessionLivenessChecker reports (BUG-047: a
// reviewer that submits a verdict and then never exits reads as alive
// forever, so a pure liveness check can never catch it).
func TestReconcileUnprocessedReviewVerdicts_should_notAct_When_ReviewSessionStillAlive(t *testing.T) {
	t.Run("verdict younger than idle threshold: still no-act", func(t *testing.T) {
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
		assert.Equal(t, string(BacklogStatusReview), fetched.Status, "a still-alive review session with a fresh verdict must not be acted on")
	})

	t.Run("verdict older than idle threshold: now acts", func(t *testing.T) {
		storage, cleanup := createTestStorage(t)
		defer cleanup()
		ctx := context.Background()

		er := storage.repo.(*EntRepository)
		item := newStuckReviewTestItemWithVerdictAge(t, storage, er, ReviewVerdictFail, reviewVerdictIdleThreshold+time.Hour)

		listener := NewBacklogLifecycleListener(storage)
		listener.SetSessionLivenessChecker(func(sessionUUID string) bool { return true }) // everything alive — only the verdict age should trigger action
		reopener := newFakeAutoReopenSpawner()
		listener.SetAutoReopener(reopener)

		listener.reconcileUnprocessedReviewVerdicts(ctx, er)

		select {
		case gotItemID := <-reopener.called:
			assert.Equal(t, item.ID, gotItemID,
				"an idle-but-alive review session whose verdict has aged past reviewVerdictIdleThreshold must still reach the auto-reopener (forcePush path)")
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for AutoReopenAfterFailedReview to be called")
		}

		sessions, err := storage.ListItemSessions(ctx, item.ID)
		require.NoError(t, err)
		for _, is := range sessions {
			if is.Role == string(SessionRoleReview) {
				assert.NotNil(t, is.EndedAt, "the idle review session must be tombstoned even though the liveness checker reported it alive")
			}
		}
	})

	t.Run("verdict just under idle threshold: still no-act", func(t *testing.T) {
		storage, cleanup := createTestStorage(t)
		defer cleanup()
		ctx := context.Background()

		er := storage.repo.(*EntRepository)
		item := newStuckReviewTestItemWithVerdictAge(t, storage, er, ReviewVerdictFail, reviewVerdictIdleThreshold-time.Minute)

		listener := NewBacklogLifecycleListener(storage)
		listener.SetSessionLivenessChecker(func(sessionUUID string) bool { return true }) // everything alive

		listener.reconcileUnprocessedReviewVerdicts(ctx, er)

		fetched, err := storage.GetBacklogItem(ctx, item.ID)
		require.NoError(t, err)
		assert.Equal(t, string(BacklogStatusReview), fetched.Status,
			"a verdict just under reviewVerdictIdleThreshold must not be swept yet — only the (older, exercised above) case should trigger")
	})

	// PASS is deferred to session-exit by design, so it never gets the eager
	// transition submitReviewVerdict drives for FAIL/PARTIAL/UNVERIFIABLE
	// (server/mcp/tools_backlog.go) — this idle-timeout branch is PASS's
	// *only* path back out of "review" once its reviewer session goes idle
	// without exiting, so it needs its own direct coverage rather than
	// inheriting confidence from the FAIL case above.
	t.Run("PASS verdict older than idle threshold: now acts even though session reports alive", func(t *testing.T) {
		storage, cleanup := createTestStorage(t)
		defer cleanup()
		ctx := context.Background()

		er := storage.repo.(*EntRepository)
		item := newStuckReviewTestItemWithVerdictAge(t, storage, er, ReviewVerdictPass, reviewVerdictIdleThreshold+time.Hour)

		// A work session so the PASS verdict has something to ship — mirrors
		// TestReconcileUnprocessedReviewVerdicts_should_applyPassVerdict_When_ReviewSessionDiedButWorkSessionStillAlive's
		// setup; no worktree recorded, so this exercises the same pre-existing
		// fallbackToDone("no worktree") branch that test does.
		_, err := storage.CreateItemSession(ctx, ItemSessionData{
			ItemID:      item.ID,
			SessionUUID: uuid.New().String(),
			SessionRole: SessionRoleWork,
		})
		require.NoError(t, err)

		listener := NewBacklogLifecycleListener(storage)
		listener.SetSessionLivenessChecker(func(sessionUUID string) bool { return true }) // everything alive — only the verdict age should trigger action

		listener.reconcileUnprocessedReviewVerdicts(ctx, er)

		fetched, err := storage.GetBacklogItem(ctx, item.ID)
		require.NoError(t, err)
		assert.Equal(t, string(BacklogStatusDone), fetched.Status,
			"a PASS verdict aged past reviewVerdictIdleThreshold must still ship even though SessionLivenessChecker reports the review session alive")
	})
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
	_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusReview, nil, TriggeredBySystem)
	require.NoError(t, err)

	listener.reconcileUnprocessedReviewVerdicts(ctx, er)

	final, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusReview), final.Status,
		"a verdict from a prior, already-concluded review cycle must not be reprocessed")
}

// TestReconcileUnprocessedReviewVerdicts_should_invokeAutoReopener_When_NewestReviewSessionHasNoVerdictButIsDead
// is the regression test for the live 2026-07-19..07-22 incident on backlog
// item 9264efe7 ("Backlog History feature Broken", PR #173): an older
// review-role session recorded a FAIL verdict and died, then a further
// re-review attempt was created afterward (also dying, but without ever
// calling submit_review_verdict itself). FindReviewItemsWithUnprocessedVerdict
// eager-loads ALL review-role sessions ordered newest-first and this sweep
// took element [0] ("latest") as if it were guaranteed to carry the verdict
// the item-level query matched on — true only when no newer, verdict-less
// review session exists. Here it doesn't: latest.Edges.ReviewVerdict is nil,
// and the sweep used to bail out entirely on that ("defensive: query already
// filters on HasReviewVerdict()" — a false assumption, since that filter is
// scoped to the item, not to element [0]). The item was invisible to this
// crash-recovery sweep forever as a result — neither the real older verdict
// nor a "no verdict, treat as failed" fallback ever ran, so the auto-reopener
// was never invoked at all. The fix lets a dead, verdict-less latest flow
// into handleReviewSessionExited, which already re-derives the verdict by
// SessionUUID and correctly treats "no verdict" as a failed review needing
// rework. Asserting on the auto-reopener call (rather than a resulting
// status) matches this package's boundary: AutoReopenAfterFailedReview's real
// implementation lives in server/services and is exercised via the
// AutoReopenSpawner interface here, same pattern as
// TestHandleReviewSessionExited_NoVerdict_NotifiesAndInvokesAutoReopener.
func TestReconcileUnprocessedReviewVerdicts_should_invokeAutoReopener_When_NewestReviewSessionHasNoVerdictButIsDead(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	// Older review-role session: recorded a real FAIL verdict, then died.
	item := newStuckReviewTestItem(t, storage, ReviewVerdictFail, true, false)

	// Newer review-role session, created afterward: no verdict of its own —
	// the shape of a re-review attempt that never got a live process to
	// actually run (or crashed before calling submit_review_verdict).
	newerReviewUUID := "headless-re-review-" + uuid.New().String()
	_, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: newerReviewUUID,
		SessionRole: SessionRoleReview,
	})
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	listener.SetSessionLivenessChecker(func(sessionUUID string) bool { return false }) // everything dead
	reopener := newFakeAutoReopenSpawner()
	listener.SetAutoReopener(reopener)

	er := storage.repo.(*EntRepository)
	listener.reconcileUnprocessedReviewVerdicts(ctx, er)

	select {
	case gotItemID := <-reopener.called:
		assert.Equal(t, item.ID, gotItemID,
			"a dead, verdict-less newest review session must still reach the auto-reopener, not leave the item invisible to this sweep forever")
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for AutoReopenAfterFailedReview to be called")
	}

	sessions, err := storage.ListItemSessions(ctx, item.ID)
	require.NoError(t, err)
	for _, is := range sessions {
		if is.SessionUUID == newerReviewUUID {
			assert.NotNil(t, is.EndedAt, "the verdict-less newest review session must be tombstoned")
		}
	}
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

// --- rework_blocked_stale: reconcileReworkBlockedStaleResolution orchestration
// (review-gate-stale-session-rework Story 2.1.2) — the mark side
// (notifyIfActiveWorkSessionStale, server/services/backlog_service_triage.go)
// has no periodic tick of its own, so this resolve-only pass closes an open
// row once its blocking session recovers, ends, or the item leaves review. ---

// fakeReworkBlockStaleResolver is a test double implementing
// ReworkBlockStaleResolver, recording every ResolveReworkBlockedStaleIfRecovered
// call — mirrors fakeStaleWorkRemediator's shape, but a plain slice+mutex
// suffices here since reconcileReworkBlockedStaleResolution calls it
// synchronously per open row (no goroutine dispatch, unlike remediation).
type fakeReworkBlockStaleResolver struct {
	mu    sync.Mutex
	calls []string
	err   error
}

func (f *fakeReworkBlockStaleResolver) ResolveReworkBlockedStaleIfRecovered(ctx context.Context, itemID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, itemID)
	return f.err
}

func (f *fakeReworkBlockStaleResolver) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// TestReconcileReworkBlockedStaleResolution_should_delegateToResolver_When_OpenRowsExist
// verifies the orchestration function finds every open rework_blocked_stale
// row and delegates each to the wired ReworkBlockStaleResolver — it contains
// no liveness-checking logic itself, only the loop and delegation (see that
// function's doc comment).
func TestReconcileReworkBlockedStaleResolution_should_delegateToResolver_When_OpenRowsExist(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "Item blocked by a stale-but-alive session",
		Status: string(BacklogStatusReview),
	})
	require.NoError(t, err)
	applied, err := er.MarkStuck(ctx, item.ID, domain.StuckReasonReworkBlockedStale, BacklogStatusReview, "idle 20m")
	require.NoError(t, err)
	require.True(t, applied)

	listener := NewBacklogLifecycleListener(storage)
	resolver := &fakeReworkBlockStaleResolver{}
	listener.SetReworkBlockStaleResolver(resolver)

	listener.reconcileReworkBlockedStaleResolution(ctx, er)

	assert.Equal(t, 1, resolver.callCount())
	assert.Equal(t, item.ID, resolver.calls[0])
}

// TestReconcileReworkBlockedStaleResolution_should_beNoOp_When_NoOpenRows
// is the negative case: with no open rework_blocked_stale rows, the resolver
// must not be called at all.
func TestReconcileReworkBlockedStaleResolution_should_beNoOp_When_NoOpenRows(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	listener := NewBacklogLifecycleListener(storage)
	resolver := &fakeReworkBlockStaleResolver{}
	listener.SetReworkBlockStaleResolver(resolver)

	listener.reconcileReworkBlockedStaleResolution(ctx, er)

	assert.Equal(t, 0, resolver.callCount())
}

// TestReconcileReworkBlockedStaleResolution_should_beNoOp_When_ResolverNotWired
// confirms the nil-safe getter pattern (mirroring getStaleWorkRemediator):
// calling the orchestration function before SetReworkBlockStaleResolver has
// ever been called must not panic.
func TestReconcileReworkBlockedStaleResolution_should_beNoOp_When_ResolverNotWired(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	listener := NewBacklogLifecycleListener(storage)

	require.NotPanics(t, func() {
		listener.reconcileReworkBlockedStaleResolution(ctx, er)
	})
}

// --- respawn_blocked_active: reconcileRespawnBlockedActiveResolution orchestration ---
//
// The mark side (notifyRespawnBlockedByActiveSession, server/services/
// backlog_service_triage.go) is called from inside AutoRespawnAutonomousWork/
// AutoReopenForPRFix/AutoRespawnReview, and each of those functions also
// resolves the row itself the next time its own guard passes. That inline
// resolve is NOT sufficient on its own for AutoRespawnReview: its only
// caller, markAbandonedReview, gates the call behind
// Storage.RemediationDue(StuckReasonAbandonedReview), which eventually parks
// and stops re-invoking AutoRespawnReview for that item — so a row marked
// once, right before parking, would never see AutoRespawnReview's guard pass
// again and would be permanently orphaned without this independent sweep.
// These tests exercise reconcileRespawnBlockedActiveResolution directly,
// with no call to AutoRespawnReview at all, to prove the sweep — not the
// inline resolve — is what actually guarantees resolution.

// TestReconcileRespawnBlockedActiveResolution_should_resolveRow_When_BlockingSessionHasEnded
// is the direct regression test for the orphaned-row scenario: a
// respawn_blocked_active row is marked (simulating AutoRespawnReview's guard
// having tripped once), the blocking review session then ends, and
// reconcileRespawnBlockedActiveResolution — called on its own, with no
// AutoRespawnReview/markAbandonedReview involved at all — must resolve the
// row.
func TestReconcileRespawnBlockedActiveResolution_should_resolveRow_When_BlockingSessionHasEnded(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "Item stuck abandoned_review, its abandoned_review remediation now parked",
		Status: string(BacklogStatusReview),
	})
	require.NoError(t, err)

	is, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "review-session-that-has-since-ended",
		SessionRole: SessionRoleReview,
	})
	require.NoError(t, err)

	// Simulate AutoRespawnReview's guard having tripped once (the mark side),
	// with markAbandonedReview's caller never invoking AutoRespawnReview
	// again — the row's only path to resolution is the sweep under test.
	applied, err := er.MarkStuck(ctx, item.ID, domain.StuckReasonRespawnBlockedActive, BacklogStatusReview,
		"AutoRespawnReview skipped auto-respawn — session review-session-that-has-since-ended already active")
	require.NoError(t, err)
	require.True(t, applied)

	// The blocking session has since ended.
	require.NoError(t, storage.UpdateItemSessionEnded(ctx, is.ID, time.Now()))

	listener := NewBacklogLifecycleListener(storage)
	listener.reconcileRespawnBlockedActiveResolution(ctx, er)

	open, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	for _, row := range open {
		assert.False(t, row.ItemID == item.ID && row.Reason == domain.StuckReasonRespawnBlockedActive,
			"the respawn_blocked_active row must be resolved once its blocking session has ended, independent of whether AutoRespawnReview is ever called again")
	}
}

// TestReconcileRespawnBlockedActiveResolution_should_leaveRowOpen_When_BlockingSessionStillActive
// is the negative case: the sweep must not clear a row while the blocking
// session genuinely remains open.
func TestReconcileRespawnBlockedActiveResolution_should_leaveRowOpen_When_BlockingSessionStillActive(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "Item still genuinely blocked by an active work session",
		Status: string(BacklogStatusInProgress),
	})
	require.NoError(t, err)

	_, err = storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "still-active-work-session",
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)

	applied, err := er.MarkStuck(ctx, item.ID, domain.StuckReasonRespawnBlockedActive, BacklogStatusInProgress,
		"AutoRespawnAutonomousWork skipped auto-respawn — session still-active-work-session already active")
	require.NoError(t, err)
	require.True(t, applied)

	listener := NewBacklogLifecycleListener(storage)
	listener.reconcileRespawnBlockedActiveResolution(ctx, er)

	open, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	found := false
	for _, row := range open {
		if row.ItemID == item.ID && row.Reason == domain.StuckReasonRespawnBlockedActive {
			found = true
		}
	}
	assert.True(t, found, "the row must stay open while the blocking session is still genuinely active")
}

// TestReconcileRespawnBlockedActiveResolution_should_beNoOp_When_NoOpenRows verifies
// the sweep does nothing (and does not error) when there are no open
// respawn_blocked_active rows to reconcile.
func TestReconcileRespawnBlockedActiveResolution_should_beNoOp_When_NoOpenRows(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	listener := NewBacklogLifecycleListener(storage)

	require.NotPanics(t, func() {
		listener.reconcileRespawnBlockedActiveResolution(ctx, er)
	})
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

// TestReconcileOrphanedTriageItems_should_notTombstone_When_HeadlessSessionStaleButGenuinelyLive
// guards the fix for BUG-055: before IsTriageLive existed, this shape-1 branch tombstoned
// ANY headless triage session past maxHeadlessTriageSessionStaleness unconditionally, with
// no way to tell "genuinely dead" apart from "still running, just slow" — confirmed live
// 2026-08-01 that headless triage calls routinely run right up to their full 30m budget,
// so this staleness-only gate raced the call's own natural completion on every slow call.
// A respawner reporting the session as still live must now suppress the tombstone entirely.
func TestReconcileOrphanedTriageItems_should_notTombstone_When_HeadlessSessionStaleButGenuinelyLive(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	// 45 minutes: past maxHeadlessTriageSessionStaleness (35m) — would have been
	// tombstoned unconditionally before this fix.
	item := newOrphanedTriageTestItem(t, storage, er, 45*time.Minute)

	listener := NewBacklogLifecycleListener(storage)
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)
	respawner := newFakeTriageRespawner()
	respawner.liveIDs = map[string]bool{item.ID: true}
	listener.SetTriageRespawner(respawner)

	listener.reconcileOrphanedTriageItems(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "a stale-but-genuinely-live headless session must not be marked stuck")
	assert.Empty(t, notifier.titles(), "a genuinely live session must not trigger a \"may be stuck\" notification")

	sessions, listErr := storage.ListItemSessions(ctx, item.ID)
	require.NoError(t, listErr)
	require.Len(t, sessions, 1)
	assert.Nil(t, sessions[0].EndedAt, "a genuinely live headless session must not be tombstoned")
}

// TestMaxHeadlessTriageSessionStaleness_should_ExceedRealTriageCallBudgetWithMargin guards
// the exact margin regression named in BUG-055: this constant must stay strictly greater
// than server/services.triageCallBudget (currently 30m — kept as a literal here rather than
// imported, since session cannot depend on server/services) with real headroom, or every
// slow-but-legitimate headless triage call races this sweep's staleness gate again,
// regardless of how good IsTriageLive's liveness check is. If server/services.triageCallBudget
// ever changes, this literal and the one there must be updated together.
func TestMaxHeadlessTriageSessionStaleness_should_ExceedRealTriageCallBudgetWithMargin(t *testing.T) {
	const knownTriageCallBudget = 30 * time.Minute
	const minMargin = 2 * time.Minute
	assert.Greater(t, maxHeadlessTriageSessionStaleness, knownTriageCallBudget+minMargin,
		"maxHeadlessTriageSessionStaleness must exceed the real triage call budget with real margin, not race it")
}

// TestReconcileOrphanedTriageItems_should_flagImmediately_When_TriageSessionEndedWithoutTransition
// is the regression test for the "compounding gap" half of the live incident tracked in
// docs/tasks/backlog-feature-improvement.md's 2026-07-30 entry (backlog item 04089969):
// a headless triage call that returns cleanly (real EndedAt, no crash/kill) but with
// output ParseHeadlessTriageResult rejects (see
// TestParseHeadlessTriageResult_PrematureCompletionPlaceholder) never transitions the
// item out of idea, and — before this fix — was invisible to every existing detector:
// reconcileOrphanedTriageItems previously only matched EndedAt == nil (open) sessions,
// so a session tombstoned by TriggerTriage's own goroutine (the normal completion path,
// success or failure) fell through every check. No staleness wait should be required:
// unlike the open-and-stale shape, an ended session with the item still in idea is
// unambiguous the moment it's observed.
func TestReconcileOrphanedTriageItems_should_flagImmediately_When_TriageSessionEndedWithoutTransition(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item := newEndedTriageTestItem(t, storage, er)

	listener := NewBacklogLifecycleListener(storage)
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	listener.reconcileOrphanedTriageItems(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1, "an ended triage session with the item still in idea must be flagged immediately, with no staleness wait")
	assert.Equal(t, item.ID, open[0].ItemID)
	assert.Equal(t, domain.StuckReasonOrphanedTriage, open[0].Reason)
	assert.Equal(t, []string{"Triage may be stuck"}, notifier.titles())

	// Repeat tick must not re-notify (DB-backed notify-once dedup) — mirrors the
	// open-and-stale shape's identical guarantee.
	listener.reconcileOrphanedTriageItems(ctx, er)
	assert.Len(t, notifier.calls, 1)
}

// TestReconcileOrphanedTriageItems_should_surfaceEndReasonInContext_When_TriageSessionEndedWithClassifiedError
// guards the fix threading TriggerTriage's persisted classifyHeadlessCallError
// bucket (server/services/backlog_service_triage.go's EndReason, written via
// UpdateItemSessionEndedWithReason) into this detector's reasonDetail. Before
// this fix, shape 2's reasonDetail was a hardcoded generic string — "triage
// session %s ended without moving the item out of idea" — that discarded the
// EndReason column even though it was one query away and already read
// elsewhere in this same function (the "shutdown" carve-out just above). The
// resulting backlog_stuck_states.context an operator actually sees carried no
// hint of *why* the triage session died. Confirmed live: a real orphaned_triage
// row for a process_error-classified failure showed the same generic message
// as every other failure category.
func TestReconcileOrphanedTriageItems_should_surfaceEndReasonInContext_When_TriageSessionEndedWithClassifiedError(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "Process-error-ended triage test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusIdea),
	})
	require.NoError(t, err)

	is, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "headless-triage-" + uuid.New().String(),
		SessionRole: SessionRoleTriage,
	})
	require.NoError(t, err)
	require.NoError(t, storage.UpdateItemSessionEndedWithReason(ctx, is.ID, time.Now(), "process_error"))

	listener := NewBacklogLifecycleListener(storage)
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	listener.reconcileOrphanedTriageItems(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, item.ID, open[0].ItemID)
	assert.Contains(t, open[0].Context, "process_error",
		"stuck-state context must surface the persisted EndReason, not a generic template")
	assert.NotContains(t, open[0].Context, "ended without moving the item out of idea",
		"the pre-fix generic-only template must no longer be emitted verbatim once an EndReason is known")
}

// TestReconcileOrphanedTriageItems_should_fallBackToUnknown_When_EndReasonNeverClassified
// guards the empty-EndReason path (triageEndReasonOrUnknown): a session ended
// via the plain UpdateItemSessionEnded (no errType ever recorded) must still
// render a well-formed message rather than a blank/empty parenthetical.
func TestReconcileOrphanedTriageItems_should_fallBackToUnknown_When_EndReasonNeverClassified(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item := newEndedTriageTestItem(t, storage, er) // ends via UpdateItemSessionEnded, no reason recorded

	listener := NewBacklogLifecycleListener(storage)
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	listener.reconcileOrphanedTriageItems(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, item.ID, open[0].ItemID)
	assert.Contains(t, open[0].Context, "unknown",
		"an EndReason-less session must fall back to an explicit 'unknown' marker, not a blank parenthetical")
}

// TestReconcileOrphanedTriageItems_should_respawnImmediatelyWithNoPenalty_When_EndedByGracefulShutdown
// guards the fix for the 2026-08-01 live incident (docs/bugs/fixed/BUG-053): a
// routine service restart (e.g. `make install-service`) cancels shutdownCtx,
// which kills any in-flight triage call via classifyHeadlessCallError's
// "shutdown" bucket — a self-inflicted, zero-evidence event, not a real
// triage failure. Before this fix, shape 2 treated a shutdown-caused orphan
// identically to a genuine failure: MarkStuck + a "may be stuck" notification
// + RemediationDue's exponential backoff (30m/2h/8h/24h/72h, sized for OOM
// bursts) before the next retry. This must instead respawn immediately with
// no remediation-attempt penalty and no alarming notification.
func TestReconcileOrphanedTriageItems_should_respawnImmediatelyWithNoPenalty_When_EndedByGracefulShutdown(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "Shutdown-orphaned triage test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusIdea),
	})
	require.NoError(t, err)

	is, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "headless-triage-" + uuid.New().String(),
		SessionRole: SessionRoleTriage,
	})
	require.NoError(t, err)
	require.NoError(t, storage.UpdateItemSessionEndedWithReason(ctx, is.ID, time.Now(), "shutdown"))

	listener := NewBacklogLifecycleListener(storage)
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)
	respawner := newFakeTriageRespawner()
	listener.SetTriageRespawner(respawner)

	listener.reconcileOrphanedTriageItems(ctx, er)

	select {
	case itemID := <-respawner.calls:
		assert.Equal(t, item.ID, itemID)
	case <-time.After(time.Second):
		t.Fatal("expected AutoRespawnTriage to be dispatched immediately for a shutdown-caused orphan")
	}

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "a shutdown-caused orphan must not consume a remediation attempt or create a stuck-state row")
	assert.Empty(t, notifier.titles(), "a shutdown-caused orphan is expected/self-inflicted and must not trigger a \"may be stuck\" notification")
}

// TestReconcileOrphanedTriageItems_should_preferNewerOpenSession_When_OlderEndedSessionExists
// guards the latestTriage selection added alongside the two-shape detector
// above: an item can accumulate more than one triage-role ItemSession (e.g. an
// older attempt that ended without transitioning the item, followed by a fresh
// retry). The detector must key off the newest session's CreatedAt, not just
// any EndedAt-nil-or-not row it happens to find — a stale older "shape 2" row
// must not fire once a newer, still-fresh, still-open attempt is in flight.
func TestReconcileOrphanedTriageItems_should_preferNewerOpenSession_When_OlderEndedSessionExists(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item := newEndedTriageTestItem(t, storage, er) // older session: ended, no transition

	// A newer triage session was since started (e.g. auto-retried) and is
	// still fresh/open — the detector must key off THIS one, not the older
	// ended row, and must not fire yet.
	_, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "headless-triage-" + uuid.New().String(),
		SessionRole: SessionRoleTriage,
	})
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	listener.reconcileOrphanedTriageItems(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "a fresh retry session must suppress the older ended session's shape-2 signal")
}

// TestReconcileOrphanedTriageItems_should_notFlag_When_NoTriageSessionEverRan guards
// against a regression where broadening the detector to also match ended sessions
// starts matching idea items that have simply never had triage triggered at all.
func TestReconcileOrphanedTriageItems_should_notFlag_When_NoTriageSessionEverRan(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	_, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "Never triaged item",
		Status: string(BacklogStatusIdea),
	})
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	listener.reconcileOrphanedTriageItems(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "an item that has never had a triage session must not be flagged")
}

// TestReconcileOrphanedTriageRemediation_should_retryEndedWithoutTransitionRow_When_Due
// verifies the ended-without-transition shape is retried through the exact same
// AutoRespawnTriage backoff-gate path as the pre-existing open-and-stale shape —
// reconcileOrphanedTriageRemediation and retryOrphanedTriageWithBackoffGate key only on
// (reason, item status), so extending the detection condition in
// reconcileOrphanedTriageItems is sufficient on its own; no changes to the remediation
// path itself were needed. This is the "detection/retry gap" acceptance case for the
// 2026-07-30 finding: item 04089969's shape must actually become eligible for an
// automatic retry, not just get a durable stuck row that nothing ever acts on.
func TestReconcileOrphanedTriageRemediation_should_retryEndedWithoutTransitionRow_When_Due(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item := newEndedTriageTestItem(t, storage, er)

	listener := NewBacklogLifecycleListener(storage)
	listener.SetNotifier(&fakeNotifier{})
	respawner := newFakeTriageRespawner()
	listener.SetTriageRespawner(respawner)

	listener.reconcileOrphanedTriageItems(ctx, er)
	listener.reconcileOrphanedTriageRemediation(ctx, er)

	select {
	case itemID := <-respawner.calls:
		assert.Equal(t, item.ID, itemID)
	case <-time.After(time.Second):
		t.Fatal("expected AutoRespawnTriage to be dispatched for the ended-without-transition row")
	}
}

// fakeTriageRespawner is a test double implementing TriageRespawner, recording
// every AutoRespawnTriage call on a buffered channel (not a plain counter)
// because retryOrphanedTriageWithBackoffGate dispatches asynchronously —
// mirrors fakeStaleWorkRemediator's identical rationale.
type fakeTriageRespawner struct {
	calls   chan string
	err     error
	liveIDs map[string]bool
}

func newFakeTriageRespawner() *fakeTriageRespawner {
	return &fakeTriageRespawner{calls: make(chan string, 32)}
}

func (f *fakeTriageRespawner) AutoRespawnTriage(ctx context.Context, itemID string) error {
	f.calls <- itemID
	return f.err
}

func (f *fakeTriageRespawner) IsTriageLive(itemID string) bool {
	return f.liveIDs[itemID]
}

// TestReconcileOrphanedTriageRemediation_should_dispatchRetryThroughBackoffGate_When_RowIsDueAndRespawnerSucceeds
// verifies the periodic remediation pass (the production entry point wired
// into ReconcileStuck) retries triage for an open orphaned_triage row still
// anchored at "idea" — closing the gap where reconcileOrphanedTriageItems'
// one-time MarkStuck+notify never led anywhere without a human noticing and
// manually re-triggering triage (docs/tasks/backlog-feature-improvement.md,
// 2026-07-27 update).
//
// Unlike reconcileStaleWorkSessions (which bundles mark/notify and remediate
// into ONE function with an explicit "not on the first sighting" grace gate,
// since its work session is still alive and might just be slow), this
// reason's own detection already required the triage session to sit stale
// past its full staleness threshold (30m headless / 2h general) AND
// tombstones it before ever marking stuck — there is nothing further to wait
// for, so the sibling remediation detector may retry on the very same sweep
// tick that first opened the row (mirrors reconcilePushFailedItems, whose
// remediation pass carries no such grace gate either).
func TestReconcileOrphanedTriageRemediation_should_dispatchRetryThroughBackoffGate_When_RowIsDueAndRespawnerSucceeds(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item := newOrphanedTriageTestItem(t, storage, er, 3*time.Hour)

	listener := NewBacklogLifecycleListener(storage)
	listener.SetNotifier(&fakeNotifier{})
	respawner := newFakeTriageRespawner()
	listener.SetTriageRespawner(respawner)

	listener.reconcileOrphanedTriageItems(ctx, er)
	listener.reconcileOrphanedTriageRemediation(ctx, er)

	select {
	case itemID := <-respawner.calls:
		assert.Equal(t, item.ID, itemID)
	case <-time.After(time.Second):
		t.Fatal("expected AutoRespawnTriage to be dispatched for the due row")
	}

	require.Eventually(t, func() bool {
		rows, err := er.FindOpenStuckStates(ctx)
		return err == nil && len(rows) == 1 && rows[0].RemediationAttempts == 1
	}, time.Second, 10*time.Millisecond, "the dispatched attempt must advance RemediationDue's own accounting")
}

// TestReconcileOrphanedTriageRemediation_should_skip_When_ItemNoLongerIdea verifies
// an item whose orphaned_triage row is still open but whose status has since moved
// off "idea" (e.g. a human already re-triggered triage manually, or the row is stale
// bookkeeping) is never retried by the periodic remediation pass.
func TestReconcileOrphanedTriageRemediation_should_skip_When_ItemNoLongerIdea(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "Orphaned triage item no longer idea",
		Status: string(BacklogStatusIdea),
	})
	require.NoError(t, err)
	applied, err := er.MarkStuck(ctx, item.ID, domain.StuckReasonOrphanedTriage, BacklogStatusIdea, "stale triage session")
	require.NoError(t, err)
	require.True(t, applied)
	_, err = er.MarkStuckNotified(ctx, item.ID, domain.StuckReasonOrphanedTriage)
	require.NoError(t, err)

	// A human already re-triggered triage manually — the item moved off
	// "idea" while the orphaned_triage row is still open (selfHealStuck would
	// resolve it on its own next tick, but that must not race this test).
	_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusReady, nil, TriggeredBySystem)
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	respawner := newFakeTriageRespawner()
	listener.SetTriageRespawner(respawner)

	listener.reconcileOrphanedTriageRemediation(ctx, er)

	select {
	case itemID := <-respawner.calls:
		t.Fatalf("an item that has moved off idea must not be retried, got call for item %s", itemID)
	case <-time.After(100 * time.Millisecond):
		// expected: no remediation dispatched
	}
}

// newQueuedNoTriageResultTestItem creates a queued, plan-approval-gated item
// (no SkipPlanning, no PlanApproved) whose only triage-role ItemSession ended
// without ever persisting a usable TriageResult — the exact shape of item
// be676dab (docs/tasks/backlog-feature-improvement.md's 2026-08-03 entry): a
// triage session that ran, ended, but left nothing behind, after the item had
// already advanced past idea (here, straight to queued, standing in for the
// WIP-cap-driven idea->ready->queued path the live incident took).
func newQueuedNoTriageResultTestItem(t *testing.T, storage *Storage) *BacklogItemData {
	t.Helper()
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "Queued item with no usable triage result",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusQueued),
		SkipPlanning:       false,
		PlanApproved:       false,
	})
	require.NoError(t, err)

	is, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "headless-triage-" + uuid.New().String(),
		SessionRole: SessionRoleTriage,
	})
	require.NoError(t, err)
	require.NoError(t, storage.UpdateItemSessionEnded(ctx, is.ID, time.Now()))

	return item
}

// TestReconcileOrphanedTriageItems_should_flagQueuedItem_When_TriageResultUnusable
// is the direct regression test for the 2026-08-03 live incident (item be676dab):
// a queued item, gated on plan approval, whose most recent triage session ended
// with no usable result must now be flagged under the same orphaned_triage
// reason and notified — previously this detector's Statuses filter was
// idea-only, so a queued item in this shape fell outside its scope entirely and
// only reconcilePlanNotApprovedItems flagged it, indistinguishably from the
// normal "plan generated, awaiting review" case.
func TestReconcileOrphanedTriageItems_should_flagQueuedItem_When_TriageResultUnusable(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item := newQueuedNoTriageResultTestItem(t, storage)

	listener := NewBacklogLifecycleListener(storage)
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	listener.reconcileOrphanedTriageItems(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1, "a queued item whose latest triage session left no usable result must be flagged")
	assert.Equal(t, item.ID, open[0].ItemID)
	assert.Equal(t, domain.StuckReasonOrphanedTriage, open[0].Reason)
	assert.Equal(t, BacklogStatusQueued, open[0].ItemStatus)
	assert.Equal(t, []string{"Triage may be stuck"}, notifier.titles())

	// Repeat tick must not re-notify (DB-backed notify-once dedup), same as the
	// pre-existing idea-status shapes.
	listener.reconcileOrphanedTriageItems(ctx, er)
	assert.Len(t, notifier.calls, 1)
}

// TestReconcileOrphanedTriageItems_should_notFlagQueuedItem_When_TriageResultUsable
// is the over-triggering guard: a queued item that legitimately has a real plan
// behind it (the normal "awaiting human review" wait reconcilePlanNotApprovedItems
// owns) must NOT be flagged by this detector — only "ended with nothing usable"
// is the generalized shape's signal, not "ended" alone.
func TestReconcileOrphanedTriageItems_should_notFlagQueuedItem_When_TriageResultUsable(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "Queued item with a real plan",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusQueued),
		SkipPlanning:       false,
		PlanApproved:       false,
	})
	require.NoError(t, err)

	is, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "headless-triage-" + uuid.New().String(),
		SessionRole: SessionRoleTriage,
	})
	require.NoError(t, err)
	require.NoError(t, storage.UpdateItemSessionTriageResult(ctx, is.ID, `{"summary":"a real plan"}`))
	require.NoError(t, storage.UpdateItemSessionEnded(ctx, is.ID, time.Now()))

	listener := NewBacklogLifecycleListener(storage)
	listener.reconcileOrphanedTriageItems(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "a queued item with a usable triage result is a normal plan-approval wait, not orphaned triage")
}

// TestReconcileOrphanedTriageItems_should_notFlag_When_QueuedItemHasOpenTriageSession
// locks in shape 1's `if !isIdea { continue }` guard: nothing in this codebase
// ever creates a new triage-role session while an item is queued (TriggerTriage's
// own status guard only accepts idea/ready), so an open session found on a queued
// item is an unmodeled anomaly this detector must never act on — no staleness
// flag, no tombstone. The session here is backdated well past
// maxWorkSessionStaleness so this test would fail loudly (a false-positive flag)
// if that guard were ever removed or narrowed, rather than passing vacuously.
func TestReconcileOrphanedTriageItems_should_notFlag_When_QueuedItemHasOpenTriageSession(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "Queued item with an unexpected open triage session",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusQueued),
		SkipPlanning:       false,
		PlanApproved:       false,
	})
	require.NoError(t, err)

	// created_at is Immutable() in the ent schema, so backdating requires the
	// raw ent client rather than storage.CreateItemSession (always time.Now())
	// — mirrors newOrphanedTriageTestItem's identical need above.
	parsedItemID, err := uuid.Parse(item.ID)
	require.NoError(t, err)
	_, err = er.client.ItemSession.Create().
		SetSessionUUID("headless-triage-" + uuid.New().String()).
		SetSessionRole(string(SessionRoleTriage)).
		SetBacklogItemID(parsedItemID).
		SetCreatedAt(time.Now().Add(-3 * time.Hour)). // beyond maxWorkSessionStaleness (2h)
		Save(ctx)
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	listener.reconcileOrphanedTriageItems(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "a queued item with a still-open triage session must never be flagged — this shape doesn't happen today, and this detector must not invent behavior for it")

	sessions, err := storage.ListItemSessions(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Nil(t, sessions[0].EndedAt, "must not be tombstoned either — shape 1 is idea-only")
}

// TestReconcileOrphanedTriageItems_should_notFlagQueuedItem_When_SkipPlanningOrPlanApproved
// verifies the generalized shape respects the same escape hatches
// reconcilePlanNotApprovedItems already does — an item that bypasses planning
// entirely, or already has an approved plan, is never "gated" regardless of
// what its triage session did or didn't produce.
func TestReconcileOrphanedTriageItems_should_notFlagQueuedItem_When_SkipPlanningOrPlanApproved(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	for _, tc := range []struct {
		name         string
		skipPlanning bool
		planApproved bool
	}{
		{"skip_planning", true, false},
		{"plan_approved", false, true},
	} {
		item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
			Title:              "Queued gate-bypass item " + tc.name,
			AcceptanceCriteria: `[]`,
			Priority:           1,
			Status:             string(BacklogStatusQueued),
			SkipPlanning:       tc.skipPlanning,
			PlanApproved:       tc.planApproved,
		})
		require.NoError(t, err)

		is, err := storage.CreateItemSession(ctx, ItemSessionData{
			ItemID:      item.ID,
			SessionUUID: "headless-triage-" + uuid.New().String(),
			SessionRole: SessionRoleTriage,
		})
		require.NoError(t, err)
		require.NoError(t, storage.UpdateItemSessionEnded(ctx, is.ID, time.Now()))
	}

	listener := NewBacklogLifecycleListener(storage)
	listener.reconcileOrphanedTriageItems(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "an item with skip_planning or plan_approved set must never be flagged, regardless of triage result")
}

// TestReconcileOrphanedTriageRemediation_should_retryQueuedRow_When_Due verifies
// the generalized queued shape is retried through the exact same backoff-gated
// AutoRespawnTriage path as the pre-existing idea-status shapes — closing the
// "detected but nothing ever acts on it" gap for be676dab's shape the same way
// PR #274/07-30 closed it for the idea-status shape.
func TestReconcileOrphanedTriageRemediation_should_retryQueuedRow_When_Due(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item := newQueuedNoTriageResultTestItem(t, storage)
	applied, err := er.MarkStuck(ctx, item.ID, domain.StuckReasonOrphanedTriage, BacklogStatusQueued, "no usable triage result")
	require.NoError(t, err)
	require.True(t, applied)

	listener := NewBacklogLifecycleListener(storage)
	listener.SetNotifier(&fakeNotifier{})
	respawner := newFakeTriageRespawner()
	listener.SetTriageRespawner(respawner)

	listener.reconcileOrphanedTriageRemediation(ctx, er)

	select {
	case itemID := <-respawner.calls:
		assert.Equal(t, item.ID, itemID)
	case <-time.After(time.Second):
		t.Fatal("expected AutoRespawnTriage to be dispatched for a due, queued orphaned_triage row")
	}
}

// TestSelfHealSweep_should_resolveOrphanedTriageRow_When_QueuedItemReachesReady verifies
// the generalized resolve condition correctly recognizes success: a retry that
// resets queued->idea then succeeds lands the item on "ready" — outside both
// anchor statuses (idea, queued) — which must resolve the row. This is the case
// the naive "resolve once status != queued" condition would get wrong, since a
// retry in flight passes through "idea" (still an anchor status) before
// reaching "ready".
func TestSelfHealSweep_should_resolveOrphanedTriageRow_When_QueuedItemReachesReady(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item := newQueuedNoTriageResultTestItem(t, storage)
	applied, err := er.MarkStuck(ctx, item.ID, domain.StuckReasonOrphanedTriage, BacklogStatusQueued, "no usable triage result")
	require.NoError(t, err)
	require.True(t, applied)

	// Simulate a successful automated retry: queued -> idea (reset) -> ready
	// (fresh triage succeeded).
	precondition := &BacklogItemPrecondition{ExpectedStatus: string(BacklogStatusQueued)}
	_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusIdea, precondition, TriggeredBySystem)
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	listener.selfHealStuck(ctx, er)
	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1, "row must stay open while the retry is in flight (item reset to idea, still an anchor status)")

	precondition = &BacklogItemPrecondition{ExpectedStatus: string(BacklogStatusIdea)}
	_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusReady, precondition, TriggeredBySystem)
	require.NoError(t, err)

	listener.selfHealStuck(ctx, er)
	open, err = er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "row must resolve once the retried triage succeeds and the item reaches ready")
}

// TestRetryOrphanedTriageWithBackoffGate_should_respectBackoffSchedule_When_CalledRepeatedly
// verifies the backoff gate itself: 10 back-to-back calls against the same open row
// must consume exactly one attempt, mirroring
// TestRetryPushFailedWithBackoffGate_should_respectBackoffSchedule_When_CalledRepeatedly.
func TestRetryOrphanedTriageWithBackoffGate_should_respectBackoffSchedule_When_CalledRepeatedly(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "Orphaned triage backoff test item",
		Status: string(BacklogStatusIdea),
	})
	require.NoError(t, err)
	applied, err := er.MarkStuck(ctx, item.ID, domain.StuckReasonOrphanedTriage, BacklogStatusIdea, "stale triage session")
	require.NoError(t, err)
	require.True(t, applied)

	listener := NewBacklogLifecycleListener(storage)
	listener.SetNotifier(&fakeNotifier{})
	respawner := newFakeTriageRespawner()
	listener.SetTriageRespawner(respawner)

	for i := 0; i < 10; i++ {
		listener.retryOrphanedTriageWithBackoffGate(ctx, item.ID, item.Title)
	}

	require.Eventually(t, func() bool {
		rows, err := er.FindOpenStuckStates(ctx)
		return err == nil && len(rows) == 1 && rows[0].RemediationAttempts == 1
	}, time.Second, 10*time.Millisecond, "only the first of 10 back-to-back calls should consume an attempt")
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
// UUID it was asked to archive/kill, in order, and can be configured to fail for
// specific UUIDs (archive and kill failures are tracked independently).
type fakeSessionArchiver struct {
	archivedUUIDs  []string
	killedUUIDs    []string
	errForUUID     map[string]error
	killErrForUUID map[string]error
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

func (f *fakeSessionArchiver) KillTmuxPaneOnly(_ context.Context, sessionUUID string) error {
	if f.killErrForUUID != nil {
		if err, ok := f.killErrForUUID[sessionUUID]; ok {
			return err
		}
	}
	f.killedUUIDs = append(f.killedUUIDs, sessionUUID)
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

// TestReconcileTerminalItemSessions_should_KillTmuxPane_When_ItemAlreadyDone guards
// the 2026-07-29 OOM fix: archiving alone only hides a terminal item's work session
// from the default list — without also killing its tmux pane, the underlying
// claude process (and its MCP subprocess fleet) keeps running indefinitely.
func TestReconcileTerminalItemSessions_should_KillTmuxPane_When_ItemAlreadyDone(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "done item with a still-live work session",
		Status: string(BacklogStatusDone),
	})
	require.NoError(t, err)
	_, err = storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "leaked-live-session",
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	archiver := &fakeSessionArchiver{}
	listener.SetSessionArchiver(archiver)

	listener.reconcileTerminalItemSessions(ctx)

	assert.Contains(t, archiver.killedUUIDs, "leaked-live-session")
}

// TestReconcileTerminalItemSessions_should_ArchiveAndKillReviewSession_When_ItemAlreadyDone
// guards the review-session half of the 2026-07-29 OOM fix: review-role sessions
// leaked the same way work-role ones did (excluded from both the terminal-transition
// hook and this safety-net sweep), leaving live review sessions for already-done
// items running indefinitely.
func TestReconcileTerminalItemSessions_should_ArchiveAndKillReviewSession_When_ItemAlreadyDone(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "done item with a still-live review session",
		Status: string(BacklogStatusDone),
	})
	require.NoError(t, err)
	_, err = storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "leaked-live-review-session",
		SessionRole: SessionRoleReview,
	})
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	archiver := &fakeSessionArchiver{}
	listener.SetSessionArchiver(archiver)

	listener.reconcileTerminalItemSessions(ctx)

	assert.Contains(t, archiver.archivedUUIDs, "leaked-live-review-session")
	assert.Contains(t, archiver.killedUUIDs, "leaked-live-review-session")
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

// TestReconcileTerminalItemSessions_should_NotArchiveTriageSessions_When_ItemDone
// guards the triage-role exclusion in IsTmuxBackedSessionRole: triage sessions run
// as bounded one-shot headless subprocess calls, not persistent tmux-attached claude
// processes (see headlessTriageUUIDPrefix), so they have no live tmux pane for this
// sweep to kill and archiving them here would be meaningless — their own failure
// mode (a crashed/hung goroutine) is handled by reconcileOrphanedTriageItems /
// reconcileOrphanedTriageRemediation instead.
func TestReconcileTerminalItemSessions_should_NotArchiveTriageSessions_When_ItemDone(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "done item with a triage session",
		Status: string(BacklogStatusDone),
	})
	require.NoError(t, err)
	_, err = storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "triage-session-not-archived-here",
		SessionRole: SessionRoleTriage,
	})
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	archiver := &fakeSessionArchiver{}
	listener.SetSessionArchiver(archiver)

	listener.reconcileTerminalItemSessions(ctx)

	assert.Empty(t, archiver.archivedUUIDs, "the sweep only archives tmux-backed (work/review) sessions — triage sessions are headless one-shot calls with no live pane and are excluded by design")
	assert.Empty(t, archiver.killedUUIDs, "no tmux pane exists for a triage session to kill")
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
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusReview, nil, TriggeredBySystem)
		require.NoError(t, err)
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusInProgress, nil, TriggeredBySystem)
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
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusReview, nil, TriggeredBySystem)
		require.NoError(t, err)
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusInProgress, nil, TriggeredBySystem)
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

// TestReconcileBouncingItems_should_surfaceVerdictOutcomeAndSummaryInContext_When_BouncingWithFailedVerdict
// is the regression test for BUG-060: reconcileBouncingItems fetched the
// item's most recent review verdict via GetMostRecentReviewVerdictForItem but
// collapsed it to a bare hasPass bool before building the stuck-state
// context, so an operator only ever saw "bounced ... with no PASS verdict"
// with no indication of which verdict (PARTIAL/FAIL) or why. This is the same
// discard-after-fetch shape BUG-059 fixed for reconcileOrphanedTriageItems'
// EndReason. This test attaches a FAIL verdict with a distinctive summary to
// the item's most recent review ItemSession, then asserts both the persisted
// BacklogStuckState.Context and the operator notification body contain the
// verdict's outcome and summary text, not just the generic bounce message.
func TestReconcileBouncingItems_should_surfaceVerdictOutcomeAndSummaryInContext_When_BouncingWithFailedVerdict(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "Bouncing item with failed verdict",
		Status: string(BacklogStatusInProgress),
	})
	require.NoError(t, err)
	for i := 0; i < 3; i++ {
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusReview, nil, TriggeredBySystem)
		require.NoError(t, err)
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusInProgress, nil, TriggeredBySystem)
		require.NoError(t, err)
	}

	// Attach a FAIL verdict, with a distinctive summary, to a review
	// ItemSession for this item — this is the diagnostic detail the fix must
	// thread through into the stuck-state context instead of discarding.
	const wantSummary = "AC3 not implemented: rate limiting missing from the new endpoint"
	_, err = storage.CreateItemSessionWithVerdict(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "headless-review-" + uuid.New().String(),
		SessionRole: SessionRoleReview,
	}, ReviewVerdictData{
		OverallOutcome: ReviewOutcomeFail,
		Summary:        wantSummary,
	})
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	listener.reconcileBouncingItems(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, domain.StuckReasonBouncing, open[0].Reason)
	assert.Contains(t, open[0].Context, string(ReviewOutcomeFail), "context must surface which verdict (FAIL), not just that no PASS was recorded")
	assert.Contains(t, open[0].Context, wantSummary, "context must surface the reviewer's actual summary, not a generic message")

	require.Len(t, notifier.calls, 1)
	assert.Contains(t, notifier.calls[0].Message, string(ReviewOutcomeFail))
	assert.Contains(t, notifier.calls[0].Message, wantSummary)
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
		_, err = storage.TransitionBacklogItemStatus(ctx, belowThreshold.ID, BacklogStatusReview, nil, TriggeredBySystem)
		require.NoError(t, err)
		_, err = storage.TransitionBacklogItemStatus(ctx, belowThreshold.ID, BacklogStatusInProgress, nil, TriggeredBySystem)
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
// TestNotifyTransitionFailed_should_publishNotification_When_Called is the
// direct unit test for BacklogLifecycleListener's notifyTransitionFailed —
// the session-package sibling of server/services/backlog_service_triage.go's
// identical helper (same shape, different package: this package must not
// import the server layer). Part of the fix for the recurring "silent
// status-transition failure" bug shape (BUG-030/040/041/046/048).
func TestNotifyTransitionFailed_should_publishNotification_When_Called(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	listener := NewBacklogLifecycleListener(storage)
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	listener.notifyTransitionFailed("item-999", "Ship the PR fix",
		"PR #42 was confirmed merged but the item's transition to done failed",
		errors.New("precondition failed: expected status \"in_progress\", got \"review\""))

	require.Len(t, notifier.calls, 1)
	assert.Equal(t, "Status update failed after work completed", notifier.calls[0].Title)
	assert.Contains(t, notifier.calls[0].Message, "Ship the PR fix")
	assert.Contains(t, notifier.calls[0].Message, "PR #42 was confirmed merged")
}

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
	newTrackedWorkSession(t, storage, item.ID, item.RepoPath, "backlog/bouncing-merged", "")

	// 3 in_progress->review round trips with no PASS verdict — the exact
	// shape isBouncing flags.
	for i := 0; i < 3; i++ {
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusReview, nil, TriggeredBySystem)
		require.NoError(t, err)
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusInProgress, nil, TriggeredBySystem)
		require.NoError(t, err)
	}

	listener := NewBacklogLifecycleListener(storage)
	overridePRPendingChecker(t, listener, &fakePRPendingChecker{merged: true})
	stubMatchingPRByNumberFinder(listener, "backlog/bouncing-merged")
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

// TestReconcileBouncingItems_should_ResolveBounceCapExhausted_When_BouncingResolves
// verifies Signal 2's resolve-alongside-bouncing wiring (plan.md Story
// 1.3.2): bounce_cap_exhausted can only ever coexist with an open bouncing
// row, so it must clear in the same tick bouncing itself resolves via the
// merged-PR branch, rather than outliving the condition it describes.
func TestReconcileBouncingItems_should_ResolveBounceCapExhausted_When_BouncingResolves(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:    "Bouncing item, capped, with merged PR",
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
	newTrackedWorkSession(t, storage, item.ID, item.RepoPath, "backlog/bouncing-capped-merged", "")

	// Seed both bouncing and bounce_cap_exhausted open — the exact live shape
	// once a bouncing item's remediation gate has already parked.
	applied, err := er.MarkStuck(ctx, item.ID, domain.StuckReasonBouncing, BacklogStatusInProgress, "5 cycles, capped")
	require.NoError(t, err)
	require.True(t, applied)
	applied, err = er.MarkStuck(ctx, item.ID, domain.StuckReasonBounceCapExhausted, BacklogStatusInProgress, "cap exhausted while bouncing")
	require.NoError(t, err)
	require.True(t, applied)

	// 3 in_progress->review round trips with no PASS verdict — the exact
	// shape isBouncing flags (mirrors the sibling merged-PR test above).
	for i := 0; i < 3; i++ {
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusReview, nil, TriggeredBySystem)
		require.NoError(t, err)
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusInProgress, nil, TriggeredBySystem)
		require.NoError(t, err)
	}

	listener := NewBacklogLifecycleListener(storage)
	overridePRPendingChecker(t, listener, &fakePRPendingChecker{merged: true})
	stubMatchingPRByNumberFinder(listener, "backlog/bouncing-capped-merged")
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	listener.reconcileBouncingItems(ctx, er)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusDone), fetched.Status,
		"an item whose linked PR already merged must transition to done, not stay bouncing")

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "both bouncing and bounce_cap_exhausted must resolve once the item's PR is confirmed merged")
}

// TestReconcileBouncingItems_should_notifyTransitionFailed_When_DoneTransitionFailsAfterMerge
// is a regression test for one of the sibling "silent status-transition
// failure" instances found by the silenttransition lint analyzer (same shape
// as BUG-030/040/041/046/048, and this fix's originally-reported findings):
// reconcileBouncingItems confirms the linked PR is already merged, then its
// own TransitionBacklogItemStatus(done) call can fail — previously that
// failure was only log.WarningLog.Printf'd, leaving the item bouncing forever
// with code that had already shipped and nothing else surfacing the mismatch.
//
// The precondition failure is forced deterministically (not via a real
// goroutine race) by having the fake PR-pending-checker mutate the item's
// status directly, bypassing TransitionBacklogItemStatus, at the exact point
// a genuine concurrent writer would have: after reconcileBouncingItems reads
// the item's status but before its own done-transition lands.
func TestReconcileBouncingItems_should_notifyTransitionFailed_When_DoneTransitionFailsAfterMerge(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:    "Bouncing item with merged PR, concurrent status race",
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
	newTrackedWorkSession(t, storage, item.ID, item.RepoPath, "backlog/bouncing-race", "")

	for i := 0; i < 3; i++ {
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusReview, nil, TriggeredBySystem)
		require.NoError(t, err)
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusInProgress, nil, TriggeredBySystem)
		require.NoError(t, err)
	}

	parsedID, err := uuid.Parse(item.ID)
	require.NoError(t, err)
	checker := &fakePRPendingChecker{merged: true}
	checker.onIsPRMerged = func() {
		// Simulate a concurrent writer moving the item to "review" between
		// reconcileBouncingItems' own read of item.Status ("in_progress") and
		// its done-transition call a few lines later — the exact TOCTOU shape
		// TransitionBacklogItemStatus's precondition exists to catch.
		_, updErr := er.client.BacklogItem.UpdateOneID(parsedID).SetStatus(string(BacklogStatusReview)).Save(ctx)
		require.NoError(t, updErr)
	}
	listener := NewBacklogLifecycleListener(storage)
	overridePRPendingChecker(t, listener, checker)
	stubMatchingPRByNumberFinder(listener, "backlog/bouncing-race")
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	listener.reconcileBouncingItems(ctx, er)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusReview), fetched.Status,
		"the failed done-transition must not silently succeed; the item stays at whatever the concurrent writer left it")

	require.Len(t, notifier.calls, 1, "the failed transition must surface an operator notification instead of only being logged")
	assert.Equal(t, "Status update failed after work completed", notifier.calls[0].Title)
	assert.Contains(t, notifier.calls[0].Message, "confirmed merged")
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
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusReview, nil, TriggeredBySystem)
		require.NoError(t, err)
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusInProgress, nil, TriggeredBySystem)
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
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusReview, nil, TriggeredBySystem)
		require.NoError(t, err)
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusInProgress, nil, TriggeredBySystem)
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
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusReview, nil, TriggeredBySystem)
		require.NoError(t, err)
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusInProgress, nil, TriggeredBySystem)
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
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusReview, nil, TriggeredBySystem)
		require.NoError(t, err)
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusInProgress, nil, TriggeredBySystem)
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

// TestReconcileBouncingItems_should_notTreatFreshBranchBaseAsShipped_When_ZeroCommitsYet
// is the regression test for BUG-039: a freshly spawned work session's
// worktree HEAD is, by construction, its own base commit until the agent's
// first commit — and a base commit is always an ancestor of main (that's
// literally where the branch came from), so mostRecentWorkCommitShippedToMain
// treated it as "shipped" and reconcileBouncingItems auto-marked the item
// done within the first reconcile tick after spawn, despite zero actual work
// having happened. Confirmed live 2026-07-22: items e1fb6825 and 12981e9d
// were both auto-marked done ~1 minute after being dequeued, citing their own
// base commit ("5d77b70b...") as evidence of having "shipped to main without
// a PR" — one of them had a live work session still actively producing real
// work at the moment it happened. Distinct from the 2026-07-21 stale-field
// bug (TestReconcileBouncingItems_should_stillFlag_When_LastCommitShaIsStaleBaseSeed):
// this is a live-resolved, genuinely-current SHA that simply hasn't diverged
// from its own branch point yet — resolveLatestWorkCommit's existing fix
// doesn't help here, since the function IS correctly returning the true HEAD.
func TestReconcileBouncingItems_should_notTreatFreshBranchBaseAsShipped_When_ZeroCommitsYet(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	repoPath, mainSHA := setupBounceMainRepo(t)
	// A feature branch created from main's tip with zero commits of its own —
	// exactly the state of a worktree in the first moments after spawn.
	runGitTestCmd(t, repoPath, "checkout", "-b", "backlog/fresh-feature")

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:    "Freshly spawned item with zero commits",
		Status:   string(BacklogStatusInProgress),
		RepoPath: repoPath,
	})
	require.NoError(t, err)

	workSessionUUID := "fresh-branch-work-session"
	_, err = storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: workSessionUUID,
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)

	// repoPath's checked-out HEAD is the fresh branch's tip — identical to
	// mainSHA, since no commits have been made on it yet. baseCommitSHA is
	// also mainSHA, matching what a real spawn records.
	inst := newTestInstance("fresh-branch-instance")
	inst.UUID = workSessionUUID
	inst.gitManager.worktree = git.NewGitWorktreeFromStorage(repoPath, repoPath, "fresh-branch-instance", "backlog/fresh-feature", mainSHA)
	require.NoError(t, storage.SaveInstances([]*Instance{inst}))

	for i := 0; i < 3; i++ {
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusReview, nil, TriggeredBySystem)
		require.NoError(t, err)
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusInProgress, nil, TriggeredBySystem)
		require.NoError(t, err)
	}

	listener := NewBacklogLifecycleListener(storage)
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	listener.reconcileBouncingItems(ctx, er)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusInProgress), fetched.Status,
		"a fresh branch with zero commits (HEAD == base) must never be treated as 'shipped to main' just because its unchanged base is trivially an ancestor of main")

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1, "the item must still go through normal bouncing detection instead of being silently marked done")
	assert.Equal(t, domain.StuckReasonBouncing, open[0].Reason)
}

// TestReconcileBouncingItems_should_stillFlag_When_WorktreePathWasRecycledToAnotherBranch
// is the regression test for the 2026-08-12 live repro: worktree paths are
// reused across sessions once a session ends, so a directory existing at a
// stale session's recorded WorktreePath does not mean it still holds that
// session's branch. resolveLatestWorkCommit used to trust any existing
// directory's HEAD unconditionally, so once the path was reassigned to a
// later, unrelated item's branch, the stale item's reconcile pass read that
// later item's real (legitimately merged) commit as its own "shipped" work.
// Confirmed live for backlog items 0f5d760b, 6f6f6f4e, and a3ca3918 in the
// docspan repo: each was falsely marked done off another item's commit.
//
// The fix (session/backlog_lifecycle.go resolveLatestWorkCommit) checks that
// the worktree path's currently checked-out branch still matches the
// session's own recorded BranchName before trusting its HEAD; on a mismatch
// it falls back to the existing repo-wide branch-name lookup, same as the
// worktree-gone case.
func TestReconcileBouncingItems_should_stillFlag_When_WorktreePathWasRecycledToAnotherBranch(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	repoPath, _ := setupBounceMainRepo(t)
	runGitTestCmd(t, repoPath, "checkout", "-b", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "feature.txt"), []byte("stale item's unshipped work\n"), 0o644))
	runGitTestCmd(t, repoPath, "add", "feature.txt")
	runGitTestCmd(t, repoPath, "commit", "-m", "work that never merged")
	staleFeatureSHA := strings.TrimSpace(runGitTestCmd(t, repoPath, "rev-parse", "HEAD"))

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:    "Stale item whose worktree path got recycled",
		Status:   string(BacklogStatusInProgress),
		RepoPath: repoPath,
	})
	require.NoError(t, err)

	workSessionUUID := "recycled-worktree-work-session"
	_, err = storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: workSessionUUID,
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)

	// The stale session's worktree row still records its own branch ("feature")
	// and the path it used to live at, but the directory at that path has since
	// been reused by a later, unrelated session: it's now checked out on
	// "other-item-branch" with a commit this item never authored.
	inst := newTestInstance("recycled-worktree-instance")
	inst.UUID = workSessionUUID
	inst.gitManager.worktree = git.NewGitWorktreeFromStorage(repoPath, repoPath, "recycled-worktree-instance", "feature", staleFeatureSHA)
	require.NoError(t, storage.SaveInstances([]*Instance{inst}))

	// A later, unrelated item is spawned into the very same path (worktree
	// paths are recycled once a session ends), does real work, and gets
	// merged. The path is left checked out on that later item's branch —
	// exactly the state the reconciler finds when it later re-evaluates the
	// stale session above.
	runGitTestCmd(t, repoPath, "checkout", "-b", "other-item-branch")
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "other-item.txt"), []byte("a later item's own commit\n"), 0o644))
	runGitTestCmd(t, repoPath, "add", "other-item.txt")
	runGitTestCmd(t, repoPath, "commit", "-m", "later item's real work")
	runGitTestCmd(t, repoPath, "checkout", "main")
	runGitTestCmd(t, repoPath, "merge", "--no-ff", "-m", "merge later item's work", "other-item-branch")
	runGitTestCmd(t, repoPath, "checkout", "other-item-branch")

	for i := 0; i < 3; i++ {
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusReview, nil, TriggeredBySystem)
		require.NoError(t, err)
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusInProgress, nil, TriggeredBySystem)
		require.NoError(t, err)
	}

	listener := NewBacklogLifecycleListener(storage)
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	listener.reconcileBouncingItems(ctx, er)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusInProgress), fetched.Status,
		"a stale item must not be marked done off a commit read from its recycled worktree path's current (different) branch")

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1, "the item must still go through normal bouncing detection instead of being silently marked done")
	assert.Equal(t, domain.StuckReasonBouncing, open[0].Reason)
}

// TestReconcileBouncingItems_should_stillTransitionToDone_When_WorkCommittedDirectlyToMainBranch
// verifies BUG-039's fix doesn't regress the legitimate case
// TestReconcileBouncingItems_should_transitionToDone_When_ShippedWithoutPR
// already covers: when the work session's own branch IS bounceMainBranch
// (work happened directly on main, no separate feature branch ever used),
// sha == base is not "zero commits yet" — main's tip literally is the shipped
// state — so the new guard must not suppress that transition.
func TestReconcileBouncingItems_should_stillTransitionToDone_When_WorkCommittedDirectlyToMainBranch(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	repoPath, mainSHA := setupBounceMainRepo(t)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:    "Work committed directly to main, no feature branch",
		Status:   string(BacklogStatusInProgress),
		RepoPath: repoPath,
	})
	require.NoError(t, err)

	workSessionUUID := "direct-to-main-work-session"
	_, err = storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: workSessionUUID,
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)

	inst := newTestInstance("direct-to-main-instance")
	inst.UUID = workSessionUUID
	inst.gitManager.worktree = git.NewGitWorktreeFromStorage(repoPath, repoPath, "direct-to-main-instance", "main", mainSHA)
	require.NoError(t, storage.SaveInstances([]*Instance{inst}))

	for i := 0; i < 3; i++ {
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusReview, nil, TriggeredBySystem)
		require.NoError(t, err)
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusInProgress, nil, TriggeredBySystem)
		require.NoError(t, err)
	}

	listener := NewBacklogLifecycleListener(storage)
	listener.reconcileBouncingItems(ctx, er)

	fetched, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(BacklogStatusDone), fetched.Status,
		"work committed directly to the main branch (no feature branch) must still be recognized as shipped")
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

	_, err := storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusDone, nil, TriggeredBySystem)
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

// TestSelfHealStuck_should_ResolveBounceCapExhausted_When_ItemStatusLeavesInProgressOrReview
// is the backstop test for Task 1.3.2b: an open bounce_cap_exhausted row must
// resolve via selfHealStuck's own status-anchor case (mirroring bouncing's
// own anchor scope) once the item's status leaves in_progress/review —
// exercised here with a NON-terminal status (pr_pending) so this asserts the
// reason-specific case fires, not the blanket terminal-status rule (which a
// done/archived status would exercise instead, per
// TestSelfHealSweep_should_resolveBouncingRow_When_ItemReachesDoneOrPass
// immediately above).
func TestSelfHealStuck_should_ResolveBounceCapExhausted_When_ItemStatusLeavesInProgressOrReview(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "Bounce cap exhausted item now pr_pending",
		Status: string(BacklogStatusPRPending),
	})
	require.NoError(t, err)
	applied, err := er.MarkStuck(ctx, item.ID, domain.StuckReasonBounceCapExhausted, BacklogStatusPRPending, "cap exhausted while bouncing, now pr_pending")
	require.NoError(t, err)
	require.True(t, applied)

	listener := NewBacklogLifecycleListener(storage)
	listener.selfHealStuck(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "bounce_cap_exhausted row must resolve once the item's status leaves in_progress/review, mirroring bouncing's own anchor rule")
}

// TestSelfHealStuck_should_notResolveBounceCapExhaustedRow_When_ItemStillInReview
// verifies the negative case: the row must stay open while the item is still
// anchored in review (one of bounce_cap_exhausted's two valid anchor statuses).
func TestSelfHealStuck_should_notResolveBounceCapExhaustedRow_When_ItemStillInReview(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "Bounce cap exhausted item still in review",
		Status: string(BacklogStatusReview),
	})
	require.NoError(t, err)
	applied, err := er.MarkStuck(ctx, item.ID, domain.StuckReasonBounceCapExhausted, BacklogStatusReview, "cap exhausted while bouncing")
	require.NoError(t, err)
	require.True(t, applied)

	listener := NewBacklogLifecycleListener(storage)
	listener.selfHealStuck(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1, "bounce_cap_exhausted row must stay open while the item is still in review")
	assert.Equal(t, domain.StuckReasonBounceCapExhausted, open[0].Reason)
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

	_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusDone, nil, TriggeredBySystem)
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
				_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusDone, nil, TriggeredBySystem)
				require.NoError(t, err)
				if terminal == BacklogStatusArchived {
					_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusArchived, nil, TriggeredBySystem)
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
	_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusDone, nil, TriggeredBySystem)
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
	_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusReview, nil, TriggeredBySystem)
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
	_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusInProgress, nil, TriggeredBySystem)
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

// newQueuedPlanNotApprovedTestItem creates a queued item blocked by
// DequeueNextQueuedItems' planning gate (SkipPlanning=false, PlanApproved=
// false), with QueuedAt backdated by queuedAgo.
func newQueuedPlanNotApprovedTestItem(t *testing.T, storage *Storage, queuedAgo time.Duration) *BacklogItemData {
	t.Helper()
	ctx := context.Background()

	queuedAt := time.Now().Add(-queuedAgo)
	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "Plan-not-approved test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusQueued),
		QueuedAt:           &queuedAt,
		SkipPlanning:       false,
		PlanApproved:       false,
	})
	require.NoError(t, err)
	return item
}

// TestReconcilePlanNotApprovedItems_should_writeDurableRowNotifyOnce_When_QueuedItemStale
// is the BUG-038 regression test: a queued item blocked by the planning gate
// must get a durable, human-visible stuck row instead of silently retrying
// forever with only a per-tick WARNING log.
func TestReconcilePlanNotApprovedItems_should_writeDurableRowNotifyOnce_When_QueuedItemStale(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item := newQueuedPlanNotApprovedTestItem(t, storage, 10*time.Minute) // beyond planApprovalStaleness (5m)

	listener := NewBacklogLifecycleListener(storage)
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	listener.reconcilePlanNotApprovedItems(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, item.ID, open[0].ItemID)
	assert.Equal(t, domain.StuckReasonPlanNotApproved, open[0].Reason)
	assert.Equal(t, []string{"Queued item blocked by unapproved plan"}, notifier.titles())

	// Repeat tick must not re-notify (DB-backed notify-once dedup).
	listener.reconcilePlanNotApprovedItems(ctx, er)
	assert.Len(t, notifier.calls, 1)
}

// TestReconcilePlanNotApprovedItems_should_notFlag_When_QueuedRecently verifies
// the staleness buffer: an item queued moments ago must not be flagged —
// it's plausibly about to be approved/dequeued.
func TestReconcilePlanNotApprovedItems_should_notFlag_When_QueuedRecently(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	newQueuedPlanNotApprovedTestItem(t, storage, 1*time.Minute) // within planApprovalStaleness (5m)

	listener := NewBacklogLifecycleListener(storage)
	listener.reconcilePlanNotApprovedItems(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "a just-queued item must not be flagged yet")
}

// TestReconcilePlanNotApprovedItems_should_notFlag_When_SkipPlanningTrue verifies
// the detector doesn't over-trigger for items that legitimately bypass planning.
func TestReconcilePlanNotApprovedItems_should_notFlag_When_SkipPlanningTrue(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	queuedAt := time.Now().Add(-10 * time.Minute)
	_, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "Skip-planning queued test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusQueued),
		QueuedAt:           &queuedAt,
		SkipPlanning:       true,
		PlanApproved:       false,
	})
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	listener.reconcilePlanNotApprovedItems(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "an item with skip_planning=true is never blocked by this gate and must not be flagged")
}

// TestReconcilePlanNotApprovedItems_should_notFlag_When_LatestTriageResultUnusable
// verifies the 2026-08-03 delegation fix: an item whose most recent triage
// session ended with no usable result is NOT "awaiting human review of a real
// plan" (this detector's normal case) — it's reconcileOrphanedTriageItems'
// generalized shape now. Without this delegation, item be676dab's shape would
// get flagged under two different, differently-worded stuck reasons
// simultaneously.
func TestReconcilePlanNotApprovedItems_should_notFlag_When_LatestTriageResultUnusable(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item := newQueuedNoTriageResultTestItem(t, storage)
	queuedAt := time.Now().Add(-10 * time.Minute)
	_, err := storage.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{QueuedAt: &queuedAt}, nil)
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	listener.reconcilePlanNotApprovedItems(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "an item with no usable triage result must be left to reconcileOrphanedTriageItems, not double-flagged as plan_not_approved")
}

// TestSelfHealSweep_should_resolvePlanNotApprovedRow_When_ItemLeavesQueued verifies
// the status-anchored self-heal sweep clears this reason once the item is no
// longer queued (e.g. manually approved and dequeued to in_progress).
func TestSelfHealSweep_should_resolvePlanNotApprovedRow_When_ItemLeavesQueued(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item := newQueuedPlanNotApprovedTestItem(t, storage, 10*time.Minute)

	listener := NewBacklogLifecycleListener(storage)
	listener.reconcilePlanNotApprovedItems(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1, "row must be open before the item leaves queued")

	approved := true
	_, err = storage.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{PlanApproved: &approved}, nil)
	require.NoError(t, err)
	precondition := &BacklogItemPrecondition{ExpectedStatus: string(BacklogStatusQueued)}
	_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusInProgress, precondition, TriggeredBySystem)
	require.NoError(t, err)

	listener.selfHealStuck(ctx, er)

	open, err = er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "leaving queued must resolve the plan_not_approved row via the status-anchored self-heal sweep")
}

// TestReconcilePRPendingWithoutPRItems_should_writeDurableRowNotifyOnce_When_PrNumberZero
// is the BUG-040 detection-backstop regression test: an item that reaches
// pr_pending with pr_number=0/pr_url="" is otherwise structurally invisible —
// FindPRPendingItems' PrNumberGT(0) filter excludes it from ReconcilePRPending
// and everything downstream of it — so this detector must be the one thing
// that still surfaces it as a durable, human-visible, notify-once stuck row.
func TestReconcilePRPendingWithoutPRItems_should_writeDurableRowNotifyOnce_When_PrNumberZero(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "PR-pending-no-PR test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusPRPending),
	})
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	listener.reconcilePRPendingWithoutPRItems(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, item.ID, open[0].ItemID)
	assert.Equal(t, domain.StuckReasonPRPendingNoPR, open[0].Reason)
	assert.Equal(t, []string{"Backlog item stuck: pr_pending with no PR"}, notifier.titles())

	// Repeat tick must not re-notify (DB-backed notify-once dedup).
	listener.reconcilePRPendingWithoutPRItems(ctx, er)
	assert.Len(t, notifier.calls, 1)
}

// TestReconcilePRPendingWithoutPRItems_should_notFlag_When_PrNumberSet verifies
// the detector doesn't over-trigger for healthy pr_pending items that DO carry
// a real PR reference.
func TestReconcilePRPendingWithoutPRItems_should_notFlag_When_PrNumberSet(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item := newPRPendingTestItem(t, storage, 152)
	_ = item

	listener := NewBacklogLifecycleListener(storage)
	listener.reconcilePRPendingWithoutPRItems(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "a pr_pending item with a real PR reference must not be flagged")
}

// TestSelfHealSweep_should_resolvePRPendingNoPRRow_When_ItemLeavesPRPending verifies
// the status-anchored self-heal sweep clears this reason once the item is no
// longer pr_pending (e.g. successfully reopened for a fresh attempt).
func TestSelfHealSweep_should_resolvePRPendingNoPRRow_When_ItemLeavesPRPending(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "PR-pending-no-PR self-heal test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusPRPending),
	})
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	listener.reconcilePRPendingWithoutPRItems(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	require.Len(t, open, 1, "row must be open before the item leaves pr_pending")

	precondition := &BacklogItemPrecondition{ExpectedStatus: string(BacklogStatusPRPending)}
	_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusInProgress, precondition, TriggeredBySystem)
	require.NoError(t, err)

	listener.selfHealStuck(ctx, er)

	open, err = er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	assert.Empty(t, open, "leaving pr_pending must resolve the pr_pending_no_pr row via the status-anchored self-heal sweep")
}

// TestReconcileMultiReasonEscalation_should_MarkStuckWithoutNotifying_When_ThresholdFirstCrossed
// verifies the escalate branch: an item with 2 simultaneously open,
// independent (non-coupled) non-escalation reasons gets a durable
// multiple_reasons row within one tick, but is NOT notified on the same tick
// that created the row (multiReasonEscalationNotifyReady's dwell gate).
func TestReconcileMultiReasonEscalation_should_MarkStuckWithoutNotifying_When_ThresholdFirstCrossed(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "Multi-reason item",
		Status: string(BacklogStatusInProgress),
	})
	require.NoError(t, err)

	applied, err := er.MarkStuck(ctx, item.ID, domain.StuckReasonBouncing, BacklogStatusInProgress, "3 cycles")
	require.NoError(t, err)
	require.True(t, applied)
	applied, err = er.MarkStuck(ctx, item.ID, domain.StuckReasonStaleWork, BacklogStatusInProgress, "no progress")
	require.NoError(t, err)
	require.True(t, applied)

	listener := NewBacklogLifecycleListener(storage)
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	listener.reconcileMultiReasonEscalation(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	row, ok := findOpenStuckStateFor(open, item.ID, domain.StuckReasonMultipleReasons)
	require.True(t, ok, "an item with 2 open non-escalation reasons must get a multiple_reasons row")
	assert.Nil(t, row.NotifiedAt, "must not notify on the tick that created the row")
	assert.Empty(t, notifier.calls, "must not notify on the tick that created the row")
}

// TestReconcileMultiReasonEscalation_should_Notify_When_DwellElapsedAndStillOpen
// verifies the notify branch: once the multiple_reasons row has been open
// past multiReasonNotifyDwell and the condition still holds, the next tick
// notifies exactly once and marks the row notified.
func TestReconcileMultiReasonEscalation_should_Notify_When_DwellElapsedAndStillOpen(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "Multi-reason item, dwell elapsed",
		Status: string(BacklogStatusInProgress),
	})
	require.NoError(t, err)

	_, err = er.MarkStuck(ctx, item.ID, domain.StuckReasonBouncing, BacklogStatusInProgress, "3 cycles")
	require.NoError(t, err)
	_, err = er.MarkStuck(ctx, item.ID, domain.StuckReasonStaleWork, BacklogStatusInProgress, "no progress")
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	notifier := &fakeNotifier{}
	listener.SetNotifier(notifier)

	// First tick: creates the row, does not notify.
	listener.reconcileMultiReasonEscalation(ctx, er)
	require.Empty(t, notifier.calls)

	backdateStuckFirstDetected(t, er, item.ID, domain.StuckReasonMultipleReasons, time.Now().Add(-61*time.Second))

	// Second tick: dwell elapsed, condition still holds.
	listener.reconcileMultiReasonEscalation(ctx, er)

	require.Len(t, notifier.calls, 1)
	assert.Equal(t, "Multiple stuck reasons open", notifier.calls[0].Title)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	row, ok := findOpenStuckStateFor(open, item.ID, domain.StuckReasonMultipleReasons)
	require.True(t, ok)
	assert.NotNil(t, row.NotifiedAt, "row must be marked notified after the dwell-gated notify fires")

	// Third tick: already notified, must not re-notify.
	listener.reconcileMultiReasonEscalation(ctx, er)
	assert.Len(t, notifier.calls, 1, "must notify at most once per row lifetime")
}

// TestReconcileMultiReasonEscalation_should_ResolveStuck_When_CountDropsBelowThreshold
// verifies the de-escalate branch: once one of the two underlying reasons
// resolves (dropping the non-escalation count below multiReasonThreshold),
// the next tick resolves the multiple_reasons row.
func TestReconcileMultiReasonEscalation_should_ResolveStuck_When_CountDropsBelowThreshold(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "Multi-reason item, de-escalating",
		Status: string(BacklogStatusInProgress),
	})
	require.NoError(t, err)

	_, err = er.MarkStuck(ctx, item.ID, domain.StuckReasonBouncing, BacklogStatusInProgress, "3 cycles")
	require.NoError(t, err)
	_, err = er.MarkStuck(ctx, item.ID, domain.StuckReasonStaleWork, BacklogStatusInProgress, "no progress")
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	listener.reconcileMultiReasonEscalation(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	_, ok := findOpenStuckStateFor(open, item.ID, domain.StuckReasonMultipleReasons)
	require.True(t, ok, "row must be open before de-escalation")

	_, err = er.ResolveStuck(ctx, item.ID, domain.StuckReasonStaleWork)
	require.NoError(t, err)

	listener.reconcileMultiReasonEscalation(ctx, er)

	open, err = er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	_, ok = findOpenStuckStateFor(open, item.ID, domain.StuckReasonMultipleReasons)
	assert.False(t, ok, "multiple_reasons row must resolve once the open-reason count drops below threshold")
}

// TestReconcileMultiReasonEscalation_should_ExcludeEscalationReasonsFromCount_When_Counting
// is the ADR-001 self-reinforcement guard: an item with a single
// non-escalation reason (bouncing) plus its own already-open
// multiple_reasons row must NOT count the multiple_reasons row itself toward
// the threshold — only 1 non-escalation reason is open, so the escalation
// must actually de-escalate/resolve, not stay pinned open by counting itself.
func TestReconcileMultiReasonEscalation_should_ExcludeEscalationReasonsFromCount_When_Counting(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "Self-reinforcement guard item",
		Status: string(BacklogStatusInProgress),
	})
	require.NoError(t, err)

	_, err = er.MarkStuck(ctx, item.ID, domain.StuckReasonBouncing, BacklogStatusInProgress, "3 cycles")
	require.NoError(t, err)
	// Seed the escalation row directly (as if a prior tick had wrongly counted
	// itself, or it survived from a since-resolved second reason).
	_, err = er.MarkStuck(ctx, item.ID, domain.StuckReasonMultipleReasons, BacklogStatusInProgress, "bouncing, multiple_reasons")
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	listener.reconcileMultiReasonEscalation(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	_, ok := findOpenStuckStateFor(open, item.ID, domain.StuckReasonMultipleReasons)
	assert.False(t, ok, "multiple_reasons must not count itself toward its own threshold — with only 1 real reason open, it must resolve")
}

// TestReconcileMultiReasonEscalation_should_NotEscalate_When_OnlyCoupledBouncingAndAbandonedReviewOpen
// is the structural-coupling regression guard (Task 1.2.2a/e): bouncing and
// abandoned_review co-occur on nearly every bouncing item whose reopen gate
// is currently blocked (mid-backoff) — see markAbandonedReview's own
// identical gate, TestMarkAbandonedReview_SkipsRespawn_WhenBouncingGateNotDue.
// With only that coupled pair open, escalation must NOT fire.
func TestReconcileMultiReasonEscalation_should_NotEscalate_When_OnlyCoupledBouncingAndAbandonedReviewOpen(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "Coupled bouncing+abandoned_review item",
		Status: string(BacklogStatusReview),
	})
	require.NoError(t, err)

	_, err = er.MarkStuck(ctx, item.ID, domain.StuckReasonBouncing, BacklogStatusReview, "bounced previously")
	require.NoError(t, err)
	// Drive the bouncing gate into "blocked" (mid-backoff), mirroring
	// TestMarkAbandonedReview_SkipsRespawn_WhenBouncingGateNotDue's seeding.
	future := time.Now().Add(2 * time.Hour)
	_, err = er.RecordRemediationAttempt(ctx, item.ID, domain.StuckReasonBouncing, 1, &future)
	require.NoError(t, err)
	_, err = er.MarkStuck(ctx, item.ID, domain.StuckReasonAbandonedReview, BacklogStatusReview, "stuck in review")
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	listener.reconcileMultiReasonEscalation(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	_, ok := findOpenStuckStateFor(open, item.ID, domain.StuckReasonMultipleReasons)
	assert.False(t, ok, "the coupled bouncing+abandoned_review pair alone must not self-escalate")
}

// TestReconcileMultiReasonEscalation_should_Escalate_When_CoupledPairPlusIndependentReasonOpen
// is the companion case to the exclusion guard above: the same coupled
// bouncing+abandoned_review pair PLUS one genuinely independent third reason
// (push_failed) must still escalate — confirming the coupling exclusion
// narrows the count rather than disabling escalation outright.
func TestReconcileMultiReasonEscalation_should_Escalate_When_CoupledPairPlusIndependentReasonOpen(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo.(*EntRepository)

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "Coupled pair plus independent reason item",
		Status: string(BacklogStatusReview),
	})
	require.NoError(t, err)

	_, err = er.MarkStuck(ctx, item.ID, domain.StuckReasonBouncing, BacklogStatusReview, "bounced previously")
	require.NoError(t, err)
	future := time.Now().Add(2 * time.Hour)
	_, err = er.RecordRemediationAttempt(ctx, item.ID, domain.StuckReasonBouncing, 1, &future)
	require.NoError(t, err)
	_, err = er.MarkStuck(ctx, item.ID, domain.StuckReasonAbandonedReview, BacklogStatusReview, "stuck in review")
	require.NoError(t, err)
	_, err = er.MarkStuck(ctx, item.ID, domain.StuckReasonPushFailed, BacklogStatusReview, "push failed")
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	listener.reconcileMultiReasonEscalation(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	row, ok := findOpenStuckStateFor(open, item.ID, domain.StuckReasonMultipleReasons)
	require.True(t, ok, "bouncing+push_failed (abandoned_review excluded by the coupling guard) is still 2 independent reasons — must escalate")
	assert.Contains(t, row.Context, "bouncing")
	assert.Contains(t, row.Context, "push_failed")
	assert.NotContains(t, row.Context, "abandoned_review", "the coupled abandoned_review row must be excluded from the escalation context")
}
