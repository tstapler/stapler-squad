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
