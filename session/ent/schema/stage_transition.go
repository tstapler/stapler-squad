package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// StageTransition holds the schema definition for the StageTransition entity:
// one legal (from_stage, to_stage) edge in the workflow graph. Story 2.2.2
// seeds one row per entry in domain.ValidTransitions() for the 9 built-in
// stages.
type StageTransition struct{ ent.Schema }

// Fields of the StageTransition.
func (StageTransition) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.UUID("from_stage_id", uuid.UUID{}),
		field.UUID("to_stage_id", uuid.UUID{}),
		field.Bool("enabled").Default(true),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").
			Default(func() time.Time { return time.Now().UTC() }).
			UpdateDefault(func() time.Time { return time.Now().UTC() }),
	}
}

// Edges of the StageTransition.
func (StageTransition) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("from_stage", BacklogStage.Type).
			Ref("outgoing_transitions").
			Field("from_stage_id").
			Unique().
			Required(),
		edge.From("to_stage", BacklogStage.Type).
			Ref("incoming_transitions").
			Field("to_stage_id").
			Unique().
			Required(),
		edge.To("gates", TransitionGate.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

// Indexes of the StageTransition.
func (StageTransition) Indexes() []ent.Index {
	return []ent.Index{
		// The uniqueness constraint this whole story exists to enforce: a
		// duplicate (from_stage_id, to_stage_id) Create must fail.
		index.Fields("from_stage_id", "to_stage_id").Unique(),
		index.Fields("from_stage_id"),
		index.Fields("to_stage_id"),
		index.Fields("enabled"),
	}
}
