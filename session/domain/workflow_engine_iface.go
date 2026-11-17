package domain

import ()

// StageConfigSnapshot is a frozen, per-item snapshot of the stage config a
// BacklogItem's current stage carried when captured, used as a fallback
// answer for CanTransition/AllowedTransitions when that stage has since been
// deleted from the live stageConfigCache (Epic 2.5, Story 2.5.2). Mirrors
// AcSnapshot's "history/behavior survives later config edits" discipline —
// see plan.md's Domain Glossary entry for StageConfigSnapshot. Building and
// persisting this snapshot per item is a follow-up concern; this type only
// defines the shape CanTransition/AllowedTransitions consume when a caller
// already has one in hand.
type StageConfigSnapshot struct {
	// StageName is the stage's human-readable name at snapshot time — the
	// same value Story 2.5.1 freezes onto BacklogStatusEvent.StageNameSnapshot.
	StageName string
	// AllowedTransitions is the set of destination stages that were legal
	// from this stage at snapshot time.
	AllowedTransitions []BacklogStatus
}

// stageConfigUnresolvableGateID identifies the single synthetic blocking gate
// PendingGates returns when the live stageConfigCache has no edge for
// (item.Status, to) but a supplied fallback confirms the edge legally existed
// when the item entered its current stage (ADR-004, Decision 4 — the
// adversarial review's fail-closed-for-gates fix).
const StageConfigUnresolvableGateID = "stage-config-unresolvable"

// WorkflowEngine is the policy object that governs which backlog status
// transitions are permitted and what guards must pass.
//
// PendingGates was added by Epic 2.3 (ADR-002-configured-workflow-engine-and-gates.md,
// Decision 2): gates and CanTransition/ValidateGates share the same
// consumers and the same triggering clock (a transition-attempt), so this
// extends WorkflowEngine rather than introducing a sibling interface — see
// ADR-002's Rationale/Alternatives Considered for why that differs from
// LivenessEngine's sibling-interface treatment (ADR-001).
type WorkflowEngine interface {
	// CanTransition returns true if transitioning from → to is structurally
	// allowed. fallback is an optional, caller-supplied StageConfigSnapshot
	// (Epic 2.5) — pass the item's own captured snapshot when from may be a
	// since-deleted custom stage, so ConfiguredWorkflowEngine has a defined
	// answer instead of always returning false for an in-flight item stranded
	// on a deleted stage. nil means "no fallback". DefaultWorkflowEngine
	// ignores it entirely (its built-in stages are never deleted).
	CanTransition(from, to BacklogStatus, fallback *StageConfigSnapshot) bool
	// PendingGates returns the per-gate satisfaction status for transitioning
	// item to to — an empty/nil slice means no gates block this transition.
	// ValidateGates is a thin wrapper over this: nil exactly when every
	// entry here is Satisfied. fallback is the same optional, caller-supplied
	// StageConfigSnapshot documented on CanTransition (ADR-004) — pass the
	// item's own captured snapshot so a from-stage absent from the live cache
	// (deleted) can still report a synthetic blocking gate instead of
	// silently reporting zero pending gates. DefaultWorkflowEngine ignores it
	// entirely, same as CanTransition/AllowedTransitions.
	PendingGates(item BacklogItemTransitionInput, to BacklogStatus, fallback *StageConfigSnapshot) ([]GateStatus, error)
	// ValidateGates runs guard rules for the transition. Returns nil if gates
	// pass. fallback is the same optional StageConfigSnapshot documented on
	// CanTransition/PendingGates above — DefaultWorkflowEngine ignores it,
	// ConfiguredWorkflowEngine threads it into its internal PendingGates call.
	ValidateGates(item BacklogItemTransitionInput, to BacklogStatus, fallback *StageConfigSnapshot) error
	// AllowedTransitions returns the set of statuses reachable from from. See
	// CanTransition's fallback doc above — same optional-snapshot contract.
	AllowedTransitions(from BacklogStatus, fallback *StageConfigSnapshot) []BacklogStatus
}
