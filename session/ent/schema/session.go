package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Session holds the schema definition for the Session entity.
type Session struct {
	ent.Schema
}

// Fields of the Session.
func (Session) Fields() []ent.Field {
	return []ent.Field{
		field.String("title").
			Unique().
			NotEmpty(),
		field.String("path").
			NotEmpty(),
		field.String("working_dir").
			Optional(),
		field.String("branch").
			Optional(),
		field.Int("status").
			Comment("Session status: Running, Paused, etc."),
		field.Int("height").
			Optional(),
		field.Int("width").
			Optional(),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
		field.Bool("auto_yes").
			Default(false),
		field.String("prompt").
			Optional(),
		field.String("program").
			NotEmpty(),
		field.String("existing_worktree").
			Optional(),
		field.String("category").
			Optional(),
		field.Bool("is_expanded").
			Default(true),
		field.String("session_type").
			Optional(),
		field.String("tmux_prefix").
			Optional(),
		field.Time("last_terminal_update").
			Optional().
			Nillable(),
		field.Time("last_meaningful_output").
			Optional().
			Nillable(),
		field.String("last_output_signature").
			Optional(),
		field.Time("last_added_to_queue").
			Optional().
			Nillable(),
		field.Time("last_viewed").
			Optional().
			Nillable(),
		field.Time("last_acknowledged").
			Optional().
			Nillable(),
	}
}

// Edges of the Session.
func (Session) Edges() []ent.Edge {
	return []ent.Edge{
		// One-to-one relationship with Worktree
		edge.To("worktree", Worktree.Type).
			Unique(),

		// One-to-one relationship with DiffStats
		edge.To("diff_stats", DiffStats.Type).
			Unique(),

		// Many-to-many relationship with Tags
		edge.To("tags", Tag.Type),

		// One-to-one relationship with ClaudeSession
		edge.To("claude_session", ClaudeSession.Type).
			Unique(),
	}
}

// Indexes of the Session.
func (Session) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("title"),
		index.Fields("status"),
		index.Fields("category"),
		index.Fields("last_meaningful_output"),
		index.Fields("last_acknowledged"),
		index.Fields("created_at"),
	}
}
