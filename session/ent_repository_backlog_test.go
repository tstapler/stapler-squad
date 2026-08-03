package session

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEntRepositoryBacklog_should_RoundTripPipelineMode_When_ItemCreatedWithNonDefaultSlug
// creates a backlog item with a non-default PipelineMode and verifies GetBacklogItem
// reads back the same slug — the ent create+read mapping round trip (Story 1.4.1/1.4.3).
func TestEntRepositoryBacklog_should_RoundTripPipelineMode_When_ItemCreatedWithNonDefaultSlug(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	created, err := repo.CreateBacklogItem(ctx, BacklogItemData{
		Title:        "item using quick mode",
		PipelineMode: "quick",
	})
	require.NoError(t, err)
	assert.Equal(t, "quick", created.PipelineMode)

	fetched, err := repo.GetBacklogItem(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, "quick", fetched.PipelineMode)
}

// TestEntRepositoryBacklog_should_DefaultPipelineModeToEmptyString_When_NotSpecifiedAtCreate
// is the zero-regression baseline: an item created without setting PipelineMode reads
// back PipelineMode == "" (the built-in default pipeline), per Story 1.4.1.
func TestEntRepositoryBacklog_should_DefaultPipelineModeToEmptyString_When_NotSpecifiedAtCreate(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	created, err := repo.CreateBacklogItem(ctx, BacklogItemData{
		Title: "item using default pipeline",
	})
	require.NoError(t, err)
	assert.Equal(t, "", created.PipelineMode)

	fetched, err := repo.GetBacklogItem(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, "", fetched.PipelineMode)
}

// TestItemSessionSnapshot_should_RemainFrozen_When_ItemPipelineModeReassignedAfterSessionStart
// is the single test proving both halves of the Domain Glossary's snapshot discipline
// (Epic 1.6, Story 1.6.2's "explicit focus area"): spawn a session under PipelineMode
// "quick" (snapshot recorded); reassign the item's live pipeline_mode to "full" (case a:
// item reassigned); edit "quick"'s own triage_prompt_template, changing its live content
// hash (case b: mode content edited). Reading the ItemSession back must show the ORIGINAL
// spawn-time slug + hash, unaffected by either later mutation.
func TestItemSessionSnapshot_should_RemainFrozen_When_ItemPipelineModeReassignedAfterSessionStart(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	pmRepo := NewEntPipelineModeRepository(repo.GetEntClient())
	_, err := pmRepo.Create(ctx, PipelineModeCreateInput{
		Slug:                 "quick",
		Name:                 "Quick Fix",
		Enabled:              true,
		TriagePromptTemplate: "original quick triage prompt",
	})
	require.NoError(t, err)
	_, err = pmRepo.Create(ctx, PipelineModeCreateInput{
		Slug:                 "full",
		Name:                 "Full Review",
		Enabled:              true,
		TriagePromptTemplate: "full-mode triage prompt",
	})
	require.NoError(t, err)

	engine, err := NewPipelineEngine(pmRepo)
	require.NoError(t, err)
	spawnTimeHash, ok := engine.ContentHashFor(PipelineMode("quick"))
	require.True(t, ok)
	require.NotEmpty(t, spawnTimeHash)

	item, err := repo.CreateBacklogItem(ctx, BacklogItemData{
		Title:        "item spawning under quick mode",
		PipelineMode: "quick",
	})
	require.NoError(t, err)

	createdIS, err := repo.CreateItemSession(ctx, ItemSessionData{
		ItemID:                   item.ID,
		SessionUUID:              "snapshot-freeze-test-session",
		SessionRole:              SessionRoleWork,
		PipelineModeSnapshot:     "quick",
		PipelineModeSnapshotHash: spawnTimeHash,
	})
	require.NoError(t, err)

	// Case (a): reassign the item's live pipeline_mode to "full".
	newMode := "full"
	_, err = repo.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{PipelineMode: &newMode}, nil)
	require.NoError(t, err)

	// Case (b): edit "quick"'s own content, changing its live content hash.
	quickMode, err := pmRepo.GetBySlug(ctx, "quick")
	require.NoError(t, err)
	newTemplate := "edited quick triage prompt — content since changed"
	_, err = pmRepo.Update(ctx, quickMode.ID, PipelineModeUpdateInput{TriagePromptTemplate: &newTemplate})
	require.NoError(t, err)

	// Sanity: "quick"'s live content hash has actually changed (proves the edit was real).
	require.NoError(t, engine.InvalidateCache(ctx))
	liveHash, ok := engine.ContentHashFor(PipelineMode("quick"))
	require.True(t, ok)
	require.NotEqual(t, spawnTimeHash, liveHash, "setup: the edit must have changed quick's live content hash")

	fetched, err := repo.GetItemSession(ctx, createdIS.ID)
	require.NoError(t, err)
	assert.Equal(t, "quick", fetched.PipelineModeSnapshot, "snapshot slug must remain frozen despite the item's live pipeline_mode being reassigned to full")
	assert.Equal(t, spawnTimeHash, fetched.PipelineModeSnapshotHash, "snapshot hash must remain frozen despite quick's own content being edited")

	// Confirm the item's live pipeline_mode really did change, proving the frozen
	// snapshot above is not just an artifact of the reassignment silently failing.
	fetchedItem, err := repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "full", fetchedItem.PipelineMode)
}

