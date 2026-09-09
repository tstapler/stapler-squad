package session

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/stagelivenessdefinition"
)

// EntLivenessRepository implements LivenessRepository using the ent ORM.
type EntLivenessRepository struct {
	client *ent.Client
}

// NewEntLivenessRepository creates a new ent-backed liveness repository.
func NewEntLivenessRepository(client *ent.Client) *EntLivenessRepository {
	return &EntLivenessRepository{client: client}
}

// Create inserts a new LivenessDefinition row. Returns ent.ConstraintError
// (wrapped as ErrConflict) when a row with the same (stage_slug,
// pipeline_mode) pair already exists — the schema's UNIQUE index.
func (r *EntLivenessRepository) Create(ctx context.Context, in LivenessCreateInput) (*LivenessDefinitionRecord, error) {
	if err := in.Definition.validate(); err != nil {
		return nil, fmt.Errorf("create liveness definition: %w", err)
	}

	c := r.client.StageLivenessDefinition.Create().
		SetStageSlug(in.StageSlug).
		SetKind(string(in.Definition.Kind))

	if in.PipelineMode != nil {
		c.SetPipelineMode(*in.PipelineMode)
	}
	if in.Enabled != nil {
		c.SetEnabled(*in.Enabled)
	}

	switch in.Definition.Kind {
	case LivenessKindDurationBudget:
		c.SetExpectedDurationMs(int64(in.Definition.ExpectedDuration / time.Millisecond))
		c.SetStalenessMarginMs(int64(in.Definition.StalenessMargin / time.Millisecond))
	case LivenessKindHeartbeat:
		c.SetMaxNoProgressDurationMs(int64(in.Definition.MaxNoProgressDuration / time.Millisecond))
	case LivenessKindCycleFrequency:
		//nolint:gosec // CycleThreshold is a small operator-configured count, never near int32 range.
		c.SetCycleThreshold(int32(in.Definition.CycleThreshold))
		c.SetCycleLookbackMs(int64(in.Definition.CycleLookback / time.Millisecond))
	}

	row, err := c.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			mode := "<all modes>"
			if in.PipelineMode != nil {
				mode = *in.PipelineMode
			}
			return nil, fmt.Errorf("%w: liveness definition for stage %q mode %q already exists", ErrConflict, in.StageSlug, mode)
		}
		return nil, fmt.Errorf("create liveness definition: %w", err)
	}
	return recordFromEntLivenessDefinition(row), nil
}

// Update applies a partial update to an existing row by UUID.
func (r *EntLivenessRepository) Update(ctx context.Context, id uuid.UUID, in LivenessUpdateInput) (*LivenessDefinitionRecord, error) {
	u := r.client.StageLivenessDefinition.UpdateOneID(id)

	if in.ExpectedDuration != nil {
		u.SetExpectedDurationMs(int64(*in.ExpectedDuration / time.Millisecond))
	}
	if in.StalenessMargin != nil {
		u.SetStalenessMarginMs(int64(*in.StalenessMargin / time.Millisecond))
	}
	if in.MaxNoProgressDuration != nil {
		u.SetMaxNoProgressDurationMs(int64(*in.MaxNoProgressDuration / time.Millisecond))
	}
	if in.CycleThreshold != nil {
		//nolint:gosec // CycleThreshold is a small operator-configured count, never near int32 range.
		u.SetCycleThreshold(int32(*in.CycleThreshold))
	}
	if in.CycleLookback != nil {
		u.SetCycleLookbackMs(int64(*in.CycleLookback / time.Millisecond))
	}
	if in.Enabled != nil {
		u.SetEnabled(*in.Enabled)
	}

	row, err := u.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: liveness definition %s", ErrNotFound, id)
		}
		return nil, fmt.Errorf("update liveness definition %s: %w", id, err)
	}
	return recordFromEntLivenessDefinition(row), nil
}

// Delete removes a LivenessDefinition row by UUID.
func (r *EntLivenessRepository) Delete(ctx context.Context, id uuid.UUID) error {
	err := r.client.StageLivenessDefinition.DeleteOneID(id).Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("%w: liveness definition %s", ErrNotFound, id)
		}
		return fmt.Errorf("delete liveness definition %s: %w", id, err)
	}
	return nil
}

// GetByStageAndMode performs an exact-match lookup only — see
// LivenessRepository's doc comment for why the (stage, mode) -> (stage, nil)
// fallback is deliberately NOT implemented here.
func (r *EntLivenessRepository) GetByStageAndMode(ctx context.Context, stageSlug string, mode PipelineMode) (*LivenessDefinitionRecord, error) {
	q := r.client.StageLivenessDefinition.Query().
		Where(stagelivenessdefinition.StageSlug(stageSlug))

	if mode == PipelineModeDefault {
		q = q.Where(stagelivenessdefinition.PipelineModeIsNil())
	} else {
		q = q.Where(stagelivenessdefinition.PipelineMode(string(mode)))
	}

	row, err := q.Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: liveness definition for stage %q mode %q", ErrNotFound, stageSlug, mode)
		}
		return nil, fmt.Errorf("get liveness definition for stage %q mode %q: %w", stageSlug, mode, err)
	}
	return recordFromEntLivenessDefinition(row), nil
}

