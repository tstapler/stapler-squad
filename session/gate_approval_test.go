package session

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestRecordGateApproval_should_PersistAndNotReAsk_When_ApprovalIsRecorded
// covers Story 2.4.1's Task 2.4.1d: given a pending human-approval gate with
// no record, RecordGateApproval(itemID, gateID) must write a satisfied
// GateSatisfactionRecord, and the next PendingGates call for that transition
// must report Satisfied: true — and must stay true even after an unrelated
// item field changes (one-shot, not re-checked against live item state, unlike
// a structural gate).
func TestRecordGateApproval_should_PersistAndNotReAsk_When_ApprovalIsRecorded(t *testing.T) {
	t.Parallel()
	engine, client := newSeededConfiguredWorkflowEngine(t)
	ctx := context.Background()

	from, to := newCustomTransitionWithGates(t, client, engine,
		[]GateKind{GateKindHumanApproval},
		[]bool{true},
	)

	gates, err := client.TransitionGate.Query().All(ctx)
	require.NoError(t, err)
	require.Len(t, gates, 1, "test fixture bug: expected exactly one gate")
	gateID := gates[0].ID

	itemID := uuid.New()
	item := BacklogItemTransitionInput{ItemID: itemID.String(), Status: from, AcCriteria: acCriteriaAllDone(t)}

	// Given: no record exists yet — the gate must report unsatisfied.
	statuses, err := engine.PendingGates(item, to)
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	require.False(t, statuses[0].Satisfied, "an unapproved human_approval gate must report unsatisfied")

	// When: RecordGateApproval is called.
	gateSatisfactionRepo := NewEntGateSatisfactionRepository(client)
	record, err := RecordGateApproval(ctx, gateSatisfactionRepo, itemID, gateID, "tester")
	require.NoError(t, err)
	require.True(t, record.Satisfied)
	require.Equal(t, "tester", record.SatisfiedBy)

	// Then: the next PendingGates call reports Satisfied: true.
	statuses, err = engine.PendingGates(item, to)
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	require.True(t, statuses[0].Satisfied, "PendingGates must reflect the recorded approval without re-asking")

	// And: it stays true even once an unrelated item field regresses — a
	// one-shot recorded approval is never re-derived from live item state,
	// unlike a structural gate (TestPendingGates_should_ReportUnsatisfied_
	// When_PreviouslySatisfiedStructuralGateHasSinceRegressed's converse).
	regressedItem := BacklogItemTransitionInput{ItemID: itemID.String(), Status: from, AcCriteria: acCriteriaOneUnchecked(t)}
	statuses, err = engine.PendingGates(regressedItem, to)
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	require.True(t, statuses[0].Satisfied, "a recorded human approval must remain satisfied regardless of unrelated item-state changes")
}

// TestRecordGateApproval_should_ReturnConflict_When_ItemAndGatePairAlreadySatisfied
// covers the one-shot guarantee's enforcement point: a second RecordGateApproval
// call for the same (item, gate) pair must fail with ErrConflict (the schema's
// UNIQUE(item_id, gate_id) index), not silently succeed or overwrite.
func TestRecordGateApproval_should_ReturnConflict_When_ItemAndGatePairAlreadySatisfied(t *testing.T) {
	t.Parallel()
	engine, client := newSeededConfiguredWorkflowEngine(t)
	ctx := context.Background()

	_, _ = newCustomTransitionWithGates(t, client, engine,
		[]GateKind{GateKindHumanApproval},
		[]bool{true},
	)
	gates, err := client.TransitionGate.Query().All(ctx)
	require.NoError(t, err)
	gateID := gates[0].ID

	itemID := uuid.New()
	gateSatisfactionRepo := NewEntGateSatisfactionRepository(client)

	_, err = RecordGateApproval(ctx, gateSatisfactionRepo, itemID, gateID, "first-approver")
	require.NoError(t, err)

	_, err = RecordGateApproval(ctx, gateSatisfactionRepo, itemID, gateID, "second-approver")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrConflict)
}
