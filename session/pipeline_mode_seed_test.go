package session

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/ent"
)

// TestEnsureDefaultSDDPipelineMode_should_CreateRow_When_Missing verifies the
// seed creates a "sdd" PipelineMode row with the expected slug/name/enabled
// state against a real ent-backed repository.
func TestEnsureDefaultSDDPipelineMode_should_CreateRow_When_Missing(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	repo := NewEntPipelineModeRepository(storage.GetEntClient())

	err := EnsureDefaultSDDPipelineMode(t.Context(), repo)
	require.NoError(t, err)

	pm, err := repo.GetBySlug(t.Context(), DefaultSDDPipelineModeSlug)
	require.NoError(t, err)
	assert.Equal(t, DefaultSDDPipelineModeSlug, pm.Slug)
	assert.Equal(t, "SDD (Stapler-Driven Development)", pm.Name)
	assert.True(t, pm.Enabled)
	assert.NotEmpty(t, pm.InitialPromptTemplate)
	assert.NotEmpty(t, pm.TriagePromptTemplate)
	assert.NotEmpty(t, pm.ReviewPromptTemplate)
}

// TestEnsureDefaultSDDPipelineMode_should_BeNoOp_When_AlreadyExists verifies
// the seed never overwrites an existing "sdd" row — in particular, an
// operator's later hand-edit via the pipeline-modes settings UI must survive
// every subsequent server restart (ADR-001's runtime-editability guarantee).
func TestEnsureDefaultSDDPipelineMode_should_BeNoOp_When_AlreadyExists(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()
	repo := NewEntPipelineModeRepository(storage.GetEntClient())

	require.NoError(t, EnsureDefaultSDDPipelineMode(t.Context(), repo))

	// Simulate an operator hand-edit after the first seed.
	first, err := repo.GetBySlug(t.Context(), DefaultSDDPipelineModeSlug)
	require.NoError(t, err)
	edited, err := repo.Update(t.Context(), first.ID, PipelineModeUpdateInput{
		Name: strPtr("Operator's Custom SDD Mode"),
	})
	require.NoError(t, err)
	assert.Equal(t, "Operator's Custom SDD Mode", edited.Name)

	// Re-run the seed (simulates a second server boot).
	require.NoError(t, EnsureDefaultSDDPipelineMode(t.Context(), repo))

	after, err := repo.GetBySlug(t.Context(), DefaultSDDPipelineModeSlug)
	require.NoError(t, err)
	assert.Equal(t, "Operator's Custom SDD Mode", after.Name, "seed must never overwrite an operator's hand-edit")
	assert.Equal(t, edited.ID, after.ID, "seed must not delete-and-recreate the row")
}

// TestEnsureDefaultSDDPipelineMode_should_PassContentValidation guards the
// seed content itself against the same structural-integrity checks the
// CreatePipelineMode/UpdatePipelineMode RPCs enforce at write time — a seed
// content bug would otherwise fail silently (EnsureDefaultSDDPipelineMode
// logs a Warn and the caller continues booting per the non-fatal-boot NFR),
// so this test is the only thing standing between a typo'd placeholder or an
// accidental shell metacharacter and a permanently-broken seed in production.
func TestEnsureDefaultSDDPipelineMode_should_PassContentValidation(t *testing.T) {
	input := defaultSDDPipelineModeInput()

	err := ValidatePipelineModeContent(PipelineModeContentFields{
		Slug:                  input.Slug,
		ValidateSlug:          true,
		StatusCommandTemplate: input.StatusCommandTemplate,
		DoneCommandTemplate:   input.DoneCommandTemplate,
		FailCommandTemplate:   input.FailCommandTemplate,
		ReviewCommandTemplate: input.ReviewCommandTemplate,
		ShipCommandTemplate:   input.ShipCommandTemplate,
		HelpCommandTemplate:   input.HelpCommandTemplate,
		TriagePromptTemplate:  input.TriagePromptTemplate,
		ReviewPromptTemplate:  input.ReviewPromptTemplate,
		InitialPromptTemplate: input.InitialPromptTemplate,
	})
	assert.NoError(t, err)
}

// TestEnsureDefaultSDDPipelineMode_should_ReturnNil_When_NilRepo guards the
// nil-repo degrade-gracefully path (mirrors how every other PipelineEngine
// call site nil-checks and falls back rather than panicking).
func TestEnsureDefaultSDDPipelineMode_should_ReturnNil_When_NilRepo(t *testing.T) {
	assert.NoError(t, EnsureDefaultSDDPipelineMode(context.Background(), nil))
}

// TestEnsureDefaultSDDPipelineMode_should_NotError_When_CreateRaceLoses uses a
// fake repository to simulate a lost create-race (another boot, or a
// concurrent caller, created the row a moment after this GetBySlug missed
// it) — the seed must treat that as success, not a fatal boot error, since
// the row exists either way.
func TestEnsureDefaultSDDPipelineMode_should_NotError_When_CreateRaceLoses(t *testing.T) {
	repo := &raceLosingPipelineModeRepo{}
	err := EnsureDefaultSDDPipelineMode(t.Context(), repo)
	assert.NoError(t, err)
	assert.True(t, repo.createCalled, "expected Create to have been attempted")
}

// raceLosingPipelineModeRepo is a minimal PipelineModeRepository test double
// whose GetBySlug always misses and whose Create always reports ErrConflict,
// simulating a lost create-race.
type raceLosingPipelineModeRepo struct {
	createCalled bool
}

func (r *raceLosingPipelineModeRepo) Create(_ context.Context, _ PipelineModeCreateInput) (*ent.PipelineMode, error) {
	r.createCalled = true
	return nil, ErrConflict
}
func (r *raceLosingPipelineModeRepo) Update(_ context.Context, _ uuid.UUID, _ PipelineModeUpdateInput) (*ent.PipelineMode, error) {
	return nil, ErrNotFound
}
func (r *raceLosingPipelineModeRepo) Delete(_ context.Context, _ uuid.UUID) error { return ErrNotFound }
func (r *raceLosingPipelineModeRepo) GetByID(_ context.Context, _ uuid.UUID) (*ent.PipelineMode, error) {
	return nil, ErrNotFound
}
func (r *raceLosingPipelineModeRepo) GetBySlug(_ context.Context, _ string) (*ent.PipelineMode, error) {
	return nil, ErrNotFound
}
func (r *raceLosingPipelineModeRepo) ListAll(_ context.Context) ([]*ent.PipelineMode, error) {
	return nil, nil
}
func (r *raceLosingPipelineModeRepo) ListEnabled(_ context.Context) ([]*ent.PipelineMode, error) {
	return nil, nil
}

var _ PipelineModeRepository = (*raceLosingPipelineModeRepo)(nil)

func strPtr(s string) *string { return &s }
