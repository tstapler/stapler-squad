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
		index.Fields("session_id"),
		index.Fields("session_id", "stage"),
		index.Fields("session_id", "session_seq"),
		// Serves deleteOldestRows' per-session "oldest N by wall_time": without it
		// SQLite reads and temp-sorts every row of the session (up to the 10,000-row cap)
		// on each flush of a capped session.
		index.Fields("session_id", "wall_time"),
		index.Fields("wall_time"),
		index.Fields("mangled"),
		index.Fields("sequence_type"),
		index.Fields("project_path"),
	}
}
