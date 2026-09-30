package session

// gate_config.go — Task 2.7.2g's typed GateConfig sum type and
// ParseGateConfig validator. Each TransitionGate row's raw JSON `config`
// column (session/ent/schema/transition_gate.go) is kind-specific and must
// never be treated as free-form: an operator-authored "custom" gate is the
// project's designated escape hatch for pluggable checks, and the whole
// point of the "named, pre-registered skill/slash-command" design
// (requirements.md, Story 2.4.4) is to keep that hatch from becoming
// arbitrary command execution. This file is the structural boundary that
// enforces it — a config naming any key outside its kind's allowlist, or a
// "custom" gate naming a skill outside the pre-registered set, is rejected
// here, at parse/validate time, not left to whatever eventually reads the
// column.
//
// This package is storage-free like graph_validator.go: ParseGateConfig
// takes/returns plain Go values, so the gate-save RPC handler (Epic 2.7)
// controls when validation runs (before persisting) and Epic 2.4's
// InvokeCustomGateCheck can reuse it to decode an already-persisted row's
// config back into a typed value, without a second parsing path.

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// GateConfig is the sealed marker interface implemented by each GateKind's
// kind-specific config struct below. A switch over GateKind should also
// switch exhaustively when producing/consuming a GateConfig — see
// ParseGateConfig's own switch for the canonical exhaustive shape.
type GateConfig interface {
	isGateConfig()
}

// HumanApprovalConfig is GateKindHumanApproval's config. An explicit,
// recorded approval action (Epic 2.4's RecordGateApproval) needs no
// additional fields — this struct exists only to give the kind a config
// value at all, and to make its (empty) allowlist explicit.
type HumanApprovalConfig struct{}

func (HumanApprovalConfig) isGateConfig() {}

// AutomatedReviewConfig is GateKindAutomatedReview's config.
type AutomatedReviewConfig struct {
	// PipelineMode selects which PipelineMode's review prompt/verdict
	// machinery the review-gate runner (Epic 2.4.3) spawns against. Empty
	// means "use the item's own PipelineMode".
	PipelineMode string
	// RequiresDiff, when true, blocks the transition if the item's most
	// recent session has produced no diff yet — nothing for a reviewer to
	// look at.
	RequiresDiff bool
}

func (AutomatedReviewConfig) isGateConfig() {}

// StructuralConfig is GateKindStructural's config: a named, closed-set
// precondition check ID (e.g. "all_criteria_done"), evaluated directly
// against item state rather than via any spawned process.
type StructuralConfig struct {
	CheckID string
}

func (StructuralConfig) isGateConfig() {}

// CustomCheckConfig is GateKindCustom's config: a named, pre-registered
// skill/slash-command identifier that InvokeCustomGateCheck (Epic 2.4.4)
// spawns, bounded by a LivenessDefinition. SkillID must be a member of
// registeredCustomCheckSkills below — ParseGateConfig enforces this.
type CustomCheckConfig struct {
	SkillID string
}

func (CustomCheckConfig) isGateConfig() {}

// registeredCustomCheckSkills is Story 2.4.4's pre-registered allowlist of
// skill/slash-command identifiers a GateKindCustom gate may name. Epic 2.4
// (InvokeCustomGateCheck, not yet implemented on this branch) owns growing
// this set as new pluggable checks are wired up; it is seeded here, ahead of
// that epic, purely so this save-time validator has a concrete allowlist to
// enforce against per Task 2.7.2g2 instead of accepting any skill name.
var registeredCustomCheckSkills = map[string]bool{
	"review-feasibility": true,
}

// gateConfigAllowedKeys maps each GateKind to its allowlisted raw-JSON key
// set. A key outside this set is rejected by ParseGateConfig — this is what
// stops a "custom" gate config from smuggling an extra field (e.g. a raw
// shell command) past validation under an innocuous-looking name.
var gateConfigAllowedKeys = map[GateKind]map[string]bool{
	GateKindHumanApproval:   {},
	GateKindAutomatedReview: {"pipeline_mode": true, "requires_diff": true},
	GateKindStructural:      {"check_id": true},
	GateKindCustom:          {"skill": true},
}

