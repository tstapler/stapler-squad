package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// BacklogActivityNote is an append-only, per-item log of free-form notes that
// ANY session can post — with or without STAPLER_SESSION_UUID, and regardless
// of whether that session is linked to the item. It is the ungated sibling of
// BacklogProgressNote, not an extension of it: report_progress's official
// AC-status audit trail (BacklogProgressNote, ProgressNoteData) stays
// structurally separate from this anyone-can-post note, so an ungated write
// can never be confused with — or accidentally clobber — the role-gated
// tools' data. See
// project_plans/backlog-item-activity-log/decisions/ADR-001-sibling-table-not-extend-progress-note.md
// for the full rationale. This table is never written by report_progress,
// submit_review_verdict, or any other role-gated tool.
type BacklogActivityNote struct {
	ent.Schema
}

func (BacklogActivityNote) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New),
		field.UUID("item_id", uuid.UUID{}),
		field.String("message").
			Comment("Freeform note text posted via the ungated post_backlog_update tool. This is the note's only payload, unlike BacklogProgressNote.note which is secondary to a status change — required, not optional."),
		field.String("author_session_uuid").
			Optional().
			Comment("Best-effort caller identity. Empty means no session (a manual/unlinked caller)."),
		field.String("author_session_title").
			Optional().
			Comment("Best-effort session title resolved from author_session_uuid. May be empty even when author_session_uuid is set."),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}

func (BacklogActivityNote) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("item", BacklogItem.Type).
			Ref("activity_notes").
			Field("item_id").
			Unique().
			Required(),
	}
}

func (BacklogActivityNote) Indexes() []ent.Index {
	return []ent.Index{
		// "all activity notes for this item, in order" queries.
		index.Fields("item_id", "created_at"),
	}
}
