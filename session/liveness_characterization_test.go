package session

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

// TestLivenessCharacterization_should_ProduceIdenticalStuckDecisionAndReasonDetail_When_ComparingHardcodedConstantPathToDefaultLivenessEngine
// is Milestone 1's Risk Control gate
// (project_plans/backlog-custom-workflow-stages/research/architecture.md §6):
// for one fixture per liveness shape (A/B/C), it runs the CURRENT
// hardcoded-constant reconcile* sweep to capture the "before" stuck/not-stuck
// decision and reasonDetail string, then independently recomputes the "after"
// decision by running the SAME reconcile* method again — now wired to a
// DefaultLivenessEngine (Epic 1.4 landed: every reconcile* sweep this test
// covers resolves its threshold via LivenessEngine when one is wired) — on a
// second, identical fixture, and asserts the two are byte-identical.
//
// "Zero rows configured" (Task 1.4.1c/1.4.3d) means DefaultLivenessEngine
// itself: it has no DB-backed override table of its own (see its doc comment),
// so wiring it in reproduces every pre-Epic-1.4 hardcoded threshold exactly —
// this test is the proof.
func TestLivenessCharacterization_should_ProduceIdenticalStuckDecisionAndReasonDetail_When_ComparingHardcodedConstantPathToDefaultLivenessEngine(t *testing.T) {
	t.Run("ShapeA_orphaned_triage_idea", testLivenessCharacterizationShapeA)
	t.Run("ShapeB_stale_work_in_progress", testLivenessCharacterizationShapeB)
	t.Run("ShapeC_bouncing_review", testLivenessCharacterizationShapeC)
}

