package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// CompactionEvent is one conversation compaction (/compact or auto-compact)
// detected from a transcript's compact_boundary entry. Claude Code runs the
// command itself, so the transcript is the only place it can be observed.
// Keyed by (session_uuid, ordinal): ordinal is the event's position among the
// session's compact_boundary entries, which is append-only and so stable across
// re-parses. turn_index is not a key because back-to-back compactions with no
// assistant turn between them share it.
type CompactionEvent struct{ ent.Schema }

// Fields of the CompactionEvent.
func (CompactionEvent) Fields() []ent.Field {
	return []ent.Field{
		field.String("session_uuid").NotEmpty().Immutable(),
		field.Int("ordinal").NonNegative().Immutable(),
		field.Int("turn_index").NonNegative(),
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
		index.Fields("session_uuid", "ordinal").Unique(),
		index.Fields("occurred_at"),
	}
}
