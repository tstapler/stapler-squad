package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// SourceSyncEvent holds the schema definition for the SourceSyncEvent entity.
type SourceSyncEvent struct {
	ent.Schema
}

// Fields of the SourceSyncEvent.
func (SourceSyncEvent) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New),
		field.Time("started_at").
			Default(time.Now).
			Immutable(),
		field.Time("finished_at").
			Optional().
			Nillable(),
		field.Int("items_created").
			Default(0),
		field.Int("items_updated").
			Default(0),
		field.Int("items_skipped").
			Default(0),
		field.Int("items_errored").
			Default(0),
		field.String("error_message").
			Optional(),
		field.String("cursor_after").
			Optional(),
	}
}

// Edges of the SourceSyncEvent.
func (SourceSyncEvent) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("source", ItemSource.Type).
			Ref("sync_events").
			Unique().
			Required(),
	}
}

// Indexes of the SourceSyncEvent.
func (SourceSyncEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Edges("source"),
	}
}
