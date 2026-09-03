package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// HandoffSummary holds the schema definition for the HandoffSummary entity — an
// automatically-generated, durably-persisted handoff summary for a session.
// Deliberately has no Edges() method: session_id is a plain (non-edge) unique
// string field, not an edge.From(...).Unique().Required() back to Session, so
// this row survives Session row deletion by construction (ADR-001/ADR-002 — see
// implementation/plan.md's Pattern Decisions "ent persistence shape" row).
type HandoffSummary struct{ ent.Schema }

// Fields of the HandoffSummary.
func (HandoffSummary) Fields() []ent.Field {
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
		field.Text("active_task").
			Optional(),
		field.Text("summary_text").
			Optional(),
		field.Int("middle_messages_summarized").
			Default(0),
		field.Time("generation_started_at").
			Optional().
			Nillable(),
		field.Time("generated_at").
			Optional().
			Nillable(),
		field.String("error_stage").
			Optional(),
		field.Text("error_message").
			Optional(),
	}
}

// Indexes of the HandoffSummary. session_id does not need its own
// index.Fields("session_id") entry — its Unique() field definition above
// already makes ent generate a unique index over that column; an explicit
// entry here would be a second, redundant, non-unique index over the same
// column.
func (HandoffSummary) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
	}
}
