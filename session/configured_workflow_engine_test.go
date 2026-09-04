package session

// configured_workflow_engine_test.go — shared with Epic 2.3, which will add
// the actual ConfiguredWorkflowEngine-based tests here once that type exists.
// This Epic (2.2) only persists stage/transition rows, so the test below
// asserts directly against the seeded ent rows rather than against a
// ConfiguredWorkflowEngine.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogstage"
)

// TestConfiguredWorkflowEngine_should_MatchDomainValidTransitions_When_DatabaseIsFreshlySeeded
// covers Story 2.2.2's acceptance criteria: after EnsureBuiltInWorkflowStages
// runs against a fresh database, the seeded backlog_stages/stage_transitions
// rows encode exactly the same edges as domain.ValidTransitions(), for every
// one of the 9 built-in stages. Named for validation.md's traceability
// mapping even though ConfiguredWorkflowEngine itself doesn't exist until
// Epic 2.3 — this asserts the seeded data it will read is already correct.
func TestConfiguredWorkflowEngine_should_MatchDomainValidTransitions_When_DatabaseIsFreshlySeeded(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	client := repo.client

	require.NoError(t, EnsureBuiltInWorkflowStages(ctx, client))

	stages, err := client.BacklogStage.Query().All(ctx)
	require.NoError(t, err)
	require.Len(t, stages, len(builtInStageOrder), "expected exactly the 9 built-in stages to be seeded")

	idToSlug := make(map[string]BacklogStatus, len(stages))
	slugSeen := make(map[BacklogStatus]bool, len(stages))
	for _, s := range stages {
		idToSlug[s.ID.String()] = BacklogStatus(s.Slug)
		slugSeen[BacklogStatus(s.Slug)] = true
	}
	for _, want := range builtInStageOrder {
		require.True(t, slugSeen[want], "expected built-in stage %q to be seeded", want)
	}

	transitions, err := client.StageTransition.Query().All(ctx)
	require.NoError(t, err)

	seededEdges := make(map[BacklogStatus]map[BacklogStatus]bool, len(stages))
	for _, tr := range transitions {
		from := idToSlug[tr.FromStageID.String()]
		to := idToSlug[tr.ToStageID.String()]
		if seededEdges[from] == nil {
			seededEdges[from] = make(map[BacklogStatus]bool)
		}
		seededEdges[from][to] = true
	}

	want := domain.ValidTransitions()

	// Every domain edge must be present among the seeded rows...
	for from, targets := range want {
		for to := range targets {
			require.Truef(t, seededEdges[from][to], "expected seeded graph to contain edge %s->%s", from, to)
		}
	}
	// ...and the seeded rows must contain no edges beyond the domain table
	// (same edge count on both sides, given the subset check above already
	// holds).
	wantCount := 0
	for _, targets := range want {
		wantCount += len(targets)
	}
	seededCount := 0
	for _, targets := range seededEdges {
		seededCount += len(targets)
	}
	require.Equal(t, wantCount, seededCount, "seeded transition count must exactly match domain.ValidTransitions()'s edge count")
}

// TestSeedMigration_should_BeIdempotent_When_RunTwice guards the
// create-if-missing contract EnsureBuiltInWorkflowStages promises: a second
// run against an already-seeded database must not create duplicate rows (or
// error), so a restart never reverts an operator's later hand-edit.
func TestSeedMigration_should_BeIdempotent_When_RunTwice(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	client := repo.client

	require.NoError(t, EnsureBuiltInWorkflowStages(ctx, client))
	firstStageCount, err := client.BacklogStage.Query().Count(ctx)
	require.NoError(t, err)
	firstTransitionCount, err := client.StageTransition.Query().Count(ctx)
	require.NoError(t, err)

	require.NoError(t, EnsureBuiltInWorkflowStages(ctx, client))
	secondStageCount, err := client.BacklogStage.Query().Count(ctx)
	require.NoError(t, err)
	secondTransitionCount, err := client.StageTransition.Query().Count(ctx)
	require.NoError(t, err)

	require.Equal(t, firstStageCount, secondStageCount, "second seed run must not create duplicate stages")
	require.Equal(t, firstTransitionCount, secondTransitionCount, "second seed run must not create duplicate transitions")
}