// ListAll returns all LivenessDefinition rows (enabled and disabled), sorted
// ascending by created_at. A safety cap of 1000 is applied, mirroring
// EntPipelineModeRepository.ListAll — this table is expected to hold at most
// dozens of operator-authored rows.
func (r *EntLivenessRepository) ListAll(ctx context.Context) ([]*LivenessDefinitionRecord, error) {
	//nolint:entfullscan capped at Limit(1000) below; doc comment states this explicitly.
	rows, err := r.client.StageLivenessDefinition.Query().
		Order(ent.Asc(stagelivenessdefinition.FieldCreatedAt)).
		Limit(1000).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all liveness definitions: %w", err)
	}
	records := make([]*LivenessDefinitionRecord, len(rows))
	for i, row := range rows {
		records[i] = recordFromEntLivenessDefinition(row)
	}
	return records, nil
}

// recordFromEntLivenessDefinition converts a persisted ent row into the
// ent-free LivenessDefinitionRecord DTO — mirrors fromEntHandoffSummary
// (session/handoff_summary_service.go)'s pattern: server/services must not
// import session/ent directly (the no_ent_in_services lint rule), so every
// LivenessRepository method returns this plain struct instead of *ent.StageLivenessDefinition.
func recordFromEntLivenessDefinition(row *ent.StageLivenessDefinition) *LivenessDefinitionRecord {
	return &LivenessDefinitionRecord{
		ID:                      row.ID,
		StageSlug:               row.StageSlug,
		PipelineMode:            row.PipelineMode,
		Kind:                    row.Kind,
		ExpectedDurationMs:      row.ExpectedDurationMs,
		StalenessMarginMs:       row.StalenessMarginMs,
		MaxNoProgressDurationMs: row.MaxNoProgressDurationMs,
		CycleThreshold:          row.CycleThreshold,
		CycleLookbackMs:         row.CycleLookbackMs,
		Enabled:                 row.Enabled,
		CreatedAt:               row.CreatedAt,
		UpdatedAt:               row.UpdatedAt,
	}
}

// livenessDefinitionFromRecord converts a LivenessDefinitionRecord back into
// the domain LivenessDefinition value, reading only the nullable fields that
// apply to record.Kind (the others are expected to be nil, per the schema's
// single-table-inheritance-adjacent design — see
// session/ent/schema/stage_liveness_definition.go). Returns an error for an
// unrecognized Kind string (a row written by a future, unrecognized schema
// version) rather than silently returning a zero-value definition.
func livenessDefinitionFromRecord(record *LivenessDefinitionRecord) (LivenessDefinition, error) {
	switch LivenessKind(record.Kind) {
	case LivenessKindDurationBudget:
		var expected, margin time.Duration
		if record.ExpectedDurationMs != nil {
			expected = time.Duration(*record.ExpectedDurationMs) * time.Millisecond
		}
		if record.StalenessMarginMs != nil {
			margin = time.Duration(*record.StalenessMarginMs) * time.Millisecond
		}
		return *mustNewLivenessDefinition(LivenessKindDurationBudget,
			WithExpectedDuration(expected),
			WithStalenessMargin(margin),
		), nil
	case LivenessKindHeartbeat:
		var maxNoProgress time.Duration
		if record.MaxNoProgressDurationMs != nil {
			maxNoProgress = time.Duration(*record.MaxNoProgressDurationMs) * time.Millisecond
		}
		return *mustNewLivenessDefinition(LivenessKindHeartbeat,
			WithMaxNoProgressDuration(maxNoProgress),
		), nil
	case LivenessKindCycleFrequency:
		var threshold int
		var lookback time.Duration
		if record.CycleThreshold != nil {
			threshold = int(*record.CycleThreshold)
		}
		if record.CycleLookbackMs != nil {
			lookback = time.Duration(*record.CycleLookbackMs) * time.Millisecond
		}
		return *mustNewLivenessDefinition(LivenessKindCycleFrequency,
			WithCycleThreshold(threshold),
			WithCycleLookback(lookback),
		), nil
	default:
		return LivenessDefinition{}, fmt.Errorf("liveness definition record %s: unrecognized kind %q", record.ID, record.Kind)
	}
}

// mustNewLivenessDefinition wraps NewLivenessDefinition for use inside
// livenessDefinitionFromRecord, where the fields passed always belong to kind
// by construction (they're read straight from that kind's own nullable
// fields) — a validation failure here would mean the record's own Kind/field
// pairing is corrupt, which is a bug worth a hard failure at the call site
// (via the error return below, not a panic — livenessDefinitionFromRecord
// still returns its own error for an unrecognized Kind).
func mustNewLivenessDefinition(kind LivenessKind, opts ...LivenessDefinitionOption) *LivenessDefinition {
	def, err := NewLivenessDefinition(kind, opts...)
	if err != nil {
		// Unreachable in practice: opts always match kind's own shape here.
		panic(fmt.Sprintf("mustNewLivenessDefinition: %v", err))
	}
	return def
}
