package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

func listedFlag(t *testing.T, svc *FeatureFlagService, name string) *sessionv1.FeatureFlag {
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
	e.svc.MergeAuditPolicies(notificationActionsFlagAuditPolicies())
	wireNotificationActionFlags(e.svc)
	return e
}

func TestFeatureFlagRegistry_ShouldListFourFlagsWithDefaultsAndScopes_WhenGetFeatureFlagsCalled(t *testing.T) {
	e := newNotificationFlagEnv(t)
	want := map[string]struct {
		on     bool
		scoped bool
	}{
		backgroundActivityFeedFlagName:    {true, false},
		notificationInlineActionsFlagName: {false, true},
		askUserQuestionStructuredFlagName: {false, true},
		agentWakeTriggersFlagName:         {false, true},
	}
	for name, w := range want {
		assert.Equal(t, w.on, listedFlag(t, e.svc, name).Enabled, name)
		var accepted []string
		for _, kf := range knownFeatureFlags {
			if kf.name == name {
				accepted = kf.scopes
			}
		}
		assert.Equal(t, w.scoped, len(accepted) > 0, name)
	}
	assert.Empty(t, listedFlag(t, e.svc, backgroundActivityFeedFlagName).StatusDetail, "ON has nothing to explain")
	for _, name := range []string{notificationInlineActionsFlagName, askUserQuestionStructuredFlagName, agentWakeTriggersFlagName} {
		assert.NotEmpty(t, listedFlag(t, e.svc, name).StatusDetail, name)
	}
}

func TestUpdateFeatureFlag_ShouldRefuseLooseningFlip_WhenAuditSinkDown(t *testing.T) {
	e := newNotificationFlagEnv(t)
	require.NoError(t, e.flip(backgroundActivityFeedFlagName, false)) // tightening: never waits on the sink
	assert.False(t, listedFlag(t, e.svc, backgroundActivityFeedFlagName).Enabled)
	assert.Contains(t, listedFlag(t, e.svc, backgroundActivityFeedFlagName).StatusDetail, "refused while the audit sink is down")

	e.mfs.setFailWrite(errDiskFull)
	err := e.flip(backgroundActivityFeedFlagName, true)
	require.Error(t, err)
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	v, ok := overrideOf(backgroundActivityFeedFlagName)
	assert.True(t, ok && !v, "the flag keeps its prior explicit OFF")
	assert.False(t, listedFlag(t, e.svc, backgroundActivityFeedFlagName).Enabled)
}

func TestUpdateFeatureFlag_ShouldReflectWithoutRestartInDispatcherStub_WhenAgentWakeTriggersFlipped(t *testing.T) {
	e := newNotificationFlagEnv(t)
	dispatcherStub := func() bool { return effectiveFlag(agentWakeTriggersFlagName, "dispatcher") }
	require.False(t, dispatcherStub())

	require.NoError(t, e.flip(agentWakeTriggersFlagName, true))
	got := listedFlag(t, e.svc, agentWakeTriggersFlagName)
	assert.True(t, got.Enabled)
	assert.True(t, dispatcherStub(), "read at decision time, no restart")

	require.NoError(t, e.flip(agentWakeTriggersFlagName, false))
	assert.False(t, dispatcherStub())
	assert.NotEmpty(t, listedFlag(t, e.svc, agentWakeTriggersFlagName).StatusDetail)
}

func TestUpdateFeatureFlag_ShouldSetAndClearNewScopes_WhenScopeIsRegistered(t *testing.T) {
	e := newNotificationFlagEnv(t)
	req := func(m sessionv1.FlagMutation, scope string) error {
		_, err := e.svc.UpdateFeatureFlag(context.Background(), connect.NewRequest(&sessionv1.UpdateFeatureFlagRequest{
			Name: askUserQuestionStructuredFlagName, Scope: scope, Mutation: m,
		}))
		return err
	}
	require.NoError(t, req(sessionv1.FlagMutation_FLAG_MUTATION_SET_ENABLED, scopeStageRender))
	assert.True(t, effectiveFlag(askUserQuestionStructuredFlagName, scopeStageRender))
	assert.False(t, effectiveFlag(askUserQuestionStructuredFlagName, scopeStageAnswer))
	require.NoError(t, req(sessionv1.FlagMutation_FLAG_MUTATION_CLEAR_SCOPE, scopeStageRender))
	assert.False(t, effectiveFlag(askUserQuestionStructuredFlagName, scopeStageRender))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(req(sessionv1.FlagMutation_FLAG_MUTATION_SET_ENABLED, "stage:bogus")))
}

func TestAskUserQuestionStructured_ShouldGateStagesInOrder_WhenRenderAnswerHookReplyScopesSet(t *testing.T) {
	e := newNotificationFlagEnv(t)
	set := func(scope string) {
		_, err := e.svc.UpdateFeatureFlag(context.Background(), connect.NewRequest(&sessionv1.UpdateFeatureFlagRequest{
			Name: askUserQuestionStructuredFlagName, Scope: scope, Enabled: true,
			Mutation: sessionv1.FlagMutation_FLAG_MUTATION_SET_ENABLED,
		}))
		require.NoError(t, err)
	}
	n := askUserQuestionStructuredFlagName
	set(scopeStageAnswer)
	assert.False(t, effectiveFlag(n, scopeStageAnswer), "answer without render is ignored")
	set(scopeStageRender)
	assert.True(t, effectiveFlag(n, scopeStageAnswer))
	set(scopeStageHookReply)
	assert.False(t, effectiveFlag(n, scopeStageHookReply), "closed until S1 PASS is recorded")
	assert.Equal(t, spikeS1HookReplyPassed, effectiveFlag(n, scopeStageHookReply))
	assert.False(t, config.LoadConfig().GetFeatureFlagWithDefault(n, false), "global stayed off throughout")
}

func TestEffectiveAnswerQuestion_ShouldBeClosed_WhenNothingConfigured(t *testing.T) {
	withIsolatedStateDir(t)
	for _, r := range []answerRoute{routeLiveKeystrokeHidden, routeLiveKeystrokeVisible, routeDurableRecord, routeHookReply} {
		assert.False(t, effectiveAnswerQuestion(r), "route %d", r)
	}
}
