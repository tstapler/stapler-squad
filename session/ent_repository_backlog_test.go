package session

import (
	"context"
	"sync"
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

// TestCreateRespawnEvent_should_PersistRowWithBothSessionUuids_When_Called verifies
// the happy path: a respawn event with both a triggering and resulting session UUID
// round-trips through GetBacklogItem's eager-loaded RespawnEvents.
func TestCreateRespawnEvent_should_PersistRowWithBothSessionUuids_When_Called(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	item, err := repo.CreateBacklogItem(ctx, BacklogItemData{Title: "item for respawn event"})
	require.NoError(t, err)

	err = repo.CreateRespawnEvent(ctx, item.ID, RespawnReasonStaleWork, "triggering-uuid", "resulting-uuid", false)
	require.NoError(t, err)

	fetched, err := repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, fetched.RespawnEvents, 1)
	ev := fetched.RespawnEvents[0]
	assert.Equal(t, RespawnReasonStaleWork, ev.Reason)
	require.NotNil(t, ev.TriggeringSessionUUID)
	assert.Equal(t, "triggering-uuid", *ev.TriggeringSessionUUID)
	require.NotNil(t, ev.ResultingSessionUUID)
	assert.Equal(t, "resulting-uuid", *ev.ResultingSessionUUID)
	assert.False(t, ev.Queued)
}

// TestCreateRespawnEvent_should_DedupeWithinWindow_When_CalledTwiceForSameTrigger reproduces
// the double-fire class pre-mortem #1 flags: two overlapping reconciliation sweeps calling
// the same respawn call site for the same item/reason/triggering session within the dedupe
// window must produce exactly one row, not two.
func TestCreateRespawnEvent_should_DedupeWithinWindow_When_CalledTwiceForSameTrigger(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	item, err := repo.CreateBacklogItem(ctx, BacklogItemData{Title: "item for dedupe test"})
	require.NoError(t, err)

	require.NoError(t, repo.CreateRespawnEvent(ctx, item.ID, RespawnReasonReviewAbandoned, "same-trigger", "result-1", false))
	require.NoError(t, repo.CreateRespawnEvent(ctx, item.ID, RespawnReasonReviewAbandoned, "same-trigger", "result-2", false))

	fetched, err := repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, fetched.RespawnEvents, 1, "overlapping calls within the dedupe window must record exactly one row")
}

// TestCreateRespawnEvent_should_DedupeUnderConcurrentGoroutines_When_TwoSweepsRaceForSameItem
// is the literal race the pre-mortem/plan called for (Task 4.4.1d: "or concurrently via two
// goroutines"), not the sequential-calls approximation above: two goroutines call
// CreateRespawnEvent for the same item/reason/triggering-session at the same time, simulating
// two overlapping reconciliation sweeps racing each other rather than one following the other.
// CreateRespawnEvent's dedupe-check-then-insert runs inside one transaction specifically to
// close this window — without it, both goroutines could pass the dedupe check before either
// commits its insert.
func TestCreateRespawnEvent_should_DedupeUnderConcurrentGoroutines_When_TwoSweepsRaceForSameItem(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	item, err := repo.CreateBacklogItem(ctx, BacklogItemData{Title: "item for concurrent dedupe test"})
	require.NoError(t, err)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range 2 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = repo.CreateRespawnEvent(ctx, item.ID, RespawnReasonAutonomousTurn, "racing-trigger", "result", false)
		}(i)
	}
	wg.Wait()
	require.NoError(t, errs[0])
	require.NoError(t, errs[1])

	fetched, err := repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, fetched.RespawnEvents, 1, "two genuinely concurrent respawn events for the same item/reason/trigger must still dedupe to exactly one row")
}

// TestCreateRespawnEvent_should_DedupeEmptyTriggeringUuidRows_When_CalledTwice covers the
// two call sites (AutoRespawnReview/AutoRespawnTriage) that have no prior active session,
// so triggeringSessionUUID is empty — a NULL-vs-NULL dedupe match, not a unique-index one.
func TestCreateRespawnEvent_should_DedupeEmptyTriggeringUuidRows_When_CalledTwice(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	item, err := repo.CreateBacklogItem(ctx, BacklogItemData{Title: "item for nil-trigger dedupe test"})
	require.NoError(t, err)

	require.NoError(t, repo.CreateRespawnEvent(ctx, item.ID, RespawnReasonTriageOrphaned, "", "result-1", false))
	require.NoError(t, repo.CreateRespawnEvent(ctx, item.ID, RespawnReasonTriageOrphaned, "", "result-2", false))

	fetched, err := repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, fetched.RespawnEvents, 1, "overlapping calls with no triggering session must still dedupe")
}

// TestCreateRespawnEvent_should_RecordQueuedAttemptWithEmptyResultingUuid_When_SpawnWasQueued
// verifies the queued/failed distinction (AC8): a queued attempt has queued=true and an
// empty resulting session UUID, distinguishable from a failed spawn attempt.
func TestCreateRespawnEvent_should_RecordQueuedAttemptWithEmptyResultingUuid_When_SpawnWasQueued(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	item, err := repo.CreateBacklogItem(ctx, BacklogItemData{Title: "item for queued respawn test"})
	require.NoError(t, err)

	require.NoError(t, repo.CreateRespawnEvent(ctx, item.ID, RespawnReasonAutonomousTurn, "", "", true))

	fetched, err := repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, fetched.RespawnEvents, 1)
	assert.True(t, fetched.RespawnEvents[0].Queued)
	assert.Nil(t, fetched.RespawnEvents[0].ResultingSessionUUID)
}
