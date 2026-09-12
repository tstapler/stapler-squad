package session

// configured_workflow_engine_test.go — shared with Epic 2.3, which will add
// the actual ConfiguredWorkflowEngine-based tests here once that type exists.
// This Epic (2.2) only persists stage/transition rows, so the test below
// asserts directly against the seeded ent rows rather than against a
// ConfiguredWorkflowEngine.

import (
	"bytes"
	"context"
	stdlog "log"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tslog "github.com/tstapler/stapler-squad/log"
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

	//nolint:entfullscan test assertion over a test-scoped in-memory DB; verifies exactly the built-in stage count exists.
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

	//nolint:entfullscan test assertion over a test-scoped in-memory DB; must see every seeded edge to prove the seeded graph exactly matches domain.ValidTransitions().
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
	//nolint:entfullscan test assertion over a test-scoped in-memory DB; verifies the unseeded table is exactly empty.
	stages, err := client.BacklogStage.Query().All(ctx)
	require.NoError(t, err)
	require.Empty(t, stages, "unseeded backlog_stages must be empty, not nil-panic or error")

	//nolint:entfullscan test assertion over a test-scoped in-memory DB; verifies the unseeded table is exactly empty.
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
	gateSatisfactionRepo := NewEntGateSatisfactionRepository(client)
	engine, err := NewConfiguredWorkflowEngine(stageRepo, gateSatisfactionRepo, nil)
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

// TestAllowedTransitions_should_ReturnSnapshottedTransitionsWithWarnLog_When_ItemsCurrentStageWasSinceDeleted
// covers Epic 2.5, Story 2.5.2's acceptance criterion: once a custom stage an
// item is currently sitting on is deleted (cascading away its
// StageTransition rows too — session/ent/schema/backlog_stage.go's
// OnDelete(Cascade) edges), AllowedTransitions/CanTransition must fall back
// to the item's own captured StageConfigSnapshot rather than reporting an
// empty slice/false, and must log a Warn noting the live config is stale.
// Not run with t.Parallel(): SetWarningLogForTest swaps a shared
// package-level logger (see liveness_cache_test.go's identical convention).
func TestAllowedTransitions_should_ReturnSnapshottedTransitionsWithWarnLog_When_ItemsCurrentStageWasSinceDeleted(t *testing.T) {
	engine, client := newSeededConfiguredWorkflowEngine(t)
	ctx := context.Background()

	customStage, err := client.BacklogStage.Create().
		SetSlug("design-review").
		SetName("Design Review").
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	readyStage, err := client.BacklogStage.Query().Where(backlogstage.Slug(string(BacklogStatusReady))).Only(ctx)
	require.NoError(t, err)

	_, err = client.StageTransition.Create().
		SetFromStageID(customStage.ID).
		SetToStageID(readyStage.ID).
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)
	require.NoError(t, engine.InvalidateCache(ctx))

	const customSlug = BacklogStatus("design-review")
	require.True(t, engine.CanTransition(customSlug, BacklogStatusReady),
		"sanity: transition must be legal while the stage is still live")
	require.ElementsMatch(t, []BacklogStatus{BacklogStatusReady}, engine.AllowedTransitions(customSlug))

	// Delete the stage while an item is still sitting on it — cascades away
	// its outgoing StageTransition row too.
	require.NoError(t, client.BacklogStage.DeleteOneID(customStage.ID).Exec(ctx))
	require.NoError(t, engine.InvalidateCache(ctx))

	// No fallback supplied: a defined empty/false answer, never a panic.
	require.Empty(t, engine.AllowedTransitions(customSlug))
	require.False(t, engine.CanTransition(customSlug, BacklogStatusReady))

	var buf bytes.Buffer
	orig := tslog.SetWarningLogForTest(stdlog.New(&buf, "WARNING: ", 0))
	t.Cleanup(func() { tslog.SetWarningLogForTest(orig) })

	// With the item's own captured StageConfigSnapshot: the transitions legal
	// at the moment it entered the now-deleted stage.
	fallback := &StageConfigSnapshot{
		StageName:          "Design Review",
		AllowedTransitions: []BacklogStatus{BacklogStatusReady},
	}
	assert.Equal(t, []BacklogStatus{BacklogStatusReady}, engine.AllowedTransitions(customSlug, fallback))
	assert.True(t, engine.CanTransition(customSlug, BacklogStatusReady, fallback))
	assert.False(t, engine.CanTransition(customSlug, BacklogStatusDone, fallback),
		"fallback must only legalize transitions actually present in the snapshot")
	assert.Contains(t, buf.String(), "design-review", "expected a Warn log naming the stale stage")
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
		c := client.TransitionGate.Create().
			SetTransitionID(transition.ID).
			SetKind(string(kind)).
			SetStateful(stateful[i]).
			SetOrderIndex(i).
			SetEnabled(true)
		// A GateKindStructural fixture needs a configured check_id (Story
		// 2.4.2's config-driven dispatch, session/gate_structural.go) or
		// evaluateStructuralGate fails closed with "unrecognized structural
		// check id" — every existing caller of this helper wants the
		// pre-Epic-2.4.2 "all AC done" semantics, so default to ac_complete.
		// A GateKindCustom fixture likewise needs a registered skill (ADR-006
		// Part B's config-error check, session/configured_workflow_engine.go)
		// or evaluateGate now reports a config error before ever reaching
		// evaluateRecordedGate — every existing caller of this helper wants
		// the pre-ADR-006 "consult GateSatisfactionRepository" semantics, so
		// default to the one pre-registered skill.
		switch kind {
		case GateKindStructural:
			c = c.SetConfig(map[string]interface{}{"check_id": StructuralCheckACComplete})
		case GateKindCustom:
			c = c.SetConfig(map[string]interface{}{"skill": "review-feasibility"})
		}
		_, err := c.Save(ctx)
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

// deletedEdgeFixture creates two custom stages with one enabled StageTransition
// between them, invalidates the cache so it's live, then deletes the from-stage
// (cascading away the StageTransition row too) and invalidates the cache again
// — leaving engine's live cache with no edge for (from,to) at all, the
// cache-miss precondition ADR-004's fail-closed PendingGates fix targets.
// Returns the (from,to) BacklogStatus pair.
func deletedEdgeFixture(t *testing.T, client *ent.Client, engine *ConfiguredWorkflowEngine) (from, to BacklogStatus) {
	t.Helper()
	ctx := context.Background()

	fromStage, err := client.BacklogStage.Create().SetSlug("gone-from").SetName("Gone From").SetEnabled(true).Save(ctx)
	require.NoError(t, err)
	toStage, err := client.BacklogStage.Create().SetSlug("gone-to").SetName("Gone To").SetEnabled(true).Save(ctx)
	require.NoError(t, err)
	_, err = client.StageTransition.Create().SetFromStageID(fromStage.ID).SetToStageID(toStage.ID).SetEnabled(true).Save(ctx)
	require.NoError(t, err)
	require.NoError(t, engine.InvalidateCache(ctx))

	require.NoError(t, client.BacklogStage.DeleteOneID(fromStage.ID).Exec(ctx))
	require.NoError(t, engine.InvalidateCache(ctx))

	return BacklogStatus(fromStage.Slug), BacklogStatus(toStage.Slug)
}

// TestPendingGates_should_ReturnSyntheticBlockingGate_When_CacheMissButFallbackConfirmsEdgeOnceExisted
// covers ADR-004's Decision 4, the adversarial review's fail-closed-for-gates
// fix: when the live cache has no edge for (item.Status, to) at all — the
// from-stage was deleted — but a supplied fallback's AllowedTransitions
// confirms to was legal when the item entered its current stage, PendingGates
// must return the single synthetic blocking "stage-config-unresolvable" gate,
// never a silent nil, nil that would let ValidateGates report zero pending
// gates for a deleted-stage item.
func TestPendingGates_should_ReturnSyntheticBlockingGate_When_CacheMissButFallbackConfirmsEdgeOnceExisted(t *testing.T) {
	t.Parallel()
	engine, client := newSeededConfiguredWorkflowEngine(t)
	from, to := deletedEdgeFixture(t, client, engine)

	fallback := &StageConfigSnapshot{StageName: "Gone From", AllowedTransitions: []BacklogStatus{to}}
	statuses, err := engine.PendingGates(BacklogItemTransitionInput{Status: from}, to, fallback)
	require.NoError(t, err)
	require.Len(t, statuses, 1, "expected exactly one synthetic blocking gate, not an empty slice")

	got := statuses[0]
	assert.Equal(t, stageConfigUnresolvableGateID, got.GateID)
	assert.Equal(t, GateKindStructural, got.Kind)
	assert.False(t, got.Satisfied, "the synthetic gate must always report unsatisfied — a deleted stage's config can never be automatically evaluated")
	assert.Contains(t, got.Description, "no longer available")
	assert.Contains(t, got.ActionHint, "Manual Override")

	// Without a fallback at all, PendingGates keeps its pre-ADR-004 nil, nil —
	// the synthetic gate only fires when a fallback is present and confirms
	// the edge.
	statuses, err = engine.PendingGates(BacklogItemTransitionInput{Status: from}, to)
	require.NoError(t, err)
	assert.Empty(t, statuses, "no fallback supplied: PendingGates must still degrade to nil/empty, unchanged")
}

// TestPendingGates_should_ReturnNilNil_When_CacheMissAndFallbackDoesNotContainDestination
// covers the negative case ADR-004 explicitly preserves: even with a
// fallback present, if it never listed to as a legal destination (this edge
// never legally existed), PendingGates keeps today's nil, nil — CanTransition
// already governs legality separately.
func TestPendingGates_should_ReturnNilNil_When_CacheMissAndFallbackDoesNotContainDestination(t *testing.T) {
	t.Parallel()
	engine, client := newSeededConfiguredWorkflowEngine(t)
	from, to := deletedEdgeFixture(t, client, engine)
	require.NotEqual(t, BacklogStatusDone, to, "test fixture bug: deletedEdgeFixture's to-stage must not collide with the destination probed below")

	fallback := &StageConfigSnapshot{StageName: "Gone From", AllowedTransitions: []BacklogStatus{BacklogStatusReady}}
	statuses, err := engine.PendingGates(BacklogItemTransitionInput{Status: from}, BacklogStatusDone, fallback)
	require.NoError(t, err)
	assert.Empty(t, statuses, "a destination never present in the snapshot must not trigger the synthetic blocking gate")
}

// newDeletedStageItemFixture creates a real item (via the EntRepository write
// path) transitioned into a custom stage with one outgoing transition,
// captures its ADR-004 snapshot fields at write time, then deletes that stage
// (cascading away its StageTransition row) and invalidates engine's cache —
// the end-to-end precondition
// TestConfiguredWorkflowEngine_should_HonorCapturedSnapshot_When_ItemsStageIsDeletedAfterTransition
// asserts against. Returns the engine, the item's (from,to) BacklogStatus
// pair, and the item reloaded post-deletion (for BuildStageConfigSnapshotFallback).
func newDeletedStageItemFixture(t *testing.T) (engine *ConfiguredWorkflowEngine, from, to BacklogStatus, reloaded *BacklogItemData) {
	t.Helper()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	client := repo.client

	fromStage, err := client.BacklogStage.Create().SetSlug("e2e-from").SetName("E2E From").SetEnabled(true).Save(ctx)
	require.NoError(t, err)
	toStage, err := client.BacklogStage.Create().SetSlug("e2e-to").SetName("E2E To").SetEnabled(true).Save(ctx)
	require.NoError(t, err)
	_, err = client.StageTransition.Create().SetFromStageID(fromStage.ID).SetToStageID(toStage.ID).SetEnabled(true).Save(ctx)
	require.NoError(t, err)

	stageRepo := NewEntStageConfigRepository(client)
	gateSatisfactionRepo := NewEntGateSatisfactionRepository(client)
	engine, err = NewConfiguredWorkflowEngine(stageRepo, gateSatisfactionRepo, nil)
	require.NoError(t, err)

	item, err := repo.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "e2e item entering a stage that will be deleted",
		Status: string(BacklogStatusIdea),
	})
	require.NoError(t, err)

	_, err = repo.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatus(fromStage.Slug), nil, TriggeredByUser)
	require.NoError(t, err)

	require.NoError(t, client.BacklogStage.DeleteOneID(fromStage.ID).Exec(ctx))
	require.NoError(t, engine.InvalidateCache(ctx))

	reloaded, err = repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	return engine, BacklogStatus(fromStage.Slug), BacklogStatus(toStage.Slug), reloaded
}

