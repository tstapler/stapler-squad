package session

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/mattn/go-sqlite3" // SQLite driver, for the raw-connection schema surgery below
	"github.com/tstapler/stapler-squad/session/ent"
)

// TestEntRepositoryBacklog_should_RoundTripPipelineMode_When_ItemCreatedWithNonDefaultSlug
// creates a backlog item with a non-default PipelineMode and verifies GetBacklogItem
// reads back the same slug — the ent create+read mapping round trip (Story 1.4.1/1.4.3).
func TestEntRepositoryBacklog_should_RoundTripPipelineMode_When_ItemCreatedWithNonDefaultSlug(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// TestGetBacklogItem_Labels_ReadsEmptyForPreExistingRow is the NULL-safety
// test for the new labels field (Epic 0.1, Story 0.1.1): a row created
// without setting Labels must read back as nil/empty, not panic, confirming
// ent's field.Strings(...).Optional() JSON-column scan handles a NULL/empty
// column safely.
func TestGetBacklogItem_Labels_ReadsEmptyForPreExistingRow(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	created, err := repo.client.BacklogItem.Create().
		SetTitle("item created before labels field existed").
		Save(ctx)
	require.NoError(t, err)

	fetched, err := repo.GetBacklogItem(ctx, created.ID.String())
	require.NoError(t, err)
	assert.Empty(t, fetched.Labels)
}

// TestBacklogItemSchema_PlanRejectionFields_AreAdditiveAndBackwardCompatible
// is the migration/backward-compatibility test flagged as an explicit gap by
// project_plans/backlog-operator-feedback-loop's Epic 3 implementer (see
// validation.md's Migration row) — the ent equivalent of
// TestMigrationShouldBeReversible_WhenBacklogItemGainsOptionalShipSnapshotFields
// above, applied to the two nullable columns RejectPlan added
// (plan_rejection_reason, plan_rejected_at). ent's auto-migration
// (createTestEntRepository -> client.Schema.Create) is additive/optional-field
// with no down-migration to execute, so "backward compatible" is verified as
// "a pre-existing-shaped row (one that never wrote these two fields) reads
// back at its zero value, not a null-pointer panic" — plus a regression
// check that the pre-existing ApprovePlan/RejectPlan round trip is
// unaffected by the additive columns.
func TestBacklogItemSchema_PlanRejectionFields_AreAdditiveAndBackwardCompatible(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	// Assertion 1: a row shaped like one created before plan_rejection_reason/
	// plan_rejected_at existed (created via the raw ent client, setting
	// neither field, exactly like TestGetBacklogItem_Labels_ReadsEmptyForPreExistingRow's
	// pre-existing-row precedent above) reads back PlanRejectionReason == ""
	// and PlanRejectedAt == nil — ent's zero-value handling, not a panic.
	preExisting, err := repo.client.BacklogItem.Create().
		SetTitle("item created before plan-rejection fields existed").
		Save(ctx)
	require.NoError(t, err)

	fetchedPre, err := repo.GetBacklogItem(ctx, preExisting.ID.String())
	require.NoError(t, err)
	assert.Equal(t, "", fetchedPre.PlanRejectionReason)
	assert.Nil(t, fetchedPre.PlanRejectedAt)

	// Assertion 2: the same pre-existing row accepts a plan-rejection write
	// (the shape RejectPlan itself performs) without disturbing fields that
	// existed before this migration.
	preUpdateStatus := fetchedPre.Status
	preUpdateTitle := fetchedPre.Title
	reason := "Please also handle the mobile navigation case."
	rejectedAt := time.Now().UTC().Truncate(time.Second)
	_, err = repo.UpdateBacklogItem(ctx, preExisting.ID.String(), BacklogItemUpdate{
		PlanRejectionReason: &reason,
		PlanRejectedAt:      &rejectedAt,
	}, nil)
	require.NoError(t, err)

	fetchedAfterUpdate, err := repo.GetBacklogItem(ctx, preExisting.ID.String())
	require.NoError(t, err)
	assert.Equal(t, reason, fetchedAfterUpdate.PlanRejectionReason)
	require.NotNil(t, fetchedAfterUpdate.PlanRejectedAt)
	assert.True(t, rejectedAt.Equal(*fetchedAfterUpdate.PlanRejectedAt))
	assert.Equal(t, preUpdateStatus, fetchedAfterUpdate.Status, "existing Status must be unchanged by the additive-field update")
	assert.Equal(t, preUpdateTitle, fetchedAfterUpdate.Title, "existing Title must be unchanged by the additive-field update")

	// Assertion 3: a brand-new item created with neither field explicitly set
	// persists successfully with no NOT NULL constraint violation, confirming
	// both columns are genuinely optional end-to-end through the generated
	// client (fresh schema creation via createTestEntRepository already
	// exercises the auto-migration path itself).
	fresh, err := repo.CreateBacklogItem(ctx, BacklogItemData{
		Title: "brand-new item, no plan-rejection fields set",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, fresh.ID)
	assert.Equal(t, "", fresh.PlanRejectionReason)
	assert.Nil(t, fresh.PlanRejectedAt)
}

// TestEntRepositoryBacklog_GitHubSyncedIssueUpdatedAt_should_RoundTrip mirrors
// TestEntRepositoryBacklog_PrFeedbackAddressedAt_should_RoundTrip (Epic 0.6,
// Story 0.6.1): create an item, assert GitHubSyncedIssueUpdatedAt is nil,
// update it with a timestamp, re-fetch, assert it round-trips exactly, then
// confirm ClearGitHubSyncedIssueUpdatedAt explicitly clears it back to nil.
func TestEntRepositoryBacklog_GitHubSyncedIssueUpdatedAt_should_RoundTrip(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	item, err := repo.CreateBacklogItem(ctx, BacklogItemData{
		Title: "item for github-synced-issue-updated-at watermark round-trip",
	})
	require.NoError(t, err)

	fetchedPre, err := repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Nil(t, fetchedPre.GitHubSyncedIssueUpdatedAt)

	watermark := time.Now().UTC().Truncate(time.Second)
	_, err = repo.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{
		GitHubSyncedIssueUpdatedAt: &watermark,
	}, nil)
	require.NoError(t, err)

	fetchedAfterUpdate, err := repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	require.NotNil(t, fetchedAfterUpdate.GitHubSyncedIssueUpdatedAt)
	assert.True(t, watermark.Equal(*fetchedAfterUpdate.GitHubSyncedIssueUpdatedAt))

	// Clearing must be explicit via ClearGitHubSyncedIssueUpdatedAt, not
	// achievable by passing a nil pointer (which means "leave untouched").
	_, err = repo.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{
		ClearGitHubSyncedIssueUpdatedAt: true,
	}, nil)
	require.NoError(t, err)

	fetchedAfterClear, err := repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Nil(t, fetchedAfterClear.GitHubSyncedIssueUpdatedAt)
}

