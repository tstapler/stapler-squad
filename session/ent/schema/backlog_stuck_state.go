package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// BacklogStuckState is a durable, resolve-in-place record of a single stuck
// condition for a backlog item.
//
// Resolve-in-place model: there is exactly one row per (item_id, reason) pair
// at all times, enforced by the plain 2-column unique index below. A stuck
// condition that clears sets resolved_at (the row becomes "closed" but is not
// deleted). If the same (item_id, reason) condition recurs later, the SAME row
// is reopened in place — resolved_at and notified_at are cleared and
// first_detected_at is reset to the new onset time — rather than inserting a
// second row. Episode/occurrence history (multiple rows per pair over time) is
// intentionally NOT retained; see ADR-001 "Durable Stuck-State Storage Model"
// for the rationale (a plain unique index cannot be an OnConflictColumns
// target for an append-only design on SQLite, since NULLs are distinct).
type BacklogStuckState struct {
	ent.Schema
}

// Fields of the BacklogStuckState.
func (BacklogStuckState) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New),
		field.UUID("item_id", uuid.UUID{}),
		field.String("reason").
			Comment("Validated in Go by domain.StuckReason.IsValid(); stored as a plain string to match the house BacklogStatus/ReviewOutcome style."),
		field.Time("first_detected_at").
			Default(time.Now).
			Comment("When this stuck condition was first observed. Reset to now on a resolve-in-place reopen."),
		field.Time("last_checked_at").
			Default(time.Now).
			Comment("Most recent reconcile tick that re-confirmed the condition still holds."),
		field.Time("notified_at").
			Optional().
			Nillable().
			Comment("When the operator notification fired. NULL = detected but not yet notified. Cleared on reopen."),
		field.Time("resolved_at").
			Optional().
			Nillable().
			Comment("When the condition was observed to clear. NULL = currently open (stuck)."),
		field.Time("snoozed_until").
			Optional().
			Nillable().
			Comment("Suppresses this row from the active view/re-notification until this time."),
		field.String("context").
			Optional().
			Comment("Human-readable 'why' string, e.g. 'last verdict: FAIL' or 'PR #148 green & mergeable 3d'."),
		field.Int32("remediation_attempts").
			Default(0).
			Comment("Count of automated remediation attempts actually made for this open row (incremented per attempt, not per detection-sweep tick). Reset to 0 by ResetStuckRemediation/BulkResetStuckRemediation, and implicitly by MarkStuck's reopen-in-place path. remediation_attempts >= maxRemediationAttempts (5) with next_remediation_at NULL is how a 'parked' row is represented — no separate boolean."),
		field.Time("next_remediation_at").
			Optional().
			Nillable().
			Comment("When this row becomes eligible for the next automated remediation attempt. NULL while remediation_attempts is 0 means 'eligible immediately'. Set back to NULL once remediation_attempts reaches the cap (parked)."),
		field.Time("grace_boot_time").
			Optional().
			Nillable().
			Comment("The server boot time (session.serverStartTime) of the most recent restart-grace pass consumed by this row, if any. A restart-grace pass lets a remediation action run without consuming remediation_attempts/advancing next_remediation_at when the detected failure coincides with a service restart (in-flight AutonomousDriver goroutines are lost on restart, not a real remediation failure) — at most one free pass per boot, tracked by comparing this field to the current boot time."),
	}
}

// Edges of the BacklogStuckState.
func (BacklogStuckState) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("item", BacklogItem.Type).
			Ref("stuck_states").
			Field("item_id").
			Unique().
			Required(),
	}
}

// Indexes of the BacklogStuckState.
func (BacklogStuckState) Indexes() []ent.Index {
	return []ent.Index{
		// Plain 2-column unique index — NOT 3-column, NOT partial. This is both
		// the OnConflictColumns target for MarkStuck's resolve-in-place upsert
		// and the correctness guarantee that exactly one row exists per pair.
		index.Fields("item_id", "reason").Unique(),
	}
}
