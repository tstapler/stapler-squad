package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// BacklogStatusEvent is an append-only log of backlog item status transitions.
type BacklogStatusEvent struct {
	ent.Schema
}

func (BacklogStatusEvent) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New),
		field.UUID("item_id", uuid.UUID{}),
		field.String("from_status"),
		field.String("to_status"),
		field.String("triggered_by").
			Default("user"),
		field.String("note").
			Optional().
			Nillable().
			Comment("Human-readable reason stored alongside the transition, e.g. 'auto-reopened after FAIL verdict'."),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}

func (BacklogStatusEvent) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("item", BacklogItem.Type).
			Ref("status_events").
			Field("item_id").
			Unique().
			Required(),
	}
}

func (BacklogStatusEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("item_id", "created_at"),
	}
}