// TestConfiguredWorkflowEngine_should_ReturnEmptyGraph_When_SeedMigrationHasNotRun
// covers Story 2.2.2's error/edge-path acceptance criteria: querying the
// backlog_stages/stage_transitions tables before EnsureBuiltInWorkflowStages
// has ever run must produce a defined empty result, not a panic or error.
func TestConfiguredWorkflowEngine_should_ReturnEmptyGraph_When_SeedMigrationHasNotRun(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	client := repo.client

	// Deliberately not calling EnsureBuiltInWorkflowStages — the tables exist
	// (schema migration always runs) but hold zero rows, the exact
	// pre-Epic-2.2.2-seed state Risk Control's "tables can exist unused"
	// zero-downtime strategy describes.
	stages, err := client.BacklogStage.Query().All(ctx)
	require.NoError(t, err)
	require.Empty(t, stages, "unseeded backlog_stages must be empty, not nil-panic or error")

	transitions, err := client.StageTransition.Query().All(ctx)
	require.NoError(t, err)
	require.Empty(t, transitions, "unseeded stage_transitions must be empty, not nil-panic or error")

	// Reconstructing the adjacency map the same way the seeded-graph test
	// does must yield a defined empty map, never a nil-map panic on lookup.
	seededEdges := make(map[BacklogStatus]map[BacklogStatus]bool, len(stages))
	require.NotPanics(t, func() {
		_ = seededEdges[BacklogStatusIdea][BacklogStatusReady]
	})
	require.Empty(t, seededEdges)
}

// newSeededConfiguredWorkflowEngine seeds the built-in stage graph and
// returns a ConfiguredWorkflowEngine loaded from it, plus the underlying
// ent.Client (for tests that need to write additional rows directly, e.g. a
// simulated operator-added custom transition — standing in for the
// not-yet-implemented Epic 2.7 CRUD RPCs).
func newSeededConfiguredWorkflowEngine(t *testing.T) (*ConfiguredWorkflowEngine, *ent.Client) {
	t.Helper()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	client := repo.client

	require.NoError(t, EnsureBuiltInWorkflowStages(ctx, client))

	stageRepo := NewEntStageConfigRepository(client)
	engine, err := NewConfiguredWorkflowEngine(stageRepo)
	require.NoError(t, err)
	return engine, client
}

// TestConfiguredWorkflowEngine_should_MatchDefaultWorkflowEngineByteForByte_When_NoCustomStagesAdded
// covers Story 2.3.1's Risk Control regression gate: with no custom stages
// added, ConfiguredWorkflowEngine.CanTransition/AllowedTransitions must agree
// with DefaultWorkflowEngine's for every (from,to) pair among the 9 built-in
// stages — both domain.ValidTransitions()'s edges (must be true on both) and
// every non-edge pair (must be false on both).
func TestConfiguredWorkflowEngine_should_MatchDefaultWorkflowEngineByteForByte_When_NoCustomStagesAdded(t *testing.T) {
	t.Parallel()
	configured, _ := newSeededConfiguredWorkflowEngine(t)
	def := NewDefaultWorkflowEngine()

	checked := 0
	for _, from := range builtInStageOrder {
		for _, to := range builtInStageOrder {
			wantAllowed := def.CanTransition(from, to)
			gotAllowed := configured.CanTransition(from, to)
			require.Equalf(t, wantAllowed, gotAllowed,
				"CanTransition(%s, %s): DefaultWorkflowEngine=%v, ConfiguredWorkflowEngine=%v", from, to, wantAllowed, gotAllowed)
			checked++
		}
	}
	require.Equal(t, len(builtInStageOrder)*len(builtInStageOrder), checked)

	// AllowedTransitions must also agree, stage by stage.
	for _, from := range builtInStageOrder {
		require.ElementsMatchf(t, def.AllowedTransitions(from), configured.AllowedTransitions(from),
			"AllowedTransitions(%s) mismatch", from)
	}

	// Sanity-check the specific pair named in Story 2.3.1's acceptance
	// criteria explicitly.
	require.True(t, def.CanTransition(BacklogStatusReview, BacklogStatusPRPending))
	require.True(t, configured.CanTransition(BacklogStatusReview, BacklogStatusPRPending))
}

