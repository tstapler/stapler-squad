package backlog

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

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

	pmRepo := session.NewEntPipelineModeRepository(repo.GetEntClient())
	_, err := pmRepo.Create(ctx, session.PipelineModeCreateInput{
		Slug:                 "quick",
		Name:                 "Quick Fix",
		Enabled:              true,
		TriagePromptTemplate: "original quick triage prompt",
	})
	require.NoError(t, err)
	_, err = pmRepo.Create(ctx, session.PipelineModeCreateInput{
		Slug:                 "full",
		Name:                 "Full Review",
		Enabled:              true,
		TriagePromptTemplate: "full-mode triage prompt",
	})
	require.NoError(t, err)

	engine, err := NewPipelineEngine(pmRepo)
	require.NoError(t, err)
	spawnTimeHash, ok := engine.ContentHashFor(backlogdto.PipelineMode("quick"))
	require.True(t, ok)
	require.NotEmpty(t, spawnTimeHash)

	item, err := repo.CreateBacklogItem(ctx, backlogdto.BacklogItemData{
		Title:        "item spawning under quick mode",
		PipelineMode: "quick",
	})
	require.NoError(t, err)

	createdIS, err := repo.CreateItemSession(ctx, backlogdto.ItemSessionData{
		ItemID:                   item.ID,
		SessionUUID:              "snapshot-freeze-test-session",
		SessionRole:              session.SessionRoleWork,
		PipelineModeSnapshot:     "quick",
		PipelineModeSnapshotHash: spawnTimeHash,
	})
	require.NoError(t, err)

	// Case (a): reassign the item's live pipeline_mode to "full".
	newMode := "full"
	_, err = repo.UpdateBacklogItem(ctx, item.ID, backlogdto.BacklogItemUpdate{PipelineMode: &newMode}, nil)
	require.NoError(t, err)

	// Case (b): edit "quick"'s own content, changing its live content hash.
	quickMode, err := pmRepo.GetBySlug(ctx, "quick")
	require.NoError(t, err)
	newTemplate := "edited quick triage prompt — content since changed"
	_, err = pmRepo.Update(ctx, quickMode.ID, session.PipelineModeUpdateInput{TriagePromptTemplate: &newTemplate})
	require.NoError(t, err)

	// Sanity: "quick"'s live content hash has actually changed (proves the edit was real).
	require.NoError(t, engine.InvalidateCache(ctx))
	liveHash, ok := engine.ContentHashFor(backlogdto.PipelineMode("quick"))
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

