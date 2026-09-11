package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// StageLivenessDefinition holds the schema definition for the
// StageLivenessDefinition entity: the persisted form of session.LivenessDefinition
// (Epic 1.1, backlog-custom-workflow-stages), keyed by (stage_slug, pipeline_mode).
// One table with kind-specific nullable columns (Pattern Decisions: "single-table
// inheritance-adjacent" — over-normalization into one table per LivenessKind isn't
// worth it for a table expected to hold dozens of operator-authored rows total).
type StageLivenessDefinition struct{ ent.Schema }

// Fields of the StageLivenessDefinition.
func (StageLivenessDefinition) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.String("stage_slug").NotEmpty(),
		// pipeline_mode is nullable: NULL means "applies to all pipeline modes."
		// GetByStageAndMode is a dumb exact-match query only — the (stage, mode) ->
		// (stage, nil) mode-less fallback is owned exclusively by livenessCache.Get
		// (Story 1.3.1), not re-implemented at this layer.
		field.String("pipeline_mode").Optional().Nillable(),
		// kind mirrors session.LivenessKind's 3 string values.
		field.String("kind").NotEmpty(),
		// Shape A (LivenessKindDurationBudget) columns.
		field.Int64("expected_duration_ms").Optional().Nillable(),
		field.Int64("staleness_margin_ms").Optional().Nillable(),
		// Shape B (LivenessKindHeartbeat) column.
		field.Int64("max_no_progress_duration_ms").Optional().Nillable(),
		// Shape C (LivenessKindCycleFrequency) columns.
		field.Int32("cycle_threshold").Optional().Nillable(),
		field.Int64("cycle_lookback_ms").Optional().Nillable(),
		field.Bool("enabled").Default(true),
		field.Time("created_at").Default(time.Now).Immutable(),
		// updated_at is stored/compared in UTC, not time.Now's default Local zone —
		// see session/ent/schema/workflow.go's identical field for the full
		// mechanism (mattn/go-sqlite3 formats a time.Time TEXT column in the
		// value's own Location, so a Local-zoned stored value would never
		// byte-match a UTC-zoned CAS precondition value).
		field.Time("updated_at").
			Default(func() time.Time { return time.Now().UTC() }).
			UpdateDefault(func() time.Time { return time.Now().UTC() }),
	}
}

// Edges of the StageLivenessDefinition.
func (StageLivenessDefinition) Edges() []ent.Edge { return nil }

// Indexes of the StageLivenessDefinition.
func (StageLivenessDefinition) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("stage_slug", "pipeline_mode").Unique(),
		index.Fields("stage_slug"),
		index.Fields("enabled"),
	}
}
