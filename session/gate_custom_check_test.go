package session

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/headless"
)

// fakeCustomCheckCaller is a test double for CustomCheckCaller: it returns a
// canned (output, err) pair and records the args it was called with, without
// spawning any real subprocess.
type fakeCustomCheckCaller struct {
	output string
	err    error

	called       bool
	lastKey      headless.FeatureKey
	lastSystem   string
	lastUserText string
}

func (f *fakeCustomCheckCaller) CallBlocking(_ context.Context, key headless.FeatureKey, systemPrompt, userPrompt string, _ headless.CallOptions, _ headless.CostSink) (string, error) {
	f.called = true
	f.lastKey = key
	f.lastSystem = systemPrompt
	f.lastUserText = userPrompt
	return f.output, f.err
}

// newCustomCheckTestItem creates a minimal BacklogItemData for
// InvokeCustomGateCheck tests — no repo path/worktree needed since this gate
// kind never computes a diff.
func newCustomCheckTestItem(t *testing.T, storage *Storage, title string) *BacklogItemData {
	t.Helper()
	item, err := storage.CreateBacklogItem(context.Background(), BacklogItemData{
		Title:              title,
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusIdea),
	})
	require.NoError(t, err)
	return item
}

// newCustomCheckTestGate creates a real TransitionGate row (idea -> ready,
// kind=custom) and returns its ID — GateSatisfactionRecord.gate_id is a
// required ent edge to TransitionGate, so a bare uuid.New() with no backing
// row fails the FK constraint on Create.
func newCustomCheckTestGate(t *testing.T, storage *Storage) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	client := storage.GetEntClient()

	fromStage, err := client.BacklogStage.Create().SetSlug("gcc-from-" + uuid.NewString()).SetName("From").Save(ctx)
	require.NoError(t, err)
	toStage, err := client.BacklogStage.Create().SetSlug("gcc-to-" + uuid.NewString()).SetName("To").Save(ctx)
	require.NoError(t, err)
	transition, err := client.StageTransition.Create().
		SetFromStageID(fromStage.ID).
		SetToStageID(toStage.ID).
		Save(ctx)
	require.NoError(t, err)
	gate, err := client.TransitionGate.Create().
		SetTransitionID(transition.ID).
		SetKind(string(GateKindCustom)).
		SetStateful(true).
		Save(ctx)
	require.NoError(t, err)
	return gate.ID
}

// TestInvokeCustomGateCheck_should_BlockTransitionFailClosed_When_SkillNotInPreRegisteredAllowlist
// covers Story 2.4.4's Task 2.4.4d: a GateKindCustom config naming a skill
// outside registeredCustomCheckSkills must return an error and never spawn a
// call — fail-closed for gates, per ADR-003.
func TestInvokeCustomGateCheck_should_BlockTransitionFailClosed_When_SkillNotInPreRegisteredAllowlist(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	item := newCustomCheckTestItem(t, storage, "Unregistered skill test item")
	caller := &fakeCustomCheckCaller{output: "VERDICT: PASS"}
	repo := NewEntGateSatisfactionRepository(storage.GetEntClient())
	gateID := newCustomCheckTestGate(t, storage)

	livenessDef := LivenessDefinition{Kind: LivenessKindDurationBudget, ExpectedDuration: 0, StalenessMargin: 0}

	_, err := InvokeCustomGateCheck(context.Background(), caller, repo, item, gateID,
		CustomCheckConfig{SkillID: "not-a-real-skill"}, livenessDef, BacklogStatusReady, PipelineModeDefault)

	require.Error(t, err, "an unregistered skill must be rejected")
	assert.False(t, caller.called, "the headless call must never be attempted for an unregistered skill")

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
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	item := newCustomCheckTestItem(t, storage, "Synchronous spawn failure test item")
	spawnErr := errors.New("exit status 127: claude: command not found")
	caller := &fakeCustomCheckCaller{err: spawnErr}
	repo := NewEntGateSatisfactionRepository(storage.GetEntClient())
	gateID := newCustomCheckTestGate(t, storage)

	livenessDef := LivenessDefinition{Kind: LivenessKindDurationBudget, ExpectedDuration: 0, StalenessMargin: 0}

	_, err := InvokeCustomGateCheck(context.Background(), caller, repo, item, gateID,
		CustomCheckConfig{SkillID: "review-feasibility"}, livenessDef, BacklogStatusReady, PipelineModeDefault)

	require.Error(t, err, "a synchronous spawn failure must block the transition immediately")
	assert.True(t, caller.called)

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
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	item := newCustomCheckTestItem(t, storage, "Pass verdict test item")
	caller := &fakeCustomCheckCaller{output: "Looks buildable.\n\nVERDICT: PASS"}
	repo := NewEntGateSatisfactionRepository(storage.GetEntClient())
	gateID := newCustomCheckTestGate(t, storage)

	livenessDef := LivenessDefinition{Kind: LivenessKindDurationBudget, ExpectedDuration: 0, StalenessMargin: 0}

	outcome, err := InvokeCustomGateCheck(context.Background(), caller, repo, item, gateID,
		CustomCheckConfig{SkillID: "review-feasibility"}, livenessDef, BacklogStatusReady, PipelineModeDefault)

	require.NoError(t, err)
	assert.Equal(t, ReviewOutcomePass, outcome.Outcome)
	assert.True(t, caller.called)

	record, getErr := repo.GetByItemAndGate(context.Background(), uuid.MustParse(item.ID), gateID)
	require.NoError(t, getErr)
	assert.True(t, record.Satisfied)
}
