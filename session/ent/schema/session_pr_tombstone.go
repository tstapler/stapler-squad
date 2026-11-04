package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SessionPRTombstone records that one owner (a session or a backlog item)
// detached a PR, so backfill, discovery and the terminal scan never recreate
// that owner's link. The PR identity is stored in canonical PRRef form.
type SessionPRTombstone struct {
	ent.Schema
}

// Annotations of the SessionPRTombstone.
func (SessionPRTombstone) Annotations() []schema.Annotation {
	return []schema.Annotation{exactlyOneOwnerCheck}
}

// Fields of the SessionPRTombstone.
func (SessionPRTombstone) Fields() []ent.Field {
	return []ent.Field{
		field.String("session_uuid").Default("").Immutable(),
		field.String("backlog_item_id").Default("").Immutable(),
		field.String("host").NotEmpty().Validate(canonicalLower("host")),
		field.String("owner").NotEmpty().Validate(canonicalLower("owner")),
		field.String("repo").NotEmpty().Validate(canonicalLower("repo")),
		field.Int("number").Positive(),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

// Indexes of the SessionPRTombstone.
func (SessionPRTombstone) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("session_uuid", "backlog_item_id", "host", "owner", "repo", "number").Unique(),
		index.Fields("session_uuid"),
		index.Fields("backlog_item_id"),
	}
}
