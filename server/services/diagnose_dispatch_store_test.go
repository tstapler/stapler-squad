package services

// diagnose_dispatch_store_test.go covers Story 5.2.1's AC: Record inserts a
// Pending row with no outcome, MarkCompleted updates that same row in place
// (never a duplicate insert), and ListByItem returns rows chronologically
// oldest-first. Mirrors nudge_cap_store_test.go's in-memory-SQLite pattern
// (session.NewTestEntRepository's doc comment).

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/diagnose"
)

// newTestDiagnoseDispatchStorage returns a *session.Storage backed by a
// fresh in-memory SQLite database, per this repo's test-isolation
// convention (session.NewTestEntRepository's doc comment).
func newTestDiagnoseDispatchStorage(t *testing.T) *session.Storage {
	t.Helper()
	repo := session.NewTestEntRepository(t)
	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)
	return storage
}

// TestDiagnoseDispatchStore_Record_ShouldInsertPendingRowWithNoOutcome
// covers Story 5.2.1's first AC: Record persists a new row with Status
// Pending and no OutcomeKind.
func TestDiagnoseDispatchStore_Record_ShouldInsertPendingRowWithNoOutcome(t *testing.T) {
	t.Parallel()
	store := NewDiagnoseDispatchStore(newTestDiagnoseDispatchStorage(t))
	ctx := context.Background()

	dispatchID, err := store.Record(ctx, DiagnoseDispatchRequest{
		ItemID:            "item-1",
		TargetSessionUUID: "target-session-uuid",
	})
	require.NoError(t, err)
	require.NotEmpty(t, dispatchID)

	rows, err := store.ListByItem(ctx, "item-1")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, dispatchID, rows[0].ID)
	assert.Equal(t, diagnose.DiagnoseDispatchStatusPending, rows[0].Status)
	assert.Nil(t, rows[0].Outcome)
	assert.Nil(t, rows[0].CompletedAt)
}

// TestDiagnoseDispatchStore_MarkCompleted_ShouldUpdateSameRow_NotInsertSecondRow
// covers Story 5.2.1's second AC example: MarkCompleted updates the same row
// (same ID/CreatedAt) to Completed with the outcome's fields, never a second
// row.
func TestDiagnoseDispatchStore_MarkCompleted_ShouldUpdateSameRow_NotInsertSecondRow(t *testing.T) {
	t.Parallel()
	store := NewDiagnoseDispatchStore(newTestDiagnoseDispatchStorage(t))
	ctx := context.Background()
	itemID := "item-2"

	dispatchID, err := store.Record(ctx, DiagnoseDispatchRequest{
		ItemID:            itemID,
		TargetSessionUUID: "target-session-uuid",
	})
	require.NoError(t, err)

	before, err := store.ListByItem(ctx, itemID)
	require.NoError(t, err)
	require.Len(t, before, 1)
	createdAt := before[0].CreatedAt

	err = store.MarkCompleted(ctx, dispatchID, diagnose.DiagnoseOutcome{Kind: diagnose.DiagnoseOutcomeKindNudged})
	require.NoError(t, err)

	after, err := store.ListByItem(ctx, itemID)
	require.NoError(t, err)
	require.Len(t, after, 1, "MarkCompleted must update the existing row, not insert a second one")
	assert.Equal(t, dispatchID, after[0].ID)
	assert.Equal(t, createdAt, after[0].CreatedAt)
	assert.Equal(t, diagnose.DiagnoseDispatchStatusCompleted, after[0].Status)
	require.NotNil(t, after[0].Outcome)
	assert.Equal(t, diagnose.DiagnoseOutcomeKindNudged, after[0].Outcome.Kind)
	require.NotNil(t, after[0].CompletedAt)
}

