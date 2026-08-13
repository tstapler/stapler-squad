package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// ItemSession holds the schema definition for the ItemSession entity.
type ItemSession struct {
	ent.Schema
}

// Fields of the ItemSession.
func (ItemSession) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New),
		field.String("session_uuid").
			Comment("Loose FK to Session; not an ent edge"),
		field.String("session_role").
			Comment("One of: work, triage, review"),
		field.Time("started_at").
			Optional().
			Nillable(),
		field.Time("ended_at").
			Optional().
			Nillable(),
		field.String("end_reason").
			Optional().
			Default("").
			Comment("Set only alongside ended_at for a headless (triage/review) call: classifyHeadlessCallError's bucket (\"shutdown\", \"timeout\", \"process_error\", \"claude_not_found\", \"other\") or \"\" for a successful end / not yet classified. Lets orphan-recovery sweeps distinguish a call killed by our own graceful shutdown (retry immediately, no penalty) from a call that actually failed on its own merits (apply the normal backoff)."),
		field.String("failure_capture_path").
			Optional().
			Default("").
			Comment("Absolute path to a durable file (see session.WriteHeadlessFailureCapture, under ~/.stapler-squad/headless-failures/) holding the size-capped raw stdout of a headless triage/review call that errored or whose result failed to parse. Set alongside end_reason on a call error, or on a successful call whose output was unparseable JSON. Exists because the log preview previously logged on parse failure is truncated to ~200 chars and the log file itself rotates out within a few hours — this file survives both, so diagnosing a failure doesn't require racing log rotation."),
		field.String("ac_snapshot").
			Optional().
			Comment("JSON []AcCriterion at spawn time"),
		field.String("pipeline_mode_snapshot").
			Default("").
			Comment("The PipelineMode slug resolved and in effect when this session first started — snapshotted so later edits to the item's live pipeline_mode don't retroactively change what this session is shown to have run. Mirrors ac_snapshot's discipline."),
		field.String("pipeline_mode_snapshot_hash").
			Default("").
			Comment("SHA-256 (hex, truncated to 16 chars) of the resolved mode's 9 raw content-template field values, concatenated in fixed order, computed at the moment this session started. Empty for the default mode (code-backed, can't drift) or an already-unresolved slug. Compared against the live mode's current hash by the \"what ran\" UI (Story 3.4.1) to detect the referenced mode's content having been edited since — the slug alone cannot detect this."),
		field.String("triage_result").
			Optional().
			Comment("JSON triage suggestions"),
		field.String("verification_notes").
			Optional().
			Comment("Freeform verification evidence reported via request_review (commands run, manual checks performed) — not visible in the diff"),
		field.String("base_commit_sha").
			Optional().
			Default("").
			Comment("The worktree's pre-work HEAD SHA, captured once when this session spawns. This is the BASE of the review gate's base..HEAD diff — by construction always an ancestor of main. It is NOT evidence of anything having shipped: any \"is this session's work on main?\" check must read last_commit_sha (live-refreshed), never this. Splitting the two is the fix for BUG-047's collateral damage, where the spawn-time base SHA was written into last_commit_sha and made IsCommitOnMain trivially true, causing closeIfSupersededByMain to close a real, unmerged PR as \"superseded\"."),
		field.String("last_commit_sha").
			Optional().
			Comment("The work session's CURRENT tip commit, re-read from the session's worktree HEAD on every reconciliation tick by refreshWorkSessionGitActivity (session/backlog_lifecycle.go) for as long as the session is active. Safe to treat as \"the latest commit this session authored\". For the pre-work baseline, use base_commit_sha."),
		field.Time("last_commit_at").
			Optional().
			Nillable(),
		field.String("last_commit_message").
			Optional(),
		field.Int("commit_count_since_spawn").
			Default(0).
			Comment("Number of commits reachable from last_commit_sha but not from base_commit_sha, recomputed alongside last_commit_sha on each refresh tick."),
		field.Time("last_file_touch_at").
			Optional().
			Nillable(),
		field.Time("last_progress_at").
			Optional().
			Nillable(),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Float("estimated_cost_usd").
			Default(0).
			Optional().
			Comment("Cost in USD; populated for headless sessions from claude -p output"),
	}
}

// Edges of the ItemSession.
func (ItemSession) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("backlog_item", BacklogItem.Type).
			Ref("item_sessions").
			Unique().
			Required(),
		edge.To("review_verdict", ReviewVerdict.Type).
			Unique(),
	}
}

// Indexes of the ItemSession.
func (ItemSession) Indexes() []ent.Index {
	return []ent.Index{
		// CRITICAL: O(1) lookup on every EventExited hook
		index.Fields("session_uuid"),
		// Composite index for "all sessions for an item ordered by time" queries.
		index.Fields("created_at").Edges("backlog_item"),
	}
}
