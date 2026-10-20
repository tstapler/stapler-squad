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
//     operator gates, except a non-empty OverrideReason on an operator call
//     skips the Configured gates and the Configured graph (never a Default
//     guard, never a system-required edge's protection in the CRUD RPC).
//
// The *For methods apply a callPolicy (ADR-007 D3); allowedTransitions is the
// operator-policy read the seed preview uses.
type enforceAnswer struct {
	def WorkflowEngine
	cfg *ConfiguredWorkflowEngine
}

// callPolicy is the caller-class half of enforceAnswer (ADR-007 D3). The zero
// value fails closed to the agent semantics.
//
//	operator: Default guards see the input unchanged; a non-empty OverrideReason
//	          also bypasses Configured operator gates and the graph, provided the
//	          target is a known stage (owner decision 2026-10-18, option B).
//	agent:    Default guards see the input unchanged, so OverrideReason frees only
//	          the exemptions the Default guard itself grants; it never bypasses a
//	          Configured gate, the graph, or an AC/plan/blocker denial.
//	system:   no override exists (stripped); Default guards run only when the
//	          registry says GuardsBuiltIn; Configured gates always apply.
//
// A forwarding caller (the generic MCP delegate) is governed by the Configured
// graph alone: it gets no SystemRequiredEdges() union.
type callPolicy struct {
	Class      CallerClass
	Guards     GuardPolicy
	Forwarding bool
}

var operatorPolicy = callPolicy{Class: ClassOperator, Guards: GuardsBuiltIn}

func (p callPolicy) runsDefaultGuards() bool { return p.Guards != GuardsNone }

// defaultGuardInput is the input the Default TransitionGuard sees.
func (p callPolicy) defaultGuardInput(item BacklogItemTransitionInput) BacklogItemTransitionInput {
	if p.Class == ClassSystem {
		item.OverrideReason = ""
	}
	return item
}

// bypassesOperatorGates reports whether item's override frees Configured gates.
func (p callPolicy) bypassesOperatorGates(item BacklogItemTransitionInput) bool {
	return p.Class == ClassOperator && item.OverrideReason != ""
}

// bypassesGraph reports whether an override frees the graph check: operator
// class only, non-empty reason, and only toward a known stage so an override
// cannot write a status that exists nowhere.
func (a enforceAnswer) bypassesGraph(pol callPolicy, overrideReason string, to BacklogStatus) bool {
	return pol.Class == ClassOperator && overrideReason != "" && a.knownStage(to)
}

func (a enforceAnswer) knownStage(s BacklogStatus) bool {
	if cfg, _ := a.usableConfigured(); cfg != nil {
		return cfg.cache.HasStage(s)
	}
	if def, ok := a.def.(*DefaultWorkflowEngine); ok {
		_, known := def.transitions[s]
		return known
	}
	return false
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

func (a enforceAnswer) canTransitionFor(pol callPolicy, from, to BacklogStatus, snap *StageConfigSnapshot) (bool, FallbackReason) {
	cfg, reason := a.usableConfigured()
	var allowed bool
	if cfg != nil {
		allowed = cfg.CanTransition(from, to, snap)
	} else {
		allowed = a.def.CanTransition(from, to, snap)
	}
	return allowed || (!pol.Forwarding && IsSystemRequired(from, to)), reason
}

// allowedTransitions unions the non-archive system edges into the base list;
// archive edges never reach this UI-facing list.
func (a enforceAnswer) allowedTransitions(from BacklogStatus, snap *StageConfigSnapshot) ([]BacklogStatus, FallbackReason) {
	return a.allowedTransitionsFor(operatorPolicy, from, snap)
}

func (a enforceAnswer) allowedTransitionsFor(pol callPolicy, from BacklogStatus, snap *StageConfigSnapshot) ([]BacklogStatus, FallbackReason) {
	cfg, reason := a.usableConfigured()
	var base []BacklogStatus
	if cfg != nil {
		base = cfg.AllowedTransitions(from, snap)
	} else {
		base = a.def.AllowedTransitions(from, snap)
	}
	seen := make(map[BacklogStatus]bool, len(base))
	out := make([]BacklogStatus, 0, len(base)+2)
	var system []BacklogStatus
	if !pol.Forwarding {
		system = systemRequiredTargets(from)
	}
	for _, to := range append(base, system...) {
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
func (a enforceAnswer) validateGatesFor(pol callPolicy, item BacklogItemTransitionInput, to BacklogStatus, snap *StageConfigSnapshot, mode gateEvalMode) gateAnswer {
	cfg, reason := a.usableConfigured()
	if pol.runsDefaultGuards() {
		if err := a.def.ValidateGates(pol.defaultGuardInput(item), to, nil); err != nil {
			return gateAnswer{Err: err, Source: DenialDefaultGuard, Fallback: reason}
		}
	}
	if cfg == nil || pol.bypassesOperatorGates(item) {
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
func (a enforceAnswer) pendingGatesFor(pol callPolicy, item BacklogItemTransitionInput, to BacklogStatus, snap *StageConfigSnapshot, mode gateEvalMode) (statuses []GateStatus, skippedStateful bool, fallback FallbackReason, err error) {
	cfg, reason := a.usableConfigured()
	if pol.runsDefaultGuards() {
		statuses, err = a.def.PendingGates(pol.defaultGuardInput(item), to, nil)
	}
	if err != nil || cfg == nil || pol.bypassesOperatorGates(item) {
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
