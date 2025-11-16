package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// DiffStats holds the schema definition for the DiffStats entity.
type DiffStats struct {
	ent.Schema
}

// Fields of the DiffStats.
func (DiffStats) Fields() []ent.Field {
	return []ent.Field{
		field.Int("added").
			Default(0),
		field.Int("removed").
			Default(0),
		field.Text("content").
			Optional(),
	}
}

// Edges of the DiffStats.
func (DiffStats) Edges() []ent.Edge {
	return []ent.Edge{
		// Back-reference to Session (one-to-one)
		edge.From("session", Session.Type).
			Ref("diff_stats").
			Unique().
			Required(),
	}
}