// TestConfiguredWorkflowEngine_should_HonorCapturedSnapshot_When_ItemsStageIsDeletedAfterTransition
// is ADR-004's end-to-end acceptance test: given a real item transitioned
// into a custom stage that is later deleted, reconstruct the fallback via
// BuildStageConfigSnapshotFallback from the reloaded item, and confirm
// AllowedTransitions/CanTransition/PendingGates all behave per the frozen
// snapshot when called with that fallback — instead of the fail-open/empty
// answers they give when called without one.
func TestConfiguredWorkflowEngine_should_HonorCapturedSnapshot_When_ItemsStageIsDeletedAfterTransition(t *testing.T) {
	t.Parallel()
	engine, from, to, reloaded := newDeletedStageItemFixture(t)

	fallback := BuildStageConfigSnapshotFallback(reloaded)
	require.NotNil(t, fallback, "expected a reconstructed snapshot from the item's own status-event history")
	require.Equal(t, "E2E From", fallback.StageName)
	require.Equal(t, []BacklogStatus{to}, fallback.AllowedTransitions)

	// Without the fallback: the deleted stage reports empty/false/nil, the
	// fail-open behavior ADR-004's fallback plumbing exists to avoid.
	assert.False(t, engine.CanTransition(from, to), "sanity: without a fallback, the deleted stage reports false")
	assert.Empty(t, engine.AllowedTransitions(from), "sanity: without a fallback, the deleted stage reports empty")
	gatesNoFallback, err := engine.PendingGates(BacklogItemTransitionInput{Status: from}, to)
	require.NoError(t, err)
	assert.Empty(t, gatesNoFallback, "sanity: without a fallback, PendingGates reports zero pending gates")

	// With the reconstructed fallback: all three answer per the frozen snapshot.
	assert.True(t, engine.CanTransition(from, to, fallback), "CanTransition must honor the captured snapshot")
	assert.Equal(t, []BacklogStatus{to}, engine.AllowedTransitions(from, fallback), "AllowedTransitions must honor the captured snapshot")

	gates, err := engine.PendingGates(BacklogItemTransitionInput{Status: from}, to, fallback)
	require.NoError(t, err)
	require.Len(t, gates, 1, "PendingGates must report the synthetic blocking gate, not silently pass the item through")
	assert.Equal(t, stageConfigUnresolvableGateID, gates[0].GateID)
	assert.False(t, gates[0].Satisfied)
}

