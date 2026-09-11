package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// GateSatisfactionRecord holds the schema definition for the
// GateSatisfactionRecord entity: a persisted one-shot gate outcome (e.g. a
// recorded human approval, or a terminal review verdict) keyed by
// (item_id, gate_id). Distinguishes stateful/one-shot gates from
// stateless/re-checkable ones — see TransitionGate.stateful.
//
// item_id is a plain UUID column, not an ent edge to BacklogItem: this Epic's
// scope is limited to the four new schema files (Epic 2.2), so BacklogItem's
// own edge list is left untouched, matching this project's precedent of
// leaving BacklogStuckState.reason as a plain unvalidated column rather than
// widening an existing entity's schema for a new consumer.
type GateSatisfactionRecord struct{ ent.Schema }

// Fields of the GateSatisfactionRecord.
func (GateSatisfactionRecord) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.UUID("item_id", uuid.UUID{}),
		field.UUID("gate_id", uuid.UUID{}),
		field.Bool("satisfied").Default(false),
		field.String("satisfied_by").Optional().
			Comment("Actor identity that satisfied the gate, e.g. a username or session ID."),
		field.Time("satisfied_at").Optional().Nillable(),
		field.JSON("outcome_detail", map[string]interface{}{}).
			Optional().
			Comment("Nullable structured detail, e.g. a ReviewOutcome payload for a review gate."),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").
			Default(func() time.Time { return time.Now().UTC() }).
			UpdateDefault(func() time.Time { return time.Now().UTC() }),
	}
}

// Edges of the GateSatisfactionRecord.
func (GateSatisfactionRecord) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("gate", TransitionGate.Type).
			Ref("satisfaction_records").
			Field("gate_id").
			Unique().
			Required(),
	}
}

// Indexes of the GateSatisfactionRecord.
func (GateSatisfactionRecord) Indexes() []ent.Index {
	return []ent.Index{
		// The uniqueness constraint this whole story exists to enforce: a
		// duplicate (item_id, gate_id) Create must fail.
		index.Fields("item_id", "gate_id").Unique(),
		index.Fields("item_id"),
		index.Fields("gate_id"),
	}
}
