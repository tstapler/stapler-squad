package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// BacklogItemDependency is an explicit join row recording that blocked_id
// (the dependent) may not be dequeued/started until blocker_id (the
// blocker) reaches a resolved status (done). See domain.TransitionGuard for
// the resolved-status check and cycle-detection at write time in
// AddBacklogItemDependency.
type BacklogItemDependency struct {
	ent.Schema
}

// Fields of the BacklogItemDependency.
func (BacklogItemDependency) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New),
		field.UUID("blocker_id", uuid.UUID{}).
			Comment("The item that must reach a resolved status before blocked_id is eligible for dequeue/start."),
		field.UUID("blocked_id", uuid.UUID{}).
			Comment("The dependent item, gated until blocker_id resolves."),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}

// Edges of the BacklogItemDependency.
func (BacklogItemDependency) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("blocker", BacklogItem.Type).
			Ref("blocking_dependencies").
			Field("blocker_id").
			Unique().
			Required(),
		edge.From("blocked", BacklogItem.Type).
			Ref("blocked_by_dependencies").
			Field("blocked_id").
			Unique().
			Required(),
	}
}

// Indexes of the BacklogItemDependency.
func (BacklogItemDependency) Indexes() []ent.Index {
	return []ent.Index{
		// Plain 2-column unique index — the upsert target that makes adding
		// the same dependency twice a no-op instead of a duplicate row.
		index.Fields("blocker_id", "blocked_id").Unique(),
		// Supports the batched "which of these candidate items have an
		// unresolved blocker" query in DequeueNextQueuedItems, keyed by
		// blocked_id (find my blockers) without a per-candidate query.
		index.Fields("blocked_id"),
	}
}