// TestDiagnoseDispatchStore_MarkCompleted_ShouldPersistBugFiledOutcomeFields
// covers a second outcome kind's field pairing round-tripping correctly.
func TestDiagnoseDispatchStore_MarkCompleted_ShouldPersistBugFiledOutcomeFields(t *testing.T) {
	t.Parallel()
	store := NewDiagnoseDispatchStore(newTestDiagnoseDispatchStorage(t))
	ctx := context.Background()
	itemID := "item-3"
	bugItemID := "bug-item-99"

	dispatchID, err := store.Record(ctx, DiagnoseDispatchRequest{
		ItemID:            itemID,
		TargetSessionUUID: "target-session-uuid",
	})
	require.NoError(t, err)

	err = store.MarkCompleted(ctx, dispatchID, diagnose.DiagnoseOutcome{
		Kind:      diagnose.DiagnoseOutcomeKindBugFiled,
		BugItemID: &bugItemID,
	})
	require.NoError(t, err)

	rows, err := store.ListByItem(ctx, itemID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.NotNil(t, rows[0].Outcome)
	assert.Equal(t, diagnose.DiagnoseOutcomeKindBugFiled, rows[0].Outcome.Kind)
	require.NotNil(t, rows[0].Outcome.BugItemID)
	assert.Equal(t, bugItemID, *rows[0].Outcome.BugItemID)
}

// TestDiagnoseDispatchStore_MarkCompleted_ShouldRejectInvalidOutcome_WhenKindFieldPairingMismatches
// covers this implementation's judgment call: MarkCompleted validates the
// outcome before persisting, so a malformed Kind/field pairing never reaches
// storage as a corrupt row.
func TestDiagnoseDispatchStore_MarkCompleted_ShouldRejectInvalidOutcome_WhenKindFieldPairingMismatches(t *testing.T) {
	t.Parallel()
	store := NewDiagnoseDispatchStore(newTestDiagnoseDispatchStorage(t))
	ctx := context.Background()

	dispatchID, err := store.Record(ctx, DiagnoseDispatchRequest{
		ItemID:            "item-4",
		TargetSessionUUID: "target-session-uuid",
	})
	require.NoError(t, err)

	// Nudged must not carry a BugItemID per DiagnoseOutcome.Validate.
	badBugItemID := "should-not-be-set"
	err = store.MarkCompleted(ctx, dispatchID, diagnose.DiagnoseOutcome{
		Kind:      diagnose.DiagnoseOutcomeKindNudged,
		BugItemID: &badBugItemID,
	})
	assert.Error(t, err)
}

// TestDiagnoseDispatchStore_ListByItem_ShouldReturnRowsChronologicalOldestFirst
// covers Story 5.2.1's first AC example: three rows recorded for one item, in
// order T1 < T2 < T3.
func TestDiagnoseDispatchStore_ListByItem_ShouldReturnRowsChronologicalOldestFirst(t *testing.T) {
	t.Parallel()
	store := NewDiagnoseDispatchStore(newTestDiagnoseDispatchStorage(t))
	ctx := context.Background()
	itemID := "item-5"

	dispatchIDs := make([]string, 0, 3)
	for range 3 {
		id, err := store.Record(ctx, DiagnoseDispatchRequest{
			ItemID:            itemID,
			TargetSessionUUID: "target-session-uuid",
		})
		require.NoError(t, err)
		dispatchIDs = append(dispatchIDs, id)
	}

	rows, err := store.ListByItem(ctx, itemID)
	require.NoError(t, err)
	require.Len(t, rows, 3)
	for i, row := range rows {
		assert.Equal(t, dispatchIDs[i], row.ID, "row %d out of chronological order", i)
	}
	assert.True(t, rows[0].CreatedAt.Before(rows[1].CreatedAt) || rows[0].CreatedAt.Equal(rows[1].CreatedAt))
	assert.True(t, rows[1].CreatedAt.Before(rows[2].CreatedAt) || rows[1].CreatedAt.Equal(rows[2].CreatedAt))
}

// TestDiagnoseDispatchStore_ListByItem_ShouldReturnMixedPendingAndCompletedRows
// covers Task 5.2.1d: a chronological list reflecting mixed pending/completed
// rows for the same item.
func TestDiagnoseDispatchStore_ListByItem_ShouldReturnMixedPendingAndCompletedRows(t *testing.T) {
	t.Parallel()
	store := NewDiagnoseDispatchStore(newTestDiagnoseDispatchStorage(t))
	ctx := context.Background()
	itemID := "item-6"

	firstID, err := store.Record(ctx, DiagnoseDispatchRequest{
		ItemID:            itemID,
		TargetSessionUUID: "target-session-uuid",
	})
	require.NoError(t, err)
	err = store.MarkCompleted(ctx, firstID, diagnose.DiagnoseOutcome{Kind: diagnose.DiagnoseOutcomeKindNudged})
	require.NoError(t, err)

	secondID, err := store.Record(ctx, DiagnoseDispatchRequest{
		ItemID:            itemID,
		TargetSessionUUID: "target-session-uuid",
	})
	require.NoError(t, err)

	rows, err := store.ListByItem(ctx, itemID)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, firstID, rows[0].ID)
	assert.Equal(t, diagnose.DiagnoseDispatchStatusCompleted, rows[0].Status)
	assert.Equal(t, secondID, rows[1].ID)
	assert.Equal(t, diagnose.DiagnoseDispatchStatusPending, rows[1].Status)
	assert.Nil(t, rows[1].Outcome)
}