// TestMigrationShouldBeReversible_WhenBacklogItemGainsOptionalShipSnapshotFields
// is the ent-based equivalent of migration reversibility (Story 3.1.2 / Step 5 of
// plan.md's Migration Plan): since ent's auto-migration is additive/optional-field
// with no down-migration to execute, "reversible" is verified as "old rows are
// unaffected and new/optional columns default safely."
func TestMigrationShouldBeReversible_WhenBacklogItemGainsOptionalShipSnapshotFields(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	// Assertion 1: a pre-existing row (created without setting any of the 6 new
	// fields, simulating a row created before this migration landed) reads back
	// all 6 fields at their nil/zero-value default.
	preExisting, err := repo.CreateBacklogItem(ctx, BacklogItemData{
		Title: "item created before ship-snapshot fields existed",
	})
	require.NoError(t, err)

	fetchedPre, err := repo.GetBacklogItem(ctx, preExisting.ID)
	require.NoError(t, err)
	assert.Equal(t, "", fetchedPre.ShippedCheckConclusion)
	assert.Equal(t, 0, fetchedPre.ShippedApprovedCount)
	assert.Equal(t, 0, fetchedPre.ShippedChangesReqCount)
	assert.Nil(t, fetchedPre.ShippedSnapshotAt)
	assert.Equal(t, "", fetchedPre.ShippedFileStats)
	assert.False(t, fetchedPre.ShippedSnapshotCaptureFailed)

	// Assertion 2: a round-trip write via the same UpdateBacklogItem call
	// CaptureShipSnapshot uses sets a subset of the 6 fields on that same
	// pre-existing row, and every field that existed on the row before this
	// migration is left unchanged (confirming the additive column add did not
	// disturb existing data).
	preUpdateStatus := fetchedPre.Status
	preUpdatePrNumber := fetchedPre.PrNumber
	snapshotAt := time.Now().UTC().Truncate(time.Second)
	approvedCount := 2
	captureFailed := true
	_, err = repo.UpdateBacklogItem(ctx, preExisting.ID, BacklogItemUpdate{
		ShippedSnapshotAt:            &snapshotAt,
		ShippedApprovedCount:         &approvedCount,
		ShippedSnapshotCaptureFailed: &captureFailed,
	}, nil)
	require.NoError(t, err)

	fetchedAfterUpdate, err := repo.GetBacklogItem(ctx, preExisting.ID)
	require.NoError(t, err)
	require.NotNil(t, fetchedAfterUpdate.ShippedSnapshotAt)
	assert.True(t, snapshotAt.Equal(*fetchedAfterUpdate.ShippedSnapshotAt))
	assert.Equal(t, approvedCount, fetchedAfterUpdate.ShippedApprovedCount)
	assert.True(t, fetchedAfterUpdate.ShippedSnapshotCaptureFailed)
	assert.Equal(t, preUpdateStatus, fetchedAfterUpdate.Status, "existing Status must be unchanged by the additive-field update")
	assert.Equal(t, preUpdatePrNumber, fetchedAfterUpdate.PrNumber, "existing PrNumber must be unchanged by the additive-field update")

	// Assertion 3: a brand-new item created with none of the 6 fields explicitly
	// set persists successfully with no NOT NULL constraint violation, confirming
	// Optional() is honored end-to-end through the generated client.
	fresh, err := repo.CreateBacklogItem(ctx, BacklogItemData{
		Title: "brand-new item, no ship-snapshot fields set",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, fresh.ID)
}

// TestUpdateBacklogItem_ShouldRoundTripAllSixSnapshotFields_ThroughEntBackedStorage
// sets all 6 new fields via UpdateBacklogItem then reads them back via GetBacklogItem
// against a real ent-backed Storage, confirming the ent_repository_backlog.go mapping
// (both the update-builder and the read-mapper) round-trips correctly end-to-end.
func TestUpdateBacklogItem_ShouldRoundTripAllSixSnapshotFields_ThroughEntBackedStorage(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	item, err := repo.CreateBacklogItem(ctx, BacklogItemData{
		Title: "item for full snapshot round-trip",
	})
	require.NoError(t, err)

	checkConclusion := "success"
	approvedCount := 3
	changesReqCount := 1
	snapshotAt := time.Now().UTC().Truncate(time.Second)
	fileStats := `[{"path":"foo.go","status":1,"additions":5,"deletions":2}]`
	captureFailed := false

	_, err = repo.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{
		ShippedCheckConclusion:       &checkConclusion,
		ShippedApprovedCount:         &approvedCount,
		ShippedChangesReqCount:       &changesReqCount,
		ShippedSnapshotAt:            &snapshotAt,
		ShippedFileStats:             &fileStats,
		ShippedSnapshotCaptureFailed: &captureFailed,
	}, nil)
	require.NoError(t, err)

	fetched, err := repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, checkConclusion, fetched.ShippedCheckConclusion)
	assert.Equal(t, approvedCount, fetched.ShippedApprovedCount)
	assert.Equal(t, changesReqCount, fetched.ShippedChangesReqCount)
	require.NotNil(t, fetched.ShippedSnapshotAt)
	assert.True(t, snapshotAt.Equal(*fetched.ShippedSnapshotAt))
	assert.Equal(t, fileStats, fetched.ShippedFileStats)
	assert.False(t, fetched.ShippedSnapshotCaptureFailed)
}

