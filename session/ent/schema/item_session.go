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
		field.String("last_commit_sha").
			Optional(),
		field.Time("last_commit_at").
			Optional().
			Nillable(),
		field.String("last_commit_message").
			Optional(),
		field.Int("commit_count_since_spawn").
			Default(0),
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
