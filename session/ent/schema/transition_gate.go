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

// TransitionGate holds the schema definition for the TransitionGate entity:
// persisted config for one gate attached to a StageTransition (Epic 2.4 wires
// gate evaluation; this Epic only persists the shape). kind discriminates the
// gate's evaluation logic (e.g. "human_approval", "automated_review"); config
// carries kind-specific fields.
type TransitionGate struct{ ent.Schema }

// Fields of the TransitionGate.
func (TransitionGate) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.UUID("transition_id", uuid.UUID{}),
		field.String("kind").NotEmpty(),
		field.JSON("config", map[string]interface{}{}).
			Optional().
			Default(map[string]interface{}{}).
			Comment("Kind-specific fields, e.g. {\"skill\": \"...\"} for an automated-review gate."),
		field.Bool("stateful").Default(false).
			Comment("True for a one-shot gate whose outcome is recorded in a GateSatisfactionRecord and never re-evaluated; false for a stateless, re-checkable gate."),
		field.Int("order_index").Default(0).
			Comment("Evaluation order among gates on the same transition."),
		field.Bool("enabled").Default(true),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").
			Default(func() time.Time { return time.Now().UTC() }).
			UpdateDefault(func() time.Time { return time.Now().UTC() }),
	}
}

// Edges of the TransitionGate.
func (TransitionGate) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("transition", StageTransition.Type).
			Ref("gates").
			Field("transition_id").
			Unique().
			Required(),
		edge.To("satisfaction_records", GateSatisfactionRecord.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

// Indexes of the TransitionGate.
func (TransitionGate) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("transition_id"),
		index.Fields("kind"),
		index.Fields("enabled"),
	}
}
