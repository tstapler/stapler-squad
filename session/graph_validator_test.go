package session

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGraphValidator_should_AcceptValidGraph_When_EveryStageIsReachableAndEveryNonTerminalHasAnOutgoingEdge
// covers Story 2.6.1's happy path: a linear entry -> middle -> terminal
// graph passes both the dead-end and reachability checks with no warnings
// (no cycle exists).
func TestGraphValidator_should_AcceptValidGraph_When_EveryStageIsReachableAndEveryNonTerminalHasAnOutgoingEdge(t *testing.T) {
	t.Parallel()
	stages := []StageDefinition{
		{Slug: "idea", IsEntry: true},
		{Slug: "in_progress"},
		{Slug: "done", IsTerminal: true},
	}
	transitions := []TransitionDefinition{
		{FromSlug: "idea", ToSlug: "in_progress", Enabled: true},
		{FromSlug: "in_progress", ToSlug: "done", Enabled: true},
	}

	warnings, err := ValidateGraph(stages, transitions)

	require.NoError(t, err)
	require.Empty(t, warnings)
}

// TestGraphValidator_should_RejectDeadEndStage_When_NonTerminalStageHasZeroOutgoingTransitions
// covers Story 2.6.1's AC1: a non-terminal stage with zero outgoing
// transitions is rejected even though it's reachable from an entry stage.
func TestGraphValidator_should_RejectDeadEndStage_When_NonTerminalStageHasZeroOutgoingTransitions(t *testing.T) {
	t.Parallel()
	stages := []StageDefinition{
		{Slug: "idea", IsEntry: true},
		{Slug: "orphan-review"},
	}
	transitions := []TransitionDefinition{
		{FromSlug: "idea", ToSlug: "orphan-review", Enabled: true},
	}

	warnings, err := ValidateGraph(stages, transitions)

	require.Error(t, err)
	require.ErrorIs(t, err, ErrStageDeadEnd)
	require.Contains(t, err.Error(), "orphan-review")
	require.Nil(t, warnings)
}

// TestGraphValidator_should_RejectUnreachableStage_When_SelfLoopMultiCycleAndDisconnectedComponentFixturesAreEachSubmitted
// covers Task 2.6.1d's mandatory adversarial coverage: a self-loop, a
// multi-node cycle, and a disconnected component are each, on their own,
// insufficient to satisfy reachability from an entry stage — the dead-end
// check alone (each has >=1 outgoing edge) would wrongly pass them.
func TestGraphValidator_should_RejectUnreachableStage_When_SelfLoopMultiCycleAndDisconnectedComponentFixturesAreEachSubmitted(t *testing.T) {
	t.Parallel()

	entry := StageDefinition{Slug: "idea", IsEntry: true, IsTerminal: true}

	tests := map[string]struct {
		stages      []StageDefinition
		transitions []TransitionDefinition
		wantStage   string
	}{
		"self-loop": {
			stages: []StageDefinition{entry, {Slug: "trapped"}},
			transitions: []TransitionDefinition{
				{FromSlug: "trapped", ToSlug: "trapped", Enabled: true},
			},
			wantStage: "trapped",
		},
		"multi-node cycle": {
			stages: []StageDefinition{entry, {Slug: "a"}, {Slug: "b"}},
			transitions: []TransitionDefinition{
				{FromSlug: "a", ToSlug: "b", Enabled: true},
				{FromSlug: "b", ToSlug: "a", Enabled: true},
			},
			wantStage: "a",
		},
		"disconnected component": {
			stages: []StageDefinition{entry, {Slug: "island1"}, {Slug: "island2", IsTerminal: true}},
			transitions: []TransitionDefinition{
				{FromSlug: "island1", ToSlug: "island2", Enabled: true},
			},
			wantStage: "island1",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			warnings, err := ValidateGraph(tc.stages, tc.transitions)

			require.Error(t, err)
			require.ErrorIs(t, err, ErrStageUnreachable)
			require.Contains(t, err.Error(), tc.wantStage)
			require.Nil(t, warnings)
		})
	}
}

// TestGraphValidator_should_ReturnNonBlockingWarning_When_ThreeStageCycleHasNoGateOnAnyEdge
// covers Story 2.6.2's AC: a gate-free cycle produces a warning, not a hard
// rejection — the save still succeeds.
func TestGraphValidator_should_ReturnNonBlockingWarning_When_ThreeStageCycleHasNoGateOnAnyEdge(t *testing.T) {
	t.Parallel()
	stages := []StageDefinition{
		{Slug: "a", IsEntry: true},
		{Slug: "b"},
		{Slug: "c"},
	}
	transitions := []TransitionDefinition{
		{FromSlug: "a", ToSlug: "b", Enabled: true},
		{FromSlug: "b", ToSlug: "c", Enabled: true},
		{FromSlug: "c", ToSlug: "a", Enabled: true},
	}

	warnings, err := ValidateGraph(stages, transitions)

	require.NoError(t, err)
	require.Len(t, warnings, 1)
	require.Contains(t, warnings[0], "cycle")
}

