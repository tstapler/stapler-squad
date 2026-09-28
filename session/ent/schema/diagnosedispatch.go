package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// DiagnoseDispatch holds the schema definition for the DiagnoseDispatch
// entity: one row per "Diagnose" dispatch against a stuck backlog item,
// recording its full lifecycle from Pending (written before the diagnostic
// session even exists, per Story 5.1.1's Task 5.1.1d) through Completed or
// Stalled. Backs DiagnoseDispatchStore (Story 5.2.1) and the durable half of
// notifyDiagnoseEvent (Story 5.2.2). item_id is a plain string column, not an
// edge to BacklogItem -- same precedent as NudgeCapRecord.item_id and
// GateSatisfactionRecord.item_id: this entity has no natural edge target of
// its own, and multiple rows share the same item_id (one per dispatch),
// unlike NudgeCapRecord's Unique() key.
//
// Status/OutcomeKind/SafetyGateReason are stored as plain strings mirroring
// session/diagnose.DiagnoseDispatchStatus/DiagnoseOutcomeKind/SafetyGateReason
// (typed-string enums), the same convention HandoffSummary.status uses --
// this schema package does not import session/diagnose to avoid coupling the
// storage layer's literal wire values to that package's Go identifiers.
//
// WriteAttemptedAt is deliberately NOT added yet -- Story 4.1.4's own worker
// adds it alongside WriteAttempted with its own ent-generate run.
//
// failure_reason is NOT in the plan's Migration Plan field list, but is added
// here anyway: session/diagnose.DiagnoseOutcome.FailureReason is required
// (Validate()) whenever Kind is DiagnoseOutcomeKindDispatchFailed, and Story
// 5.1.3 (dispatch-failure handling) depends on this exact store being able to
// persist that outcome durably -- omitting the column would make MarkCompleted
// silently drop the one piece of data DispatchFailed exists to record.
type DiagnoseDispatch struct {
	ent.Schema
}

// Fields of the DiagnoseDispatch.
func (DiagnoseDispatch) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Unique().
			NotEmpty().
			Immutable(),
		field.String("item_id").
			NotEmpty().
			Comment("Backlog item this dispatch was requested for."),
		field.String("target_session_uuid").
			NotEmpty().
			Comment("UUID of the stuck session being diagnosed."),
		field.String("diagnostic_session_uuid").
			Optional().
			Comment("UUID of the headless diagnostic agent session dispatched to investigate. Empty until that session is created -- Record persists the Pending row first (Task 5.1.1d), so a mid-dispatch page refresh has something to read."),
		field.String("status").
			Default("pending").
			Comment("Lifecycle: pending, completed, or stalled -- mirrors diagnose.DiagnoseDispatchStatus."),
		field.String("outcome_kind").
			Optional().
			Nillable().
			Comment("Set only once status is completed; mirrors diagnose.DiagnoseOutcomeKind. Null for pending and stalled alike."),
		field.String("safety_gate_reason").
			Optional().
			Nillable().
			Comment("Set only when outcome_kind is skipped_safety_gate; mirrors diagnose.SafetyGateReason."),
		field.String("bug_item_id").
			Optional().
			Nillable().
			Comment("Set only when outcome_kind is bug_filed."),
		field.Text("note_text").
			Optional().
			Nillable().
			Comment("Set only when outcome_kind is inconclusive_note_filed."),
		field.Bool("write_attempted").
			Optional().
			Nillable().
			Comment("Set only when a write call was attempted but its outcome could not be confirmed (Story 4.1.4); orthogonal to outcome_kind."),
		field.Text("failure_reason").
			Optional().
			Nillable().
			Comment("Set only when outcome_kind is dispatch_failed; mirrors diagnose.DiagnoseOutcome.FailureReason."),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("completed_at").
			Optional().
			Nillable().
			Comment("Set when status transitions to completed; null for pending and stalled."),
	}
}

// Edges of the DiagnoseDispatch.
func (DiagnoseDispatch) Edges() []ent.Edge {
	return nil
}

// Indexes of the DiagnoseDispatch. "all dispatches for this item, in order"
// queries (ListByItem) drive this composite index, mirroring
// BacklogActivityNote's index.Fields("item_id", "created_at") precedent.
func (DiagnoseDispatch) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("item_id", "created_at"),
	}
}
