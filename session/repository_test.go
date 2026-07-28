package session

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpdateBacklogItem_ShouldPersistShippedCheckConclusionAndApprovedCount_WhenPartialUpdateApplied
// verifies BacklogItemUpdate's partial-update semantics for the two new snapshot
// fields: setting ShippedCheckConclusion and ShippedApprovedCount persists those two
// fields while leaving every other field on the row unchanged (Story 3.1.3).
func TestUpdateBacklogItem_ShouldPersistShippedCheckConclusionAndApprovedCount_WhenPartialUpdateApplied(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	created, err := repo.CreateBacklogItem(ctx, BacklogItemData{
		Title:    "item for partial snapshot update",
		PrURL:    "https://github.com/example/repo/pull/1",
		PrNumber: 1,
	})
	require.NoError(t, err)

	checkConclusion := "success"
	approvedCount := 2
	_, err = repo.UpdateBacklogItem(ctx, created.ID, BacklogItemUpdate{
		ShippedCheckConclusion: &checkConclusion,
		ShippedApprovedCount:   &approvedCount,
	}, nil)
	require.NoError(t, err)

	fetched, err := repo.GetBacklogItem(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, checkConclusion, fetched.ShippedCheckConclusion)
	assert.Equal(t, approvedCount, fetched.ShippedApprovedCount)

	// All other fields remain unchanged from before the update.
	assert.Equal(t, created.Title, fetched.Title)
	assert.Equal(t, created.PrURL, fetched.PrURL)
	assert.Equal(t, created.PrNumber, fetched.PrNumber)
	assert.Equal(t, created.Status, fetched.Status)
	assert.Equal(t, 0, fetched.ShippedChangesReqCount)
	assert.Nil(t, fetched.ShippedSnapshotAt)
	assert.Equal(t, "", fetched.ShippedFileStats)
	assert.False(t, fetched.ShippedSnapshotCaptureFailed)
}

// TestUpdateBacklogItem_ShouldLeaveCheckConclusionUnchanged_WhenOnlySnapshotCaptureFailedSet
// confirms ShippedCheckConclusion and ShippedSnapshotCaptureFailed are written and read
// independently: setting only ShippedSnapshotCaptureFailed leaves ShippedCheckConclusion
// at its prior/zero value — the architecture-review BLOCKER fix that keeps a capture
// failure from ever being smuggled into the CI-conclusion field (Story 3.1.3).
func TestUpdateBacklogItem_ShouldLeaveCheckConclusionUnchanged_WhenOnlySnapshotCaptureFailedSet(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	created, err := repo.CreateBacklogItem(ctx, BacklogItemData{
		Title: "item for capture-failed-only update",
	})
	require.NoError(t, err)
	require.Equal(t, "", created.ShippedCheckConclusion, "setup: zero-value baseline before the update")

	captureFailed := true
	_, err = repo.UpdateBacklogItem(ctx, created.ID, BacklogItemUpdate{
		ShippedSnapshotCaptureFailed: &captureFailed,
	}, nil)
	require.NoError(t, err)

	fetched, err := repo.GetBacklogItem(ctx, created.ID)
	require.NoError(t, err)
	assert.True(t, fetched.ShippedSnapshotCaptureFailed)
	assert.Equal(t, "", fetched.ShippedCheckConclusion, "ShippedCheckConclusion must remain at its prior/zero value — never overloaded with a capture-failure sentinel")
}
