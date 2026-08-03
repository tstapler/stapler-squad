package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SessionSummary holds the schema definition for the SessionSummary entity — an
// automatically-generated, durably-persisted completion summary for a session.
// Deliberately has no Edges() method: session_id is a plain (non-edge) unique
// string field, not an edge.From(...).Unique().Required() back to Session, so
// this row survives Session row deletion by construction (ADR-001/ADR-002 — see
// implementation/plan.md's Pattern Decisions "ent persistence shape" row).
type SessionSummary struct{ ent.Schema }

// Fields of the SessionSummary.
func (SessionSummary) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			NotEmpty().
			Immutable(),
		field.String("session_id").
			NotEmpty().
			Unique(),
		field.String("session_title").
			Optional(),
		field.String("status").
			Default("pending"),
		field.Text("narrative").
			Optional(),
		field.Bool("narrative_fallback_used").
			Default(false),
		field.Int("diff_files_changed").
			Default(0),
		field.Int("diff_added").
			Default(0),
		field.Int("diff_removed").
			Default(0),
		field.Int("decisions_auto_approved").
			Default(0),
		field.Int("decisions_manually_approved").
			Default(0),
		field.Int("decisions_denied").
			Default(0),
		field.Int("decisions_review_queue_resolved").
			Default(0),
		field.Int("decisions_still_open").
			Default(0),
		field.Time("session_started_at").
			Optional().
			Nillable(),
		field.Time("session_stopped_at").
			Optional().
			Nillable(),
		field.Int64("duration_ms").
			Optional().
			Nillable(),
		field.Int64("total_tokens").
			Optional().
			Nillable(),
		field.Float("estimated_cost_usd").
			Optional().
			Nillable(),
		field.Bool("cost_data_unavailable").
			Default(false),
		field.Text("markdown").
			Optional(),
		field.String("error_message").
			Optional(),
		field.String("error_stage").
			Optional(),
		field.Time("generation_started_at").
			Optional().
			Nillable(),
		field.Time("generated_at").
			Optional().
			Nillable(),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Indexes of the SessionSummary.
func (SessionSummary) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("session_id"),
		index.Fields("status"),
	}
}
