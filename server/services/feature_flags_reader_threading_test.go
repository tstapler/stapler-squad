package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/server/services/flagsvc"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/server/services/questionkit"
)

// Constructor threading: each site below builds its flagReader from the registry it was given, so an
// injected registry whose default differs from the process one must change the answer.
var inlineApprove = flagsvc.ScopePrefixAction + "APPROVE"

func inlineOnRegistry() *featureflags.Registry {
	return flippedDefaults(flagsvc.NotificationInlineActionsFlagName, flagsvc.AskUserQuestionStructuredFlagName)
}

func TestNewGuidanceHookMirror_ShouldGateOnInjectedRegistry(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	assert.True(t, questionkit.NewGuidanceHookMirror(nil, nil, nil, nil, AskUserQuestionRenderGate(inlineOnRegistry())).Enabled())
	assert.False(t, questionkit.NewGuidanceHookMirror(nil, nil, nil, nil, AskUserQuestionRenderGate(testFlagRegistry())).Enabled())
}

func TestAskUserQuestionRenderGate_ShouldReadTheRenderStageOnly(t *testing.T) {
	e := newNotificationFlagEnv(t)
	set := func(scope string) {
		_, err := e.svc.UpdateFeatureFlag(context.Background(), connect.NewRequest(&sessionv1.UpdateFeatureFlagRequest{
			Name: flagsvc.AskUserQuestionStructuredFlagName, Scope: scope, Enabled: true,
			Mutation: sessionv1.FlagMutation_FLAG_MUTATION_SET_ENABLED,
		}))
		require.NoError(t, err)
	}
	gate := AskUserQuestionRenderGate(testFlagRegistry())
	assert.False(t, gate())
	set(flagsvc.ScopeStageRender)
	assert.True(t, gate(), "the gate is read per call, so a live flip applies; the answer stage is still off")
}

func TestAskUserQuestionRenderGate_ShouldStayClosed_WhenOnlyTheAnswerStageIsSet(t *testing.T) {
	e := newNotificationFlagEnv(t)
	_, err := e.svc.UpdateFeatureFlag(context.Background(), connect.NewRequest(&sessionv1.UpdateFeatureFlagRequest{
		Name: flagsvc.AskUserQuestionStructuredFlagName, Scope: flagsvc.ScopeStageAnswer, Enabled: true,
		Mutation: sessionv1.FlagMutation_FLAG_MUTATION_SET_ENABLED,
	}))
	require.NoError(t, err)
	assert.False(t, AskUserQuestionRenderGate(testFlagRegistry())())
}

func TestWireAnswerRouter_ShouldUseServiceRegistry(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	assert.True(t, newGatedSessionService(svctest.CreateTestStorage(t), inlineOnRegistry()).WireAnswerRouter(AnswerRouterDeps{}).flags.(interface{ Inline(string) bool }).Inline(inlineApprove))
	assert.False(t, newGatedSessionService(svctest.CreateTestStorage(t), testFlagRegistry()).WireAnswerRouter(AnswerRouterDeps{}).flags.(interface{ Inline(string) bool }).Inline(inlineApprove))
}

func TestWireHookReply_ShouldUseServiceRegistry(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	injected := newGatedSessionService(svctest.CreateTestStorage(t), inlineOnRegistry())
	injected.wireHookReply(svctest.CreateTestStorage(t), nil)
	assert.True(t, injected.hookReply.Load().flags.(interface{ Inline(string) bool }).Inline(inlineApprove))
	global := newGatedSessionService(svctest.CreateTestStorage(t), testFlagRegistry())
	global.wireHookReply(svctest.CreateTestStorage(t), nil)
	assert.False(t, global.hookReply.Load().flags.(interface{ Inline(string) bool }).Inline(inlineApprove))
}

func TestSessionService_ShouldNotTouchProcessRegistry_WhenNoneInjected(t *testing.T) {
	t.Parallel()
	var zero *SessionService
	assert.NotPanics(t, func() { _ = zero.flagReader() })
	assert.NotPanics(t, func() { _ = (&SessionService{}).flagReader() })
	assert.Nil(t, (&SessionService{}).InjectedFlagRegistry())
}

func TestWireNotificationActions_ShouldThreadServiceRegistryIntoBuilderAndDispatcher(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	for _, tc := range []struct {
		name string
		reg  *featureflags.Registry
		want bool
	}{{"injected", inlineOnRegistry(), true}, {"process", testFlagRegistry(), false}} {
		t.Run(tc.name, func(t *testing.T) {
			svc := newGatedSessionService(svctest.CreateTestStorage(t), tc.reg)
			svc.WireNotificationActions(NotificationActionWiring{})
			assert.Equal(t, tc.want, svc.notificationSvc.InlineActionEnabledForTest(inlineApprove), "descriptor builder")
			assert.Equal(t, tc.want, svc.actionDispatcher.InlineActionEnabledForTest(inlineApprove), "dispatcher")
		})
	}
}