// --- Story 4.1.4: dispatch-level duplicate-write guard ---

// TestDiagnoseDispatchStore_FindByDiagnosticSessionUUID_ShouldReturnDispatchID_WhenRowExists
// covers Task 4.1.4g's lookup: a dispatch recorded with a diagnostic session
// UUID is resolvable back to its dispatch ID.
func TestDiagnoseDispatchStore_FindByDiagnosticSessionUUID_ShouldReturnDispatchID_WhenRowExists(t *testing.T) {
	t.Parallel()
	store := NewDiagnoseDispatchStore(newTestDiagnoseDispatchStorage(t))
	ctx := context.Background()

	dispatchID, err := store.Record(ctx, DiagnoseDispatchRequest{
		ItemID:                "item-7",
		TargetSessionUUID:     "target-session-uuid",
		DiagnosticSessionUUID: "diagnostic-session-uuid-7",
	})
	require.NoError(t, err)

	gotID, found, err := store.FindByDiagnosticSessionUUID(ctx, "diagnostic-session-uuid-7")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, dispatchID, gotID)
}

// TestDiagnoseDispatchStore_FindByDiagnosticSessionUUID_ShouldReturnNotFound_WhenNoMatchingRow
// covers Task 4.1.4g's explicit carve-out: no matching row is not an error --
// a caller acting on behalf of a real diagnose dispatch is the only one with
// a row at all (e.g. Tyler manually steering a session has none).
func TestDiagnoseDispatchStore_FindByDiagnosticSessionUUID_ShouldReturnNotFound_WhenNoMatchingRow(t *testing.T) {
	t.Parallel()
	store := NewDiagnoseDispatchStore(newTestDiagnoseDispatchStorage(t))
	ctx := context.Background()

	gotID, found, err := store.FindByDiagnosticSessionUUID(ctx, "no-such-session-uuid")
	require.NoError(t, err)
	assert.False(t, found)
	assert.Empty(t, gotID)
}

// TestDiagnoseDispatchStore_FindPendingByDiagnosticSessionUUID_ShouldReturnRecordWithItemID_WhenRowIsPending
// covers Story 5.2.2's outcome-notify lookup: unlike FindByDiagnosticSessionUUID,
// this returns the full record (including ItemID, which RecordDiagnoseOutcome
// needs) when the matching row is still Pending.
func TestDiagnoseDispatchStore_FindPendingByDiagnosticSessionUUID_ShouldReturnRecordWithItemID_WhenRowIsPending(t *testing.T) {
	t.Parallel()
	store := NewDiagnoseDispatchStore(newTestDiagnoseDispatchStorage(t))
	ctx := context.Background()

	dispatchID, err := store.Record(ctx, DiagnoseDispatchRequest{
		ItemID:                "item-pending",
		TargetSessionUUID:     "target-session-uuid",
		DiagnosticSessionUUID: "diagnostic-session-uuid-pending",
	})
	require.NoError(t, err)

	record, found, err := store.FindPendingByDiagnosticSessionUUID(ctx, "diagnostic-session-uuid-pending")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, dispatchID, record.ID)
	assert.Equal(t, "item-pending", record.ItemID)
	assert.Equal(t, diagnose.DiagnoseDispatchStatusPending, record.Status)
}

// TestDiagnoseDispatchStore_FindPendingByDiagnosticSessionUUID_ShouldReturnNotFound_WhenRowAlreadyCompleted
// covers the idempotency guarantee this method's doc comment names: once a
// dispatch row completes, every later outcome-notify call for the same
// diagnostic session UUID becomes a silent no-op rather than a duplicate
// notification.
func TestDiagnoseDispatchStore_FindPendingByDiagnosticSessionUUID_ShouldReturnNotFound_WhenRowAlreadyCompleted(t *testing.T) {
	t.Parallel()
	store := NewDiagnoseDispatchStore(newTestDiagnoseDispatchStorage(t))
	ctx := context.Background()

	dispatchID, err := store.Record(ctx, DiagnoseDispatchRequest{
		ItemID:                "item-completed",
		TargetSessionUUID:     "target-session-uuid",
		DiagnosticSessionUUID: "diagnostic-session-uuid-completed",
	})
	require.NoError(t, err)
	require.NoError(t, store.MarkCompleted(ctx, dispatchID, diagnose.DiagnoseOutcome{Kind: diagnose.DiagnoseOutcomeKindNudged}))

	record, found, err := store.FindPendingByDiagnosticSessionUUID(ctx, "diagnostic-session-uuid-completed")
	require.NoError(t, err)
	assert.False(t, found)
	assert.Equal(t, DiagnoseDispatchRecord{}, record)
}

