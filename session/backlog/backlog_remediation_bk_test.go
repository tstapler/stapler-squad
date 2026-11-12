package backlog

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/internal/sessiontest"
)

// TestRemediationDue_should_ReturnTrue_When_ParkedOrphanedTriageRowGetsLivenessOverrideWithZeroCodeChange
// is Epic 1.5 Story 1.5.1's regression test for the specific Success Metrics
// claim: a BacklogStuckState row parked BEFORE Milestone 1 ships (under the
// old hardcoded 35m orphaned_triage threshold) recovers via the existing
// RemediationDue cold-retry heartbeat (BUG-083) once the sdd-mode
// LivenessDefinition override is created — with ZERO code change to
// RemediationDue/evaluateRemediation (backlog_remediation.go). This proves
// BUG-083's "write-side fixed (Epic 1.4's reconcile* sweeps now consult
// LivenessEngine), recovery-side untested" pattern doesn't repeat here: the
// write side and the pre-existing cold-retry heartbeat compose correctly with
// no glue code, because RemediationDue never references LivenessEngine at
// all — it gates purely on remediation_attempts/next_remediation_at, which is
// exactly why creating the override cannot require touching it.
func TestRemediationDue_should_ReturnTrue_When_ParkedOrphanedTriageRowGetsLivenessOverrideWithZeroCodeChange(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)
	ctx := context.Background()

	// The parked row: an sdd-mode idea-stage item that hit the fast-schedule
	// cap under the OLD flat 35m orphaned_triage threshold, exactly the shape
	// of the 12 real items this epic exists for (reason="orphaned_triage",
	// remediation_attempts=5, next_remediation_at 7+ days in the past — the
	// BUG-083 cold-retry deadline already elapsed).
	item, err := repo.CreateBacklogItem(ctx, backlogdto.BacklogItemData{
		Title:        "parked orphaned-triage item (sdd mode)",
		Status:       string(session.BacklogStatusIdea),
		PipelineMode: "sdd",
	})
	require.NoError(t, err)
	itemID := item.ID

	_, err = repo.MarkStuck(ctx, itemID, domain.StuckReasonOrphanedTriage, session.BacklogStatusIdea, "orphaned triage")
	require.NoError(t, err)

	sevenDaysAgo := time.Now().Add(-7*24*time.Hour - time.Hour)
	applied, err := repo.RecordRemediationAttempt(ctx, itemID, domain.StuckReasonOrphanedTriage, session.MaxRemediationAttempts, sessiontest.TimePtr(sevenDaysAgo))
	require.NoError(t, err)
	require.True(t, applied)

	// Confirm the fixture is actually parked (attempts at cap) before
	// introducing the override — otherwise a passing assertion below would
	// prove nothing about the parked-row recovery path specifically.
	rowsBefore, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	rowBefore, ok := session.FindOpenStuckStateFor(rowsBefore, itemID, domain.StuckReasonOrphanedTriage)
	require.True(t, ok)
	require.Equal(t, session.MaxRemediationAttempts, rowBefore.RemediationAttempts)

	// Milestone 1 "ships" here: the sdd-mode override is created via the
	// Epic 1.3 repository (the same path CreateLivenessDefinition's RPC
	// handler drives), raising the idea-stage/sdd-mode expected duration well
	// past the old flat 35m constant.
	livenessRepo := NewEntLivenessRepository(storage.GetEntClient())
	sddMode := "sdd"
	_, err = livenessRepo.Create(ctx, LivenessCreateInput{
		StageSlug:    string(session.BacklogStatusIdea),
		PipelineMode: &sddMode,
		Definition: LivenessDefinition{
			Kind:             LivenessKindDurationBudget,
			ExpectedDuration: 45 * time.Minute,
			StalenessMargin:  10 * time.Minute,
		},
	})
	require.NoError(t, err)
	engine, err := NewCachingLivenessEngine(livenessRepo)
	require.NoError(t, err)

	// Prove the override actually resolves through LivenessEngine — the
	// "write side" this test composes against, so a failure here (rather than
	// below) would mean the fixture is wrong, not that RemediationDue broke.
	resolved, err := engine.LivenessFor(session.BacklogStatusIdea, PipelineMode("sdd"))
	require.NoError(t, err)
	require.Equal(t, LivenessKindDurationBudget, resolved.Kind)
	assert.Equal(t, 55*time.Minute, resolved.StalenessThreshold(), "sdd-mode override must resolve to the new 55m derived threshold, not the old 35m default")

	// The actual regression assertion: RemediationDue — completely unaware
	// that a LivenessEngine or any override even exists — still reports this
	// parked row as due, purely because its own cold-retry deadline elapsed.
	// No call to ResetStuckRemediation/BulkResetStuckRemediation anywhere in
	// this test.
	due, justParked, gateErr := storage.RemediationDue(ctx, itemID, domain.StuckReasonOrphanedTriage)
	require.NoError(t, gateErr)
	assert.True(t, due, "a parked row past its cold-retry deadline must become due once Milestone 1's override is configured, with zero backlog_remediation.go change")
	assert.False(t, justParked, "this is a cold retry of an already-parked row, not a fresh park event")

	rowsAfter, err := storage.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	rowAfter, ok := session.FindOpenStuckStateFor(rowsAfter, itemID, domain.StuckReasonOrphanedTriage)
	require.True(t, ok)
	assert.Equal(t, session.MaxRemediationAttempts+1, rowAfter.RemediationAttempts, "cold retry must count against MaxRemediationColdRetries")
	require.NotNil(t, rowAfter.NextRemediationAt)
	assert.WithinDuration(t, time.Now().Add(session.RemediationColdRetryInterval), *rowAfter.NextRemediationAt, 5*time.Second, "cold-retry deadline must advance another full interval")
}
