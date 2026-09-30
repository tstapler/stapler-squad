package session

// ent_stage_config_repository_test.go verifies the ent-schema-level
// constraints for the Epic 2.2 stage/transition/gate schemas
// (session/ent/schema/backlog_stage.go, stage_transition.go,
// transition_gate.go) directly against a fresh in-memory ent client, ahead of
// any repository/CRUD layer (that's Epic 2.3+). No *Repository type named in
// this file exists yet — the Test* names below mirror validation.md's naming
// intent for the constraint they exercise.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/ent"
)

// TestStageTransitionRepository_should_RejectDuplicate_When_FromStageAndToStagePairAlreadyExists
// covers Story 2.2.1's acceptance criteria: given an existing stage_transitions
// row (idea_id, ready_id), a duplicate Create must violate the
// UNIQUE(from_stage_id, to_stage_id) index.
func TestStageTransitionRepository_should_RejectDuplicate_When_FromStageAndToStagePairAlreadyExists(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	client := repo.client

	idea, err := client.BacklogStage.Create().
		SetSlug("idea").
		SetName("Idea").
		Save(ctx)
	require.NoError(t, err)

	ready, err := client.BacklogStage.Create().
		SetSlug("ready").
		SetName("Ready").
		Save(ctx)
	require.NoError(t, err)

	_, err = client.StageTransition.Create().
		SetFromStageID(idea.ID).
		SetToStageID(ready.ID).
		Save(ctx)
	require.NoError(t, err, "first (idea, ready) transition must succeed")

	_, err = client.StageTransition.Create().
		SetFromStageID(idea.ID).
		SetToStageID(ready.ID).
		Save(ctx)
	require.Error(t, err, "duplicate (from_stage_id, to_stage_id) insert must violate the unique index")
	require.True(t, ent.IsConstraintError(err), "expected a constraint-violation error, got: %v", err)
}

// TestStageTransitionRepository_should_Allow_When_SameFromStageDifferentToStage
// confirms the unique index is the (from_stage_id, to_stage_id) pair, not
// from_stage_id alone.
func TestStageTransitionRepository_should_Allow_When_SameFromStageDifferentToStage(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	client := repo.client

	idea, err := client.BacklogStage.Create().SetSlug("idea").SetName("Idea").Save(ctx)
	require.NoError(t, err)
	ready, err := client.BacklogStage.Create().SetSlug("ready").SetName("Ready").Save(ctx)
	require.NoError(t, err)
	refining, err := client.BacklogStage.Create().SetSlug("refining").SetName("Refining").Save(ctx)
	require.NoError(t, err)

	_, err = client.StageTransition.Create().SetFromStageID(idea.ID).SetToStageID(ready.ID).Save(ctx)
	require.NoError(t, err)

	_, err = client.StageTransition.Create().SetFromStageID(idea.ID).SetToStageID(refining.ID).Save(ctx)
	require.NoError(t, err, "a different to_stage for the same from_stage must not conflict")
}