// TestGraphValidator_should_ReturnNoWarning_When_CycleHasAtLeastOneGatedEdge
// covers Story 2.6.2's negative path: a cycle with at least one gated edge
// is a legitimate loop (e.g. review -> in_progress) and produces no warning.
func TestGraphValidator_should_ReturnNoWarning_When_CycleHasAtLeastOneGatedEdge(t *testing.T) {
	t.Parallel()
	stages := []StageDefinition{
		{Slug: "a", IsEntry: true},
		{Slug: "b"},
		{Slug: "c"},
	}
	transitions := []TransitionDefinition{
		{FromSlug: "a", ToSlug: "b", Enabled: true},
		{FromSlug: "b", ToSlug: "c", Enabled: true},
		{FromSlug: "c", ToSlug: "a", Enabled: true, GateCount: 1},
	}

	warnings, err := ValidateGraph(stages, transitions)

	require.NoError(t, err)
	require.Empty(t, warnings)
}

// TestGraphValidator_should_RejectDisable_When_LastEnabledOutgoingEdgeHasLiveItems
// covers Task 2.6.1e/f: disabling a stage's only enabled outgoing edge while
// a live item is still on that stage is rejected, naming the stage and the
// live-item count.
func TestGraphValidator_should_RejectDisable_When_LastEnabledOutgoingEdgeHasLiveItems(t *testing.T) {
	t.Parallel()
	// candidateEdges reflects the graph AFTER the proposed disable.
	candidateEdges := []TransitionDefinition{
		{FromSlug: "design-review", ToSlug: "ready", Enabled: false},
	}
	liveItemCountByStage := map[string]int{"design-review": 1}

	err := ValidateDisableTransition(candidateEdges, "design-review", liveItemCountByStage)

	require.Error(t, err)
	require.ErrorIs(t, err, ErrDisableWouldStrandLiveItems)
	require.Contains(t, err.Error(), "design-review")
	require.Contains(t, err.Error(), "1")
}

// TestGraphValidator_should_AllowDisable_When_MultipleEnabledOutgoingEdgesRemain
// covers Task 2.6.1f's second AC: disabling one of several enabled outgoing
// edges remains allowed, since the stage still has >=1 enabled outgoing
// transition available to the live item.
func TestGraphValidator_should_AllowDisable_When_MultipleEnabledOutgoingEdgesRemain(t *testing.T) {
	t.Parallel()
	candidateEdges := []TransitionDefinition{
		{FromSlug: "design-review", ToSlug: "ready", Enabled: false},
		{FromSlug: "design-review", ToSlug: "legal-review", Enabled: true},
	}
	liveItemCountByStage := map[string]int{"design-review": 1}

	err := ValidateDisableTransition(candidateEdges, "design-review", liveItemCountByStage)

	require.NoError(t, err)
}

// TestGraphValidator_should_AllowDisable_When_StageHasZeroLiveItems covers
// Task 2.6.1f's third AC: disabling any edge for a stage with zero live
// items is always allowed, regardless of remaining enabled-edge count.
func TestGraphValidator_should_AllowDisable_When_StageHasZeroLiveItems(t *testing.T) {
	t.Parallel()
	candidateEdges := []TransitionDefinition{
		{FromSlug: "design-review", ToSlug: "ready", Enabled: false},
	}
	liveItemCountByStage := map[string]int{"design-review": 0}

	err := ValidateDisableTransition(candidateEdges, "design-review", liveItemCountByStage)

	require.NoError(t, err)
}

// TestGraphValidator_should_TreatUnknownStageAsZeroLiveItems_When_MapHasNoEntry
// is a small defensive-default regression: a stage absent from
// liveItemCountByStage (e.g. a brand-new stage the caller hasn't populated
// yet) must never panic or be treated as having live items.
func TestGraphValidator_should_TreatUnknownStageAsZeroLiveItems_When_MapHasNoEntry(t *testing.T) {
	t.Parallel()
	candidateEdges := []TransitionDefinition{
		{FromSlug: "design-review", ToSlug: "ready", Enabled: false},
	}

	err := ValidateDisableTransition(candidateEdges, "design-review", map[string]int{})

	require.NoError(t, err)
	require.False(t, errors.Is(err, ErrDisableWouldStrandLiveItems))
}
