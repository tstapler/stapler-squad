package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// BacklogItem holds the schema definition for the BacklogItem entity.
type BacklogItem struct {
	ent.Schema
}

// Fields of the BacklogItem.
func (BacklogItem) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New),
		field.String("title").
			NotEmpty(),
		field.String("description").
			Optional(),
		field.String("acceptance_criteria").
			Optional().
			Comment("JSON []AcCriterion"),
		field.Int("priority").
			Default(3).
			Min(1).
			Max(5),
		field.String("status").
			Default("idea"),
		field.String("repo_path").
			Optional(),
		field.Bool("skip_review_gate").
			Default(false),
		field.Bool("skip_planning").
			Default(false),
		field.Bool("plan_approved").
			Default(false),
		field.Time("plan_approved_at").
			Optional().
			Nillable(),
		field.String("plan_artifacts_path").
			Optional(),
		field.String("user_modified_fields").
			Optional().
			Comment("JSON set of field names modified by the user"),
		field.String("notes").
			Optional(),
		field.String("external_id").
			Optional(),
		field.Time("user_modified_status_at").
			Optional().
			Nillable(),
		field.Time("archived_at").
			Optional().
			Nillable(),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the BacklogItem.
func (BacklogItem) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("item_sessions", ItemSession.Type),
		edge.To("sessions", Session.Type),
		edge.From("source", ItemSource.Type).
			Ref("backlog_items").
			Unique(),
	}
}

// Indexes of the BacklogItem.
func (BacklogItem) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status", "priority"),
		index.Fields("status", "updated_at"),
		index.Fields("external_id"),
		index.Fields("status"),
	}
}
