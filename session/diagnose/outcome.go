// Package diagnose holds the domain types for the backlog "diagnose stuck
// item" feature: the dispatch outcome sum type, the safety-gate reasons that
// can block a nudge write, and the diagnostic-bundle section budgets.
package diagnose

import "fmt"

// DiagnoseOutcomeKind is the closed, post-completion outcome of a diagnose
// dispatch. It does not represent the pre-completion "Diagnosing (in-flight)"
// or "Stalled" states -- those live on DiagnoseDispatchStatus, which is set
// on the DiagnoseDispatch row itself, not on DiagnoseOutcome. Typed-string
// enum, mirroring HandoffSummaryStatus's convention (session/handoff_summary_service.go).
type DiagnoseOutcomeKind string

const (
	DiagnoseOutcomeKindNudged                DiagnoseOutcomeKind = "nudged"
	DiagnoseOutcomeKindBugFiled              DiagnoseOutcomeKind = "bug_filed"
	DiagnoseOutcomeKindInconclusiveNoteFiled DiagnoseOutcomeKind = "inconclusive_note_filed"
	DiagnoseOutcomeKindSkippedSafetyGate     DiagnoseOutcomeKind = "skipped_safety_gate"
	DiagnoseOutcomeKindDispatchFailed        DiagnoseOutcomeKind = "dispatch_failed"
)

// DiagnoseDispatchStatus is the lifecycle enum on the DiagnoseDispatch row
// (session/ent/schema/diagnosedispatch.go, added in a later phase): Pending
// (dispatch started, no outcome yet), Completed (an outcome has been
// recorded), or Stalled (the dispatch's session ended without ever calling a
// terminal tool, detected by a reconciler). Distinct from, and orthogonal to,
// DiagnoseOutcomeKind -- a Pending or Stalled row has no OutcomeKind.
type DiagnoseDispatchStatus string

const (
	DiagnoseDispatchStatusPending   DiagnoseDispatchStatus = "pending"
	DiagnoseDispatchStatusCompleted DiagnoseDispatchStatus = "completed"
	DiagnoseDispatchStatusStalled   DiagnoseDispatchStatus = "stalled"
)

// DiagnoseOutcome is the closed sum type describing what a diagnose dispatch
// ultimately did. Exactly one kind-specific field is populated, determined by
// Kind -- never a bag of independent booleans. Validate enforces the
// kind/field pairing.
type DiagnoseOutcome struct {
	Kind DiagnoseOutcomeKind

	// GateReason is set only when Kind == DiagnoseOutcomeKindSkippedSafetyGate.
	GateReason *SafetyGateReason
	// BugItemID is set only when Kind == DiagnoseOutcomeKindBugFiled.
	BugItemID *string
	// NoteText is set only when Kind == DiagnoseOutcomeKindInconclusiveNoteFiled.
	NoteText *string
	// FailureReason is set only when Kind == DiagnoseOutcomeKindDispatchFailed.
	FailureReason *string
	// WriteAttempted is non-nil only when a write call was attempted but its
	// outcome could not be confirmed (Story 4.1.4); orthogonal to Kind, so
	// Validate does not constrain it.
	WriteAttempted *bool
}

// diagnoseOutcomeRequiredField names, per DiagnoseOutcomeKind, the one
// kind-specific field that must be set (empty string means none must be).
// Keep in sync with DiagnoseOutcomeKind's const block -- see
// TestDiagnoseOutcomeKind_ExhaustiveSwitchCoverage, which fails if a new kind
// is added here or in the const block without the other following.
var diagnoseOutcomeRequiredField = map[DiagnoseOutcomeKind]string{
	DiagnoseOutcomeKindNudged:                "",
	DiagnoseOutcomeKindBugFiled:              "BugItemID",
	DiagnoseOutcomeKindInconclusiveNoteFiled: "NoteText",
	DiagnoseOutcomeKindSkippedSafetyGate:     "GateReason",
	DiagnoseOutcomeKindDispatchFailed:        "FailureReason",
}

// Validate rejects a DiagnoseOutcome whose populated kind-specific fields
// don't match its Kind (e.g. Nudged with a non-nil GateReason), and rejects
// an unrecognized Kind outright.
func (o DiagnoseOutcome) Validate() error {
	required, known := diagnoseOutcomeRequiredField[o.Kind]
	if !known {
		return fmt.Errorf("diagnose: unrecognized DiagnoseOutcomeKind %q", o.Kind)
	}

	set := map[string]bool{
		"GateReason":    o.GateReason != nil,
		"BugItemID":     o.BugItemID != nil,
		"NoteText":      o.NoteText != nil,
		"FailureReason": o.FailureReason != nil,
	}

	if required != "" && !set[required] {
		return fmt.Errorf("diagnose: %s outcome requires %s to be set", o.Kind, required)
	}

	for field, isSet := range set {
		if field != required && isSet {
			return fmt.Errorf("diagnose: %s outcome must not set %s", o.Kind, field)
		}
	}

	return nil
}

// SafetyGateReason names which nudge-write safety gate failed. Typed-string
// enum, mirroring HandoffSummaryStatus's convention.
//
// DuplicateWriteAttemptForDispatch (Story 4.1.4/Task 4.1.4g) is a separate,
// additive guard keyed by DiagnoseDispatch's dispatchID -- it runs outside
// NudgeGate.Evaluate and does not consult or consume the nudge cap; see
// server/mcp/diagnose_gate_wiring.go's checkDuplicateWriteGuard.
type SafetyGateReason string

const (
	SafetyGateReasonNotIdle                          SafetyGateReason = "not_idle"
	SafetyGateReasonIdentityMismatchInstance         SafetyGateReason = "identity_mismatch_instance"
	SafetyGateReasonIdentityMismatchTmuxMarker       SafetyGateReason = "identity_mismatch_tmux_marker"
	SafetyGateReasonNudgeCapReached                  SafetyGateReason = "nudge_cap_reached"
	SafetyGateReasonNudgeCooldownActive              SafetyGateReason = "nudge_cooldown_active"
	SafetyGateReasonNudgeExecutionDisabled           SafetyGateReason = "nudge_execution_disabled"
	SafetyGateReasonDuplicateWriteAttemptForDispatch SafetyGateReason = "duplicate_write_attempt_for_dispatch"
)

// String returns a human-readable label for log-line formatting.
func (r SafetyGateReason) String() string {
	switch r {
	case SafetyGateReasonNotIdle:
		return "session not idle"
	case SafetyGateReasonIdentityMismatchInstance:
		return "identity mismatch (instance snapshot)"
	case SafetyGateReasonIdentityMismatchTmuxMarker:
		return "identity mismatch (tmux marker)"
	case SafetyGateReasonNudgeCapReached:
		return "nudge cap reached"
	case SafetyGateReasonNudgeCooldownActive:
		return "nudge cooldown active"
	case SafetyGateReasonNudgeExecutionDisabled:
		return "nudge execution disabled"
	case SafetyGateReasonDuplicateWriteAttemptForDispatch:
		return "duplicate write attempt for this dispatch"
	default:
		return string(r)
	}
}