// TestUnresolvedBlockerItemIDs_should_ReportStillBlocked_When_BlockerNotYetResolved
// is the baseline "still blocked" case: a blocker sitting at a non-terminal
// status (in_progress) must keep its dependent out of the eligible set.
func TestUnresolvedBlockerItemIDs_should_ReportStillBlocked_When_BlockerNotYetResolved(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	blocker, err := repo.CreateBacklogItem(ctx, BacklogItemData{Title: "blocker item"})
	require.NoError(t, err)
	blocked, err := repo.CreateBacklogItem(ctx, BacklogItemData{Title: "blocked item"})
	require.NoError(t, err)

	_, err = repo.TransitionBacklogItemStatus(ctx, blocker.ID, BacklogStatusInProgress, nil, "test")
	require.NoError(t, err)

	err = repo.AddBacklogItemDependency(ctx, BacklogItemDependencyEdge{
		BlockerID: blocker.ID,
		BlockedID: blocked.ID,
	})
	require.NoError(t, err)

	unresolved, err := repo.UnresolvedBlockerItemIDs(ctx, []string{blocked.ID})
	require.NoError(t, err)
	assert.True(t, unresolved[blocked.ID], "expected blocked item to still be reported as unresolved")
}

// TestUnresolvedBlockerItemIDs_should_ResolveDependency_When_BlockerReachesDone
// is the regression baseline for the pre-existing intended behavior: a
// blocker that ships (reaches BacklogStatusDone) must unblock its dependent.
func TestUnresolvedBlockerItemIDs_should_ResolveDependency_When_BlockerReachesDone(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	blocker, err := repo.CreateBacklogItem(ctx, BacklogItemData{Title: "blocker item"})
	require.NoError(t, err)
	blocked, err := repo.CreateBacklogItem(ctx, BacklogItemData{Title: "blocked item"})
	require.NoError(t, err)

	err = repo.AddBacklogItemDependency(ctx, BacklogItemDependencyEdge{
		BlockerID: blocker.ID,
		BlockedID: blocked.ID,
	})
	require.NoError(t, err)

	unresolved, err := repo.UnresolvedBlockerItemIDs(ctx, []string{blocked.ID})
	require.NoError(t, err)
	assert.True(t, unresolved[blocked.ID], "expected blocked item to be unresolved before blocker ships")

	_, err = repo.TransitionBacklogItemStatus(ctx, blocker.ID, BacklogStatusDone, nil, "test")
	require.NoError(t, err)

	unresolved, err = repo.UnresolvedBlockerItemIDs(ctx, []string{blocked.ID})
	require.NoError(t, err)
	assert.False(t, unresolved[blocked.ID], "expected blocked item to be eligible once blocker reaches done")
}

