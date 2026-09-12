package session

import "sort"

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
	// on a deleted stage. At most the first element is used; DefaultWorkflowEngine
	// ignores it entirely (its built-in stages are never deleted).
	CanTransition(from, to BacklogStatus, fallback ...*StageConfigSnapshot) bool
	// PendingGates returns the per-gate satisfaction status for transitioning
	// item to to — an empty/nil slice means no gates block this transition.
	// ValidateGates is a thin wrapper over this: nil exactly when every
	// entry here is Satisfied. fallback is the same optional, caller-supplied
	// StageConfigSnapshot documented on CanTransition (ADR-004) — pass the
	// item's own captured snapshot so a from-stage absent from the live cache
	// (deleted) can still report a synthetic blocking gate instead of
	// silently reporting zero pending gates. DefaultWorkflowEngine ignores it
	// entirely, same as CanTransition/AllowedTransitions.
	PendingGates(item BacklogItemTransitionInput, to BacklogStatus, fallback ...*StageConfigSnapshot) ([]GateStatus, error)
	// ValidateGates runs guard rules for the transition. Returns nil if gates pass.
	ValidateGates(item BacklogItemTransitionInput, to BacklogStatus) error
	// AllowedTransitions returns the set of statuses reachable from from. See
	// CanTransition's fallback doc above — same optional-snapshot contract.
	AllowedTransitions(from BacklogStatus, fallback ...*StageConfigSnapshot) []BacklogStatus
}

// DefaultWorkflowEngine implements WorkflowEngine using the hardcoded
// validTransitions map and TransitionGuard function from backlog.go.
type DefaultWorkflowEngine struct {
	transitions map[BacklogStatus]map[BacklogStatus]bool
}

// NewDefaultWorkflowEngine constructs an engine backed by the static
// validTransitions map. The map is deep-copied to avoid shared mutable state.
func NewDefaultWorkflowEngine() *DefaultWorkflowEngine {
	t := make(map[BacklogStatus]map[BacklogStatus]bool, len(validTransitions))
	for from, targets := range validTransitions {
		inner := make(map[BacklogStatus]bool, len(targets))
		for to, v := range targets {
			inner[to] = v
		}
		t[from] = inner
	}
	return &DefaultWorkflowEngine{transitions: t}
}

// CanTransition implements WorkflowEngine. fallback is unused: the built-in
// stage graph is static and never deleted, so there is never a stage for a
// per-item snapshot to fall back for.
func (e *DefaultWorkflowEngine) CanTransition(from, to BacklogStatus, _ ...*StageConfigSnapshot) bool {
	targets, ok := e.transitions[from]
	if !ok {
		return false
	}
	return targets[to]
}

// PendingGates implements WorkflowEngine by translating TransitionGuard's
// switch (session/domain/backlog.go) into an equivalent GateStatus — ADR-002's
// requirement that DefaultWorkflowEngine's built-in-stage gate behavior stay
// identical to today's guard logic, just exposed as structured data. Returns
// a single GateKindStructural entry for a (from,to) pair TransitionGuard has
// a specific branch for, or a nil slice for its `default:` (no additional
// guards) branch. fallback is unused: the built-in stage graph is static and
// never deleted, so there is never a stage for a per-item snapshot to fall
// back for — see CanTransition's identical fallback doc comment above.
func (e *DefaultWorkflowEngine) PendingGates(item BacklogItemTransitionInput, to BacklogStatus, _ ...*StageConfigSnapshot) ([]GateStatus, error) {
	id, description, ok := builtInGuardGate(item.Status, to)
	if !ok {
		return nil, nil
	}
	satisfied := TransitionGuard(item, to) == nil
	hint := ""
	if !satisfied {
		hint = "resolve the blocking condition, or supply an override reason where supported"
	}
	return []GateStatus{{
		GateID:      id,
		Kind:        GateKindStructural,
		Satisfied:   satisfied,
		Description: description,
		ActionHint:  hint,
	}}, nil
}

// builtInGuardGate mirrors TransitionGuard's switch conditions (not its
// pass/fail bodies) to name the built-in guard that applies to a (from,to)
// pair, if any — kept in lockstep with TransitionGuard by construction,
// since both switch on the same case list. TransitionGuard itself remains
// the single source of truth for pass/fail logic; PendingGates calls it
// directly for the Satisfied bool, so behavior stays identical to the
// pre-Epic-2.3 direct-delegation ValidateGates implementation.
func builtInGuardGate(from, to BacklogStatus) (id, description string, ok bool) {
	switch {
	case from == BacklogStatusIdea && to == BacklogStatusReady:
		return "builtin:idea->ready:ac_required", "acceptance criteria required before marking ready", true
	case from == BacklogStatusReady && to == BacklogStatusInProgress,
		from == BacklogStatusQueued && to == BacklogStatusInProgress:
		return "builtin:->in_progress:plan_and_blockers", "no unresolved blockers, and plan approved (or skipped) with artifacts recorded", true
	case from == BacklogStatusRefining && to == BacklogStatusReady:
		return "builtin:refining->ready:ac_required", "acceptance criteria required before marking ready", true
	case (from == BacklogStatusReview || from == BacklogStatusPRPending) && to == BacklogStatusReady:
		return "builtin:->ready:verdict_clear", "a recorded PASS verdict must be cleared (or overridden) before returning to ready", true
	case to == BacklogStatusDone:
		return "builtin:->done:verdict_and_shipped", "a PASS verdict and shipped code are required before marking done", true
	default:
		return "", "", false
	}
}

// ValidateGates implements WorkflowEngine as a thin wrapper over PendingGates
// (ADR-002's Decision 2). Re-invokes TransitionGuard directly on the
// unsatisfied path (rather than synthesizing an error from GateStatus) so
// callers matching on TransitionGuard's specific sentinel errors (ErrACRequired,
// ErrVerdictRequired, etc. — session/domain/backlog.go) keep seeing the exact
// same error values as before this method became a wrapper.
func (e *DefaultWorkflowEngine) ValidateGates(item BacklogItemTransitionInput, to BacklogStatus) error {
	statuses, err := e.PendingGates(item, to)
	if err != nil {
		return err
	}
	for _, s := range statuses {
		if !s.Satisfied {
			return TransitionGuard(item, to)
		}
	}
	return nil
}

// AllowedTransitions implements WorkflowEngine. fallback is unused — see
// CanTransition's doc comment above.
func (e *DefaultWorkflowEngine) AllowedTransitions(from BacklogStatus, _ ...*StageConfigSnapshot) []BacklogStatus {
	targets, ok := e.transitions[from]
	if !ok {
		return []BacklogStatus{}
	}
	result := make([]BacklogStatus, 0, len(targets))
	for to, allowed := range targets {
		if allowed {
			result = append(result, to)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

// GuardedTransitionAllowed evaluates whether a transition is both structurally
// valid (CanTransition) and passes business-rule gates (ValidateGates),
// WITHOUT executing it — the read-only counterpart to transitionWithGuard
// (server/services/backlog_service_triage.go), for callers in package
// session (like SyncOne) that cannot import server/services.
func GuardedTransitionAllowed(engine WorkflowEngine, item BacklogItemTransitionInput, to BacklogStatus) bool {
	if !engine.CanTransition(item.Status, to) {
		return false
	}
	return engine.ValidateGates(item, to) == nil
}