// TestEntRepositoryBacklog_PrFeedbackAddressedAt_should_RoundTrip mirrors
// ShippedSnapshotAt's round-trip test above: create an item, assert
// PrFeedbackAddressedAt is nil, update it with a timestamp, re-fetch, assert
// it round-trips exactly — also incidentally exercises the ent auto-migration
// path for the new nullable pr_feedback_addressed_at column (fresh schema
// creation includes it).
func TestEntRepositoryBacklog_PrFeedbackAddressedAt_should_RoundTrip(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	item, err := repo.CreateBacklogItem(ctx, BacklogItemData{
		Title: "item for pr-feedback-watermark round-trip",
	})
	require.NoError(t, err)

	fetchedPre, err := repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Nil(t, fetchedPre.PrFeedbackAddressedAt)

	watermark := time.Now().UTC().Truncate(time.Second)
	_, err = repo.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{
		PrFeedbackAddressedAt: &watermark,
	}, nil)
	require.NoError(t, err)

	fetchedAfterUpdate, err := repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	require.NotNil(t, fetchedAfterUpdate.PrFeedbackAddressedAt)
	assert.True(t, watermark.Equal(*fetchedAfterUpdate.PrFeedbackAddressedAt))

	// Clearing must be explicit via ClearPrFeedbackAddressedAt, not achievable
	// by passing a nil pointer (which means "leave untouched").
	_, err = repo.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{
		ClearPrFeedbackAddressedAt: true,
	}, nil)
	require.NoError(t, err)

	fetchedAfterClear, err := repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Nil(t, fetchedAfterClear.PrFeedbackAddressedAt)
}