// TestUnresolvedBlockerItemIDs_should_TreatArchivedBlockerAsResolved_When_BlockerNeverShipped
// locks in the fix: an archived blocker never transitions to done under
// normal product flow, so treating archived as still-unresolved would
// permanently strand its dependent. Per research/pitfalls.md's Pitfall 4,
// archived must count as resolved just like done.
func TestUnresolvedBlockerItemIDs_should_TreatArchivedBlockerAsResolved_When_BlockerNeverShipped(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	blocker, err := repo.CreateBacklogItem(ctx, BacklogItemData{Title: "blocker item"})
	require.NoError(t, err)
	blocked, err := repo.CreateBacklogItem(ctx, BacklogItemData{Title: "blocked item"})
	require.NoError(t, err)

	err = repo.AddBacklogItemDependency(ctx, BacklogItemDependencyEdge{
		BlockerID: blocker.ID,
		BlockedID: blocked.ID,
	})
	require.NoError(t, err)

	unresolved, err := repo.UnresolvedBlockerItemIDs(ctx, []string{blocked.ID})
	require.NoError(t, err)
	assert.True(t, unresolved[blocked.ID], "expected blocked item to be unresolved before blocker is archived")

	_, err = repo.TransitionBacklogItemStatus(ctx, blocker.ID, BacklogStatusArchived, nil, "test")
	require.NoError(t, err)

	unresolved, err = repo.UnresolvedBlockerItemIDs(ctx, []string{blocked.ID})
	require.NoError(t, err)
	assert.False(t, unresolved[blocked.ID], "expected blocked item to be eligible once blocker is archived")
}

// TestAddBacklogItemDependency_should_RejectSelfDependency_When_BlockerEqualsBlocked
// covers AC6: an item cannot be marked as depending on itself.
func TestAddBacklogItemDependency_should_RejectSelfDependency_When_BlockerEqualsBlocked(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	item, err := repo.CreateBacklogItem(ctx, BacklogItemData{Title: "solo item"})
	require.NoError(t, err)

	err = repo.AddBacklogItemDependency(ctx, BacklogItemDependencyEdge{
		BlockerID: item.ID,
		BlockedID: item.ID,
	})
	require.ErrorIs(t, err, ErrDependencyCycle)
}

// TestAddBacklogItemDependency_should_RejectCycle_When_TwoNodeCycleWouldClose
// covers AC5's 2-node case: A already blocks B, so adding B blocks A would
// close a cycle and must be rejected before any storage write.
func TestAddBacklogItemDependency_should_RejectCycle_When_TwoNodeCycleWouldClose(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	a, err := repo.CreateBacklogItem(ctx, BacklogItemData{Title: "a"})
	require.NoError(t, err)
	b, err := repo.CreateBacklogItem(ctx, BacklogItemData{Title: "b"})
	require.NoError(t, err)

	require.NoError(t, repo.AddBacklogItemDependency(ctx, BacklogItemDependencyEdge{BlockerID: a.ID, BlockedID: b.ID}))

	err = repo.AddBacklogItemDependency(ctx, BacklogItemDependencyEdge{BlockerID: b.ID, BlockedID: a.ID})
	require.ErrorIs(t, err, ErrDependencyCycle)
}

// TestAddBacklogItemDependency_should_RejectCycle_When_LongerCycleWouldClose
// covers AC5's longer-chain case: A blocks B blocks C, so adding C blocks A
// would close a 3-node cycle and must be rejected.
func TestAddBacklogItemDependency_should_RejectCycle_When_LongerCycleWouldClose(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	a, err := repo.CreateBacklogItem(ctx, BacklogItemData{Title: "a"})
	require.NoError(t, err)
	b, err := repo.CreateBacklogItem(ctx, BacklogItemData{Title: "b"})
	require.NoError(t, err)
	c, err := repo.CreateBacklogItem(ctx, BacklogItemData{Title: "c"})
	require.NoError(t, err)

	require.NoError(t, repo.AddBacklogItemDependency(ctx, BacklogItemDependencyEdge{BlockerID: a.ID, BlockedID: b.ID}))
	require.NoError(t, repo.AddBacklogItemDependency(ctx, BacklogItemDependencyEdge{BlockerID: b.ID, BlockedID: c.ID}))

	err = repo.AddBacklogItemDependency(ctx, BacklogItemDependencyEdge{BlockerID: c.ID, BlockedID: a.ID})
	require.ErrorIs(t, err, ErrDependencyCycle)
}

