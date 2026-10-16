package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ContextSample is one assistant turn's context-window occupancy, persisted so
// "time near the ceiling" survives restarts and transcript pruning. Keyed by
// (session_uuid, turn_index): turn_index is the position in the parsed
// TurnTimeline, which is append-only, so re-parsing never duplicates rows.
type ContextSample struct{ ent.Schema }

// Fields of the ContextSample.
func (ContextSample) Fields() []ent.Field {
	return []ent.Field{
		field.String("session_uuid").NotEmpty().Immutable(),
		field.Int("turn_index").NonNegative().Immutable(),
		field.Time("sampled_at").Immutable(),
		field.String("model").Optional(),
		field.Int64("context_tokens").NonNegative(),
		field.Int64("context_max").NonNegative(),
	}
}

// Indexes of the ContextSample.
func (ContextSample) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("session_uuid", "turn_index").Unique(),
		index.Fields("sampled_at"),
	}
}