// --- Epic 2.4 follow-up: resolver + evaluateGate wiring for
// automated_review/custom gates (the gap this task closes) ---

// TestResolveAutomatedReviewGateContext_should_ReturnGateIDAndConfig_When_GateConfigured
// covers Story 2.4.3's follow-up resolver: a GateKindAutomatedReview gate
// configured on a custom (from,to) edge must resolve to its own GateID and
// parsed AutomatedReviewConfig (RequiresDiff/PipelineMode), not the built-in
// literal's defaults.
func TestResolveAutomatedReviewGateContext_should_ReturnGateIDAndConfig_When_GateConfigured(t *testing.T) {
	t.Parallel()
	engine, client := newSeededConfiguredWorkflowEngine(t)
	ctx := context.Background()

	fromStage, err := client.BacklogStage.Create().SetSlug("ar-from").SetName("From").SetEnabled(true).Save(ctx)
	require.NoError(t, err)
	toStage, err := client.BacklogStage.Create().SetSlug("ar-to").SetName("To").SetEnabled(true).Save(ctx)
	require.NoError(t, err)
	transition, err := client.StageTransition.Create().SetFromStageID(fromStage.ID).SetToStageID(toStage.ID).SetEnabled(true).Save(ctx)
	require.NoError(t, err)
	gate, err := client.TransitionGate.Create().
		SetTransitionID(transition.ID).
		SetKind(string(GateKindAutomatedReview)).
		SetStateful(true).
		SetEnabled(true).
		SetConfig(map[string]interface{}{"requires_diff": false, "pipeline_mode": "sdd"}).
		Save(ctx)
	require.NoError(t, err)
	require.NoError(t, engine.InvalidateCache(ctx))

	gateID, cfg, ok := engine.ResolveAutomatedReviewGateContext(BacklogStatus(fromStage.Slug), BacklogStatus(toStage.Slug))
	require.True(t, ok)
	assert.Equal(t, gate.ID.String(), gateID)
	assert.False(t, cfg.RequiresDiff)
	assert.Equal(t, "sdd", cfg.PipelineMode)
}

