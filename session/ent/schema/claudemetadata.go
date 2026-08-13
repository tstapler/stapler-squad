package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// ClaudeMetadata holds the schema definition for the ClaudeMetadata entity.
type ClaudeMetadata struct {
	ent.Schema
}

// Fields of the ClaudeMetadata.
func (ClaudeMetadata) Fields() []ent.Field {
	return []ent.Field{
		field.String("key").
			NotEmpty(),
		field.String("value").
			NotEmpty(),
	}
}

// Edges of the ClaudeMetadata.
func (ClaudeMetadata) Edges() []ent.Edge {
	return []ent.Edge{
		// Back-reference to ClaudeSession (many-to-one)
		edge.From("claude_session", ClaudeSession.Type).
			Ref("metadata").
			Unique().
			Required(),
	}
}
