package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// SessionGoal holds the schema definition for the SessionGoal entity.
// Stores the current goal, status, and task tree for a session (1:1 per session).
type SessionGoal struct{ ent.Schema }

// Fields of the SessionGoal.
func (SessionGoal) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.String("session_uuid").NotEmpty(),
		field.String("goal").MaxLen(2000).NotEmpty(),
		field.String("status").Default("idle"),
		field.String("tasks").Optional().Comment("JSON []TaskNode"),
		field.String("set_by").Optional(),
		field.String("workspace_key").Optional().
			Comment("Canonical repo/workspace identity (gh:owner/repo or path:<main repo path>); used to group peer sessions"),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

// Edges of the SessionGoal.
func (SessionGoal) Edges() []ent.Edge { return nil }

// Indexes of the SessionGoal.
func (SessionGoal) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("session_uuid").Unique(), // 1:1 per session
		index.Fields("status"),
		index.Fields("workspace_key"),
	}
}
