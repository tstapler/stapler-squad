package session

// ent_gate_satisfaction_repository_test.go covers ADR-006 Part A's Concurrency
// paragraph: EntGateSatisfactionRepository.Update's GateSatisfactionUpdateInput.
// ExpectedSatisfied turns Update into a compare-and-swap via a single
// conditional bulk update (rather than the pre-ADR-006 select-then-UpdateOneID
// TOCTOU-prone shape), disambiguating "row doesn't exist" (ErrNotFound) from
// "another writer already changed Satisfied out from under this CAS"
// (ErrConflict).

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/ent"
)

// newTestGateSatisfactionFixture creates a fresh stage/transition/gate row
// triple (the FK chain GateSatisfactionRecord.gate_id requires) and returns
// the repo plus that gate's ID, ready for a caller to Create/Update
// GateSatisfactionRecord rows against.
func newTestGateSatisfactionFixture(t *testing.T) (*EntGateSatisfactionRepository, *ent.Client, uuid.UUID) {
	t.Helper()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	client := repo.client

	fromStage, err := client.BacklogStage.Create().SetSlug("cas-from-" + uuid.NewString()).SetName("From").Save(ctx)
	require.NoError(t, err)
	toStage, err := client.BacklogStage.Create().SetSlug("cas-to-" + uuid.NewString()).SetName("To").Save(ctx)
	require.NoError(t, err)
	transition, err := client.StageTransition.Create().
		SetFromStageID(fromStage.ID).SetToStageID(toStage.ID).Save(ctx)
	require.NoError(t, err)
	gate, err := client.TransitionGate.Create().
		SetTransitionID(transition.ID).SetKind(string(GateKindHumanApproval)).Save(ctx)
	require.NoError(t, err)

	return NewEntGateSatisfactionRepository(client), client, gate.ID
}

// TestEntGateSatisfactionRepository_Update_should_Succeed_When_ExpectedSatisfiedMatchesCurrentValue
// covers the CAS happy path: ExpectedSatisfied matching the row's current
// Satisfied value lets the write through.
func TestEntGateSatisfactionRepository_Update_should_Succeed_When_ExpectedSatisfiedMatchesCurrentValue(t *testing.T) {
	t.Parallel()
	repo, _, gateID := newTestGateSatisfactionFixture(t)
	ctx := context.Background()
	itemID := uuid.New()

	_, err := repo.Create(ctx, GateSatisfactionCreateInput{ItemID: itemID, GateID: gateID, Satisfied: false, SatisfiedBy: "first"})
	require.NoError(t, err)

	expected := false
	newSatisfied := true
	updated, err := repo.Update(ctx, itemID, gateID, GateSatisfactionUpdateInput{
		Satisfied:         &newSatisfied,
		ExpectedSatisfied: &expected,
	})
	require.NoError(t, err)
	assert.True(t, updated.Satisfied)
}

// TestEntGateSatisfactionRepository_Update_should_ReturnConflictAndLeaveRowUnchanged_When_ExpectedSatisfiedMismatches
// covers the CAS's whole purpose: a mismatched ExpectedSatisfied must return
// ErrConflict and must NOT apply the write.
func TestEntGateSatisfactionRepository_Update_should_ReturnConflictAndLeaveRowUnchanged_When_ExpectedSatisfiedMismatches(t *testing.T) {
	t.Parallel()
	repo, _, gateID := newTestGateSatisfactionFixture(t)
	ctx := context.Background()
	itemID := uuid.New()

	_, err := repo.Create(ctx, GateSatisfactionCreateInput{ItemID: itemID, GateID: gateID, Satisfied: false, SatisfiedBy: "first"})
	require.NoError(t, err)

	wrongExpectation := true // row is actually Satisfied: false
	newSatisfied := true
	_, err = repo.Update(ctx, itemID, gateID, GateSatisfactionUpdateInput{
		Satisfied:         &newSatisfied,
		ExpectedSatisfied: &wrongExpectation,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrConflict)

	// The row must be entirely unchanged — the CAS must never partially apply.
	row, err := repo.GetByItemAndGate(ctx, itemID, gateID)
	require.NoError(t, err)
	assert.False(t, row.Satisfied, "a failed CAS must leave the row exactly as it was")
	assert.Equal(t, "first", row.SatisfiedBy)
}

// TestEntGateSatisfactionRepository_Update_should_ReturnNotFound_When_RowDoesNotExistAndExpectedSatisfiedIsNil
// covers today's pre-ADR-006 unconditional-Update behavior, unchanged: no row
// at all still returns ErrNotFound, not ErrConflict, when the caller passed no
// ExpectedSatisfied.
func TestEntGateSatisfactionRepository_Update_should_ReturnNotFound_When_RowDoesNotExistAndExpectedSatisfiedIsNil(t *testing.T) {
	t.Parallel()
	repo, _, gateID := newTestGateSatisfactionFixture(t)
	ctx := context.Background()

	newSatisfied := true
	_, err := repo.Update(ctx, uuid.New(), gateID, GateSatisfactionUpdateInput{Satisfied: &newSatisfied})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}

// TestEntGateSatisfactionRepository_Update_should_ReturnNotFound_When_RowDoesNotExistAndExpectedSatisfiedIsSet
// covers the CAS's disambiguation requirement: a nonexistent row must still
// report ErrNotFound (not ErrConflict) even when the caller did pass an
// ExpectedSatisfied — "the row doesn't exist" and "another writer changed it"
// are different failure modes and must not be conflated.
func TestEntGateSatisfactionRepository_Update_should_ReturnNotFound_When_RowDoesNotExistAndExpectedSatisfiedIsSet(t *testing.T) {
	t.Parallel()
	repo, _, gateID := newTestGateSatisfactionFixture(t)
	ctx := context.Background()

	expected := false
	newSatisfied := true
	_, err := repo.Update(ctx, uuid.New(), gateID, GateSatisfactionUpdateInput{
		Satisfied:         &newSatisfied,
		ExpectedSatisfied: &expected,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
	assert.NotErrorIs(t, err, ErrConflict)
}
