package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// EscapeEvent holds the schema for a terminal escape sequence observation.
type EscapeEvent struct{ ent.Schema }

func (EscapeEvent) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Unique().NotEmpty().Immutable(),
		field.String("session_id").NotEmpty(),
		field.String("project_path").Optional(),
		field.String("stage").NotEmpty(),
		field.String("sequence_type").NotEmpty(),
		field.String("sequence_subtype").Optional(),
		field.String("sequence_signature").Optional(),
		field.Int("byte_length"),
		field.String("payload_hash").Optional(),
		field.Bytes("raw_bytes").Optional(),
		field.Bool("mangled").Default(false),
		field.String("mangle_type").Optional(),
		field.Time("wall_time").Default(time.Now).Immutable(),
		field.Int64("session_seq"),
	}
}

func (EscapeEvent) Indexes() []ent.Index {
	return []ent.Index{
		// Only the three indexes below are read by a query: timeline paging
		// (session_id, session_seq), per-session oldest-N eviction (session_id, wall_time)
		// and age/time-range scans (wall_time). The former session_id, (session_id, stage),
		// mangled, sequence_type and project_path indexes were redundant prefixes or
		// low-selectivity and cost ~190 MB; server/analytics drops them from existing DBs.
		index.Fields("session_id", "session_seq"),
		// Serves deleteOldestRows' per-session "oldest N by wall_time": without it
		// SQLite reads and temp-sorts every row of the session (up to the 10,000-row cap)
		// on each flush of a capped session.
		index.Fields("session_id", "wall_time"),
		index.Fields("wall_time"),
	}
}
