package services

// diagnose_dispatch_store_test.go covers Story 5.2.1's AC: Record inserts a
// Pending row with no outcome, MarkCompleted updates that same row in place
// (never a duplicate insert), and ListByItem returns rows chronologically
// oldest-first. Mirrors nudge_cap_store_test.go's in-memory-SQLite pattern
// (session.NewTestEntRepository's doc comment).

import (
	"context"
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