// decodeConfigString decodes raw as a string, tolerating a raw JSON scalar
// (number/bool) by stringifying it — the gate-save RPC's wire config is a
// proto map<string,string>, so every value arrives JSON-marshaled as a
// quoted string, but a value read back from an already-persisted
// map[string]interface{} column (Epic 2.4's read path) may carry a native
// JSON type instead. Both must decode the same way.
func decodeConfigString(raw json.RawMessage) (string, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil
	}
	var anyVal interface{}
	if err := json.Unmarshal(raw, &anyVal); err != nil {
		return "", fmt.Errorf("invalid string value %s: %w", raw, err)
	}
	return fmt.Sprintf("%v", anyVal), nil
}

// decodeConfigBool decodes raw as a bool, tolerating the same wire-string
// form as decodeConfigString above (e.g. "true" from a proto
// map<string,string>) in addition to a native JSON boolean.
func decodeConfigBool(raw json.RawMessage) (bool, error) {
	var b bool
	if err := json.Unmarshal(raw, &b); err == nil {
		return b, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		parsed, parseErr := strconv.ParseBool(s)
		if parseErr != nil {
			return false, fmt.Errorf("invalid bool value %q: %w", s, parseErr)
		}
		return parsed, nil
	}
	return false, fmt.Errorf("invalid bool value: %s", raw)
}

// ParseGateConfig validates raw against kind's allowlisted key set and
// returns the corresponding typed GateConfig. It rejects (a) any key not in
// kind's allowlist and (b) for GateKindCustom, a skill not present in
// registeredCustomCheckSkills. Called from the gate-save RPC handler (Task
// 2.7.2g3) before persisting — not only at InvokeCustomGateCheck's
// invocation time — so a config mistake surfaces at the moment of the save
// that caused it, matching graph_validator.go's ValidateGraph convention.
func ParseGateConfig(kind GateKind, raw json.RawMessage) (GateConfig, error) {
	allowed, ok := gateConfigAllowedKeys[kind]
	if !ok {
		return nil, fmt.Errorf("gate_config: unknown gate kind %q", kind)
	}

	fields := map[string]json.RawMessage{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &fields); err != nil {
			return nil, fmt.Errorf("gate_config: invalid config JSON: %w", err)
		}
	}
	for key := range fields {
		if !allowed[key] {
			return nil, fmt.Errorf("gate_config: unrecognized key %q for gate kind %q", key, kind)
		}
	}

	switch kind {
	case GateKindHumanApproval:
		return HumanApprovalConfig{}, nil

	case GateKindAutomatedReview:
		cfg := AutomatedReviewConfig{}
		if v, ok := fields["pipeline_mode"]; ok {
			s, err := decodeConfigString(v)
			if err != nil {
				return nil, fmt.Errorf("gate_config: pipeline_mode: %w", err)
			}
			cfg.PipelineMode = s
		}
		if v, ok := fields["requires_diff"]; ok {
			b, err := decodeConfigBool(v)
			if err != nil {
				return nil, fmt.Errorf("gate_config: requires_diff: %w", err)
			}
			cfg.RequiresDiff = b
		}
		return cfg, nil

	case GateKindStructural:
		cfg := StructuralConfig{}
		if v, ok := fields["check_id"]; ok {
			s, err := decodeConfigString(v)
			if err != nil {
				return nil, fmt.Errorf("gate_config: check_id: %w", err)
			}
			cfg.CheckID = s
		}
		if cfg.CheckID == "" {
			return nil, fmt.Errorf("gate_config: structural gate requires a non-empty check_id")
		}
		return cfg, nil

	case GateKindCustom:
		cfg := CustomCheckConfig{}
		if v, ok := fields["skill"]; ok {
			s, err := decodeConfigString(v)
			if err != nil {
				return nil, fmt.Errorf("gate_config: skill: %w", err)
			}
			cfg.SkillID = s
		}
		if cfg.SkillID == "" {
			return nil, fmt.Errorf("gate_config: custom gate requires a non-empty skill")
		}
		if !registeredCustomCheckSkills[cfg.SkillID] {
			return nil, fmt.Errorf("gate_config: skill %q is not in the pre-registered allowlist", cfg.SkillID)
		}
		return cfg, nil

	default:
		return nil, fmt.Errorf("gate_config: unhandled gate kind %q", kind)
	}
}
