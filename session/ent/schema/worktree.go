package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Worktree holds the schema definition for the Worktree entity.
type Worktree struct {
	ent.Schema
}

// Fields of the Worktree.
func (Worktree) Fields() []ent.Field {
	return []ent.Field{
		field.String("repo_path").
			NotEmpty(),
		field.String("worktree_path").
			NotEmpty(),
		field.String("session_name").
			NotEmpty(),
		field.String("branch_name").
			NotEmpty(),
		field.String("base_commit_sha").
			NotEmpty(),
	}
}

// Edges of the Worktree.
func (Worktree) Edges() []ent.Edge {
	return []ent.Edge{
		// Back-reference to Session (one-to-one)
		edge.From("session", Session.Type).
			Ref("worktree").
			Unique().
			Required(),
	}
}