// TestDiagnoseDispatchStore_FindPendingByDiagnosticSessionUUID_ShouldReturnNotFound_WhenNoMatchingRow
// mirrors FindByDiagnosticSessionUUID's own carve-out: a manual, non-diagnose
// caller has no row at all.
func TestDiagnoseDispatchStore_FindPendingByDiagnosticSessionUUID_ShouldReturnNotFound_WhenNoMatchingRow(t *testing.T) {
	t.Parallel()
	store := NewDiagnoseDispatchStore(newTestDiagnoseDispatchStorage(t))
	ctx := context.Background()

	record, found, err := store.FindPendingByDiagnosticSessionUUID(ctx, "no-such-session-uuid")
	require.NoError(t, err)
	assert.False(t, found)
	assert.Equal(t, DiagnoseDispatchRecord{}, record)
}

// TestDiagnoseDispatchStore_CheckAndSetWriteAttempted_ShouldReturnFalseAndSetTimestamp_WhenFirstAttemptForDispatch
// is the validation.md-named test: WriteAttemptedAt nil -> sets to now,
// returns (false, nil).
func TestDiagnoseDispatchStore_CheckAndSetWriteAttempted_ShouldReturnFalseAndSetTimestamp_WhenFirstAttemptForDispatch(t *testing.T) {
	t.Parallel()
	store := NewDiagnoseDispatchStore(newTestDiagnoseDispatchStorage(t))
	ctx := context.Background()

	dispatchID, err := store.Record(ctx, DiagnoseDispatchRequest{
		ItemID:                "item-8",
		TargetSessionUUID:     "target-session-uuid",
		DiagnosticSessionUUID: "diagnostic-session-uuid-8",
	})
	require.NoError(t, err)

	alreadyAttempted, err := store.CheckAndSetWriteAttempted(ctx, dispatchID)
	require.NoError(t, err)
	assert.False(t, alreadyAttempted)
}

// TestDiagnoseDispatchStore_CheckAndSetWriteAttempted_ShouldReturnTrueWithoutModifying_WhenSecondAttemptForSameDispatch
// is the validation.md-named test: WriteAttemptedAt already set -> returns
// (true, nil), timestamp unchanged.
func TestDiagnoseDispatchStore_CheckAndSetWriteAttempted_ShouldReturnTrueWithoutModifying_WhenSecondAttemptForSameDispatch(t *testing.T) {
	t.Parallel()
	store := NewDiagnoseDispatchStore(newTestDiagnoseDispatchStorage(t))
	ctx := context.Background()

	dispatchID, err := store.Record(ctx, DiagnoseDispatchRequest{
		ItemID:                "item-9",
		TargetSessionUUID:     "target-session-uuid",
		DiagnosticSessionUUID: "diagnostic-session-uuid-9",
	})
	require.NoError(t, err)

	first, err := store.CheckAndSetWriteAttempted(ctx, dispatchID)
	require.NoError(t, err)
	require.False(t, first)

	second, err := store.CheckAndSetWriteAttempted(ctx, dispatchID)
	require.NoError(t, err)
	assert.True(t, second, "a second attempt for the same dispatch must be rejected")

	third, err := store.CheckAndSetWriteAttempted(ctx, dispatchID)
	require.NoError(t, err)
	assert.True(t, third, "a third attempt must still be rejected, not reset")
}

// --- Story 6.1.5: cross-item stalled-dispatch sweep ---

