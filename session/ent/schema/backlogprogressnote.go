package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// BacklogProgressNote is an append-only log of every report_progress call made
// against a backlog item's acceptance criteria. Unlike BacklogItem.AcceptanceCriteria
// (which stores only the *current* note per criterion, overwritten on each call),
// this table preserves the full history so a reviewer can see the entire timeline of
// notes reported across a work session, not just the latest one per criterion.
type BacklogProgressNote struct {
	ent.Schema
}

func (BacklogProgressNote) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New),
		field.UUID("item_id", uuid.UUID{}),
		field.Int("criterion_index").
			Min(0),
		field.String("note").
			Optional().
			Comment("Freeform note text reported via report_progress. Rendered call sites are responsible for truncation (see sanitizeField); stored unbounded here."),
		field.String("status").
			Comment("AC criterion status at the time of this report: one of pending, in_progress, done, fail"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}

func (BacklogProgressNote) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("item", BacklogItem.Type).
			Ref("progress_notes").
			Field("item_id").
			Unique().
			Required(),
	}
}

func (BacklogProgressNote) Indexes() []ent.Index {
	return []ent.Index{
		// "all notes for this item, in order" queries.
		index.Fields("item_id", "created_at"),
	}
}
