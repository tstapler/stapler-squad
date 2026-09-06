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

// BacklogStage holds the schema definition for the BacklogStage entity: the
// persisted form of a workflow stage (built-in or operator-defined), keyed by
// slug. Story 2.2.2 seeds the 9 built-in session.BacklogStatus values as rows
// here at boot, so ConfiguredWorkflowEngine (Epic 2.3,
// project_plans/backlog-custom-workflow-stages/implementation/plan.md) has
// real rows to load from day one instead of a hardcoded fallback.
type BacklogStage struct{ ent.Schema }

// Fields of the BacklogStage.
func (BacklogStage) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.String("slug").Unique().NotEmpty().
			Comment("Stable identifier: one of the 9 built-in session.BacklogStatus values, or an operator-chosen slug for a custom stage."),
		field.String("name").NotEmpty(),
		field.String("description").Optional(),
		field.Bool("is_entry").Default(false).
			Comment("True for a stage a new item may be created directly into (e.g. idea)."),
		field.Bool("is_terminal").Default(false).
			Comment("True for a stage that ends an item's lifecycle (e.g. archived) — see Epic 2.1's IsTerminalStatus-equivalent check."),
		field.Bool("enabled").Default(true),
		field.Time("created_at").Default(time.Now).Immutable(),
		// updated_at is stored/compared in UTC, not time.Now's default Local
		// zone — see session/ent/schema/stage_liveness_definition.go's
		// identical field for the full mechanism.
		field.Time("updated_at").
			Default(func() time.Time { return time.Now().UTC() }).
			UpdateDefault(func() time.Time { return time.Now().UTC() }),
	}
}

// Edges of the BacklogStage.
func (BacklogStage) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("outgoing_transitions", StageTransition.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("incoming_transitions", StageTransition.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

// Indexes of the BacklogStage.
func (BacklogStage) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("slug"),
		index.Fields("enabled"),
	}
}