// TestDiagnoseDispatchStore_ListAllPending_ShouldReturnOnlyPendingRows_AcrossItems
// covers Task 6.1.5b: a cross-item query, and the "already Completed/Stalled
// rows are never reconsidered" AC -- ListAllPending itself is what enforces
// that, by only ever selecting Pending rows.
func TestDiagnoseDispatchStore_ListAllPending_ShouldReturnOnlyPendingRows_AcrossItems(t *testing.T) {
	t.Parallel()
	store := NewDiagnoseDispatchStore(newTestDiagnoseDispatchStorage(t))
	ctx := context.Background()

	pendingA, err := store.Record(ctx, DiagnoseDispatchRequest{ItemID: "item-pending-a", TargetSessionUUID: "t", DiagnosticSessionUUID: "diag-a"})
	require.NoError(t, err)
	pendingB, err := store.Record(ctx, DiagnoseDispatchRequest{ItemID: "item-pending-b", TargetSessionUUID: "t", DiagnosticSessionUUID: "diag-b"})
	require.NoError(t, err)
	completed, err := store.Record(ctx, DiagnoseDispatchRequest{ItemID: "item-completed", TargetSessionUUID: "t", DiagnosticSessionUUID: "diag-c"})
	require.NoError(t, err)
	require.NoError(t, store.MarkCompleted(ctx, completed, diagnose.DiagnoseOutcome{Kind: diagnose.DiagnoseOutcomeKindNudged}))
	stalled, err := store.Record(ctx, DiagnoseDispatchRequest{ItemID: "item-stalled", TargetSessionUUID: "t", DiagnosticSessionUUID: "diag-d"})
	require.NoError(t, err)
	require.NoError(t, store.MarkStalled(ctx, stalled))

	rows, err := store.ListAllPending(ctx)
	require.NoError(t, err)
	gotIDs := make([]string, len(rows))
	for i, r := range rows {
		gotIDs[i] = r.ID
	}
	assert.ElementsMatch(t, []string{pendingA, pendingB}, gotIDs)
}

// TestDiagnoseDispatchStore_MarkStalled_ShouldUpdateSameRow_LeavingOutcomeNull
// covers Task 6.1.5c's AC: Status flips to Stalled, CompletedAt is set,
// OutcomeKind stays null -- never a second row.
func TestDiagnoseDispatchStore_MarkStalled_ShouldUpdateSameRow_LeavingOutcomeNull(t *testing.T) {
	t.Parallel()
	store := NewDiagnoseDispatchStore(newTestDiagnoseDispatchStorage(t))
	ctx := context.Background()
	itemID := "item-stall-1"

	dispatchID, err := store.Record(ctx, DiagnoseDispatchRequest{ItemID: itemID, TargetSessionUUID: "t", DiagnosticSessionUUID: "diag-1"})
	require.NoError(t, err)

	before, err := store.ListByItem(ctx, itemID)
	require.NoError(t, err)
	require.Len(t, before, 1)
	createdAt := before[0].CreatedAt

	require.NoError(t, store.MarkStalled(ctx, dispatchID))

	after, err := store.ListByItem(ctx, itemID)
	require.NoError(t, err)
	require.Len(t, after, 1, "MarkStalled must update the existing row, not insert a second one")
	assert.Equal(t, dispatchID, after[0].ID)
	assert.Equal(t, createdAt, after[0].CreatedAt)
	assert.Equal(t, diagnose.DiagnoseDispatchStatusStalled, after[0].Status)
	assert.Nil(t, after[0].Outcome)
	require.NotNil(t, after[0].CompletedAt)
}

// TestDiagnoseDispatchStore_CheckAndSetWriteAttempted_ShouldAllowExactlyOneOfTwoConcurrentCalls_WhenRaceTestedForSameDispatchID
// is the validation.md-named `-race` test, mirroring
// TestNudgeCapStore_CheckAndReserve_ShouldAllowExactlyOneSuccess_WhenTwoGoroutinesRaceForSameItemAtCapOne's
// pattern: run with `go test -race`.
func TestDiagnoseDispatchStore_CheckAndSetWriteAttempted_ShouldAllowExactlyOneOfTwoConcurrentCalls_WhenRaceTestedForSameDispatchID(t *testing.T) {
	store := NewDiagnoseDispatchStore(newTestDiagnoseDispatchStorage(t))
	ctx := context.Background()

	for i := range 100 {
		dispatchID, err := store.Record(ctx, DiagnoseDispatchRequest{
			ItemID:                fmt.Sprintf("race-item-%d", i),
			TargetSessionUUID:     "target-session-uuid",
			DiagnosticSessionUUID: fmt.Sprintf("race-diagnostic-session-%d", i),
		})
		require.NoError(t, err)

		var wg sync.WaitGroup
		alreadyAttempted := make([]bool, 2)
		errs := make([]error, 2)
		for g := range 2 {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				alreadyAttempted[idx], errs[idx] = store.CheckAndSetWriteAttempted(ctx, dispatchID)
			}(g)
		}
		wg.Wait()

		for _, err := range errs {
			require.NoError(t, err)
		}
		firstAttemptCount := 0
		for _, already := range alreadyAttempted {
			if !already {
				firstAttemptCount++
			}
		}
		assert.Equalf(t, 1, firstAttemptCount, "iteration %d: expected exactly one call to see (false, nil), got %d", i, firstAttemptCount)
	}
}