// TestResolveAutomatedReviewGateContext_should_ReturnNotOK_When_EdgeHasNoSuchGate
// covers the negative case: an edge with no configured automated_review gate
// (including one that doesn't exist in the graph at all) resolves ok=false.
func TestResolveAutomatedReviewGateContext_should_ReturnNotOK_When_EdgeHasNoSuchGate(t *testing.T) {
	t.Parallel()
	engine, client := newSeededConfiguredWorkflowEngine(t)
	from, to := newCustomTransitionWithGates(t, client, engine, []GateKind{GateKindStructural}, []bool{false})

	_, _, ok := engine.ResolveAutomatedReviewGateContext(from, to)
	assert.False(t, ok, "an edge with only a structural gate must not resolve an automated-review gate context")

	_, _, ok = engine.ResolveAutomatedReviewGateContext(BacklogStatus("no-such-from"), BacklogStatus("no-such-to"))
	assert.False(t, ok, "a nonexistent edge must not resolve")
}

// TestResolveCustomCheckGateContext_should_ReturnGateIDAndConfig_When_GateConfigured
// covers Story 2.4.4's follow-up resolver — the sibling of
// ResolveAutomatedReviewGateContext for GateKindCustom.
func TestResolveCustomCheckGateContext_should_ReturnGateIDAndConfig_When_GateConfigured(t *testing.T) {
	t.Parallel()
	engine, client := newSeededConfiguredWorkflowEngine(t)
	ctx := context.Background()

	fromStage, err := client.BacklogStage.Create().SetSlug("cc-from").SetName("From").SetEnabled(true).Save(ctx)
	require.NoError(t, err)
	toStage, err := client.BacklogStage.Create().SetSlug("cc-to").SetName("To").SetEnabled(true).Save(ctx)
	require.NoError(t, err)
	transition, err := client.StageTransition.Create().SetFromStageID(fromStage.ID).SetToStageID(toStage.ID).SetEnabled(true).Save(ctx)
	require.NoError(t, err)
	gate, err := client.TransitionGate.Create().
		SetTransitionID(transition.ID).
		SetKind(string(GateKindCustom)).
		SetStateful(true).
		SetEnabled(true).
		SetConfig(map[string]interface{}{"skill": "review-feasibility"}).
		Save(ctx)
	require.NoError(t, err)
	require.NoError(t, engine.InvalidateCache(ctx))

	gateID, cfg, ok := engine.ResolveCustomCheckGateContext(BacklogStatus(fromStage.Slug), BacklogStatus(toStage.Slug))
	require.True(t, ok)
	assert.Equal(t, gate.ID, gateID)
	assert.Equal(t, "review-feasibility", cfg.SkillID)
}

