package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// AnalyticsEvent holds the schema definition for the AnalyticsEvent entity.
type AnalyticsEvent struct{ ent.Schema }

// Fields of the AnalyticsEvent.
func (AnalyticsEvent) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			NotEmpty().
			Immutable(),
		field.String("event_name").
			NotEmpty(),
		field.String("event_category").
			NotEmpty(),
		field.String("session_id").
			Optional(),
		field.Int64("duration_ms").
			Optional().
			Nillable(),
		field.String("page").
			Optional(),
		field.String("component").
			Optional(),
		field.JSON("labels", map[string]string{}).
			Optional(),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}

// Indexes of the AnalyticsEvent.
func (AnalyticsEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("event_name"),
		index.Fields("event_category"),
		index.Fields("session_id"),
		index.Fields("created_at"),
		index.Fields("event_name", "created_at"),
	}
}
