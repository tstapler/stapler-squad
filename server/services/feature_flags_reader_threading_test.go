package services

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
)

// Constructor threading: each site below builds its flagReader from the registry it was given, so an
// injected registry whose default differs from the process one must change the answer.
var inlineApprove = scopePrefixAction + "APPROVE"

func inlineOnRegistry() *featureflags.Registry {
	return flippedDefaults(notificationInlineActionsFlagName, askUserQuestionStructuredFlagName)
}

func TestNewGuidanceHookMirror_ShouldGateOnInjectedRegistry(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	assert.True(t, NewGuidanceHookMirror(nil, nil, nil, nil, inlineOnRegistry()).enabled())
	assert.False(t, NewGuidanceHookMirror(nil, nil, nil, nil, nil).enabled())
}

func TestNewDescriptorBuilder_ShouldFlagOnInjectedRegistry(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	assert.True(t, NewDescriptorBuilder(DescriptorSources{}, inlineOnRegistry()).flag(inlineApprove))
	assert.False(t, NewDescriptorBuilder(DescriptorSources{}, nil).flag(inlineApprove))
}

func TestNewNotificationActionDispatcher_ShouldUseInjectedRegistry(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	assert.True(t, NewNotificationActionDispatcher(ActionDelegates{}, nil, inlineOnRegistry()).flags.Inline(inlineApprove))
	assert.False(t, NewNotificationActionDispatcher(ActionDelegates{}, nil, nil).flags.Inline(inlineApprove))
}

func TestWireAnswerRouter_ShouldUseServiceRegistry(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	assert.True(t, newGatedSessionService(svctest.CreateTestStorage(t), inlineOnRegistry()).WireAnswerRouter(AnswerRouterDeps{}).flags.Inline(inlineApprove))
	assert.False(t, newGatedSessionService(svctest.CreateTestStorage(t), featureflags.Current()).WireAnswerRouter(AnswerRouterDeps{}).flags.Inline(inlineApprove))
}

func TestWireHookReply_ShouldUseServiceRegistry(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	injected := newGatedSessionService(svctest.CreateTestStorage(t), inlineOnRegistry())
	injected.wireHookReply(svctest.CreateTestStorage(t), nil)
	assert.True(t, injected.hookReply.Load().flags.Inline(inlineApprove))
	global := newGatedSessionService(svctest.CreateTestStorage(t), featureflags.Current())
	global.wireHookReply(svctest.CreateTestStorage(t), nil)
	assert.False(t, global.hookReply.Load().flags.Inline(inlineApprove))
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
	}{{"injected", inlineOnRegistry(), true}, {"process", featureflags.Current(), false}} {
		t.Run(tc.name, func(t *testing.T) {
			svc := newGatedSessionService(svctest.CreateTestStorage(t), tc.reg)
			svc.WireNotificationActions(NotificationActionWiring{})
			assert.Equal(t, tc.want, svc.notificationSvc.descriptors.flag(inlineApprove), "descriptor builder")
			assert.Equal(t, tc.want, svc.actionDispatcher.flags.Inline(inlineApprove), "dispatcher")
		})
	}
}
