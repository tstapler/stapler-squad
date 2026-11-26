package services

import (
	"context"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/flagsvc"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session/domain"
)

func listedFlag(t *testing.T, svc *flagsvc.FeatureFlagService, name string) *sessionv1.FeatureFlag {
	t.Helper()
	resp, err := svc.GetFeatureFlags(context.Background(), connect.NewRequest(&sessionv1.GetFeatureFlagsRequest{}))
	require.NoError(t, err)
	for _, f := range resp.Msg.Flags {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("flag %q not listed", name)
	return nil
}

func newNotificationFlagEnv(t *testing.T) *auditEnv {
	t.Helper()
	e := newAuditEnv(t)
	e.svc.MergeAuditPolicies(flagsvc.NotificationActionsFlagAuditPolicies())
	flagsvc.WireNotificationActionFlags(e.svc)
	return e
}

func TestFeatureFlagRegistry_ShouldListFourFlagsWithDefaultsAndScopes_WhenGetFeatureFlagsCalled(t *testing.T) {
	e := newNotificationFlagEnv(t)
	want := map[string]struct {
		on     bool
		scoped bool
	}{
		flagsvc.BackgroundActivityFeedFlagName:    {true, false},
		flagsvc.NotificationInlineActionsFlagName: {false, true},
		flagsvc.AskUserQuestionStructuredFlagName: {false, true},
		flagsvc.AgentWakeTriggersFlagName:         {false, true},
	}
	for name, w := range want {
		assert.Equal(t, w.on, listedFlag(t, e.svc, name).Enabled, name)
		spec, found := testFlagRegistry().Lookup(name)
		require.True(t, found, "%s registered", name)
		assert.Equal(t, w.scoped, len(spec.Scopes) > 0, name)
	}
	assert.Empty(t, listedFlag(t, e.svc, flagsvc.BackgroundActivityFeedFlagName).StatusDetail, "ON has nothing to explain")
	for _, name := range []string{flagsvc.NotificationInlineActionsFlagName, flagsvc.AskUserQuestionStructuredFlagName, flagsvc.AgentWakeTriggersFlagName} {
		assert.NotEmpty(t, listedFlag(t, e.svc, name).StatusDetail, name)
	}
}

func TestUpdateFeatureFlag_ShouldRefuseLooseningFlip_WhenAuditSinkDown(t *testing.T) {
	e := newNotificationFlagEnv(t)
	require.NoError(t, e.flip(flagsvc.BackgroundActivityFeedFlagName, false)) // tightening: never waits on the sink
	assert.False(t, listedFlag(t, e.svc, flagsvc.BackgroundActivityFeedFlagName).Enabled)
	assert.Contains(t, listedFlag(t, e.svc, flagsvc.BackgroundActivityFeedFlagName).StatusDetail, "refused while the audit sink is down")

	e.mfs.SetFailWrite(errDiskFull)
	err := e.flip(flagsvc.BackgroundActivityFeedFlagName, true)
	require.Error(t, err)
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	assert.Contains(t, err.Error(), "flag change not audited, nothing persisted")
	v, ok := overrideOf(flagsvc.BackgroundActivityFeedFlagName)
	assert.True(t, ok && !v, "the flag keeps its prior explicit OFF")
	assert.False(t, listedFlag(t, e.svc, flagsvc.BackgroundActivityFeedFlagName).Enabled)
}

func TestUpdateFeatureFlag_ShouldReflectWithoutRestartInDispatcherStub_WhenAgentWakeTriggersFlipped(t *testing.T) {
	e := newNotificationFlagEnv(t)
	dispatcherStub := func() bool {
		return flagsvc.FlagReader{Reg: testFlagRegistry()}.EffectiveFlag(flagsvc.AgentWakeTriggersFlagName, "dispatcher")
	}
	require.False(t, dispatcherStub())

	require.NoError(t, e.flip(flagsvc.AgentWakeTriggersFlagName, true))
	got := listedFlag(t, e.svc, flagsvc.AgentWakeTriggersFlagName)
	assert.True(t, got.Enabled)
	assert.True(t, dispatcherStub(), "read at decision time, no restart")

	require.NoError(t, e.flip(flagsvc.AgentWakeTriggersFlagName, false))
	assert.False(t, dispatcherStub())
	assert.NotEmpty(t, listedFlag(t, e.svc, flagsvc.AgentWakeTriggersFlagName).StatusDetail)
}

func TestUpdateFeatureFlag_ShouldSetAndClearNewScopes_WhenScopeIsRegistered(t *testing.T) {
	e := newNotificationFlagEnv(t)
	req := func(m sessionv1.FlagMutation, scope string) error {
		_, err := e.svc.UpdateFeatureFlag(context.Background(), connect.NewRequest(&sessionv1.UpdateFeatureFlagRequest{
			Name: flagsvc.AskUserQuestionStructuredFlagName, Scope: scope, Mutation: m,
		}))
		return err
	}
	require.NoError(t, req(sessionv1.FlagMutation_FLAG_MUTATION_SET_ENABLED, flagsvc.ScopeStageRender))
	assert.True(t, flagsvc.FlagReader{Reg: testFlagRegistry()}.EffectiveFlag(flagsvc.AskUserQuestionStructuredFlagName, flagsvc.ScopeStageRender))
	assert.False(t, flagsvc.FlagReader{Reg: testFlagRegistry()}.EffectiveFlag(flagsvc.AskUserQuestionStructuredFlagName, flagsvc.ScopeStageAnswer))
	require.NoError(t, req(sessionv1.FlagMutation_FLAG_MUTATION_CLEAR_SCOPE, flagsvc.ScopeStageRender))
	assert.False(t, flagsvc.FlagReader{Reg: testFlagRegistry()}.EffectiveFlag(flagsvc.AskUserQuestionStructuredFlagName, flagsvc.ScopeStageRender))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(req(sessionv1.FlagMutation_FLAG_MUTATION_SET_ENABLED, "stage:bogus")))
}

