package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// Workflow holds the schema definition for the Workflow entity.
type Workflow struct{ ent.Schema }

// Fields of the Workflow.
func (Workflow) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.String("slug").Unique().NotEmpty(),
		field.String("name").NotEmpty(),
		field.String("description").Optional(),
		field.String("command").NotEmpty(),
		field.String("target_directory").Optional(),
		field.String("input_template").Optional(),
		field.String("session_type").Optional().Default("directory"),
		field.String("model").Optional(),
		field.String("agent_type").Optional(),
		field.String("cron_expression").Optional(),
		field.Bool("cron_enabled").Default(false),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

// Edges of the Workflow.
func (Workflow) Edges() []ent.Edge { return nil }

// Indexes of the Workflow.
func (Workflow) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("slug"),
		index.Fields("cron_enabled"),
		index.Fields("created_at"),
	}
}