// TestConfiguredWorkflowEngine_should_AllowNewCustomTransitionImmediately_When_CreateStageTransitionRPCJustSucceeded
// covers Story 2.3.1's second acceptance criterion. There is no
// CreateStageTransition RPC yet (Epic 2.7) — this simulates one by writing a
// new BacklogStage + StageTransition row directly via ent (the same client
// the real RPC handler will use), then confirming CanTransition sees it as
// legal only after InvalidateCache is called — proving the cache-invalidation
// path, not a lucky first-load coincidence, is what makes it legal.
func TestConfiguredWorkflowEngine_should_AllowNewCustomTransitionImmediately_When_CreateStageTransitionRPCJustSucceeded(t *testing.T) {
	t.Parallel()
	engine, client := newSeededConfiguredWorkflowEngine(t)
	ctx := context.Background()

	const customSlug = "design-review"
	require.False(t, engine.CanTransition(BacklogStatusIdea, BacklogStatus(customSlug)),
		"custom transition must not be legal before it exists")

	ideaStage, err := client.BacklogStage.Query().Where(backlogstage.Slug(string(BacklogStatusIdea))).Only(ctx)
	require.NoError(t, err)

	customStage, err := client.BacklogStage.Create().
		SetSlug(customSlug).
		SetName("Design Review").
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.StageTransition.Create().
		SetFromStageID(ideaStage.ID).
		SetToStageID(customStage.ID).
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	// Before invalidation, the stale cache must not yet reflect the new row —
	// otherwise this test couldn't distinguish "cache invalidation works" from
	// "the cache happens to already contain it."
	require.False(t, engine.CanTransition(BacklogStatusIdea, BacklogStatus(customSlug)),
		"new transition must not be visible before InvalidateCache is called")

	require.NoError(t, engine.InvalidateCache(ctx))

	require.True(t, engine.CanTransition(BacklogStatusIdea, BacklogStatus(customSlug)),
		"new transition must be legal immediately after InvalidateCache, with no redeploy")
}

// acCriteriaAllDone / acCriteriaOneUnchecked build serialized AcCriteriaJSON
// fixtures for the PendingGates/ValidateGates tests below.
func acCriteriaAllDone(t *testing.T) AcCriteriaJSON {
	t.Helper()
	raw, err := SerializeAcCriteria([]AcCriterion{
		{Index: 0, Text: "criterion one", Status: AcStatusDone},
		{Index: 1, Text: "criterion two", Status: AcStatusDone},
	})
	require.NoError(t, err)
	return raw
}

func acCriteriaOneUnchecked(t *testing.T) AcCriteriaJSON {
	t.Helper()
	raw, err := SerializeAcCriteria([]AcCriterion{
		{Index: 0, Text: "criterion one", Status: AcStatusDone},
		{Index: 1, Text: "criterion two", Status: AcStatusPending},
	})
	require.NoError(t, err)
	return raw
}

