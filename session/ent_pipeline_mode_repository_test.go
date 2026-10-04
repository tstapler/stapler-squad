package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEntPipelineModeRepository_Create_should_PersistAndRoundTrip_When_ValidInput(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	repo := NewEntPipelineModeRepository(storage.GetEntClient())

	created, err := repo.Create(t.Context(), PipelineModeCreateInput{
		Slug:                 "quick",
		Name:                 "Quick Fix",
		Enabled:              true,
		TriagePromptTemplate: "Fix {{item_id}} quickly.",
	})
	require.NoError(t, err)
	assert.Equal(t, "quick", created.Slug)
	assert.Equal(t, "Quick Fix", created.Name)
	assert.True(t, created.Enabled)
	assert.Equal(t, "Fix {{item_id}} quickly.", created.TriagePromptTemplate)

	fetched, err := repo.GetBySlug(t.Context(), "quick")
	require.NoError(t, err)
	assert.Equal(t, created.ID, fetched.ID)
	assert.Equal(t, created.Slug, fetched.Slug)
	assert.Equal(t, created.Name, fetched.Name)
	assert.Equal(t, created.TriagePromptTemplate, fetched.TriagePromptTemplate)
}

func TestEntPipelineModeRepository_Create_should_ReturnConstraintError_When_DuplicateSlug(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	repo := NewEntPipelineModeRepository(storage.GetEntClient())

	_, err := repo.Create(t.Context(), PipelineModeCreateInput{
		Slug: "quick",
		Name: "Quick Fix",
	})
	require.NoError(t, err)

	_, err = repo.Create(t.Context(), PipelineModeCreateInput{
		Slug: "quick",
		Name: "Quick Fix Again",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrConflict)
}

func TestEntPipelineModeRepository_Update_should_OnlyChangeSuppliedFields_When_PartialUpdateInput(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	repo := NewEntPipelineModeRepository(storage.GetEntClient())

	created, err := repo.Create(t.Context(), PipelineModeCreateInput{
		Slug:                  "quick",
		Name:                  "Quick Fix",
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
	})
	require.NoError(t, err)

	newName := "Quick Fix Renamed"
	updated, err := repo.Update(t.Context(), created.ID, PipelineModeUpdateInput{
		Name: &newName,
	})
	require.NoError(t, err)

	assert.Equal(t, newName, updated.Name)
	// All 9 content-template fields must remain untouched.
	assert.Equal(t, created.StatusCommandTemplate, updated.StatusCommandTemplate)
	assert.Equal(t, created.DoneCommandTemplate, updated.DoneCommandTemplate)
	assert.Equal(t, created.FailCommandTemplate, updated.FailCommandTemplate)
	assert.Equal(t, created.ReviewCommandTemplate, updated.ReviewCommandTemplate)
	assert.Equal(t, created.ShipCommandTemplate, updated.ShipCommandTemplate)
	assert.Equal(t, created.HelpCommandTemplate, updated.HelpCommandTemplate)
	assert.Equal(t, created.TriagePromptTemplate, updated.TriagePromptTemplate)
	assert.Equal(t, created.ReviewPromptTemplate, updated.ReviewPromptTemplate)
	assert.Equal(t, created.InitialPromptTemplate, updated.InitialPromptTemplate)
	assert.Equal(t, created.Slug, updated.Slug)
	assert.Equal(t, created.Enabled, updated.Enabled)
}

func TestEntPipelineModeRepository_should_RoundTripStageExecutorsJSON_When_CreatedWithTriageOverride(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	repo := NewEntPipelineModeRepository(storage.GetEntClient())

	created, err := repo.Create(t.Context(), PipelineModeCreateInput{
		Slug: "cheap-triage",
		Name: "Cheap Triage",
		StageExecutors: map[StageRole]PipelineStageExecutor{
			StageRoleTriage: {Model: "claude-haiku-4-5"},
		},
	})
	require.NoError(t, err)

	parsed, err := ParseStageExecutors(created.StageExecutorsJSON)
	require.NoError(t, err)
	assert.Equal(t, "claude-haiku-4-5", parsed[StageRoleTriage].Model)
	assert.Empty(t, parsed[StageRoleTriage].Program)

	fetched, err := repo.GetBySlug(t.Context(), "cheap-triage")
	require.NoError(t, err)
	fetchedParsed, err := ParseStageExecutors(fetched.StageExecutorsJSON)
	require.NoError(t, err)
	assert.Equal(t, parsed, fetchedParsed)
}

func TestEntPipelineModeRepository_Update_should_ReplaceStageExecutors_When_StageExecutorsPointerNonNil(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	repo := NewEntPipelineModeRepository(storage.GetEntClient())

	created, err := repo.Create(t.Context(), PipelineModeCreateInput{
		Slug: "cheap-triage",
		Name: "Cheap Triage",
		StageExecutors: map[StageRole]PipelineStageExecutor{
			StageRoleTriage: {Model: "claude-haiku-4-5"},
		},
	})
	require.NoError(t, err)

	newExecutors := map[StageRole]PipelineStageExecutor{
		StageRoleReview: {Model: "claude-opus-4-5"},
	}
	updated, err := repo.Update(t.Context(), created.ID, PipelineModeUpdateInput{
		StageExecutors: &newExecutors,
	})
	require.NoError(t, err)

	parsed, err := ParseStageExecutors(updated.StageExecutorsJSON)
	require.NoError(t, err)
	assert.Equal(t, "claude-opus-4-5", parsed[StageRoleReview].Model)
	_, hasTriage := parsed[StageRoleTriage]
	assert.False(t, hasTriage, "Update with a non-nil StageExecutors pointer replaces the map entirely")
}

func TestEntPipelineModeRepository_Update_should_LeaveStageExecutorsUntouched_When_StageExecutorsPointerNil(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	repo := NewEntPipelineModeRepository(storage.GetEntClient())

	created, err := repo.Create(t.Context(), PipelineModeCreateInput{
		Slug: "cheap-triage",
		Name: "Cheap Triage",
		StageExecutors: map[StageRole]PipelineStageExecutor{
			StageRoleTriage: {Model: "claude-haiku-4-5"},
		},
	})
	require.NoError(t, err)

	newName := "Cheap Triage Renamed"
	updated, err := repo.Update(t.Context(), created.ID, PipelineModeUpdateInput{
		Name: &newName,
	})
	require.NoError(t, err)

	assert.Equal(t, created.StageExecutorsJSON, updated.StageExecutorsJSON)
}

func TestEntPipelineModeRepository_ListEnabled_should_ExcludeDisabledRows_When_MixedEnabledState(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	repo := NewEntPipelineModeRepository(storage.GetEntClient())

	enabledMode, err := repo.Create(t.Context(), PipelineModeCreateInput{
		Slug:    "quick",
		Name:    "Quick Fix",
		Enabled: true,
	})
	require.NoError(t, err)

	_, err = repo.Create(t.Context(), PipelineModeCreateInput{
		Slug:    "disabled-mode",
		Name:    "Disabled Mode",
		Enabled: false,
	})
	require.NoError(t, err)

	all, err := repo.ListAll(t.Context())
	require.NoError(t, err)
	assert.Len(t, all, 2)

	enabled, err := repo.ListEnabled(t.Context())
	require.NoError(t, err)
	require.Len(t, enabled, 1)
	assert.Equal(t, enabledMode.Slug, enabled[0].Slug)
}