// TestMigrationShouldBeReversible_WhenPipelineModeAndItemSessionGainStageExecutorFields
// mirrors TestMigrationShouldBeReversible_WhenBacklogItemGainsOptionalShipSnapshotFields
// above, applied to the stage-executor fields this project added to PipelineMode
// (stage_executors_json) and ItemSession (resolved_program, resolved_model,
// executor_snapshot_hash, configured_program, executor_fallback_reason,
// cost_priced). Per validation.md's Migration Test spec: (1) Up — a
// pre-existing-shaped row (created via the repository methods that predate
// this project, setting none of the new fields, exactly like
// newTestItemSessionForCost's pattern above) reads back safe defaults; (2)
// behavior parity — PipelineEngine.ExecutorFor on that pre-existing mode
// returns ("", "") for every StageRole, proving it is never treated as if it
// had overrides it never configured; (3) rollback safety — the row round
// trips through pre-project accessors that don't reference the new fields at
// all, with no error and no data loss on the fields those accessors do use.
func TestMigrationShouldBeReversible_WhenPipelineModeAndItemSessionGainStageExecutorFields(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	// --- (1) Up: pre-existing PipelineMode and ItemSession rows read back safe defaults ---

	pipelineModeRepo := session.NewEntPipelineModeRepository(repo.Client)
	preExistingMode, err := pipelineModeRepo.Create(ctx, session.PipelineModeCreateInput{
		Slug:                  "pre-existing-mode",
		Name:                  "Mode created before stage-executor fields existed",
		Enabled:               true,
		StatusCommandTemplate: "status template",
		DoneCommandTemplate:   "done template",
		FailCommandTemplate:   "fail template",
		ReviewCommandTemplate: "review template",
		ShipCommandTemplate:   "ship template",
		HelpCommandTemplate:   "help template",
		TriagePromptTemplate:  "triage template",
		ReviewPromptTemplate:  "review prompt template",
		InitialPromptTemplate: "initial prompt template",
		// StageExecutors deliberately omitted: simulates a row created before
		// this field existed.
	})
	require.NoError(t, err)

	parsedExecutors, err := session.ParseStageExecutors(preExistingMode.StageExecutorsJSON)
	require.NoError(t, err, "a pre-existing row's stage_executors_json must parse cleanly")
	assert.Equal(t, "{}", preExistingMode.StageExecutorsJSON)
	assert.Empty(t, parsedExecutors)

	preExistingItem, err := repo.CreateBacklogItem(ctx, backlogdto.BacklogItemData{
		Title:        "item for pre-existing item-session stage-executor fields",
		PipelineMode: preExistingMode.Slug,
	})
	require.NoError(t, err)

	preExistingSession, err := repo.CreateItemSession(ctx, backlogdto.ItemSessionData{
		ItemID:      preExistingItem.ID,
		SessionUUID: "pre-existing-stage-executor-session",
		SessionRole: session.SessionRoleWork,
		// ResolvedProgram/ResolvedModel/ExecutorSnapshotHash/ConfiguredProgram/
		// ExecutorFallbackReason/CostUnpriced deliberately omitted: simulates a
		// row created before these fields existed.
	})
	require.NoError(t, err)

	fetchedSession, err := repo.GetItemSession(ctx, preExistingSession.ID)
	require.NoError(t, err)
	assert.True(t, fetchedSession.CostPriced, "existing Claude-only cost data must be retroactively priced")
	assert.Equal(t, "", fetchedSession.ResolvedProgram)
	assert.Equal(t, "", fetchedSession.ResolvedModel)
	assert.Equal(t, "", fetchedSession.ExecutorSnapshotHash)
	assert.Equal(t, "", fetchedSession.ConfiguredProgram)
	assert.Equal(t, "", fetchedSession.ExecutorFallbackReason)

	// --- (2) Behavior parity: ExecutorFor must never invent overrides for a mode that never configured any ---

	engine, err := NewPipelineEngine(pipelineModeRepo)
	require.NoError(t, err)
	itemOnPreExistingMode := &backlogdto.BacklogItemData{ID: preExistingItem.ID, PipelineMode: preExistingMode.Slug}

	for _, role := range []session.StageRole{session.StageRoleTriage, session.StageRoleReview, session.StageRoleWork} {
		program, model := engine.ExecutorFor(itemOnPreExistingMode, role)
		assert.Equal(t, "", program, "role %s: pre-existing mode must never resolve a program override", role)
		assert.Equal(t, "", model, "role %s: pre-existing mode must never resolve a model override", role)
	}

	// --- (3) Rollback safety: round trip through pre-project accessors that never reference the new fields ---

	newName := "Renamed pre-existing mode"
	updatedMode, err := pipelineModeRepo.Update(ctx, preExistingMode.ID, session.PipelineModeUpdateInput{
		Name: &newName,
	})
	require.NoError(t, err)
	assert.Equal(t, newName, updatedMode.Name)
	assert.Equal(t, preExistingMode.Slug, updatedMode.Slug, "existing Slug must be unchanged by an update that never references stage_executors_json")
	assert.Equal(t, "{}", updatedMode.StageExecutorsJSON, "stage_executors_json must be undisturbed by an update that never references it")

	startedAt := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, repo.UpdateItemSessionStarted(ctx, preExistingSession.ID, startedAt))

	fetchedAfterUpdate, err := repo.GetItemSession(ctx, preExistingSession.ID)
	require.NoError(t, err)
	require.NotNil(t, fetchedAfterUpdate.StartedAt)
	assert.True(t, startedAt.Equal(*fetchedAfterUpdate.StartedAt), "StartedAt must round-trip through an accessor that never references the new executor fields")
	assert.True(t, fetchedAfterUpdate.CostPriced, "cost_priced must be undisturbed by an update that never references it")
	assert.Equal(t, "", fetchedAfterUpdate.ResolvedProgram, "ResolvedProgram must be undisturbed by an update that never references it")
}
