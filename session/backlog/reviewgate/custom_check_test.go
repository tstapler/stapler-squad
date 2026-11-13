package reviewgate

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/backlog/internal/blktest"
	"github.com/tstapler/stapler-squad/session/backlog/liveness"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

// TestInvokeCustomGateCheck_should_BlockTransitionFailClosed_When_SkillNotInPreRegisteredAllowlist
// covers Story 2.4.4's Task 2.4.4d: a GateKindCustom config naming a skill
// outside registeredCustomCheckSkills must return an error and never spawn a
// call — fail-closed for gates, per ADR-003.
func TestInvokeCustomGateCheck_should_BlockTransitionFailClosed_When_SkillNotInPreRegisteredAllowlist(t *testing.T) {
	t.Parallel()
	storage, cleanup := blktest.CreateTestStorage(t)
	defer cleanup()

	item := blktest.NewCustomCheckTestItem(t, storage, "Unregistered skill test item")
	caller := &blktest.FakeCustomCheckCaller{Output: "VERDICT: PASS"}
	repo := session.NewEntGateSatisfactionRepository(storage.GetEntClient())
	gateID := blktest.NewCustomCheckTestGate(t, storage)

	livenessDef := liveness.LivenessDefinition{Kind: liveness.LivenessKindDurationBudget, ExpectedDuration: 0, StalenessMargin: 0}

	_, err := InvokeCustomGateCheck(context.Background(), caller, repo, item, gateID,
		domain.CustomCheckConfig{SkillID: "not-a-real-skill"}, livenessDef, domain.BacklogStatusReady, backlogdto.PipelineModeDefault)

	require.Error(t, err, "an unregistered skill must be rejected")
	assert.False(t, caller.Called, "the headless call must never be attempted for an unregistered skill")

	_, getErr := repo.GetByItemAndGate(context.Background(), uuid.MustParse(item.ID), gateID)
	assert.Error(t, getErr, "no GateSatisfactionRecord should be created for a rejected invocation")
}

// TestInvokeCustomGateCheck_should_BlockAndLogWarn_When_SpawnFailsSynchronously
// covers Story 2.4.4's Task 2.4.4e: a synchronous (non-timeout) spawn failure
// — the fake's CallBlocking returning an error immediately, modeling a
// non-zero exit / missing runtime dependency — must block the transition
// (a returned error) rather than silently passing or merely relying on the
// liveness sweep to eventually notice, distinct from the timeout-detection
// path covered by TestReconcileCustomGateChecks_* in
// backlog_lifecycle_stuck_test.go.
func TestInvokeCustomGateCheck_should_BlockAndLogWarn_When_SpawnFailsSynchronously(t *testing.T) {
	t.Parallel()
	storage, cleanup := blktest.CreateTestStorage(t)
	defer cleanup()

	item := blktest.NewCustomCheckTestItem(t, storage, "Synchronous spawn failure test item")
	spawnErr := errors.New("exit status 127: claude: command not found")
	caller := &blktest.FakeCustomCheckCaller{Err: spawnErr}
	repo := session.NewEntGateSatisfactionRepository(storage.GetEntClient())
	gateID := blktest.NewCustomCheckTestGate(t, storage)

	livenessDef := liveness.LivenessDefinition{Kind: liveness.LivenessKindDurationBudget, ExpectedDuration: 0, StalenessMargin: 0}

	_, err := InvokeCustomGateCheck(context.Background(), caller, repo, item, gateID,
		domain.CustomCheckConfig{SkillID: "review-feasibility"}, livenessDef, domain.BacklogStatusReady, backlogdto.PipelineModeDefault)

	require.Error(t, err, "a synchronous spawn failure must block the transition immediately")
	assert.True(t, caller.Called)

	record, getErr := repo.GetByItemAndGate(context.Background(), uuid.MustParse(item.ID), gateID)
	require.NoError(t, getErr, "the in-flight row must still exist, now recording the failure")
	assert.False(t, record.Satisfied)
	assert.Contains(t, record.OutcomeDetail, "error")
}

// TestInvokeCustomGateCheck_should_RecordPassVerdict_When_SkillReportsPass
// covers the happy path: a registered skill whose headless call returns a
// recognizable "VERDICT: PASS" line records a satisfied GateSatisfactionRecord.
func TestInvokeCustomGateCheck_should_RecordPassVerdict_When_SkillReportsPass(t *testing.T) {
	t.Parallel()
	storage, cleanup := blktest.CreateTestStorage(t)
	defer cleanup()

	item := blktest.NewCustomCheckTestItem(t, storage, "Pass verdict test item")
	caller := &blktest.FakeCustomCheckCaller{Output: "Looks buildable.\n\nVERDICT: PASS"}
	repo := session.NewEntGateSatisfactionRepository(storage.GetEntClient())
	gateID := blktest.NewCustomCheckTestGate(t, storage)

	livenessDef := liveness.LivenessDefinition{Kind: liveness.LivenessKindDurationBudget, ExpectedDuration: 0, StalenessMargin: 0}

	outcome, err := InvokeCustomGateCheck(context.Background(), caller, repo, item, gateID,
		domain.CustomCheckConfig{SkillID: "review-feasibility"}, livenessDef, domain.BacklogStatusReady, backlogdto.PipelineModeDefault)

	require.NoError(t, err)
	assert.Equal(t, domain.ReviewOutcomePass, outcome.Outcome)
	assert.True(t, caller.Called)

	record, getErr := repo.GetByItemAndGate(context.Background(), uuid.MustParse(item.ID), gateID)
	require.NoError(t, getErr)
	assert.True(t, record.Satisfied)
}