// TestAddBacklogItemDependency_should_UnblockDependent_When_BlockerIsHardDeleted
// covers AC7's hard-delete half: a deleted item can never reach `done`, so the
// schema's entsql.OnDelete(Cascade) annotation on blocking_dependencies /
// blocked_by_dependencies (session/ent/schema/backlog_item.go) removes the
// dependency row along with the deleted blocker, which — via
// UnresolvedBlockerItemIDs's row-based check — unblocks the dependent on the
// next pass. This mirrors the archived-blocker-is-resolved behavior
// (TestUnresolvedBlockerItemIDs_should_TreatArchivedBlockerAsResolved_When_BlockerNeverShipped)
// and is the intended outcome, not an oversight.
func TestAddBacklogItemDependency_should_UnblockDependent_When_BlockerIsHardDeleted(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	blocker, err := repo.CreateBacklogItem(ctx, BacklogItemData{Title: "blocker item"})
	require.NoError(t, err)
	blocked, err := repo.CreateBacklogItem(ctx, BacklogItemData{Title: "blocked item"})
	require.NoError(t, err)

	err = repo.AddBacklogItemDependency(ctx, BacklogItemDependencyEdge{
		BlockerID: blocker.ID,
		BlockedID: blocked.ID,
	})
	require.NoError(t, err)

	unresolved, err := repo.UnresolvedBlockerItemIDs(ctx, []string{blocked.ID})
	require.NoError(t, err)
	assert.True(t, unresolved[blocked.ID], "expected blocked item to be unresolved before blocker is deleted")

	require.NoError(t, repo.DeleteBacklogItem(ctx, blocker.ID))

	unresolved, err = repo.UnresolvedBlockerItemIDs(ctx, []string{blocked.ID})
	require.NoError(t, err)
	assert.False(t, unresolved[blocked.ID], "expected blocked item to be eligible once blocker is hard-deleted")

	count, err := repo.GetEntClient().BacklogItemDependency.Query().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "expected the dependency row to be cascade-deleted along with the blocker")
}

// TestAddBacklogItemDependency_should_NoOp_When_SamePairAddedTwice locks in
// the upsert semantics: adding an already-existing (blocker, blocked) pair
// must succeed silently rather than erroring on the unique-index conflict.
func TestAddBacklogItemDependency_should_NoOp_When_SamePairAddedTwice(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	blocker, err := repo.CreateBacklogItem(ctx, BacklogItemData{Title: "blocker"})
	require.NoError(t, err)
	blocked, err := repo.CreateBacklogItem(ctx, BacklogItemData{Title: "blocked"})
	require.NoError(t, err)

	edge := BacklogItemDependencyEdge{BlockerID: blocker.ID, BlockedID: blocked.ID}
	require.NoError(t, repo.AddBacklogItemDependency(ctx, edge))
	require.NoError(t, repo.AddBacklogItemDependency(ctx, edge))
}

// TestAddBacklogItemDependency_should_ReturnNotFound_When_BlockerDoesNotExist
// guards against the FK-vs-unique-index ambiguity: without an explicit
// existence check, a foreign-key violation from a nonexistent blocker ID is
// indistinguishable from the intentional duplicate-pair unique-index
// conflict under ent.IsConstraintError, and would silently succeed instead
// of surfacing as a not-found error.
func TestAddBacklogItemDependency_should_ReturnNotFound_When_BlockerDoesNotExist(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	blocked, err := repo.CreateBacklogItem(ctx, BacklogItemData{Title: "blocked"})
	require.NoError(t, err)

	err = repo.AddBacklogItemDependency(ctx, BacklogItemDependencyEdge{
		BlockerID: uuid.New().String(),
		BlockedID: blocked.ID,
	})
	require.ErrorIs(t, err, ErrNotFound)
}

// TestAddBacklogItemDependency_should_ReturnNotFound_When_BlockedDoesNotExist
// is the mirror of the above for the blocked side of the edge.
func TestAddBacklogItemDependency_should_ReturnNotFound_When_BlockedDoesNotExist(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	blocker, err := repo.CreateBacklogItem(ctx, BacklogItemData{Title: "blocker"})
	require.NoError(t, err)

	err = repo.AddBacklogItemDependency(ctx, BacklogItemDependencyEdge{
		BlockerID: blocker.ID,
		BlockedID: uuid.New().String(),
	})
	require.ErrorIs(t, err, ErrNotFound)
}

