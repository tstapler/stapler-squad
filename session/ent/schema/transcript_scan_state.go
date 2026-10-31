package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// TranscriptScanState is one background scanner's resume point for one JSONL
// transcript, so a restart re-reads only appended bytes instead of every
// transcript from byte 0. Keyed by (consumer, path): "tokens" stores the
// parser checkpoint in state, "artifacts" needs only the offset.
type TranscriptScanState struct{ ent.Schema }

// Fields of the TranscriptScanState.
func (TranscriptScanState) Fields() []ent.Field {
	return []ent.Field{
		field.String("consumer").NotEmpty().Immutable(),
		field.String("path").NotEmpty().Immutable(),
		// size and mod_time_ns are the file's stat when last scanned; equality
		// means nothing was appended.
		field.Int64("size").NonNegative(),
		field.Int64("mod_time_ns"),
		// offset is the byte just past the last complete line consumed.
		field.Int64("offset").NonNegative(),
		// head_hash/tail_hash fingerprint the bytes before offset so a replaced
		// or rewritten file is detected even when it grew.
		// Stored as the int64 bit pattern of the FNV-64 hash: SQLite rejects
		// uint64 values with the high bit set.
		field.Int64("head_hash"),
		field.Int64("tail_hash"),
		field.Bytes("state").Optional(),
		field.Time("updated_at"),
	}
}

// Indexes of the TranscriptScanState.
func (TranscriptScanState) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("consumer", "path").Unique(),
	}
}
