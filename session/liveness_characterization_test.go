package session

import (
	"context"
	"fmt"
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
// decision by resolving DefaultLivenessEngine.LivenessFor and manually
// applying its returned definition the same way each reconcile* function's
// own logic does, and asserts the two are byte-identical.
//
// No reconcile* sweep is wired to LivenessEngine yet — that's Epic 1.4 — so
// this test cannot yet run one code path through the engine and diff outputs
// of a single call. Each subtest below is deliberately split into a
// "before" half (calls the real, unmodified reconcile* method) and an
// "after" half (a few lines manually mirroring that same method's
// decision/formatting logic, but reading thresholds from the engine instead
// of the package-level constant) so that once Epic 1.4 lands, the "after"
// half of each subtest can be replaced with a call through the real sweep's
// new LivenessEngine-backed code path with no change to fixture setup or the
// "before" capture.
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
	item := newOrphanedTriageTestItem(t, storage, er, ageAgo)

	sessionsBefore, err := storage.ListItemSessions(ctx, item.ID)
	require.NoError(t, err)
	sessionUUID := requireSingleTriageSessionUUID(t, sessionsBefore)

	// Before: the real, unmodified hardcoded-constant sweep.
	listener := NewBacklogLifecycleListener(storage)
	listener.reconcileOrphanedTriageItems(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	beforeRow, ok := findOpenStuckStateFor(open, item.ID, domain.StuckReasonOrphanedTriage)
	require.True(t, ok, "hardcoded-constant sweep must mark this fixture stuck")

	// After: DefaultLivenessEngine-resolved decision, manually mirroring
	// reconcileOrphanedTriageItems' Shape 1 branch (session/backlog_lifecycle_triage.go:
	// `reasonDetail = fmt.Sprintf("triage session %s still open after %s", ...)`).
	engine := NewDefaultLivenessEngine()
	def, err := engine.LivenessFor(BacklogStatusIdea, PipelineModeDefault)
	require.NoError(t, err)
	require.Equal(t, LivenessKindDurationBudget, def.Kind)
	threshold := def.StalenessThreshold()

	afterStuck := ageAgo > threshold
	afterReasonDetail := fmt.Sprintf("triage session %s still open after %s", sessionUUID, threshold)

	assert.True(t, afterStuck, "engine-resolved threshold must agree the fixture is stuck")
	assert.Equal(t, beforeRow.Context, afterReasonDetail, "reasonDetail must be byte-identical between the hardcoded path and the engine-derived path")
}

// requireSingleTriageSessionUUID returns the single triage-role session's
// SessionUUID from sessions, failing the test if there isn't exactly one.
func requireSingleTriageSessionUUID(t *testing.T, sessions []ItemSessionSummary) string {
	t.Helper()
	for _, s := range sessions {
		if s.Role == string(SessionRoleTriage) {
			return s.SessionUUID
		}
	}
	require.Fail(t, "no triage-role session found in fixture")
	return ""
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
	item := newStaleWorkTestItem(t, storage, er)

	sessions, err := storage.ListItemSessions(ctx, item.ID)
	require.NoError(t, err)
	lastProgress := requireActiveWorkSessionLastProgress(t, sessions)

	// Before: the real, unmodified hardcoded-constant sweep.
	listener := NewBacklogLifecycleListener(storage)
	listener.reconcileStaleWorkSessions(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	beforeRow, ok := findOpenStuckStateFor(open, item.ID, domain.StuckReasonStaleWork)
	require.True(t, ok, "hardcoded-constant sweep must mark this fixture stuck")

	// After: DefaultLivenessEngine-resolved decision, manually mirroring
	// reconcileStaleWorkSessions (session/backlog_lifecycle_stale.go:
	// `staleWork(lastProgress, now)` and
	// `fmt.Sprintf("no progress since %s", lastProgress)`).
	engine := NewDefaultLivenessEngine()
	def, err := engine.LivenessFor(BacklogStatusInProgress, PipelineModeDefault)
	require.NoError(t, err)
	require.Equal(t, LivenessKindHeartbeat, def.Kind)

	afterStuck := time.Since(lastProgress) > def.MaxNoProgressDuration
	afterReasonDetail := fmt.Sprintf("no progress since %s", lastProgress)

	assert.True(t, afterStuck, "engine-resolved threshold must agree the fixture is stuck")
	assert.Equal(t, beforeRow.Context, afterReasonDetail, "reasonDetail must be byte-identical between the hardcoded path and the engine-derived path")
}

// requireActiveWorkSessionLastProgress returns the effective last-progress
// timestamp (LastProgressAt if set, else CreatedAt — mirroring
// reconcileStaleWorkSessions' own fallback) of the single active
// (EndedAt == nil) work-role session in sessions.
func requireActiveWorkSessionLastProgress(t *testing.T, sessions []ItemSessionSummary) time.Time {
	t.Helper()
	for _, s := range sessions {
		if s.Role == string(SessionRoleWork) && s.EndedAt == nil {
			if s.LastProgressAt != nil {
				return *s.LastProgressAt
			}
			return s.CreatedAt
		}
	}
	require.Fail(t, "no active work-role session found in fixture")
	return time.Time{}
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
	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "Liveness characterization bouncing item",
		Status: string(BacklogStatusInProgress),
	})
	require.NoError(t, err)
	for i := 0; i < bounceThreshold; i++ {
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusReview, nil, TriggeredBySystem)
		require.NoError(t, err)
		_, err = storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusInProgress, nil, TriggeredBySystem)
		require.NoError(t, err)
	}

	// Before: the real, unmodified hardcoded-constant sweep.
	listener := NewBacklogLifecycleListener(storage)
	listener.reconcileBouncingItems(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	beforeRow, ok := findOpenStuckStateFor(open, item.ID, domain.StuckReasonBouncing)
	require.True(t, ok, "hardcoded-constant sweep must mark this fixture stuck")

	// After: DefaultLivenessEngine-resolved decision, manually mirroring
	// reconcileBouncingItems (session/backlog_lifecycle.go: `isBouncing(count,
	// hasPass)` and `fmt.Sprintf("bounced in_progress<->review %d times in the
	// last %s with no PASS verdict", count, bounceLookback)`). This fixture
	// records no review verdict at all, so hasPass is false on both paths.
	engine := NewDefaultLivenessEngine()
	def, err := engine.LivenessFor(BacklogStatusReview, PipelineModeDefault)
	require.NoError(t, err)
	require.Equal(t, LivenessKindCycleFrequency, def.Kind)

	count, err := er.CountReviewCyclesSince(ctx, item.ID, time.Now().Add(-def.CycleLookback))
	require.NoError(t, err)
	const hasPass = false
	afterStuck := count >= def.CycleThreshold && !hasPass
	afterReasonDetail := fmt.Sprintf("bounced in_progress<->review %d times in the last %s with no PASS verdict", count, def.CycleLookback)

	assert.True(t, afterStuck, "engine-resolved threshold must agree the fixture is bouncing")
	assert.Equal(t, beforeRow.Context, afterReasonDetail, "reasonDetail must be byte-identical between the hardcoded path and the engine-derived path")
}
