package session

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// LivenessDefinitionRecord is an ent-free DTO representation of a persisted
// stage_liveness_definitions row. LivenessRepository's methods return this
// type (never *ent.StageLivenessDefinition directly) so server/services can
// consume them without importing session/ent — mirrors
// session.HandoffSummary/fromEntHandoffSummary's established pattern
// (session/handoff_summary_service.go) for the same no_ent_in_services lint
// rule (.golangci.yml).
type LivenessDefinitionRecord struct {
	ID        uuid.UUID
	StageSlug string
	// PipelineMode is nil for a mode-less row (applies to all pipeline modes).
	PipelineMode *string
	// Kind mirrors LivenessKind's 3 string values.
	Kind string
	// Shape-specific fields, nil when not applicable to Kind — mirrors the
	// schema's nullable columns exactly (session/ent/schema/stage_liveness_definition.go).
	ExpectedDurationMs      *int64
	StalenessMarginMs       *int64
	MaxNoProgressDurationMs *int64
	CycleThreshold          *int32
	CycleLookbackMs         *int64
	Enabled                 bool
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

// LivenessRepository defines persistence operations for LivenessDefinition
// rows (session/liveness_definition.go), backed by the stage_liveness_definitions
// table (session/ent/schema/stage_liveness_definition.go, Epic 1.1). Mirrors
// WorkflowRepository's interface shape (session/workflow_repository.go).
//
// GetByStageAndMode is a DUMB exact-match query only — it does NOT implement
// the (stage, mode) -> (stage, nil) sparse-override fallback. That fallback
// lives exclusively in livenessCache.Get (session/liveness_cache.go), per
// Story 1.1.2's corrected acceptance criteria: the fallback logic exists in
// exactly one place, not two.
type LivenessRepository interface {
	Create(ctx context.Context, in LivenessCreateInput) (*LivenessDefinitionRecord, error)
	// Update applies a partial update to an existing row by UUID. StageSlug,
	// PipelineMode, and Kind are immutable after creation (mirrors
	// PipelineModeUpdateInput's Slug precedent) — changing the shape or
	// identity of a row requires Delete + Create instead.
	Update(ctx context.Context, id uuid.UUID, in LivenessUpdateInput) (*LivenessDefinitionRecord, error)
	Delete(ctx context.Context, id uuid.UUID) error
	// GetByStageAndMode returns the row matching stageSlug and mode exactly:
	// mode == PipelineModeDefault ("") queries for pipeline_mode IS NULL (the
	// mode-less row itself, not "any mode"); any other mode queries for
	// pipeline_mode = mode. Returns ErrNotFound if no such row exists,
	// regardless of its enabled state — callers needing an "enabled only"
	// check (e.g. CreateLivenessDefinition's duplicate-pair rejection) inspect
	// the returned row's Enabled field themselves.
	GetByStageAndMode(ctx context.Context, stageSlug string, mode PipelineMode) (*LivenessDefinitionRecord, error)
	ListAll(ctx context.Context) ([]*LivenessDefinitionRecord, error)
}

// LivenessCreateInput holds the fields for creating a new LivenessDefinition
// row. Definition carries Kind plus the kind-specific fields (see
// LivenessDefinition's tagged-union doc comment) — it is validated via
// LivenessDefinition.validate() before insert, so a field set for the wrong
// Kind is rejected here rather than silently written to an unrelated nullable
// column. This diverges from WorkflowCreateInput's flat-fields shape
// deliberately: LivenessDefinition's own tagged-union validation already
// exists and re-flattening it here would duplicate that logic.
type LivenessCreateInput struct {
	StageSlug string
	// PipelineMode is nil for a mode-less row (applies to all pipeline modes
	// absent a more specific override).
	PipelineMode *string
	Definition   LivenessDefinition
	// Enabled: nil = use the ent schema default (true), matching
	// WorkflowCreateInput.Enabled's "nil = use schema default" shape.
	Enabled *bool
}

// LivenessUpdateInput holds optional fields for updating an existing
// LivenessDefinition row. All-pointer, partial-update: a nil field is left
// untouched. Only the kind-specific value fields and Enabled are updatable —
// StageSlug/PipelineMode/Kind are immutable (see LivenessRepository.Update's
// doc comment). The repository does not validate that a set field matches the
// row's stored Kind; callers (the CRUD RPC handlers) are responsible for only
// setting fields that apply to the row they're updating.
type LivenessUpdateInput struct {
	ExpectedDuration      *time.Duration
	StalenessMargin       *time.Duration
	MaxNoProgressDuration *time.Duration
	CycleThreshold        *int
	CycleLookback         *time.Duration
	Enabled               *bool
}
