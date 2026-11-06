package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// BacklogStuckEpisode is an append-only log of stuck episodes. Unlike
// BacklogStuckState (resolve-in-place, one row per (item_id, reason)), a
// recurrence inserts a new row, so stuck history survives reopen. See ADR-003.
//
// item_id is deliberately a plain UUID, not an edge: episodes are history and
// must not cascade away or block with item lifecycle.
type BacklogStuckEpisode struct {
	ent.Schema
}

// Fields of the BacklogStuckEpisode.
func (BacklogStuckEpisode) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New),
		field.UUID("item_id", uuid.UUID{}),
		field.String("reason").
			Comment("Same vocabulary as BacklogStuckState.reason (domain.StuckReason)."),
		field.Time("opened_at").
			Default(func() time.Time { return time.Now().UTC() }).
			Comment("UTC instant the episode opened; equals the first_detected_at MarkStuck wrote in the same tx."),
		field.Time("resolved_at").
			Optional().
			Nillable().
			Comment("When the episode closed. NULL = currently open."),
	}
}

// Indexes of the BacklogStuckEpisode.
func (BacklogStuckEpisode) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("item_id", "reason", "opened_at"),
		// At most one open episode per pair is a database fact: concurrent
		// MarkStuck calls cannot insert duplicate open episodes.
		index.Fields("item_id", "reason").
			Unique().
			Annotations(entsql.IndexWhere("resolved_at IS NULL")),
	}
}
