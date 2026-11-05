package services

// Story 1.5.9b: Success Metric 1 for the agent and system classes. A config edit
// (an operator gate, a disabled edge) must change what each class's writers can
// do, and the denial must reach a human instead of being swallowed. The operator
// class is proven by backlog_engine_enforcement_test.go (Story 1.2.4).
//
// The per-caller table drives the gate (Storage.Gated*) with each registered
// caller, the choke point every production writer calls; the writers that reach
// it through a handler are driven end to end in backlog_gate_operator_test.go and
// backlog_gate_system_test.go. Helpers carry the cls prefix.

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
)

// --- Task 1.5.9c: the four mode flags, read by the production class reader -------

// clsModeProbe is one class's view of the selector and what it can observe.
type clsModeProbe struct {
	class  session.CallerClass
	caller session.TransitionCaller
	flag   string
}

var clsModeProbes = []clsModeProbe{
	{session.ClassOperator, session.CallerOperatorTransitionRPC, config.WorkflowEngineEnforceFeatureFlag},
	{session.ClassAgent, session.CallerMCPResumeWork, config.WorkflowEngineEnforceAgentFeatureFlag},
	{session.ClassSystem, session.CallerAutonomousComplete, config.WorkflowEngineEnforceSystemFeatureFlag},
}

// clsSelector builds a selector over the production per-class reader after the
// flags in `set` were written through the production flag RPC.
func clsSelector(t *testing.T, set map[string]bool, opts ...session.SelectorOption) *session.SwitchableWorkflowEngine {
	t.Helper()
	svc := NewFeatureFlagService()
	for _, flag := range []string{
		config.WorkflowEngineShadowFeatureFlag, config.WorkflowEngineEnforceFeatureFlag,
		config.WorkflowEngineEnforceAgentFeatureFlag, config.WorkflowEngineEnforceSystemFeatureFlag,
	} {
		want, given := set[flag]
		if !given {
			want = flag == config.WorkflowEngineShadowFeatureFlag
		}
		require.NoError(t, flagUpdate(svc, flag, want))
	}
	clock := &modeTestClock{now: time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)}
	return session.NewSwitchableWorkflowEngine(session.NewDefaultWorkflowEngine(), nil, NewWorkflowEngineClassModeSource(config.LoadConfig, clock), opts...)
}

// clsEnforced reports whether the class's own answer is the enforced one: the
// operator class asks the engine today, so enforcement adds the system-required
// archived->archived edge; an unchecked agent or system caller is allowed an
// edge-less idea->done until its class enforces.
func clsEnforced(t *testing.T, sel *session.SwitchableWorkflowEngine, p clsModeProbe) bool {
	t.Helper()
	spec, ok := session.LookupTransitionCaller(p.caller)
	require.True(t, ok)
	view := sel.ForCaller(spec)
	if p.class == session.ClassOperator {
		return view.CanTransition(session.BacklogStatusArchived, session.BacklogStatusArchived, nil)
	}
	return !view.CanTransition(session.BacklogStatusIdea, session.BacklogStatusDone, nil)
}

func TestConfigSurfaceProof_WorkflowEngineShadowFlag_ChangesShadowCallsForEveryClass(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	for _, p := range clsModeProbes {
		svctest.AssertConfigChangesBehavior(t, func(setting any) any {
			rec := &countingShadow{}
			sel := clsSelector(t, map[string]bool{config.WorkflowEngineShadowFeatureFlag: setting.(bool)}, session.WithShadowRecorder(rec))
			spec, ok := session.LookupTransitionCaller(p.caller)
			require.True(t, ok)
			sel.ForCaller(spec).CanTransition(session.BacklogStatusIdea, session.BacklogStatusReady, nil)
			return rec.calls.Load() > 0
		}, false, true)
	}
}

func TestConfigSurfaceProof_WorkflowEngineEnforceFlag_ChangesOperatorAnswerAndNoOtherClass(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	op := clsModeProbes[0]
	svctest.AssertConfigChangesBehavior(t, func(setting any) any {
		return clsEnforced(t, clsSelector(t, map[string]bool{op.flag: setting.(bool)}), op)
	}, false, true)

	sel := clsSelector(t, map[string]bool{op.flag: true})
	for _, other := range clsModeProbes[1:] {
		assert.False(t, clsEnforced(t, sel, other), "the operator flag must not enforce the %s class", other.class)
	}
}

// TestWorkflowEngineModeFlags_EachEnforceFlagEnforcesExactlyItsOwnClass is the
// mode-behavior proof for all four flags: with one enforce flag on, only the
// class it names is enforced; shadow alone enforces nothing.
func TestWorkflowEngineModeFlags_EachEnforceFlagEnforcesExactlyItsOwnClass(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	none := clsSelector(t, nil)
	for _, p := range clsModeProbes {
		assert.False(t, clsEnforced(t, none, p), "defaults (shadow on, every enforce flag off) enforce no class: %s", p.class)
	}
	for _, flagged := range clsModeProbes {
		sel := clsSelector(t, map[string]bool{flagged.flag: true})
		for _, p := range clsModeProbes {
			assert.Equal(t, p.class == flagged.class, clsEnforced(t, sel, p), "%s on: class %s", flagged.flag, p.class)
		}
	}
}