// TestEvaluateGate_AutomatedReviewAndCustom_should_ConsultGateSatisfactionRepository
// covers this Epic's follow-up to evaluateGate's automated_review/custom
// branches (previously an unconditional Satisfied:false placeholder — see the
// pre-follow-up doc comment this change replaced): no record yet reports the
// placeholder, a satisfied record reports Satisfied:true, and an unsatisfied
// record reports Satisfied:false with its recorded description — for both
// stateful gate kinds this task wires up.
func TestEvaluateGate_AutomatedReviewAndCustom_should_ConsultGateSatisfactionRepository(t *testing.T) {
	t.Parallel()

	for _, kind := range []GateKind{GateKindAutomatedReview, GateKindCustom} {
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()
			engine, client := newSeededConfiguredWorkflowEngine(t)
			ctx := context.Background()
			from, to := newCustomTransitionWithGates(t, client, engine, []GateKind{kind}, []bool{true})

			item, err := NewTestEntRepository(t).client.BacklogItem.Create().
				SetTitle("evaluateGate test item").
				SetStatus(string(from)).
				SetPriority(1).
				Save(ctx)
			require.NoError(t, err)

			edge, ok := engine.cache.Get(from, to)
			require.True(t, ok)
			require.Len(t, edge.Gates, 1)
			gateID := edge.Gates[0].ID

			// No record yet: falls back to the "not yet actionable" placeholder.
			statuses, err := engine.PendingGates(BacklogItemTransitionInput{ItemID: item.ID.String(), Status: from}, to)
			require.NoError(t, err)
			require.Len(t, statuses, 1)
			assert.False(t, statuses[0].Satisfied)
			assert.Contains(t, statuses[0].Description, "nothing has run yet")

			// Satisfied record: reports Satisfied:true with its recorded detail.
			gateRepo := NewEntGateSatisfactionRepository(client)
			now := time.Now()
			_, err = gateRepo.Create(ctx, GateSatisfactionCreateInput{
				ItemID:        item.ID,
				GateID:        gateID,
				Satisfied:     true,
				SatisfiedAt:   &now,
				OutcomeDetail: map[string]interface{}{"detail": "looks good"},
			})
			require.NoError(t, err)

			statuses, err = engine.PendingGates(BacklogItemTransitionInput{ItemID: item.ID.String(), Status: from}, to)
			require.NoError(t, err)
			require.Len(t, statuses, 1)
			assert.True(t, statuses[0].Satisfied)
			assert.Equal(t, "looks good", statuses[0].Description)

			// Flip to an unsatisfied outcome: reports Satisfied:false with the new detail.
			satisfiedFalse := false
			_, err = gateRepo.Update(ctx, item.ID, gateID, GateSatisfactionUpdateInput{
				Satisfied:     &satisfiedFalse,
				OutcomeDetail: map[string]interface{}{"detail": "needs rework"},
			})
			require.NoError(t, err)

			statuses, err = engine.PendingGates(BacklogItemTransitionInput{ItemID: item.ID.String(), Status: from}, to)
			require.NoError(t, err)
			require.Len(t, statuses, 1)
			assert.False(t, statuses[0].Satisfied)
			assert.Equal(t, "needs rework", statuses[0].Description)
		})
	}
}

