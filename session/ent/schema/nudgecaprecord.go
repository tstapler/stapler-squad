package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// NudgeCapRecord holds the schema definition for the NudgeCapRecord entity:
// ADR-003's durable per-backlog-item nudge cap/cooldown state
// (project_plans/backlog-diagnose-and-nudge/decisions/ADR-003-nudge-cap-cooldown-shape.md),
// keyed by item_id. Mirrors DismissedFinding's convention of a bare
// .Unique() string key column with no separate ID/edge, rather than
// GateSatisfactionRecord's UUID-id-plus-edge shape -- this entity has no
// natural edge target of its own (item_id is not a foreign key to
// BacklogItem, same precedent as GateSatisfactionRecord.item_id).
type NudgeCapRecord struct {
	ent.Schema
}

// Fields of the NudgeCapRecord.
func (NudgeCapRecord) Fields() []ent.Field {
	return []ent.Field{
		field.String("item_id").
			Unique().
			NotEmpty().
			Comment("Backlog item ID this nudge cap/cooldown state tracks."),
		field.Int("nudge_count").
			Default(0).
			NonNegative().
			Comment("Number of nudges reserved for this item within the current window."),
		field.Time("window_start_at").
			Optional().
			Comment("When the current nudge-count window started; unset until the first reservation."),
		field.Time("last_nudge_at").
			Optional().
			Nillable().
			Comment("Timestamp of the most recently reserved nudge, read by the cooldown check."),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(func() time.Time { return time.Now().UTC() }).
			UpdateDefault(func() time.Time { return time.Now().UTC() }),
	}
}

// Edges of the NudgeCapRecord.
func (NudgeCapRecord) Edges() []ent.Edge {
	return nil
}

// Indexes of the NudgeCapRecord.
func (NudgeCapRecord) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("item_id"),
	}
}
