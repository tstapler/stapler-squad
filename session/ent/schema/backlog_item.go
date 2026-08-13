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
		field.String("plan_rejection_reason").
			Optional().
			Comment("Free-text reason from the most recent RejectPlan call. Cleared on ApprovePlan, on the next TriggerTriage completion, and on backward transition to idea/refining. See project_plans/plan-approval-ux/decisions/ADR-001."),
		field.Time("plan_rejected_at").
			Optional().
			Nillable(),
		field.String("user_modified_fields").
			Optional().
			Comment("JSON set of field names modified by the user"),
		field.String("notes").
			Optional(),
		field.String("external_id").
			Optional(),
		field.String("external_url").
			Optional(),
		field.Strings("labels").
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
		field.Time("pr_feedback_addressed_at").
			Optional().
			Nillable().
			Comment("Per-item high-water mark: the newest substantive PR review-feedback timestamp a fix session has already been dispatched to address. GitHub never clears COMMENTED reviews/comments on push, so this watermark is what stops already-addressed feedback from re-triggering a fix session on every ReconcilePRPending tick."),
		field.Time("github_synced_issue_updated_at").
			Optional().
			Nillable().
			Comment("Per-item high-water mark: the GitHub issue updated_at value most recently synced from GitHub into this item. Stops a forward-sync write from GitHub from being re-observed and re-synced back to GitHub on the next poll (loop prevention)."),
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
		field.UUID("next_workflow_id", uuid.UUID{}).
			Optional().
			Nillable().
			Comment("webhook-triggers pipeline chaining (FR10/AC5): the Workflow to fire when this item reaches BacklogStatusDone. Set at chain-configuration time, not computed reactively at completion."),
		field.Bool("chain_fired").
			Default(false).
			Comment("webhook-triggers pipeline chaining: true once the NextWorkflowID chain-fire has been attempted to a terminal outcome (fired, depth-capped, or expired) — never retried again once true. Crash-consistency marker: TriggerChainReconciler scans for status=done AND next_workflow_id != nil AND chain_fired=false."),
		field.Time("chained_at").
			Optional().
			Nillable().
			Comment("webhook-triggers pipeline chaining: set atomically with the terminal status transition (same UPDATE as the done transition) when NextWorkflowID is already configured — the eligibility timestamp TriggerChainReconciler's maxChainWaitDuration ceiling measures age against, independent of whether/when the fire itself later succeeds."),
		field.Int("triggered_by_chain_depth").
			Default(0).
			Comment("webhook-triggers pipeline chaining (Epic 6.3): how many chain hops produced this item, propagated session->session and hard-capped at maxChainDepth as a runaway-loop backstop independent of the WIP-limit gate."),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		// updated_at is stored/compared in UTC, not time.Now's default Local
		// zone. mattn/go-sqlite3 binds time.Time by formatting it as TEXT in
		// the value's own Location (sqlite3.go's statementBind, `case
		// time.Time: b := []byte(v.Format(SQLiteTimestampFormats[0]))`), so
		// two time.Time values representing the identical instant but with
		// different Locations (e.g. Local "-07:00" vs UTC "Z") serialize to
		// different bytes and fail a `WHERE updated_at = ?` CAS comparison
		// even though they're semantically equal (confirmed: time.Now() and
		// time.Now().UTC() satisfy .Equal() but format to different
		// strings). Every value that arrives via a protobuf Timestamp
		// (google.golang.org/protobuf/types/known/timestamppb's AsTime()
		// always returns UTC) — e.g. TransitionBacklogItemStatusRequest's
		// expected_updated_at, round-tripped from any RPC client — could
		// therefore never match a Local-zoned stored value, making that CAS
		// precondition unconditionally fail. Storing in UTC here makes the
		// column consistent with what every protobuf-sourced comparison
		// value already is.
		field.Time("updated_at").
			Default(func() time.Time { return time.Now().UTC() }).
			UpdateDefault(func() time.Time { return time.Now().UTC() }),
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
		edge.From("source", ItemSource.Type).
			Ref("backlog_items").
			Unique(),
		edge.To("blocking_dependencies", BacklogItemDependency.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("blocked_by_dependencies", BacklogItemDependency.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
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
		index.Fields("status", "chain_fired"),
	}
}