// TestNewEntRepository_should_MigrateExistingDatabaseSuccessfully_When_BacklogItemDependenciesTableIsMissing
// proves ent's auto-migration (client.Schema.Create, invoked inside NewEntRepository) is
// safe against a real, non-empty database that pre-dates the backlog_item_dependencies
// table — the exact gap flagged in review: every other test builds its schema from scratch
// via createTestEntRepository, so none of them exercise migration against pre-existing data.
//
// It builds a database file with the *current* full schema, inserts backlog items to stand
// in for pre-existing production rows, closes that repository, then drops the
// backlog_item_dependencies table via a raw connection to reconstruct the "old schema"
// state. Reopening via NewEntRepository against that same file re-runs the real migration
// code path (client.Schema.Create), so a passing test proves: (1) migration succeeds against
// a database with existing backlog_items rows, (2) those rows survive migration intact, and
// (3) the new table is created and queryable afterward.
func TestNewEntRepository_should_MigrateExistingDatabaseSuccessfully_When_BacklogItemDependenciesTableIsMissing(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, fmt.Sprintf("migration-test-%d.db", time.Now().UnixNano()))

	// Step 1: build the database with the current full schema and seed pre-existing data.
	repo, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)
	ctx := context.Background()

	preExisting, err := repo.CreateBacklogItem(ctx, BacklogItemData{
		Title: "item created before dependency table existed",
	})
	require.NoError(t, err)
	require.NoError(t, repo.Close())

	// Step 2: reconstruct the pre-feature schema by dropping the new join table via a raw
	// connection. backlog_items itself gained no new columns for this feature (only two new
	// edges to an entirely new entity), so this is the only DDL delta to undo.
	rawDB, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_timeout=5000&_fk=1")
	require.NoError(t, err)
	_, err = rawDB.Exec(`DROP TABLE IF EXISTS backlog_item_dependencies;`)
	require.NoError(t, err)
	require.NoError(t, rawDB.Close())

	// Step 3: reopen via the real production entry point. Its internal client.Schema.Create
	// call is the actual migration under test — this must succeed against the now-missing
	// table with pre-existing backlog_items rows still in place.
	migrated, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err, "auto-migration must succeed against a database missing the backlog_item_dependencies table")
	defer migrated.Close()

	// Assertion: the pre-existing row survived migration intact.
	fetched, err := migrated.GetBacklogItem(ctx, preExisting.ID)
	require.NoError(t, err)
	assert.Equal(t, preExisting.ID, fetched.ID)
	assert.Equal(t, "item created before dependency table existed", fetched.Title)

	// Assertion: the new table exists and is queryable post-migration.
	count, err := migrated.GetEntClient().BacklogItemDependency.Query().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

// --- Activity note history (ADR-001) — Phase 8 tests ---

// TestBacklogActivityNote_should_CascadeDelete_When_ParentItemDeleted (Epic
// 8.1) proves the entsql.OnDelete(Cascade) annotation on BacklogItem's
// activity_notes edge (session/ent/schema/backlog_item.go) removes
// activity-note rows along with their parent item, mirroring
// TestAddBacklogItemDependency_should_UnblockDependent_When_BlockerIsHardDeleted's
// GetEntClient().<Type>.Query().Count(ctx) idiom above.
func TestBacklogActivityNote_should_CascadeDelete_When_ParentItemDeleted(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	item, err := repo.CreateBacklogItem(ctx, BacklogItemData{Title: "item with an activity note"})
	require.NoError(t, err)

	require.NoError(t, repo.AppendActivityNote(ctx, item.ID, "", "", "a note"))

	count, err := repo.GetEntClient().BacklogActivityNote.Query().Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count, "expected the note row to exist before the parent item is deleted")

	require.NoError(t, repo.DeleteBacklogItem(ctx, item.ID))

	count, err = repo.GetEntClient().BacklogActivityNote.Query().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "expected the activity note row to be cascade-deleted along with its parent item")
}

// TestAppendActivityNote_should_PersistAndBeListable_When_Called (Epic 8.2,
// Task 8.2.1a) appends two notes and confirms ListActivityNotesForItem
// returns both, ordered created_at ascending.
func TestAppendActivityNote_should_PersistAndBeListable_When_Called(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	item, err := repo.CreateBacklogItem(ctx, BacklogItemData{Title: "item for activity-note list test"})
	require.NoError(t, err)

	require.NoError(t, repo.AppendActivityNote(ctx, item.ID, "session-uuid-1", "worker-1", "first note"))
	require.NoError(t, repo.AppendActivityNote(ctx, item.ID, "session-uuid-2", "worker-2", "second note"))

	notes, err := repo.ListActivityNotesForItem(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, notes, 2)

	assert.Equal(t, "first note", notes[0].Message)
	assert.Equal(t, "session-uuid-1", notes[0].AuthorSessionUUID)
	assert.Equal(t, "worker-1", notes[0].AuthorSessionTitle)

	assert.Equal(t, "second note", notes[1].Message)
	assert.Equal(t, "session-uuid-2", notes[1].AuthorSessionUUID)
	assert.Equal(t, "worker-2", notes[1].AuthorSessionTitle)

	assert.False(t, notes[1].CreatedAt.Before(notes[0].CreatedAt), "expected notes ordered created_at ascending")
}

// TestGetBacklogItem_should_ReturnActivityNotes_When_ItemHasThem (Epic 8.2,
// Task 8.2.1d / validation.md Gap 2) proves GetBacklogItem's
// WithActivityNotes(...) eager-load actually populates
// BacklogItemData.ActivityNotes — a distinct code path from get_backlog_item's
// MCP-tool rendering (Epic 8.6), which calls ListActivityNotesForItem
// directly and never exercises this eager-load at all.
func TestGetBacklogItem_should_ReturnActivityNotes_When_ItemHasThem(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	item, err := repo.CreateBacklogItem(ctx, BacklogItemData{Title: "item for eager-load test"})
	require.NoError(t, err)

	require.NoError(t, repo.AppendActivityNote(ctx, item.ID, "", "", "first"))
	require.NoError(t, repo.AppendActivityNote(ctx, item.ID, "", "", "second"))

	fetched, err := repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, fetched.ActivityNotes, 2, "GetBacklogItem's WithActivityNotes eager-load must populate ActivityNotes")
	assert.Equal(t, "first", fetched.ActivityNotes[0].Message)
	assert.Equal(t, "second", fetched.ActivityNotes[1].Message)
}

