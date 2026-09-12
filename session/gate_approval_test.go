package session

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestRecordGateApproval_should_PersistAndNotReAsk_When_ApprovalIsRecorded
// covers Story 2.4.1's Task 2.4.1d: given a pending human-approval gate with
// no record, RecordGateApproval(itemID, gateID, by, true) must write a
// satisfied GateSatisfactionRecord, and the next PendingGates call for that
// transition must report Satisfied: true — and must stay true even after an
// unrelated item field changes (one-shot, not re-checked against live item
// state, unlike a structural gate).
func TestRecordGateApproval_should_PersistAndNotReAsk_When_ApprovalIsRecorded(t *testing.T) {
	t.Parallel()
	engine, client := newSeededConfiguredWorkflowEngine(t)
	ctx := context.Background()

	from, to := newCustomTransitionWithGates(t, client, engine,
		[]GateKind{GateKindHumanApproval},
		[]bool{true},
	)

	//nolint:entfullscan test assertion over a test-scoped in-memory DB; verifies exactly one gate exists.
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

	// When: RecordGateApproval is called with approved: true.
	gateSatisfactionRepo := NewEntGateSatisfactionRepository(client)
	record, err := RecordGateApproval(ctx, gateSatisfactionRepo, itemID, gateID, "tester", true)
	require.NoError(t, err)
	require.True(t, record.Satisfied)
	require.Equal(t, "tester", record.SatisfiedBy)
	require.NotNil(t, record.SatisfiedAt, "an approval must set SatisfiedAt")

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

// TestRecordGateApproval_should_RecordRejectionWithNoSatisfiedAt_When_NoExistingRecord
// covers ADR-006 Part A's initial-reject case: a first-ever RecordGateApproval
// call with approved: false must Create a Satisfied: false row, and must NOT
// set SatisfiedAt — a reject has no "satisfied at" moment.
func TestRecordGateApproval_should_RecordRejectionWithNoSatisfiedAt_When_NoExistingRecord(t *testing.T) {
	t.Parallel()
	repo, _, gateID := newTestGateSatisfactionFixture(t)
	ctx := context.Background()
	itemID := uuid.New()

	record, err := RecordGateApproval(ctx, repo, itemID, gateID, "rejector", false)
	require.NoError(t, err)
	require.False(t, record.Satisfied)
	require.Equal(t, "rejector", record.SatisfiedBy)
	require.Nil(t, record.SatisfiedAt, "a reject must not set SatisfiedAt")
}

// TestRecordGateApproval_should_ReturnConflict_When_ItemAndGatePairAlreadySatisfied
// covers the one-shot guarantee's enforcement point: a second RecordGateApproval
// call — approve or reject — for an already-approved (item, gate) pair must
// fail with ErrConflict (approval is permanently final, per ADR-006 Part A
// case 3), and must leave the existing record untouched.
func TestRecordGateApproval_should_ReturnConflict_When_ItemAndGatePairAlreadySatisfied(t *testing.T) {
	t.Parallel()
	repo, _, gateID := newTestGateSatisfactionFixture(t)
	ctx := context.Background()
	itemID := uuid.New()

	_, err := RecordGateApproval(ctx, repo, itemID, gateID, "first-approver", true)
	require.NoError(t, err)

	_, err = RecordGateApproval(ctx, repo, itemID, gateID, "second-approver", true)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrConflict)

	// Rejecting an already-approved gate must also fail — approval is final
	// regardless of the second call's approved value.
	_, err = RecordGateApproval(ctx, repo, itemID, gateID, "would-be-rejector", false)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrConflict)

	record, err := repo.GetByItemAndGate(ctx, itemID, gateID)
	require.NoError(t, err)
	require.True(t, record.Satisfied)
	require.Equal(t, "first-approver", record.SatisfiedBy, "a rejected ErrConflict attempt must not have touched the existing record")
}

// TestRecordGateApproval_should_ReverseIntoApproval_When_PreviouslyRejected
// covers ADR-006 Part A case 4: a rejected gate (Satisfied: false) can be
// reversed into an approval, which then locks it per case 3 — a second
// approve attempt after the reversal must fail with ErrConflict.
func TestRecordGateApproval_should_ReverseIntoApproval_When_PreviouslyRejected(t *testing.T) {
	t.Parallel()
	repo, _, gateID := newTestGateSatisfactionFixture(t)
	ctx := context.Background()
	itemID := uuid.New()

	_, err := RecordGateApproval(ctx, repo, itemID, gateID, "rejector", false)
	require.NoError(t, err)

	record, err := RecordGateApproval(ctx, repo, itemID, gateID, "approver", true)
	require.NoError(t, err)
	require.True(t, record.Satisfied)
	require.Equal(t, "approver", record.SatisfiedBy)
	require.NotNil(t, record.SatisfiedAt)

	// Now locked: a further call of either kind must be rejected.
	_, err = RecordGateApproval(ctx, repo, itemID, gateID, "late-comer", true)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrConflict)
}