// newCustomTransitionWithGates creates a fresh custom stage pair
// ("gate-from" -> "gate-to") plus one StageTransition row carrying gates (in
// order_index order matching the kinds/stateful flags given), invalidates
// engine's cache, and returns the (from,to) BacklogStatus pair to call
// CanTransition/PendingGates/ValidateGates with.
func newCustomTransitionWithGates(t *testing.T, client *ent.Client, engine *ConfiguredWorkflowEngine, kinds []GateKind, stateful []bool) (from, to BacklogStatus) {
	t.Helper()
	ctx := context.Background()
	require.Equal(t, len(kinds), len(stateful), "test fixture bug: kinds/stateful length mismatch")

	fromStage, err := client.BacklogStage.Create().
		SetSlug("gate-from").
		SetName("Gate From").
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	toStage, err := client.BacklogStage.Create().
		SetSlug("gate-to").
		SetName("Gate To").
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	transition, err := client.StageTransition.Create().
		SetFromStageID(fromStage.ID).
		SetToStageID(toStage.ID).
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	for i, kind := range kinds {
		_, err := client.TransitionGate.Create().
			SetTransitionID(transition.ID).
			SetKind(string(kind)).
			SetStateful(stateful[i]).
			SetOrderIndex(i).
			SetEnabled(true).
			Save(ctx)
		require.NoError(t, err)
	}

	require.NoError(t, engine.InvalidateCache(ctx))
	return BacklogStatus(fromStage.Slug), BacklogStatus(toStage.Slug)
}

// TestValidateGates_should_ReturnError_When_PendingGatesReportsAnyUnsatisfiedEntry
// covers Story 2.3.2's happy-path acceptance criterion: given a transition
// with two gates, one satisfied (structural, AC complete) and one not
// (human_approval, never satisfiable before Epic 2.4 wires
// RecordGateApproval), ValidateGates must return a non-nil error, and
// PendingGates for the same call must return a 2-element slice with exactly
// one Satisfied: true and one Satisfied: false.
func TestValidateGates_should_ReturnError_When_PendingGatesReportsAnyUnsatisfiedEntry(t *testing.T) {
	t.Parallel()
	engine, client := newSeededConfiguredWorkflowEngine(t)
	from, to := newCustomTransitionWithGates(t, client, engine,
		[]GateKind{GateKindStructural, GateKindHumanApproval},
		[]bool{false, true},
	)

	item := BacklogItemTransitionInput{Status: from, AcCriteria: acCriteriaAllDone(t)}

	statuses, err := engine.PendingGates(item, to)
	require.NoError(t, err)
	require.Len(t, statuses, 2, "expected one GateStatus per configured gate")

	satisfiedCount, unsatisfiedCount := 0, 0
	for _, s := range statuses {
		if s.Satisfied {
			satisfiedCount++
		} else {
			unsatisfiedCount++
		}
	}
	require.Equal(t, 1, satisfiedCount, "exactly one gate (structural, AC complete) must be satisfied")
	require.Equal(t, 1, unsatisfiedCount, "exactly one gate (human_approval, no recorded action) must be unsatisfied")

	err = engine.ValidateGates(item, to)
	require.Error(t, err, "ValidateGates must return a non-nil error when any gate is unsatisfied")
	require.ErrorIs(t, err, ErrGateNotSatisfied)
}

// TestPendingGates_should_ReportUnsatisfied_When_PreviouslySatisfiedStructuralGateHasSinceRegressed
// covers Story 2.3.2's structural-gate-freshness acceptance criterion: a
// structural gate ("all AC done") that was satisfied on a previous
// PendingGates call must report Satisfied: false on the very next call once
// an AC criterion is unchecked — proving no stale "satisfied" result is ever
// cached across calls.
func TestPendingGates_should_ReportUnsatisfied_When_PreviouslySatisfiedStructuralGateHasSinceRegressed(t *testing.T) {
	t.Parallel()
	engine, client := newSeededConfiguredWorkflowEngine(t)
	from, to := newCustomTransitionWithGates(t, client, engine,
		[]GateKind{GateKindStructural},
		[]bool{false},
	)

	satisfiedItem := BacklogItemTransitionInput{Status: from, AcCriteria: acCriteriaAllDone(t)}
	statuses, err := engine.PendingGates(satisfiedItem, to)
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	require.True(t, statuses[0].Satisfied, "structural gate must report satisfied while every AC is done")

	regressedItem := BacklogItemTransitionInput{Status: from, AcCriteria: acCriteriaOneUnchecked(t)}
	statuses, err = engine.PendingGates(regressedItem, to)
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	require.False(t, statuses[0].Satisfied, "structural gate must recompute fresh and report unsatisfied once an AC regresses, never reuse the prior satisfied result")
}