// TestMapAppendActivityNoteCreateError_should_ReturnErrNotFound_When_ErrIsConstraintViolation
// and its sibling below unit-test mapAppendActivityNoteCreateError directly
// (Epic 8.2, Task 8.2.1e / validation.md Gap 1) instead of trying to
// reproduce the delete-between-steps race the FK-violation branch actually
// guards against, which is real but impractical to trigger deterministically.
func TestMapAppendActivityNoteCreateError_should_ReturnErrNotFound_When_ErrIsConstraintViolation(t *testing.T) {
	t.Parallel()
	// *ent.ConstraintError's fields are all unexported, but a zero-value
	// composite literal is still constructible from another package — its
	// Error() string ("ent: constraint failed: ") is irrelevant here, only
	// its type matters to ent.IsConstraintError's errors.As check.
	constraintErr := fmt.Errorf("create failed: %w", &ent.ConstraintError{})

	got := mapAppendActivityNoteCreateError(constraintErr, "item-1")

	require.Error(t, got)
	assert.True(t, errors.Is(got, ErrNotFound), "a constraint-violation error must map to ErrNotFound")
}

func TestMapAppendActivityNoteCreateError_should_WrapPlainError_When_ErrIsNotConstraintViolation(t *testing.T) {
	t.Parallel()
	plain := errors.New("boom")

	got := mapAppendActivityNoteCreateError(plain, "item-1")

	require.Error(t, got)
	assert.False(t, errors.Is(got, ErrNotFound), "a non-constraint error must not be misclassified as ErrNotFound")
	assert.True(t, errors.Is(got, plain), "the original error must remain unwrappable")
}

// Note: the "AppendActivityNote publishes ChangeActivityNoteAdded" and
// "...populates Status/RepoPath on the published snapshot" behaviors (Epic
// 8.2, Tasks 8.2.1b/8.2.1c) are tested in the external session_test package
// instead of here — see TestAppendActivityNote_should_PublishActivityNoteAddedEvent_When_Called
// and TestAppendActivityNote_should_PopulateStatusAndRepoPathOnPublishedSnapshot_When_Called
// in session/ent_repository_backlog_events_test.go, which mirror this
// repo's established precedent for this exact kind of assertion
// (TestUpdateAcCriterionStatus_should_publishItemUpdatedEvent_When_CriterionStatusChanges,
// same file) by wiring the real server/services.BacklogItemEventPublisher
// adapter rather than a package-internal recording double — an internal
// session-package test file cannot import server/services without an
// import cycle, since server/services itself imports session.

// TestEntRepositoryBacklog_should_EnforceUniqueConstraint_When_DuplicatePublicIdInserted
// is the ent-level regression guard for Story 1.2's public_id column
// (session/ent/schema/backlog_item.go): two rows left with public_id unset
// must coexist without colliding — the field's Optional() (without
// Nillable()) makes the underlying SQL column a real NULL, and standard SQL
// UNIQUE semantics never treat two NULLs as equal, per that field's
// documented decision — while a genuine duplicate non-empty value must still
// be rejected by the same unique index. Also asserts the unset rows round
// trip through the BacklogItemData.PublicID() accessor as absent, not as a
// parseable id and not as accidentally "equal" to one another.
func TestEntRepositoryBacklog_should_EnforceUniqueConstraint_When_DuplicatePublicIdInserted(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	unsetA, err := repo.client.BacklogItem.Create().SetTitle("unset a").Save(ctx)
	require.NoError(t, err, "creating a row with public_id unset must succeed")
	unsetB, err := repo.client.BacklogItem.Create().SetTitle("unset b").Save(ctx)
	require.NoError(t, err, "a second row with public_id unset must also succeed — NULL != NULL for uniqueness")

	dataA := backlogItemToData(unsetA)
	dataB := backlogItemToData(unsetB)
	if _, ok := dataA.PublicID(); ok {
		t.Fatalf("PublicID() reported present for row %s created without public_id", unsetA.ID)
	}
	if _, ok := dataB.PublicID(); ok {
		t.Fatalf("PublicID() reported present for row %s created without public_id", unsetB.ID)
	}

	const dup = "bl_01JDUPLICATETESTVALUE0001"
	_, err = repo.client.BacklogItem.Create().SetTitle("dup 1").SetPublicID(dup).Save(ctx)
	require.NoError(t, err, "first row with an explicit public_id must succeed")

	_, err = repo.client.BacklogItem.Create().SetTitle("dup 2").SetPublicID(dup).Save(ctx)
	require.Error(t, err, "a second row with the same non-empty public_id must be rejected")
	assert.True(t, ent.IsConstraintError(err), "expected a unique-constraint violation for duplicate public_id, got: %v", err)
}

