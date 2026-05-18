package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// ItemSource holds the schema definition for the ItemSource entity.
type ItemSource struct {
	ent.Schema
}

// Fields of the ItemSource.
func (ItemSource) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New),
		field.String("plugin_id").
			Comment("e.g. github_issues"),
		field.String("display_name"),
		field.String("config").
			Optional().
			Comment("JSON with encrypted PAT"),
		field.Bool("enabled").
			Default(true),
		field.String("sync_cursor").
			Optional(),
		field.Time("last_synced_at").
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

// Edges of the ItemSource.
func (ItemSource) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("backlog_items", BacklogItem.Type),
		edge.To("sync_events", SourceSyncEvent.Type),
	}
}

// Indexes of the ItemSource.
func (ItemSource) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("plugin_id"),
		index.Fields("enabled"),
	}
}
