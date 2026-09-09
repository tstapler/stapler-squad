package session

import "errors"

// GateKind is the sum type of transition-gate evaluation strategies a
// TransitionGate row can declare. Matches the four "Actors for Transition
// Gates" named in requirements.md exactly, by name — see plan.md's Domain
// Glossary entry for GateKind. Sealed set: every consumer switch over
// GateKind must be exhaustive (a default: case reporting an error, never a
// silent no-op or a panic).
type GateKind string

const (
	// GateKindHumanApproval requires one explicit recorded approval action
	// (Epic 2.4's RecordGateApproval) — a stateful, one-shot gate.
	GateKindHumanApproval GateKind = "human_approval"
	// GateKindAutomatedReview requires a PASS verdict from a spawned review
	// session, reusing ReviewGateRunner's verdict machinery (Epic 2.4.3) — a
	// stateful, one-shot gate.
	GateKindAutomatedReview GateKind = "automated_review"
	// GateKindStructural evaluates a named precondition directly against
	// item state (e.g. "all acceptance criteria done") — stateless: always
	// recomputed fresh on every PendingGates call, never cached as
	// previously satisfied (Story 2.3.2's acceptance criterion).
	GateKindStructural GateKind = "structural"
	// GateKindCustom invokes a named, pre-registered skill/slash-command
	// bounded by a LivenessDefinition (Epic 2.4.4's InvokeCustomGateCheck) —
	// a stateful, one-shot gate.
	GateKindCustom GateKind = "custom"
)

// GateStatus describes one gate's current satisfaction state for a pending
// transition attempt. Returned by WorkflowEngine.PendingGates; drives the
// item-detail "what's blocking this" UI (Epic 2.10).
//
// GateID is a string, not a uuid.UUID: ConfiguredWorkflowEngine's gates carry
// a real, persisted TransitionGate.ID, but DefaultWorkflowEngine's built-in
// gates (translated from TransitionGuard's switch, ADR-002) have no
// persisted row to key on. A string lets both engines populate a stable,
// human-readable identifier without a second ID type or a sentinel UUID.
type GateStatus struct {
	GateID      string
	Kind        GateKind
	Satisfied   bool
	Description string
	ActionHint  string
}

// ErrGateNotSatisfied is the sentinel error wrapped by a WorkflowEngine's
// ValidateGates when PendingGates reports at least one unsatisfied gate for
// the attempted transition.
var ErrGateNotSatisfied = errors.New("transition gate not satisfied")