// --- ADR-006 Part B: config-error detection ahead of the satisfaction lookup ---

// failOnCallGateSatisfactionRepo is a GateSatisfactionRepository test double
// that fails the test immediately if any method is called — used to prove
// evaluateGate's config-error short-circuit never reaches the satisfaction
// lookup at all for a gate whose config doesn't resolve.
type failOnCallGateSatisfactionRepo struct {
	t *testing.T
}

func (f *failOnCallGateSatisfactionRepo) Create(context.Context, GateSatisfactionCreateInput) (*GateSatisfactionData, error) {
	f.t.Fatal("GateSatisfactionRepository.Create must not be called when a config error short-circuits evaluation")
	return nil, nil //nolint:nilnil // unreachable after t.Fatal (which calls runtime.Goexit); only here to satisfy the compiler's return requirement
}

func (f *failOnCallGateSatisfactionRepo) GetByItemAndGate(context.Context, uuid.UUID, uuid.UUID) (*GateSatisfactionData, error) {
	f.t.Fatal("GateSatisfactionRepository.GetByItemAndGate must not be called when a config error short-circuits evaluation")
	return nil, nil //nolint:nilnil // unreachable after t.Fatal (which calls runtime.Goexit); only here to satisfy the compiler's return requirement
}

func (f *failOnCallGateSatisfactionRepo) Update(context.Context, uuid.UUID, uuid.UUID, GateSatisfactionUpdateInput) (*GateSatisfactionData, error) {
	f.t.Fatal("GateSatisfactionRepository.Update must not be called when a config error short-circuits evaluation")
	return nil, nil //nolint:nilnil // unreachable after t.Fatal (which calls runtime.Goexit); only here to satisfy the compiler's return requirement
}

func (f *failOnCallGateSatisfactionRepo) ListUnsatisfied(context.Context) ([]*GateSatisfactionData, error) {
	f.t.Fatal("GateSatisfactionRepository.ListUnsatisfied must not be called when a config error short-circuits evaluation")
	return nil, nil
}

var _ GateSatisfactionRepository = (*failOnCallGateSatisfactionRepo)(nil)

// newGateWithConfig creates a fresh custom stage pair plus one enabled gate of
// kind on the transition between them, carrying config, invalidates engine's
// cache, and returns the (from,to) pair plus the gate's ID. Sibling to
// newCustomTransitionWithGates above, for tests that need an explicit,
// possibly-invalid Config map (e.g. a skill/pipeline_mode that no longer
// resolves) rather than that helper's structural-check-only default.
func newGateWithConfig(t *testing.T, client *ent.Client, engine *ConfiguredWorkflowEngine, kind GateKind, config map[string]interface{}) (from, to BacklogStatus, gateID uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	fromStage, err := client.BacklogStage.Create().SetSlug("cfgerr-from-" + uuid.NewString()).SetName("From").SetEnabled(true).Save(ctx)
	require.NoError(t, err)
	toStage, err := client.BacklogStage.Create().SetSlug("cfgerr-to-" + uuid.NewString()).SetName("To").SetEnabled(true).Save(ctx)
	require.NoError(t, err)
	transition, err := client.StageTransition.Create().SetFromStageID(fromStage.ID).SetToStageID(toStage.ID).SetEnabled(true).Save(ctx)
	require.NoError(t, err)
	gate, err := client.TransitionGate.Create().
		SetTransitionID(transition.ID).
		SetKind(string(kind)).
		SetStateful(true).
		SetEnabled(true).
		SetConfig(config).
		Save(ctx)
	require.NoError(t, err)

	require.NoError(t, engine.InvalidateCache(ctx))
	return BacklogStatus(fromStage.Slug), BacklogStatus(toStage.Slug), gate.ID
}