// pragmaIndexColumns returns the column names covered by a sqlite index,
// via PRAGMA index_info(<name>) on the given raw connection.
func pragmaIndexColumns(t *testing.T, rawDB *sql.DB, indexName string) []string {
	t.Helper()
	rows, err := rawDB.Query(fmt.Sprintf("PRAGMA index_info(%q);", indexName))
	require.NoError(t, err)
	defer rows.Close()

	var cols []string
	for rows.Next() {
		var seqno, cid int
		var name sql.NullString
		require.NoError(t, rows.Scan(&seqno, &cid, &name))
		if name.Valid {
			cols = append(cols, name.String)
		}
	}
	require.NoError(t, rows.Err())
	return cols
}

// backlogItemsHasColumn reports whether backlog_items currently has a column
// with the given name, via PRAGMA table_info on the given raw connection.
func backlogItemsHasColumn(t *testing.T, rawDB *sql.DB, columnName string) bool {
	t.Helper()
	rows, err := rawDB.Query(`PRAGMA table_info(backlog_items);`)
	require.NoError(t, err)
	defer rows.Close()

	found := false
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dfltValue sql.NullString
		require.NoError(t, rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk))
		if name == columnName {
			found = true
		}
	}
	require.NoError(t, rows.Err())
	return found
}

