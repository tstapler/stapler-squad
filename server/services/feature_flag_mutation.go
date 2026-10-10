package services

import (
	"context"
	"fmt"
	"strings"

	"connectrpc.com/connect"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/deliverygate"
)

// globalScopeLiteral is the scope a client sends for a global set, so a client
// that drops `scope` on a per-kind click is refused instead of writing the
// global value.
const globalScopeLiteral = "global"

// ScopedFeatureController is an optional extension of FeatureController for a
// flag whose in-process component reacts to a per-scope change. A failure rolls
// the scoped key back (by deletion when it was absent).
type ScopedFeatureController interface {
	ApplyScope(scope string, enabled bool) error
}

// gateFlagScopes lists the scopes hidden_session_gate accepts: the closed set
// of reachable hidden kinds (deliverygate.ScopableKinds).
func gateFlagScopes() []string {
	out := make([]string, 0, len(deliverygate.ScopableKinds))
	for _, k := range deliverygate.ScopableKinds {
		out = append(out, deliverygate.ScopeOf(k))
	}
	return out
}

// flagOp is one validated UpdateFeatureFlag request. A legacy request (no
// scope, no mutation) is normalized to a global set.
type flagOp struct {
	name     string
	mutation sessionv1.FlagMutation
	scope    string // "" for a global operation, "kind:<name>" otherwise
	enabled  bool   // the value of a set
}

func (o flagOp) isSet() bool {
	return o.mutation == sessionv1.FlagMutation_FLAG_MUTATION_SET_ENABLED ||
		o.mutation == sessionv1.FlagMutation_FLAG_MUTATION_SET_DISABLED
}

// enabling reports whether the operation turns something on (the guarded way).
func (o flagOp) enabling() bool {
	return o.mutation == sessionv1.FlagMutation_FLAG_MUTATION_SET_ENABLED
}

// clearTurnsKindOn reports whether a CLEAR_SCOPE removes an explicit false
// override while the global value is on, so the kind starts following global on.
func clearTurnsKindOn(o flagOp) bool {
	if o.mutation != sessionv1.FlagMutation_FLAG_MUTATION_CLEAR_SCOPE {
		return false
	}
	cfg := config.LoadConfig()
	override, ok := cfg.GetFeatureFlagScopedOverride(o.name, o.scope)
	return ok && !override && cfg.GetFeatureFlagWithDefault(o.name, featureFlagDefault(o.name))
}

func (o flagOp) mutationName() string {
	return strings.TrimPrefix(o.mutation.String(), "FLAG_MUTATION_")
}

// auditScope is the scope recorded on audit lines and history entries.
func (o flagOp) auditScope() string {
	if o.scope == "" {
		return globalScopeLiteral
	}
	return o.scope
}

func invalidFlagRequest(format string, args ...any) error {
	return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf(format, args...))
}

// parseFlagOp validates scope and mutation against the flag's registry entry.
// Nothing is read or written here: a refused request changes no persisted value.
func parseFlagOp(req *sessionv1.UpdateFeatureFlagRequest, scopes []string) (flagOp, error) {
	op := flagOp{name: req.GetName(), mutation: req.GetMutation(), scope: req.GetScope(), enabled: req.GetEnabled()}
	validKind := func() error {
		for _, s := range scopes {
			if s == op.scope {
				return nil
			}
		}
		if len(scopes) == 0 {
			return invalidFlagRequest("feature flag %q has no scopes: %q is not accepted", op.name, op.scope)
		}
		return invalidFlagRequest("unknown scope %q for %q: valid scopes are %v", op.scope, op.name, scopes)
	}
	switch op.mutation {
	case sessionv1.FlagMutation_FLAG_MUTATION_UNSPECIFIED:
		if op.scope != "" {
			return op, invalidFlagRequest("scope %q needs an explicit mutation", op.scope)
		}
		op.mutation = sessionv1.FlagMutation_FLAG_MUTATION_SET_DISABLED
		if op.enabled {
			op.mutation = sessionv1.FlagMutation_FLAG_MUTATION_SET_ENABLED
		}
	case sessionv1.FlagMutation_FLAG_MUTATION_SET_ENABLED, sessionv1.FlagMutation_FLAG_MUTATION_SET_DISABLED:
		op.enabled = op.enabling()
		switch op.scope {
		case "":
			return op, invalidFlagRequest("a set needs a scope: %q for the global value or one of %v", globalScopeLiteral, scopes)
		case globalScopeLiteral:
			op.scope = ""
		default:
			if err := validKind(); err != nil {
				return op, err
			}
		}
	case sessionv1.FlagMutation_FLAG_MUTATION_CLEAR_SCOPE:
		if !strings.HasPrefix(op.scope, "kind:") {
			return op, invalidFlagRequest("CLEAR_SCOPE needs a kind: scope, got %q", op.scope)
		}
		if err := validKind(); err != nil {
			return op, err
		}
	case sessionv1.FlagMutation_FLAG_MUTATION_RESET_GLOBAL:
		if op.scope != "" {
			return op, invalidFlagRequest("RESET_GLOBAL takes no scope, got %q", op.scope)
		}
		op.enabled = featureFlagDefault(op.name)
	default:
		return op, invalidFlagRequest("unknown flag mutation %d", int32(op.mutation))
	}
	return op, nil
}