// TestEvaluateGate_should_ReportConfigErrorAndSkipSatisfactionLookup_When_CustomGateSkillNoLongerRegistered
// covers ADR-006 Part B's custom-gate branch: a GateKindCustom gate whose
// configured skill is no longer in registeredCustomCheckSkills (it was valid
// at save time, since ParseGateConfig enforces the allowlist then, but has
// since been deregistered) must report ConfigError/Satisfied:false and must
// never reach GateSatisfactionRepository at all — proven here via a repo
// double that fails the test if any method is called.
func TestEvaluateGate_should_ReportConfigErrorAndSkipSatisfactionLookup_When_CustomGateSkillNoLongerRegistered(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	client := repo.client
	stageRepo := NewEntStageConfigRepository(client)
	engine, err := NewConfiguredWorkflowEngine(stageRepo, &failOnCallGateSatisfactionRepo{t: t}, nil)
	require.NoError(t, err)

	from, to, _ := newGateWithConfig(t, client, engine, GateKindCustom, map[string]interface{}{"skill": "no-longer-registered-skill"})

	statuses, err := engine.PendingGates(BacklogItemTransitionInput{ItemID: uuid.New().String(), Status: from}, to)
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	assert.False(t, statuses[0].Satisfied)
	assert.NotEmpty(t, statuses[0].ConfigError, "expected a config error for a deregistered custom-check skill")
	assert.Contains(t, statuses[0].ConfigError, "no-longer-registered-skill")
	assert.Equal(t, statuses[0].ConfigError, statuses[0].Description, "ConfigError and Description must carry the same reason")
}

// TestEvaluateGate_should_ReportConfigErrorAndSkipSatisfactionLookup_When_AutomatedReviewPipelineModeUnresolvable
// covers ADR-006 Part B's automated_review branch: a GateKindAutomatedReview
// gate whose configured pipeline_mode no longer resolves via
// PipelineModeRepository.GetBySlug must report ConfigError/Satisfied:false
// and must never reach GateSatisfactionRepository — same short-circuit proof
// as the custom-gate case above.
func TestEvaluateGate_should_ReportConfigErrorAndSkipSatisfactionLookup_When_AutomatedReviewPipelineModeUnresolvable(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	client := repo.client
	stageRepo := NewEntStageConfigRepository(client)
	pipelineModeRepo := NewEntPipelineModeRepository(client)
	engine, err := NewConfiguredWorkflowEngine(stageRepo, &failOnCallGateSatisfactionRepo{t: t}, pipelineModeRepo)
	require.NoError(t, err)

	from, to, _ := newGateWithConfig(t, client, engine, GateKindAutomatedReview, map[string]interface{}{"pipeline_mode": "ghost-mode"})

	statuses, err := engine.PendingGates(BacklogItemTransitionInput{ItemID: uuid.New().String(), Status: from}, to)
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	assert.False(t, statuses[0].Satisfied)
	assert.NotEmpty(t, statuses[0].ConfigError, "expected a config error for an unresolvable pipeline mode")
	assert.Contains(t, statuses[0].ConfigError, "ghost-mode")
	assert.Equal(t, statuses[0].ConfigError, statuses[0].Description)
}

// TestEvaluateGate_should_ReportConfigErrorAndSkipSatisfactionLookup_When_AutomatedReviewPipelineModeRepoUnwired
// covers the nil-pipelineModeRepo branch explicitly: a gate naming a
// pipeline_mode with no pipelineModeRepo wired at all (mirrors
// gateSatisfactionRepo's own nil-guarded-optional-dependency pattern) must
// degrade to the same blocking config error, never a nil-pointer panic.
func TestEvaluateGate_should_ReportConfigErrorAndSkipSatisfactionLookup_When_AutomatedReviewPipelineModeRepoUnwired(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	client := repo.client
	stageRepo := NewEntStageConfigRepository(client)
	engine, err := NewConfiguredWorkflowEngine(stageRepo, &failOnCallGateSatisfactionRepo{t: t}, nil)
	require.NoError(t, err)

	from, to, _ := newGateWithConfig(t, client, engine, GateKindAutomatedReview, map[string]interface{}{"pipeline_mode": "sdd"})

	statuses, err := engine.PendingGates(BacklogItemTransitionInput{ItemID: uuid.New().String(), Status: from}, to)
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	assert.False(t, statuses[0].Satisfied)
	assert.NotEmpty(t, statuses[0].ConfigError, "a nil pipelineModeRepo must degrade to a config error, not a panic")
}

