package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// ReviewVerdict holds the schema definition for the ReviewVerdict entity.
type ReviewVerdict struct {
	ent.Schema
}

// Fields of the ReviewVerdict.
func (ReviewVerdict) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New),
		field.String("overall_outcome").
			Comment("One of: PASS, FAIL, PARTIAL, UNVERIFIABLE"),
		field.String("per_criterion").
			Optional().
			Comment("JSON []CriterionVerdict"),
		field.String("summary").
			Optional(),
		field.String("diff_hash").
			Optional(),
		field.String("prompt_hash").
			Optional(),
		field.Int("diff_token_count").
			Optional(),
		field.Bool("diff_truncated").
			Default(false),
		field.String("override_by").
			Optional(),
		field.String("override_reason").
			Optional(),
		field.Time("override_at").
			Optional().
			Nillable(),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}

// Edges of the ReviewVerdict.
func (ReviewVerdict) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("item_session", ItemSession.Type).
			Ref("review_verdict").
			Unique().
			Required(),
	}
}

// Indexes of the ReviewVerdict.
func (ReviewVerdict) Indexes() []ent.Index {
	return []ent.Index{
		index.Edges("item_session").Unique(),
	}
}
