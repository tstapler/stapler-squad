package session

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent/backlogstage"
	"github.com/tstapler/stapler-squad/session/ent/stagetransition"
)

// newGateTimeoutTestFixture creates a real TransitionGate row (fromSlug ->
// toSlug, kind=custom) plus a backlog item, and inserts a
// GateSatisfactionRecord row for that (item, gate) pair with Satisfied:false
// and CreatedAt backdated by age — modeling InvokeCustomGateCheck's in-flight
// row (session/gate_custom_check.go) at invocation time. Returns the item and
// the gate's ID.
func newGateTimeoutTestFixture(t *testing.T, storage *Storage, toSlug string, age time.Duration, expectedDuration, stalenessMargin time.Duration) (*BacklogItemData, uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	client := storage.GetEntClient()

	fromStage, err := client.BacklogStage.Create().SetSlug("gate-timeout-from-" + uuid.NewString()).SetName("From").Save(ctx)
	require.NoError(t, err)
	toStage, err := client.BacklogStage.Create().SetSlug(toSlug).SetName(toSlug).Save(ctx)
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

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{
		Title:              "Gate timeout test item",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusIdea),
	})
	require.NoError(t, err)

	_, err = client.GateSatisfactionRecord.Create().
		SetItemID(uuid.MustParse(item.ID)).
		SetGateID(gate.ID).
		SetSatisfied(false).
		SetCreatedAt(time.Now().Add(-age)).
		SetOutcomeDetail(map[string]interface{}{
			"skill":                     "review-feasibility",
			"expected_duration_seconds": expectedDuration.Seconds(),
			"staleness_margin_seconds":  stalenessMargin.Seconds(),
			"target_stage":              toSlug,
			"pipeline_mode":             "",
		}).
		Save(ctx)
	require.NoError(t, err)

	return item, gate.ID
}

