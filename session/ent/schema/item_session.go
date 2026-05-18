package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// ItemSession holds the schema definition for the ItemSession entity.
type ItemSession struct {
	ent.Schema
}

// Fields of the ItemSession.
func (ItemSession) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New),
		field.String("session_uuid").
			Comment("Loose FK to Session; not an ent edge"),
		field.String("session_role").
			Comment("One of: work, triage, review"),
		field.Time("started_at").
			Optional().
			Nillable(),
		field.Time("ended_at").
			Optional().
			Nillable(),
		field.String("ac_snapshot").
			Optional().
			Comment("JSON []AcCriterion at spawn time"),
		field.String("triage_result").
			Optional().
			Comment("JSON triage suggestions"),
		field.String("last_commit_sha").
			Optional(),
		field.Time("last_commit_at").
			Optional().
			Nillable(),
		field.String("last_commit_message").
			Optional(),
		field.Int("commit_count_since_spawn").
			Default(0),
		field.Time("last_file_touch_at").
			Optional().
			Nillable(),
		field.Time("last_progress_at").
			Optional().
			Nillable(),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}

// Edges of the ItemSession.
func (ItemSession) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("backlog_item", BacklogItem.Type).
			Ref("item_sessions").
			Unique().
			Required(),
		edge.To("review_verdict", ReviewVerdict.Type).
			Unique(),
	}
}

// Indexes of the ItemSession.
func (ItemSession) Indexes() []ent.Index {
	return []ent.Index{
		// CRITICAL: O(1) lookup on every EventExited hook
		index.Fields("session_uuid"),
	}
}