// TestRecordGateApproval_should_RemainRejectedIdempotently_When_RejectedTwice
// covers ADR-006 Part A case 4's re-reject path: rejecting an already-rejected
// gate a second time succeeds (idempotent-ish) and refreshes SatisfiedBy,
// rather than erroring.
func TestRecordGateApproval_should_RemainRejectedIdempotently_When_RejectedTwice(t *testing.T) {
	t.Parallel()
	repo, _, gateID := newTestGateSatisfactionFixture(t)
	ctx := context.Background()
	itemID := uuid.New()

	_, err := RecordGateApproval(ctx, repo, itemID, gateID, "first-rejector", false)
	require.NoError(t, err)

	record, err := RecordGateApproval(ctx, repo, itemID, gateID, "second-rejector", false)
	require.NoError(t, err)
	require.False(t, record.Satisfied)
	require.Equal(t, "second-rejector", record.SatisfiedBy)
	require.Nil(t, record.SatisfiedAt)
}

// orderedWriteGateSatisfactionRepo wraps a real GateSatisfactionRepository,
// letting two concurrent RecordGateApproval callers both read the existing
// record freely (their reads race genuinely — it doesn't matter which goes
// first since neither has written yet) but forcing "second"'s Update call to
// block until "first"'s Update call has fully completed. This is the
// deterministic, non-flaky sequencing the CAS actually needs to prove: with
// the pre-ADR-006 unconditional Update, a reject arriving after a concurrent
// approve committed would silently clobber the just-approved row back to
// false — exactly the TOCTOU bug the adversarial review found. Forcing that
// exact write order (winner's write-then-commit strictly before loser's
// write attempt) makes the loser's ExpectedSatisfied check deterministically
// see the winner's already-changed value and get ErrConflict every run,
// rather than depending on which goroutine happens to reach the DB first —
// the "controlled sequencing... not a flaky real-race" approach.
type orderedWriteGateSatisfactionRepo struct {
	GateSatisfactionRepository
	goesFirst bool
	firstDone chan struct{}
}

func (r *orderedWriteGateSatisfactionRepo) Update(ctx context.Context, itemID, gateID uuid.UUID, in GateSatisfactionUpdateInput) (*GateSatisfactionData, error) {
	if !r.goesFirst {
		<-r.firstDone
	}
	rec, err := r.GateSatisfactionRepository.Update(ctx, itemID, gateID, in)
	if r.goesFirst {
		close(r.firstDone)
	}
	return rec, err
}

// TestRecordGateApproval_should_YieldExactlyOneWinner_When_ApproveAndRejectRaceConcurrently
// covers ADR-006 Part A's Concurrency paragraph end to end — the exact bug the
// adversarial review found (see ADR-006's "Revisions from review"): two
// concurrent RecordGateApproval calls for the same (item, gate), an approve
// and a reject, both reading the same pre-race state and racing to write.
// The approve's write is forced to land first (orderedWriteGateSatisfactionRepo
// above); the reject arriving second must get ErrConflict rather than silently
// clobbering the just-approved row back to Satisfied: false with no error to
// either caller.
func TestRecordGateApproval_should_YieldExactlyOneWinner_When_ApproveAndRejectRaceConcurrently(t *testing.T) {
	t.Parallel()
	repo, _, gateID := newTestGateSatisfactionFixture(t)
	ctx := context.Background()
	itemID := uuid.New()

	_, err := repo.Create(ctx, GateSatisfactionCreateInput{ItemID: itemID, GateID: gateID, Satisfied: false, SatisfiedBy: "original-rejector"})
	require.NoError(t, err)

	firstDone := make(chan struct{})
	approveRepo := &orderedWriteGateSatisfactionRepo{GateSatisfactionRepository: repo, goesFirst: true, firstDone: firstDone}
	rejectRepo := &orderedWriteGateSatisfactionRepo{GateSatisfactionRepository: repo, goesFirst: false, firstDone: firstDone}

	var runWG sync.WaitGroup
	runWG.Add(2)
	var approveErr, rejectErr error

	go func() {
		defer runWG.Done()
		_, approveErr = RecordGateApproval(ctx, approveRepo, itemID, gateID, "approver", true)
	}()
	go func() {
		defer runWG.Done()
		_, rejectErr = RecordGateApproval(ctx, rejectRepo, itemID, gateID, "rejector", false)
	}()
	runWG.Wait()

	require.NoError(t, approveErr, "the racer whose write lands first must win")
	require.Error(t, rejectErr, "the racer whose write lands second must lose, never silently clobber the winner")
	require.ErrorIs(t, rejectErr, ErrConflict)

	// The persisted row must reflect the winner's decision exactly — never a
	// hybrid/corrupted state, and never silently reverted by the loser.
	final, err := repo.GetByItemAndGate(ctx, itemID, gateID)
	require.NoError(t, err)
	require.True(t, final.Satisfied, "the approve that landed first must be the persisted outcome")
	require.Equal(t, "approver", final.SatisfiedBy)
}