// TestReconcileCustomGateChecks_should_MarkStuckViaLivenessEngine_When_CustomCheckExceedsExpectedDurationPlusMargin
// covers Story 2.4.4's Task 2.4.4d/validation.md scenario: a custom-check
// invocation bounded by a LivenessDefinition{ExpectedDuration: 10m,
// StalenessMargin: 5m} that has been open 16m is caught by the same
// reconcile* sweep infrastructure Epic 1.4 built, via a live
// LivenessEngine.LivenessFor resolution for the gate's bound target stage —
// not a new detector.
func TestReconcileCustomGateChecks_should_MarkStuckViaLivenessEngine_When_CustomCheckExceedsExpectedDurationPlusMargin(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo

	const toSlug = "gate-timeout-ready"
	livenessRepo := NewEntLivenessRepository(storage.GetEntClient())
	_, err := livenessRepo.Create(ctx, LivenessCreateInput{
		StageSlug: toSlug,
		Definition: LivenessDefinition{
			Kind:             LivenessKindDurationBudget,
			ExpectedDuration: 10 * time.Minute,
			StalenessMargin:  5 * time.Minute,
		},
	})
	require.NoError(t, err)
	engine, err := NewCachingLivenessEngine(livenessRepo)
	require.NoError(t, err)

	item, gateID := newGateTimeoutTestFixture(t, storage, toSlug, 16*time.Minute, 10*time.Minute, 5*time.Minute)

	repo := NewEntGateSatisfactionRepository(storage.GetEntClient())
	listener := NewBacklogLifecycleListener(storage)
	listener.livenessEngine = engine
	listener.gateSatisfactionRepo = repo

	listener.reconcileCustomGateChecks(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	row, ok := findOpenStuckStateFor(open, item.ID, domain.StuckReasonGateTimeout)
	require.True(t, ok, "a 16m-old invocation must be marked StuckReasonGateTimeout under the 15m (10m+5m) threshold")
	assert.Contains(t, row.Context, gateID.String())
}

// TestReconcileCustomGateChecks_should_NotMarkStuck_When_CustomCheckWithinBudget
// covers the zero-regression converse: an invocation still within its bound
// budget must not be flagged.
func TestReconcileCustomGateChecks_should_NotMarkStuck_When_CustomCheckWithinBudget(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo

	item, _ := newGateTimeoutTestFixture(t, storage, "gate-within-budget", 2*time.Minute, 10*time.Minute, 5*time.Minute)

	repo := NewEntGateSatisfactionRepository(storage.GetEntClient())
	listener := NewBacklogLifecycleListener(storage)
	listener.gateSatisfactionRepo = repo

	listener.reconcileCustomGateChecks(ctx, er)

	open, err := er.FindOpenStuckStates(ctx)
	require.NoError(t, err)
	_, ok := findOpenStuckStateFor(open, item.ID, domain.StuckReasonGateTimeout)
	assert.False(t, ok, "a 2m-old invocation under a 15m threshold must not be flagged")
}

// TestReconcileCustomGateChecks_should_BeNoOp_When_NoGateSatisfactionRepoWired
// covers the nil-safe default: an unwired repo makes the sweep a no-op,
// matching every other optional-dependency detector's convention.
func TestReconcileCustomGateChecks_should_BeNoOp_When_NoGateSatisfactionRepoWired(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	er := storage.repo

	listener := NewBacklogLifecycleListener(storage)
	require.NotPanics(t, func() {
		listener.reconcileCustomGateChecks(ctx, er)
	})
}

// --- Epic 2.4 follow-up: production call sites for resolveReviewGateContext/
// resolveCustomCheckGateContext (the gap this task closes) ---

// TestResolveCustomCheckGateContext_should_ReturnNotOK_When_NoWorkflowEngineWired
// covers the nil-safe default: an unwired workflowEngine (today's production
// default) must never spawn a custom-check gate.
func TestResolveCustomCheckGateContext_should_ReturnNotOK_When_NoWorkflowEngineWired(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	listener := NewBacklogLifecycleListener(storage)
	_, _, ok := listener.resolveCustomCheckGateContext(BacklogStatusInProgress, BacklogStatusReview)
	assert.False(t, ok)
}

// TestResolveCustomCheckGateContext_should_ReturnGateAndConfig_When_TransitionHasCustomCheckGate
// covers Story 2.4.4's follow-up resolver wired through the listener: a
// GateKindCustom gate configured on the in_progress->review edge (the exact
// edge onSessionExited's session-exit event drives) resolves to its own
// GateID + CustomCheckConfig via the listener's wired ConfiguredWorkflowEngine.
func TestResolveCustomCheckGateContext_should_ReturnGateAndConfig_When_TransitionHasCustomCheckGate(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	client := storage.GetEntClient()

	require.NoError(t, EnsureBuiltInWorkflowStages(ctx, client))
	fromStage, err := client.BacklogStage.Query().Where(backlogstage.Slug(string(BacklogStatusInProgress))).Only(ctx)
	require.NoError(t, err)
	toStage, err := client.BacklogStage.Query().Where(backlogstage.Slug(string(BacklogStatusReview))).Only(ctx)
	require.NoError(t, err)
	transition, err := client.StageTransition.Query().
		Where(stagetransition.FromStageID(fromStage.ID), stagetransition.ToStageID(toStage.ID)).
		Only(ctx)
	require.NoError(t, err)
	gate, err := client.TransitionGate.Create().
		SetTransitionID(transition.ID).
		SetKind(string(GateKindCustom)).
		SetStateful(true).
		SetEnabled(true).
		SetConfig(map[string]interface{}{"skill": "review-feasibility"}).
		Save(ctx)
	require.NoError(t, err)

	stageRepo := NewEntStageConfigRepository(client)
	gateRepo := NewEntGateSatisfactionRepository(client)
	engine, err := NewConfiguredWorkflowEngine(stageRepo, gateRepo, nil)
	require.NoError(t, err)

	listener := NewBacklogLifecycleListener(storage)
	listener.SetWorkflowEngine(engine)

	gateID, cfg, ok := listener.resolveCustomCheckGateContext(BacklogStatusInProgress, BacklogStatusReview)
	require.True(t, ok)
	assert.Equal(t, gate.ID, gateID)
	assert.Equal(t, "review-feasibility", cfg.SkillID)
}

// TestRunCustomGateCheckWithCaller_should_InvokeCallerAndRecordOutcome_When_Triggered
// covers Story 2.4.4's follow-up (gap #4): runCustomGateCheckWithCaller —
// the seam runCustomGateCheck delegates to once it has a real *headless.Pool
// — actually invokes InvokeCustomGateCheck via a test double CustomCheckCaller,
// and the terminal outcome becomes visible via a subsequent PendingGates call.
// This is the previously-unreachable production path: before this Epic's
// follow-up, InvokeCustomGateCheck had zero call sites outside tests (see
// gate_custom_check.go's own doc comment).
func TestRunCustomGateCheckWithCaller_should_InvokeCallerAndRecordOutcome_When_Triggered(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	ctx := context.Background()
	client := storage.GetEntClient()

	gateID := newCustomCheckTestGate(t, storage)
	item := newCustomCheckTestItem(t, storage, "runCustomGateCheckWithCaller wiring test")

	repo := NewEntGateSatisfactionRepository(client)
	listener := NewBacklogLifecycleListener(storage)
	listener.SetGateSatisfactionRepository(repo)

	caller := &fakeCustomCheckCaller{output: "Looks buildable.\n\nVERDICT: PASS"}
	cfg := CustomCheckConfig{SkillID: "review-feasibility"}

	listener.runCustomGateCheckWithCaller(ctx, caller, gateID, cfg, BacklogStatusReady, item)

	assert.True(t, caller.called, "InvokeCustomGateCheck must actually invoke the caller")

	record, getErr := repo.GetByItemAndGate(ctx, uuid.MustParse(item.ID), gateID)
	require.NoError(t, getErr)
	assert.True(t, record.Satisfied, "a VERDICT: PASS output must record a satisfied outcome")
}
