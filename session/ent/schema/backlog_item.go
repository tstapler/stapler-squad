package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// BacklogItem holds the schema definition for the BacklogItem entity.
type BacklogItem struct {
	ent.Schema
}

// Fields of the BacklogItem.
func (BacklogItem) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New),
		field.String("title").
			NotEmpty(),
		field.String("description").
			Optional(),
		field.String("acceptance_criteria").
			Optional().
			Comment("JSON []AcCriterion"),
		field.Int("priority").
			Default(3).
			Min(1).
			Max(5),
		field.String("status").
			Default("idea"),
		field.String("repo_path").
			Optional(),
		field.Bool("skip_review_gate").
			Default(false),
		field.Bool("skip_planning").
			Default(false),
		field.Bool("auto_spawn_session").
			Default(false).
			Comment("When true, a work session is spawned automatically once the item reaches ready — no manual 'Spawn Session' click required."),
		field.Bool("auto_create_pr").
			Default(false).
			Comment("When true, a PR is created automatically (via the same one-shot prompt the manual Review Queue 'Create PR' button uses) once a work session for this item reaches TASK_COMPLETE — no manual click required."),
		field.String("pipeline_mode").
			Default("").
			Comment("Slug of the PipelineMode this item uses to drive triage/work/review content. Empty string means the built-in default (today's fixed hardcoded pipeline)."),
		field.String("category").
			Default("").
			Comment("Coarse classification (bugfix/feature/chore/refactor) used by the frontend to pre-fill sane automation-toggle defaults at creation time. Empty string means uncategorized (today's behavior, preserved exactly). See session.IsValidBacklogCategory for the validated enum."),
		field.Bool("plan_approved").
			Default(false),
		field.Time("plan_approved_at").
			Optional().
			Nillable(),
		field.Time("queued_at").
			Optional().
			Nillable().
			Comment("Set when a fresh spawn hits the concurrency cap and the item is queued instead of rejected. Drives FIFO dequeue ordering."),
		field.Bool("queued_autonomous").
			Default(false).
			Comment("Preserves the Autonomous flag from the spawn request that got queued, so dequeue replays it faithfully."),
		field.String("plan_artifacts_path").
			Optional(),
		field.String("user_modified_fields").
			Optional().
			Comment("JSON set of field names modified by the user"),
		field.String("notes").
			Optional(),
		field.String("external_id").
			Optional(),
		field.Time("user_modified_status_at").
			Optional().
			Nillable(),
		field.Time("archived_at").
			Optional().
			Nillable(),
		field.String("pr_url").
			Optional(),
		field.Int("pr_number").
			Optional().
			Default(0),
		field.String("shipped_check_conclusion").
			Optional().
			Comment("Durable GitHub CI-conclusion snapshot captured at ship time — genuine GitHub CI-conclusion values only, never a capture-failure sentinel. See shipped_snapshot_capture_failed."),
		field.Int("shipped_approved_count").
			Optional().
			Default(0).
			Comment("Durable review-approval-count snapshot captured at ship time."),
		field.Int("shipped_changes_req_count").
			Optional().
			Default(0).
			Comment("Durable \"changes requested\" review-count snapshot captured at ship time."),
		field.Time("shipped_snapshot_at").
			Optional().
			Nillable().
			Comment("Timestamp the durable ship snapshot was captured at."),
		field.String("shipped_file_stats").
			Optional().
			Comment("JSON []ShippedFileStat{Path,Status,Additions,Deletions} — per-file diff stats captured at ship time"),
		field.Bool("shipped_snapshot_capture_failed").
			Optional().
			Default(false).
			Comment("true when CaptureShipSnapshot's GitHub fetch or file-stats computation failed — distinct from shipped_check_conclusion, which holds only genuine CI-conclusion values"),
		field.Int("rework_cap_override").
			Optional().
			Nillable().
			Comment("Per-item override for the auto-rework cap (MaxAutoReworkIterationsOrDefault). Nil = use the global default. 0 = unlimited for this item. >0 = this item's own cap, replacing (not adding to) the global value."),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the BacklogItem.
func (BacklogItem) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("item_sessions", ItemSession.Type),
		edge.To("sessions", Session.Type),
		edge.To("status_events", BacklogStatusEvent.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("stuck_states", BacklogStuckState.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("progress_notes", BacklogProgressNote.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("respawn_events", RespawnEvent.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.From("source", ItemSource.Type).
			Ref("backlog_items").
			Unique(),
	}
}

// Indexes of the BacklogItem.
func (BacklogItem) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status", "priority"),
		index.Fields("status", "updated_at"),
		index.Fields("status", "queued_at"),
		index.Fields("external_id"),
		index.Fields("status"),
	}
}
