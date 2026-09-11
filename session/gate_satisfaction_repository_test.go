package session

// gate_satisfaction_repository_test.go verifies the ent-schema-level
// UNIQUE(item_id, gate_id) constraint on gate_satisfaction_records
// (session/ent/schema/gate_satisfaction_record.go), directly against a fresh
// in-memory ent client. No GateSatisfactionRepository type exists yet (gate
// evaluation is Epic 2.4+) — the Test name mirrors validation.md's naming
// intent for the constraint it exercises.

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/ent"
)

// TestGateSatisfactionRepository_should_RejectDuplicate_When_ItemAndGatePairAlreadyExists
// covers Story 2.2.1's acceptance criteria for gate_satisfaction_records:
// a duplicate (item_id, gate_id) Create must violate the
// UNIQUE(item_id, gate_id) index.
func TestGateSatisfactionRepository_should_RejectDuplicate_When_ItemAndGatePairAlreadyExists(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	client := repo.client

	fromStage, err := client.BacklogStage.Create().SetSlug("in_progress").SetName("In Progress").Save(ctx)
	require.NoError(t, err)
	toStage, err := client.BacklogStage.Create().SetSlug("review").SetName("Review").Save(ctx)
	require.NoError(t, err)
	transition, err := client.StageTransition.Create().
		SetFromStageID(fromStage.ID).
		SetToStageID(toStage.ID).
		Save(ctx)
	require.NoError(t, err)
	gate, err := client.TransitionGate.Create().
		SetTransitionID(transition.ID).
		SetKind("human_approval").
		Save(ctx)
	require.NoError(t, err)

	itemID := uuid.New()

	_, err = client.GateSatisfactionRecord.Create().
		SetItemID(itemID).
		SetGateID(gate.ID).
		SetSatisfied(true).
		Save(ctx)
	require.NoError(t, err, "first (item, gate) satisfaction record must succeed")

	_, err = client.GateSatisfactionRecord.Create().
		SetItemID(itemID).
		SetGateID(gate.ID).
		SetSatisfied(true).
		Save(ctx)
	require.Error(t, err, "duplicate (item_id, gate_id) insert must violate the unique index")
	require.True(t, ent.IsConstraintError(err), "expected a constraint-violation error, got: %v", err)
}

// TestGateSatisfactionRepository_should_Allow_When_SameItemDifferentGate
// confirms the unique index is the (item_id, gate_id) pair, not item_id alone.
func TestGateSatisfactionRepository_should_Allow_When_SameItemDifferentGate(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	client := repo.client

	fromStage, err := client.BacklogStage.Create().SetSlug("in_progress").SetName("In Progress").Save(ctx)
	require.NoError(t, err)
	toStage, err := client.BacklogStage.Create().SetSlug("review").SetName("Review").Save(ctx)
	require.NoError(t, err)
	transition, err := client.StageTransition.Create().
		SetFromStageID(fromStage.ID).
		SetToStageID(toStage.ID).
		Save(ctx)
	require.NoError(t, err)
	gateA, err := client.TransitionGate.Create().SetTransitionID(transition.ID).SetKind("human_approval").Save(ctx)
	require.NoError(t, err)
	gateB, err := client.TransitionGate.Create().SetTransitionID(transition.ID).SetKind("automated_review").Save(ctx)
	require.NoError(t, err)

	itemID := uuid.New()

	_, err = client.GateSatisfactionRecord.Create().SetItemID(itemID).SetGateID(gateA.ID).Save(ctx)
	require.NoError(t, err)

	_, err = client.GateSatisfactionRecord.Create().SetItemID(itemID).SetGateID(gateB.ID).Save(ctx)
	require.NoError(t, err, "a different gate for the same item must not conflict")
}
