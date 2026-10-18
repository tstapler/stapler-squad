package session

import "sort"

// FallbackReason says why the enforce composition could not use the
// Configured engine and answered from the Default engine instead.
type FallbackReason string

const (
	FallbackCfgNil     FallbackReason = "cfg_nil"
	FallbackCacheEmpty FallbackReason = "cache_empty"
	FallbackLoadError  FallbackReason = "load_error"
)

// DenialSource attributes an enforce-mode denial to the layer that made it.
type DenialSource string

const (
	DenialConfigGraph  DenialSource = "config_graph"
	DenialDefaultGuard DenialSource = "default_guard"
	DenialOperatorGate DenialSource = "operator_gate"
)

// gateEvalMode says whether Configured stateful gates may read the database.
type gateEvalMode int

const (
	evalAllGates       gateEvalMode = iota // Enforce: evaluate every gate kind
	evalStructuralOnly                     // Shadow: skip stateful gate kinds, no I/O
)

// enforceAnswer is the single implementation of what Enforce returns and what
// Shadow compares Default against (ADR-001):
//
//   - graph: Configured graph UNION SystemRequiredEdges(); when Configured is
//     unusable, Default graph UNION SystemRequiredEdges();
//   - guards: Default TransitionGuard on the input unchanged, AND Configured
//     operator gates, except a non-empty OverrideReason skips the Configured
//     gates (never the graph, never a Default guard).
type enforceAnswer struct {
	def WorkflowEngine
	cfg *ConfiguredWorkflowEngine
}

// usableConfigured returns cfg when it can answer the graph, else the reason
// it cannot.
func (a enforceAnswer) usableConfigured() (*ConfiguredWorkflowEngine, FallbackReason) {
	switch {
	case a.cfg == nil:
		return nil, FallbackCfgNil
	case a.cfg.HasAnyStage():
		return a.cfg, ""
	case a.cfg.LoadError() != nil:
		return nil, FallbackLoadError
	default:
		return nil, FallbackCacheEmpty
	}
}

func (a enforceAnswer) canTransition(from, to BacklogStatus, snap *StageConfigSnapshot) (bool, FallbackReason) {
	cfg, reason := a.usableConfigured()
	var allowed bool
	if cfg != nil {
		allowed = cfg.CanTransition(from, to, snap)
	} else {
		allowed = a.def.CanTransition(from, to, snap)
	}
	return allowed || IsSystemRequired(from, to), reason
}

// allowedTransitions unions the non-archive system edges into the base list;
// archive edges never reach this UI-facing list.
func (a enforceAnswer) allowedTransitions(from BacklogStatus, snap *StageConfigSnapshot) ([]BacklogStatus, FallbackReason) {
	cfg, reason := a.usableConfigured()
	var base []BacklogStatus
	if cfg != nil {
		base = cfg.AllowedTransitions(from, snap)
	} else {
		base = a.def.AllowedTransitions(from, snap)
	}
	seen := make(map[BacklogStatus]bool, len(base))
	out := make([]BacklogStatus, 0, len(base)+2)
	for _, to := range append(base, systemRequiredTargets(from)...) {
		if !seen[to] {
			seen[to] = true
			out = append(out, to)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, reason
}

// gateAnswer is the outcome of the guard half of enforceAnswer.
type gateAnswer struct {
	Err error
	// Source is set only when Err is non-nil.
	Source DenialSource
	// SkippedStateful reports that a stateful gate was left unevaluated
	// because the caller asked for no I/O.
	SkippedStateful bool
	Fallback        FallbackReason
}

// validateGates runs Default guards first so their sentinels keep precedence,
// then the Configured operator gates. evalStructuralOnly forbids any
// GateSatisfactionRepository read (shadow).
func (a enforceAnswer) validateGates(item BacklogItemTransitionInput, to BacklogStatus, snap *StageConfigSnapshot, mode gateEvalMode) gateAnswer {
	cfg, reason := a.usableConfigured()
	if err := a.def.ValidateGates(item, to, nil); err != nil {
		return gateAnswer{Err: err, Source: DenialDefaultGuard, Fallback: reason}
	}
	if cfg == nil || item.OverrideReason != "" {
		return gateAnswer{Fallback: reason}
	}
	statuses, skipped := configuredGates(cfg, item, to, snap, mode)
	if err := firstUnsatisfiedGateError(statuses); err != nil {
		return gateAnswer{Err: err, Source: DenialOperatorGate, SkippedStateful: skipped}
	}
	return gateAnswer{SkippedStateful: skipped}
}

// pendingGates is Default's guard statuses plus the Configured operator gates
// (omitted under override or when Configured is unusable).
func (a enforceAnswer) pendingGates(item BacklogItemTransitionInput, to BacklogStatus, snap *StageConfigSnapshot, mode gateEvalMode) (statuses []GateStatus, skippedStateful bool, fallback FallbackReason, err error) {
	cfg, reason := a.usableConfigured()
	statuses, err = a.def.PendingGates(item, to, nil)
	if err != nil || cfg == nil || item.OverrideReason != "" {
		return statuses, false, reason, err
	}
	extra, skipped := configuredGates(cfg, item, to, snap, mode)
	return append(statuses, extra...), skipped, reason, nil
}

func configuredGates(cfg *ConfiguredWorkflowEngine, item BacklogItemTransitionInput, to BacklogStatus, snap *StageConfigSnapshot, mode gateEvalMode) ([]GateStatus, bool) {
	if mode == evalAllGates {
		statuses, _ := cfg.PendingGates(item, to, snap)
		return statuses, false
	}
	return cfg.PendingGatesStructuralOnly(item, to, snap)
}
