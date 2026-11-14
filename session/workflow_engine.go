package session

import (
	"sort"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

// DefaultWorkflowEngine implements WorkflowEngine using the hardcoded
// validTransitions map and TransitionGuard function from backlog.go.
type DefaultWorkflowEngine struct {
	transitions map[domain.BacklogStatus]map[domain.BacklogStatus]bool
}

// NewDefaultWorkflowEngine constructs an engine backed by the static
// validTransitions map. The map is deep-copied to avoid shared mutable state.
func NewDefaultWorkflowEngine() *DefaultWorkflowEngine {
	t := make(map[domain.BacklogStatus]map[domain.BacklogStatus]bool, len(instance.ValidTransitions))
	for from, targets := range instance.ValidTransitions {
		inner := make(map[domain.BacklogStatus]bool, len(targets))
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
func (e *DefaultWorkflowEngine) CanTransition(from, to domain.BacklogStatus, _ *domain.StageConfigSnapshot) bool {
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
func (e *DefaultWorkflowEngine) PendingGates(item domain.BacklogItemTransitionInput, to domain.BacklogStatus, _ *domain.StageConfigSnapshot) ([]domain.GateStatus, error) {
	id, description, ok := builtInGuardGate(item.Status, to)
	if !ok {
		return nil, nil
	}
	satisfied := domain.TransitionGuard(item, to) == nil
	hint := ""
	if !satisfied {
		hint = "resolve the blocking condition, or supply an override reason where supported"
	}
	return []domain.GateStatus{{
		GateID:      id,
		Kind:        domain.GateKindStructural,
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
func builtInGuardGate(from, to domain.BacklogStatus) (id, description string, ok bool) {
	switch {
	case from == domain.BacklogStatusIdea && to == domain.BacklogStatusReady:
		return "builtin:idea->ready:ac_required", "acceptance criteria required before marking ready", true
	case from == domain.BacklogStatusReady && to == domain.BacklogStatusInProgress,
		from == domain.BacklogStatusQueued && to == domain.BacklogStatusInProgress:
		return "builtin:->in_progress:plan_and_blockers", "no unresolved blockers, and plan approved (or skipped) with artifacts recorded", true
	case from == domain.BacklogStatusRefining && to == domain.BacklogStatusReady:
		return "builtin:refining->ready:ac_required", "acceptance criteria required before marking ready", true
	case (from == domain.BacklogStatusReview || from == domain.BacklogStatusPRPending) && to == domain.BacklogStatusReady:
		return "builtin:->ready:verdict_clear", "a recorded PASS verdict must be cleared (or overridden) before returning to ready", true
	case to == domain.BacklogStatusDone:
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
func (e *DefaultWorkflowEngine) ValidateGates(item domain.BacklogItemTransitionInput, to domain.BacklogStatus, _ *domain.StageConfigSnapshot) error {
	statuses, err := e.PendingGates(item, to, nil)
	if err != nil {
		return err
	}
	for _, s := range statuses {
		if !s.Satisfied {
			return domain.TransitionGuard(item, to)
		}
	}
	return nil
}

// AllowedTransitions implements WorkflowEngine. fallback is unused — see
// CanTransition's doc comment above.
func (e *DefaultWorkflowEngine) AllowedTransitions(from domain.BacklogStatus, _ *domain.StageConfigSnapshot) []domain.BacklogStatus {
	targets, ok := e.transitions[from]
	if !ok {
		return []domain.BacklogStatus{}
	}
	result := make([]domain.BacklogStatus, 0, len(targets))
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
func GuardedTransitionAllowed(engine domain.WorkflowEngine, item domain.BacklogItemTransitionInput, to domain.BacklogStatus) bool {
	if !engine.CanTransition(item.Status, to, nil) {
		return false
	}
	return engine.ValidateGates(item, to, nil) == nil
}
