package session

import "sort"

// WorkflowEngine is the policy object that governs which backlog status
// transitions are permitted and what guards must pass.
type WorkflowEngine interface {
	// CanTransition returns true if transitioning from → to is structurally allowed.
	CanTransition(from, to BacklogStatus) bool
	// ValidateGates runs guard rules for the transition. Returns nil if gates pass.
	ValidateGates(item BacklogItemTransitionInput, to BacklogStatus) error
	// AllowedTransitions returns the set of statuses reachable from from.
	AllowedTransitions(from BacklogStatus) []BacklogStatus
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

// CanTransition implements WorkflowEngine.
func (e *DefaultWorkflowEngine) CanTransition(from, to BacklogStatus) bool {
	targets, ok := e.transitions[from]
	if !ok {
		return false
	}
	return targets[to]
}

// ValidateGates implements WorkflowEngine by delegating to TransitionGuard.
func (e *DefaultWorkflowEngine) ValidateGates(item BacklogItemTransitionInput, to BacklogStatus) error {
	return TransitionGuard(item, to)
}

// AllowedTransitions implements WorkflowEngine.
func (e *DefaultWorkflowEngine) AllowedTransitions(from BacklogStatus) []BacklogStatus {
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
