package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// CompactionEvent is one conversation compaction (/compact or auto-compact)
// detected from a transcript's compact_boundary entry. Claude Code runs the
// command itself, so the transcript is the only place it can be observed.
// Keyed by (session_uuid, turn_index): the boundary position is stable across
// re-parses.
type CompactionEvent struct{ ent.Schema }

// Fields of the CompactionEvent.
func (CompactionEvent) Fields() []ent.Field {
	return []ent.Field{
		field.String("session_uuid").NotEmpty().Immutable(),
		field.Int("turn_index").NonNegative().Immutable(),
		field.Time("occurred_at").Immutable(),
		field.String("trigger").Optional(),
		field.Int64("tokens_before").NonNegative(),
		field.Int64("tokens_after").NonNegative(),
		field.Int64("tokens_freed").NonNegative(),
	}
}

// Indexes of the CompactionEvent.
func (CompactionEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("session_uuid", "turn_index").Unique(),
		index.Fields("occurred_at"),
	}
}