// TestEvaluateGate_should_FallThroughToSatisfactionLookup_When_CustomGateConfigIsValid
// is the regression test for ADR-006 Part B: a custom gate whose skill IS
// registered must behave exactly as before this change — falling through to
// the existing GateSatisfactionRepository lookup — proven here via the same
// spy-repo trick used by the config-error tests above, but this time the spy
// answers GetByItemAndGate instead of failing, confirming the call is
// actually reached (not short-circuited).
func TestEvaluateGate_should_FallThroughToSatisfactionLookup_When_CustomGateConfigIsValid(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	client := repo.client
	stageRepo := NewEntStageConfigRepository(client)
	gateSatisfactionRepo := NewEntGateSatisfactionRepository(client)
	engine, err := NewConfiguredWorkflowEngine(stageRepo, gateSatisfactionRepo, nil)
	require.NoError(t, err)
	ctx := context.Background()

	from, to, gateID := newGateWithConfig(t, client, engine, GateKindCustom, map[string]interface{}{"skill": "review-feasibility"})

	// No record yet: falls through to evaluateRecordedGate's unchanged
	// "nothing has run yet" placeholder — proving evaluateGate did NOT
	// short-circuit into a config error for a still-valid skill.
	statuses, err := engine.PendingGates(BacklogItemTransitionInput{ItemID: uuid.New().String(), Status: from}, to)
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	assert.Empty(t, statuses[0].ConfigError)
	assert.False(t, statuses[0].Satisfied)
	assert.Contains(t, statuses[0].Description, "nothing has run yet")

	// And a recorded satisfaction is still honored exactly as before.
	itemID := uuid.New()
	_, err = gateSatisfactionRepo.Create(ctx, GateSatisfactionCreateInput{ItemID: itemID, GateID: gateID, Satisfied: true})
	require.NoError(t, err)
	statuses, err = engine.PendingGates(BacklogItemTransitionInput{ItemID: itemID.String(), Status: from}, to)
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	assert.Empty(t, statuses[0].ConfigError)
	assert.True(t, statuses[0].Satisfied)
}

// TestEvaluateGate_should_FallThroughToSatisfactionLookup_When_AutomatedReviewPipelineModeIsEmptyOrResolvable
// is the regression test for automated_review: an empty pipeline_mode ("use
// the item's own PipelineMode") and a pipeline_mode that actually resolves via
// PipelineModeRepository must both fall through unchanged, never reporting a
// config error.
func TestEvaluateGate_should_FallThroughToSatisfactionLookup_When_AutomatedReviewPipelineModeIsEmptyOrResolvable(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	client := repo.client
	stageRepo := NewEntStageConfigRepository(client)
	gateSatisfactionRepo := NewEntGateSatisfactionRepository(client)
	pipelineModeRepo := NewEntPipelineModeRepository(client)
	engine, err := NewConfiguredWorkflowEngine(stageRepo, gateSatisfactionRepo, pipelineModeRepo)
	require.NoError(t, err)
	ctx := context.Background()

	t.Run("empty pipeline_mode", func(t *testing.T) {
		from, to, _ := newGateWithConfig(t, client, engine, GateKindAutomatedReview, map[string]interface{}{})
		statuses, err := engine.PendingGates(BacklogItemTransitionInput{ItemID: uuid.New().String(), Status: from}, to)
		require.NoError(t, err)
		require.Len(t, statuses, 1)
		assert.Empty(t, statuses[0].ConfigError)
	})

	t.Run("resolvable pipeline_mode", func(t *testing.T) {
		_, err := pipelineModeRepo.Create(ctx, PipelineModeCreateInput{Slug: "real-mode", Name: "Real Mode", Enabled: true})
		require.NoError(t, err)

		from, to, _ := newGateWithConfig(t, client, engine, GateKindAutomatedReview, map[string]interface{}{"pipeline_mode": "real-mode"})
		statuses, err := engine.PendingGates(BacklogItemTransitionInput{ItemID: uuid.New().String(), Status: from}, to)
		require.NoError(t, err)
		require.Len(t, statuses, 1)
		assert.Empty(t, statuses[0].ConfigError)
	})
}
