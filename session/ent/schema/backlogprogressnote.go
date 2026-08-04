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
		// Min(-1), not Min(0): -1 is an established sentinel for "not tied to
		// a specific criterion" (see session.Storage.SetBacklogItemPRAndTransition's
		// and TransitionBacklogItemStatus's manual-override note, both of
		// which append item-level history with no single criterion to
		// attach to). Before this fix the field rejected -1 outright, so
		// every -1 call silently failed ent validation and
		// AppendProgressNote's caller — which treats this as best-effort,
		// per this type's own doc comment — logged a warning and moved on;
		// the audit note was never actually persisted.
		field.Int("criterion_index").
			Min(-1),
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
