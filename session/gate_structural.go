package session

import "fmt"

// gate_structural.go — Story 2.4.2's closed set of structural/mechanical
// check identifiers a GateKindStructural gate's config.CheckID may name
// (session.StructuralConfig, session/gate_config.go). Every evaluator here is
// stateless: it recomputes fresh from item state on every PendingGates call,
// never trusting a previously-satisfied result — see GateKindStructural's own
// doc comment (session/gate_status.go).

const (
	// StructuralCheckACComplete reports satisfied iff every acceptance
	// criterion is marked done.
	StructuralCheckACComplete = "ac_complete"
	// StructuralCheckPRGreen is intended to report satisfied iff the item's
	// linked PR has passing CI and is mergeable. NOT YET WIRED: no PR/CI
	// status field exists on domain.BacklogItemTransitionInput today (that
	// data lives on BacklogItemData, one layer up, and PendingGates'
	// signature — shared with DefaultWorkflowEngine — takes only
	// BacklogItemTransitionInput). Evaluates fail-closed (never a false
	// PASS) with a description naming the gap, rather than silently
	// reporting satisfied.
	StructuralCheckPRGreen = "pr_green"
	// StructuralCheckNoOpenBlockers reports satisfied iff the item has no
	// unresolved BacklogItemDependency blockers.
	StructuralCheckNoOpenBlockers = "no_open_blockers"
)

// structuralCheckResult is the return shape shared by every structural-check
// evaluator: whether the check passed, the human-readable description shown
// in the gate UI (GateStatus.Description), and an ActionHint to unblock it
// when not satisfied ("" when satisfied).
type structuralCheckResult struct {
	Satisfied   bool
	Description string
	ActionHint  string
}

// structuralCheckEvaluators maps each closed-set check ID to its evaluator.
// Sealed set: an unrecognized check ID (a config that predates a check's
// removal, or a typo that somehow bypassed ParseGateConfig's save-time
// validation) is handled explicitly by evaluateStructuralCheck's own
// fallback below, never a silent panic or a permissive default-true.
var structuralCheckEvaluators = map[string]func(item BacklogItemTransitionInput) structuralCheckResult{
	StructuralCheckACComplete:     evaluateACCompleteCheck,
	StructuralCheckPRGreen:        evaluatePRGreenCheck,
	StructuralCheckNoOpenBlockers: evaluateNoOpenBlockersCheck,
}

// evaluateACCompleteCheck reports satisfied iff item has at least one
// acceptance criterion and every one is AcStatusDone.
func evaluateACCompleteCheck(item BacklogItemTransitionInput) structuralCheckResult {
	criteria, err := ParseAcCriteria(item.AcCriteria)
	if err != nil || len(criteria) == 0 {
		return structuralCheckResult{
			Satisfied:   false,
			Description: "no acceptance criteria defined",
			ActionHint:  "define at least one acceptance criterion",
		}
	}
	incomplete := 0
	for _, c := range criteria {
		if c.Status != AcStatusDone {
			incomplete++
		}
	}
	if incomplete == 0 {
		return structuralCheckResult{Satisfied: true, Description: "all acceptance criteria complete"}
	}
	return structuralCheckResult{
		Satisfied:   false,
		Description: fmt.Sprintf("%d of %d acceptance criteria incomplete", incomplete, len(criteria)),
		ActionHint:  "mark every acceptance criterion done",
	}
}

// evaluateNoOpenBlockersCheck reports satisfied iff the item has no
// unresolved blocker dependency (domain.BacklogItemTransitionInput.HasUnresolvedBlockers,
// populated by DequeueNextQueuedItems' batched dependency query).
func evaluateNoOpenBlockersCheck(item BacklogItemTransitionInput) structuralCheckResult {
	if item.HasUnresolvedBlockers {
		return structuralCheckResult{
			Satisfied:   false,
			Description: "item has one or more unresolved blocker dependencies",
			ActionHint:  "resolve all blocker items first",
		}
	}
	return structuralCheckResult{Satisfied: true, Description: "no open blockers"}
}

// evaluatePRGreenCheck always reports unsatisfied — see StructuralCheckPRGreen's
// doc comment for why. Fail-closed: an operator who configures this check
// today gets a clearly-labeled "not yet wired" block, never a silent,
// incorrect PASS.
func evaluatePRGreenCheck(BacklogItemTransitionInput) structuralCheckResult {
	return structuralCheckResult{
		Satisfied:   false,
		Description: "pr_green structural check has no PR/CI status data source yet",
		ActionHint:  "not yet actionable in this build",
	}
}

// evaluateStructuralCheck dispatches checkID to its evaluator. An
// unrecognized checkID (including empty) reports unsatisfied rather than
// panicking or defaulting to satisfied — fail-closed, matching
// TransitionGuard's ErrACRequired guards elsewhere in this package.
func evaluateStructuralCheck(checkID string, item BacklogItemTransitionInput) structuralCheckResult {
	eval, ok := structuralCheckEvaluators[checkID]
	if !ok {
		return structuralCheckResult{
			Satisfied:   false,
			Description: fmt.Sprintf("unrecognized structural check id %q", checkID),
		}
	}
	return eval(item)
}