// flagScopeReadback lists the persisted scope values of a scopable flag in a
// stable order, restricted to the registered scopes (a hand-edited key outside
// the closed set is neither reported nor applied).
func flagScopeReadback(cfg *config.Config, name string, scopes []string) []*sessionv1.FeatureFlagScopeOverride {
	var out []*sessionv1.FeatureFlagScopeOverride
	for _, s := range scopes {
		if v, ok := cfg.GetFeatureFlagScopedOverride(name, s); ok {
			out = append(out, &sessionv1.FeatureFlagScopeOverride{Scope: s, Enabled: v})
		}
	}
	return out
}

// persistScoped writes or clears one scope key. The caller holds updateMu. The
// previous effective value (the inherited global when the key was absent) goes
// to the audit line; a failed controller apply restores or deletes the key.
func (f *FeatureFlagService) persistScoped(ctx context.Context, op flagOp, audit *flagAudit) error {
	cfg := config.LoadConfig()
	previous, hadKey := cfg.GetFeatureFlagScopedOverride(op.name, op.scope)
	if !hadKey {
		previous = cfg.GetFeatureFlagWithDefault(op.name, featureFlagDefault(op.name))
	}
	if audit.active() {
		f.flagSeq++
		audit.seq, audit.previous = f.flagSeq, previous
	}
	var err error
	if op.isSet() {
		err = cfg.SetFeatureFlagScope(op.name, op.scope, op.enabled)
	} else {
		err = cfg.DeleteFeatureFlagScope(op.name, op.scope)
	}
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to persist feature flag scope: %w", err))
	}
	effective := op.enabled
	if !op.isSet() {
		effective = cfg.GetFeatureFlagWithDefault(op.name, featureFlagDefault(op.name))
		if audit.active() {
			audit.enabled = effective
		}
	}
	sc, ok := f.featureControllers[op.name].(ScopedFeatureController)
	if !ok {
		f.finishApplied(op, audit)
		return nil
	}
	ctrlErr := sc.ApplyScope(op.scope, effective)
	if ctrlErr == nil {
		f.finishApplied(op, audit)
		return nil
	}
	log.Error("feature controller scope apply failed, rolling back persisted scope",
		"feature", op.name, "scope", op.scope, "err", ctrlErr)
	var rollbackErr error
	if hadKey {
		rollbackErr = cfg.SetFeatureFlagScope(op.name, op.scope, previous)
	} else {
		rollbackErr = cfg.DeleteFeatureFlagScope(op.name, op.scope)
	}
	if rollbackErr != nil {
		if audit.active() {
			audit.outcome = flagOutcomeControllerFailed
		}
		return connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to apply scope %q of %q: %w (rollback also failed, disk state may be inconsistent: %v)",
				op.scope, op.name, ctrlErr, rollbackErr))
	}
	if audit.active() {
		audit.outcome = flagOutcomeRolledBack
	}
	f.notifyObserver(op.name)
	return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to apply scope %q of %q: %w", op.scope, op.name, ctrlErr))
}

// explicitOffKinds lists the scopable kinds with a persisted explicit false
// override of the gate flag, in the registry's order.
func explicitOffKinds(cfg *config.Config) []deliverygate.HiddenKind {
	var out []deliverygate.HiddenKind
	for _, k := range deliverygate.ScopableKinds {
		if v, ok := cfg.GetFeatureFlagScopedOverride(config.HiddenSessionGateFeatureFlag, deliverygate.ScopeOf(k)); ok && !v {
			out = append(out, k)
		}
	}
	return out
}

// kindOffStatusDetail is the per-kind half of the explicit-false status line
// (Task 2.11g): one phrase per kind whose override is false. It is a source on
// the composite provider, so registration order cannot hide it.
func kindOffStatusDetail() string {
	kinds := explicitOffKinds(config.LoadConfig())
	parts := make([]string, 0, len(kinds))
	for _, k := range kinds {
		parts = append(parts, fmt.Sprintf("Gate is OFF for kind %s: hidden %s sessions deliver everything", k, k))
	}
	return strings.Join(parts, "; ")
}

const (
	gateShadowStatusDetail = "Shadow mode: hidden sessions still notify; would-suppress counts are logged"
	gateOffStatusDetail    = "Gate is OFF: hidden sessions deliver everything"
)

// globalOffStatusDetail is the global half of the off status line (FG-4): the
// effective global value is off and either never set (shadow mode) or
// explicitly false. It reads the effective value, so it goes silent once the
// default flips on.
func globalOffStatusDetail() string {
	cfg := config.LoadConfig()
	name := config.HiddenSessionGateFeatureFlag
	if cfg.GetFeatureFlagWithDefault(name, featureFlagDefault(name)) {
		return ""
	}
	if _, explicit := cfg.GetFeatureFlagOverride(name); explicit {
		return gateOffStatusDetail
	}
	return gateShadowStatusDetail
}