// TestMigrationShouldBeReversible_WhenBacklogItemPublicIdColumnAddedThenRemoved
// is Story 1.2's migration-reversibility guard for the additive `public_id`
// column (session/ent/schema/backlog_item.go), matching the test mapped in
// project_plans/backlog-deep-linking/implementation/validation.md ("Migration
// Plan: additive public_id column" row): run the additive migration up
// against a scratch DB, verify the public_id column exists as
// Optional/Unique/indexed and pre-existing rows are untouched, then run the
// "down" direction and verify the column is gone and the id/UUID-keyed data
// is unaffected.
//
// ent has no first-class down-migration (client.Schema.Create only ever
// migrates forward, additively, to match the current Go schema — see
// TestNewEntRepository_should_MigrateExistingDatabaseSuccessfully_When_BacklogItemDependenciesTableIsMissing
// above for the same up-migration-only reality applied to a whole table).
// So, mirroring that test's established pattern, both migration directions
// are exercised via raw-connection DDL against a real sqlite file: "up" is
// reconstructed by opening a pre-feature-schema DB (public_id column
// dropped) through the real production entry point (NewEntRepository,
// whose internal client.Schema.Create call is the actual code under test),
// and "down" is simulated by dropping the column again afterward — the
// mechanical operation an actual down-migration would perform — then
// confirming the id/UUID column and its data survive untouched.
func TestMigrationShouldBeReversible_WhenBacklogItemPublicIdColumnAddedThenRemoved(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, fmt.Sprintf("public-id-migration-test-%d.db", time.Now().UnixNano()))

	// Step 1: build the database with the current full schema (public_id
	// included) and seed pre-existing rows, one with public_id left unset
	// (simulating a row created before Story 1.4's backfill ever runs) and
	// one with an explicit value.
	repo, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)
	ctx := context.Background()

	unsetItem, err := repo.CreateBacklogItem(ctx, BacklogItemData{
		Title: "item predating the public_id column",
	})
	require.NoError(t, err)

	setItem, err := repo.client.BacklogItem.Create().
		SetTitle("item with an explicit public_id").
		SetPublicID("bl_01JMIGRATIONREVERSIBLETEST1").
		Save(ctx)
	require.NoError(t, err)
	require.NoError(t, repo.Close())

	// Step 2: reconstruct the pre-migration ("before up") schema state by
	// dropping the public_id column via a raw connection — sqlite (bundled
	// via mattn/go-sqlite3 v1.14.40) supports ALTER TABLE ... DROP COLUMN.
	rawDB, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_timeout=5000&_fk=1")
	require.NoError(t, err)
	// sqlite refuses ALTER TABLE ... DROP COLUMN when the column still backs
	// an index, so the unique index over public_id (Story 1.2) must be
	// dropped first.
	_, err = rawDB.Exec(`DROP INDEX IF EXISTS backlog_items_public_id_key;`)
	require.NoError(t, err)
	_, err = rawDB.Exec(`ALTER TABLE backlog_items DROP COLUMN public_id;`)
	require.NoError(t, err)
	require.False(t, backlogItemsHasColumn(t, rawDB, "public_id"), "public_id column must be gone after the pre-migration reconstruction")
	require.NoError(t, rawDB.Close())

	// Step 3 ("up"): reopen via the real production entry point. Its
	// internal client.Schema.Create call is the actual additive migration
	// under test — it must succeed against the now-missing column with
	// pre-existing rows still in place, and must not disturb their id/title
	// data.
	migrated, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err, "the additive public_id migration must succeed against a database missing that column")

	// NewEntRepository's constructor runs BackfillBacklogItemPublicIDs
	// (Story 1.4) immediately after Schema.Create, so the row that predated
	// the column is expected to come back with a freshly minted public_id
	// rather than staying absent — the assertion that matters here is that
	// its pre-existing id/title data is untouched by either the schema
	// migration or the backfill.
	fetchedUnset, err := migrated.GetBacklogItem(ctx, unsetItem.ID)
	require.NoError(t, err)
	assert.Equal(t, unsetItem.ID, fetchedUnset.ID)
	assert.Equal(t, "item predating the public_id column", fetchedUnset.Title)
	if _, ok := fetchedUnset.PublicID(); !ok {
		t.Fatalf("expected public_id to have been backfilled for %s after migration re-added the column", unsetItem.ID)
	}

	// setItem's original explicit value ("bl_01JMIGRATIONREVERSIBLETEST1")
	// cannot survive this round-trip: ALTER TABLE ... DROP COLUMN discards
	// that column's data for every row, not just the ones that were unset —
	// there is no way for a raw column drop to distinguish "had a value" from
	// "didn't." That's the correct, expected behavior of a real down
	// migration on a lossy column removal, not a test bug. What must hold is
	// that setItem's other data (id, title) is unaffected, and that it too
	// receives a freshly backfilled public_id distinct from fetchedUnset's —
	// proving the backfill doesn't collide when multiple rows need new values
	// at once.
	fetchedSet, err := migrated.GetBacklogItem(ctx, setItem.ID.String())
	require.NoError(t, err)
	assert.Equal(t, setItem.ID.String(), fetchedSet.ID)
	assert.Equal(t, "item with an explicit public_id", fetchedSet.Title)
	if _, ok := fetchedSet.PublicID(); !ok {
		t.Fatalf("expected public_id to have been backfilled for %s after migration re-added the column", setItem.ID)
	}
	assert.NotEqual(t, fetchedUnset.PublicIDRaw, fetchedSet.PublicIDRaw, "backfilled public_id values must be unique across rows")

	// Verify the re-added column is Optional (nullable) and Unique+indexed,
	// per Story 1.2's schema decision.
	verifyDB, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_timeout=5000&_fk=1")
	require.NoError(t, err)
	require.True(t, backlogItemsHasColumn(t, verifyDB, "public_id"), "public_id column must exist after the up migration")

	indexRows, err := verifyDB.Query(`PRAGMA index_list(backlog_items);`)
	require.NoError(t, err)
	foundUniquePublicIDIndex := false
	for indexRows.Next() {
		var seq int
		var name string
		var unique, partial int
		var origin string
		require.NoError(t, indexRows.Scan(&seq, &name, &unique, &origin, &partial))
		if unique != 1 {
			continue
		}
		cols := pragmaIndexColumns(t, verifyDB, name)
		if len(cols) == 1 && cols[0] == "public_id" {
			foundUniquePublicIDIndex = true
		}
	}
	require.NoError(t, indexRows.Err())
	require.NoError(t, indexRows.Close())
	require.True(t, foundUniquePublicIDIndex, "expected a unique index over public_id after the up migration")
	require.NoError(t, migrated.Close())

	// Step 4 ("down"): simulate a down-migration by dropping the column
	// again via the same raw-connection mechanism used to build the "before"
	// state in step 2, then verify the id/UUID-keyed rows and their other
	// data are unaffected — the column removal touches only that column.
	_, err = verifyDB.Exec(`DROP INDEX IF EXISTS backlog_items_public_id_key;`)
	require.NoError(t, err)
	_, err = verifyDB.Exec(`ALTER TABLE backlog_items DROP COLUMN public_id;`)
	require.NoError(t, err, "down migration (dropping public_id) must succeed")
	require.False(t, backlogItemsHasColumn(t, verifyDB, "public_id"), "public_id column must be gone after the down migration")

	var unsetTitleAfterDown, setTitleAfterDown string
	require.NoError(t, verifyDB.QueryRow(`SELECT title FROM backlog_items WHERE id = ?;`, unsetItem.ID).Scan(&unsetTitleAfterDown))
	assert.Equal(t, "item predating the public_id column", unsetTitleAfterDown, "id/UUID-keyed row data must be unaffected by the down migration")
	require.NoError(t, verifyDB.QueryRow(`SELECT title FROM backlog_items WHERE id = ?;`, setItem.ID.String()).Scan(&setTitleAfterDown))
	assert.Equal(t, "item with an explicit public_id", setTitleAfterDown, "id/UUID-keyed row data must be unaffected by the down migration")

	require.NoError(t, verifyDB.Close())
}
