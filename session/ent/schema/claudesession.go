package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// ClaudeSession holds the schema definition for the ClaudeSession entity.
type ClaudeSession struct {
	ent.Schema
}

// Fields of the ClaudeSession.
func (ClaudeSession) Fields() []ent.Field {
	return []ent.Field{
		field.String("claude_session_id").
			NotEmpty(),
		field.String("conversation_id").
			Optional(),
		field.String("project_name").
			Optional(),
		field.Time("last_attached").
			Optional().
			Nillable(),
		// Settings fields
		field.Bool("auto_reattach").
			Default(false),
		field.String("preferred_session_name").
			Optional(),
		field.Bool("create_new_on_missing").
			Default(false),
		field.Bool("show_session_selector").
			Default(false),
		field.Int("session_timeout_minutes").
			Default(30),
	}
}

// Edges of the ClaudeSession.
func (ClaudeSession) Edges() []ent.Edge {
	return []ent.Edge{
		// Back-reference to Session (one-to-one)
		edge.From("session", Session.Type).
			Ref("claude_session").
			Unique().
			Required(),

		// One-to-many relationship with ClaudeMetadata
		edge.To("metadata", ClaudeMetadata.Type),
	}
}