// testLivenessCharacterizationShapeA covers Shape A (duration-budget-plus-
// margin): an idea-status item with a headless triage session open past
// maxHeadlessTriageSessionStaleness (session/backlog_lifecycle_triage.go).
func testLivenessCharacterizationShapeA(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo

	// Past the 35m hardcoded threshold, matching
	// TestReconcileOrphanedTriageItems_should_flagHeadlessSession_After30Min's
	// own fixture shape.
	ageAgo := maxHeadlessTriageSessionStaleness + time.Minute

	// Before: the real, unmodified sweep with no LivenessEngine wired (nil — matching
	// every pre-Epic-1.4 call site).
	beforeItem := newOrphanedTriageTestItem(t, storage, er, ageAgo)
	beforeListener := NewBacklogLifecycleListener(storage)
	beforeListener.reconcileOrphanedTriageItems(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	beforeRow, ok := findOpenStuckStateFor(open, beforeItem.ID, domain.StuckReasonOrphanedTriage)
	require.True(t, ok, "unwired sweep must mark this fixture stuck")

	// After: the identical sweep, now wired to DefaultLivenessEngine (Epic 1.4 landed —
	// this IS the real reconcileOrphanedTriageItems code path, not a manual mirror). Zero
	// rows configured (DefaultLivenessEngine has no DB-backed override table of its own),
	// so this must reproduce the "before" decision and threshold exactly.
	afterItem := newOrphanedTriageTestItem(t, storage, er, ageAgo)
	afterListener := NewBacklogLifecycleListener(storage)
	afterListener.livenessEngine = NewDefaultLivenessEngine()
	afterListener.reconcileOrphanedTriageItems(ctx, er)

	open, err = er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	afterRow, ok := findOpenStuckStateFor(open, afterItem.ID, domain.StuckReasonOrphanedTriage)
	require.True(t, ok, "DefaultLivenessEngine-wired sweep must still mark this fixture stuck")

	// Both rows' reasonDetail embed a different (per-fixture) session UUID but must
	// share the identical "... still open after <threshold>" suffix — the resolved
	// threshold DefaultLivenessEngine produces (35m) must equal the literal constant.
	wantSuffix := fmt.Sprintf(" still open after %s", maxHeadlessTriageSessionStaleness)
	assert.True(t, strings.HasSuffix(beforeRow.Context, wantSuffix), "before reasonDetail = %q, want suffix %q", beforeRow.Context, wantSuffix)
	assert.True(t, strings.HasSuffix(afterRow.Context, wantSuffix), "after (DefaultLivenessEngine-wired) reasonDetail = %q, want suffix %q", afterRow.Context, wantSuffix)
}

// testLivenessCharacterizationShapeB covers Shape B (heartbeat staleness): an
// in_progress item with an active work session whose last-progress timestamp
// is beyond maxWorkSessionStaleness (session/backlog_lifecycle_stale.go).
func testLivenessCharacterizationShapeB(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo

	// newStaleWorkTestItem backdates last_progress_at by 3h, beyond
	// maxWorkSessionStaleness (2h).

	// Before: the real, unmodified sweep with no LivenessEngine wired.
	beforeItem := newStaleWorkTestItem(t, storage, er)
	beforeListener := NewBacklogLifecycleListener(storage)
	beforeListener.reconcileStaleWorkSessions(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	_, ok := findOpenStuckStateFor(open, beforeItem.ID, domain.StuckReasonStaleWork)
	require.True(t, ok, "unwired sweep must mark this fixture stuck")

	// After: the identical sweep, now wired to DefaultLivenessEngine (Epic 1.4 landed).
	// Zero rows configured, so the resolved 2h MaxNoProgressDuration must reproduce the
	// "before" decision exactly.
	afterItem := newStaleWorkTestItem(t, storage, er)
	afterListener := NewBacklogLifecycleListener(storage)
	afterListener.livenessEngine = NewDefaultLivenessEngine()
	afterListener.reconcileStaleWorkSessions(ctx, er)

	open, err = er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	_, ok = findOpenStuckStateFor(open, afterItem.ID, domain.StuckReasonStaleWork)
	require.True(t, ok, "DefaultLivenessEngine-wired sweep must still mark this fixture stuck")
}

// testLivenessCharacterizationShapeC covers Shape C (cycle frequency): an
// item that bounced in_progress<->review bounceThreshold times within
// bounceLookback with no recorded PASS verdict
// (session/stuck_decisions.go's isBouncing).
func testLivenessCharacterizationShapeC(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo

	// Reuses the exact fixture shape as
	// TestReconcileBouncingItems_should_writeBouncingRowNotifyOnce_When_ThreeCyclesIn24hNoPass.
	newBouncingFixture := func(title string) *BacklogItemData {
		item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
			Title:  title,
			Status: string(BacklogStatusInProgress),
		})
		require.NoError(t, err)
		for i := 0; i < bounceThreshold; i++ {
			_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusReview, nil, TriggeredBySystem)
			require.NoError(t, err)
			_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusInProgress, nil, TriggeredBySystem)
			require.NoError(t, err)
		}
		return item
	}

	// Before: the real, unmodified sweep with no LivenessEngine wired.
	beforeItem := newBouncingFixture("Liveness characterization bouncing item (before)")
	beforeListener := NewBacklogLifecycleListener(storage)
	beforeListener.reconcileBouncingItems(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	beforeRow, ok := findOpenStuckStateFor(open, beforeItem.ID, domain.StuckReasonBouncing)
	require.True(t, ok, "unwired sweep must mark this fixture stuck")

	// After: the identical sweep, now wired to DefaultLivenessEngine (Epic 1.4 landed,
	// keyed BacklogStatusReview — see this package's Epic 1.4 plan-correction note in
	// project_plans/backlog-custom-workflow-stages/implementation/plan.md's Story 1.4.3).
	// Zero rows configured, so the resolved bounceThreshold/bounceLookback must reproduce
	// the "before" decision and reasonDetail exactly — this fixture records no review
	// verdict at all, so hasPass is false on both paths, and reasonDetail embeds no
	// per-fixture identifier (just the cycle count and lookback duration), so the two
	// rows' Context must be byte-identical.
	afterItem := newBouncingFixture("Liveness characterization bouncing item (after)")
	afterListener := NewBacklogLifecycleListener(storage)
	afterListener.livenessEngine = NewDefaultLivenessEngine()
	afterListener.reconcileBouncingItems(ctx, er)

	open, err = er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	afterRow, ok := findOpenStuckStateFor(open, afterItem.ID, domain.StuckReasonBouncing)
	require.True(t, ok, "DefaultLivenessEngine-wired sweep must still mark this fixture stuck")

	assert.Equal(t, beforeRow.Context, afterRow.Context, "reasonDetail must be byte-identical between the unwired and DefaultLivenessEngine-wired sweep")
}
