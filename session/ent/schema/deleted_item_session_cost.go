package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// DeletedItemSessionCost holds the schema definition for the
// DeletedItemSessionCost entity — a durable, additive ledger row written just
// before DeleteBacklogItem hard-deletes an item's ItemSession rows, so the
// item's cost stays attributable to Insights after the item itself is gone.
// No FK/edge to anything: by the time a row exists, both the BacklogItem and
// the ItemSession it snapshots are already deleted, so item_id and
// item_title are plain string snapshots, not ent edges.
type DeletedItemSessionCost struct {
	ent.Schema
}

// Fields of the DeletedItemSessionCost.
func (DeletedItemSessionCost) Fields() []ent.Field {
	return []ent.Field{
		field.String("conversation_uuid").
			Optional().
			Default("").
			Comment("Claude conversation UUID (transcript JSONL name), snapshotted from the deleted ItemSession row. Insights folds ledger rows in keyed by this, not session_uuid — the session row was already gone before the item was even deleted."),
		field.String("session_uuid").
			Comment("Snapshot of the deleted ItemSession's loose FK to Session; not an ent edge"),
		field.String("session_role").
			Comment("Snapshot of the deleted ItemSession's session_role: one of work, triage, review"),
		field.String("item_id").
			Comment("String snapshot of the deleted BacklogItem's ID — not a real FK, the row is gone"),
		field.String("item_title").
			Comment("String snapshot of the deleted BacklogItem's title at delete time"),
		field.Float("estimated_cost_usd").
			Default(0).
			Comment("Snapshot of the deleted ItemSession's estimated_cost_usd"),
		field.Bool("cost_priced").
			Default(true).
			Comment("Snapshot of the deleted ItemSession's cost_priced"),
		field.Time("created_at").
			Comment("The original ItemSession's created_at (not this row's own creation time) — Insights' time-range filter needs the work's original timestamp, not the deletion timestamp"),
		field.Time("deleted_at").
			Default(time.Now).
			Immutable(),
	}
}

// Edges of the DeletedItemSessionCost.
func (DeletedItemSessionCost) Edges() []ent.Edge {
	return nil
}

// Indexes of the DeletedItemSessionCost.
func (DeletedItemSessionCost) Indexes() []ent.Index {
	return []ent.Index{
		// Mirrors item_sessions' own conversation_uuid index — same lookup shape.
		index.Fields("conversation_uuid"),
	}
}
