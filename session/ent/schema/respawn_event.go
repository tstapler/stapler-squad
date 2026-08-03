package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// RespawnEvent is an append-only audit log of automated respawn/remediation
// attempts against a backlog item (AutoRespawnReview, AutoRespawnAutonomousWork,
// AutoRespawnTriage, RemediateStaleWorkSession).
type RespawnEvent struct {
	ent.Schema
}

func (RespawnEvent) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New),
		field.UUID("item_id", uuid.UUID{}),
		field.String("reason"),
		// Plain string references, not ent edges — mirrors ItemSession.session_uuid's
		// existing "loose FK, not an edge" convention, and must tolerate a resulting
		// session that never gets created (queued or failed spawn attempt).
		field.String("triggering_session_uuid").
			Optional().
			Nillable(),
		field.String("resulting_session_uuid").
			Optional().
			Nillable(),
		field.Bool("queued").
			Default(false).
			Comment("True when the spawn attempt hit the concurrency cap and was queued instead of spawning a session; distinguishes 'queued' from 'spawn attempt failed' when resulting_session_uuid is empty."),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}

func (RespawnEvent) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("item", BacklogItem.Type).
			Ref("respawn_events").
			Field("item_id").
			Unique().
			Required(),
	}
}

func (RespawnEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("item_id", "created_at"),
	}
}