func TestAskUserQuestionStructured_ShouldGateStagesInOrder_WhenRenderAnswerHookReplyScopesSet(t *testing.T) {
	e := newNotificationFlagEnv(t)
	set := func(scope string) {
		_, err := e.svc.UpdateFeatureFlag(context.Background(), connect.NewRequest(&sessionv1.UpdateFeatureFlagRequest{
			Name: flagsvc.AskUserQuestionStructuredFlagName, Scope: scope, Enabled: true,
			Mutation: sessionv1.FlagMutation_FLAG_MUTATION_SET_ENABLED,
		}))
		require.NoError(t, err)
	}
	n := flagsvc.AskUserQuestionStructuredFlagName
	set(flagsvc.ScopeStageAnswer)
	assert.False(t, flagsvc.FlagReader{Reg: testFlagRegistry()}.EffectiveFlag(n, flagsvc.ScopeStageAnswer), "answer without render is ignored")
	set(flagsvc.ScopeStageRender)
	assert.True(t, flagsvc.FlagReader{Reg: testFlagRegistry()}.EffectiveFlag(n, flagsvc.ScopeStageAnswer))
	set(flagsvc.ScopeStageHookReply)
	assert.False(t, flagsvc.FlagReader{Reg: testFlagRegistry()}.EffectiveFlag(n, flagsvc.ScopeStageHookReply), "closed until S1 PASS is recorded")
	assert.Equal(t, flagsvc.SpikeS1HookReplyPassed, flagsvc.FlagReader{Reg: testFlagRegistry()}.EffectiveFlag(n, flagsvc.ScopeStageHookReply))
	assert.False(t, config.LoadConfig().GetFeatureFlagWithDefault(n, false), "global stayed off throughout")
}

func TestEffectiveAnswerQuestion_ShouldBeClosed_WhenNothingConfigured(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	for _, r := range []flagsvc.AnswerRoute{flagsvc.RouteLiveKeystrokeHidden, flagsvc.RouteLiveKeystrokeVisible, flagsvc.RouteDurableRecord, flagsvc.RouteHookReply} {
		assert.False(t, flagsvc.FlagReader{Reg: testFlagRegistry()}.EffectiveAnswerQuestion(r), "route %d", r)
	}
}

func TestUpdateFeatureFlag_ShouldNotifyWakeListener_WhenAgentWakeTriggersFlippedGloballyOrByScope(t *testing.T) {
	e := newNotificationFlagEnv(t)
	var calls int
	var sawOn []bool
	flagsvc.SetWakeFlagListener(func() {
		calls++
		sawOn = append(sawOn, flagsvc.FlagReader{Reg: testFlagRegistry()}.EffectiveFlag(flagsvc.AgentWakeTriggersFlagName, "dispatcher"))
	})
	t.Cleanup(func() { flagsvc.SetWakeFlagListener(nil) })

	require.NoError(t, e.flip(flagsvc.AgentWakeTriggersFlagName, true))
	require.NoError(t, e.flip(flagsvc.AgentWakeTriggersFlagName, false))
	_, err := e.svc.UpdateFeatureFlag(context.Background(), connect.NewRequest(&sessionv1.UpdateFeatureFlagRequest{
		Name: flagsvc.AgentWakeTriggersFlagName, Scope: flagsvc.ScopePrefixTrigger + "at", Mutation: sessionv1.FlagMutation_FLAG_MUTATION_SET_ENABLED,
	}))
	require.NoError(t, err)

	assert.Equal(t, 3, calls)
	assert.Equal(t, []bool{true, false, true}, sawOn, "the listener sees the already-persisted value")
}

// TestAgentWakeTriggersFlagHasScopePerWakeTriggerType ties the registered
// agent_wake_triggers scopes to domain.AllWakeTriggerTypes in both directions.
// The "trigger:" literal is deliberate: it pins the wire format independently of
// scopePrefixTrigger.
func TestAgentWakeTriggersFlagHasScopePerWakeTriggerType(t *testing.T) {
	t.Parallel()
	var scopes []string
	for _, spec := range flagsvc.CoreFlagSpecs() {
		if spec.Name == flagsvc.AgentWakeTriggersFlagName {
			scopes = spec.Scopes
		}
	}
	require.NotEmpty(t, scopes, "%s spec not registered", flagsvc.AgentWakeTriggersFlagName)

	types := domain.AllWakeTriggerTypes()
	want := make([]string, 0, len(types))
	for _, tt := range types {
		want = append(want, "trigger:"+string(tt))
	}
	var got []string
	for _, s := range scopes {
		if strings.HasPrefix(s, "trigger:") {
			got = append(got, s)
		}
	}
	assert.Equal(t, want, got, "every wake trigger type needs exactly one trigger:<name> scope, in domain order")
}
