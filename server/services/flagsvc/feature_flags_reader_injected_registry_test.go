package flagsvc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
)

// flippedDefaults is a registry shaped like the default one with every named flag's default inverted, so
// a read that reaches the default registry disagrees with it on exactly those flags.
func flippedDefaults(names ...string) *featureflags.Registry {
	flip := map[string]bool{}
	for _, n := range names {
		flip[n] = true
	}
	specs := testFlagRegistry().Specs()
	for i := range specs {
		if flip[specs[i].Name] {
			specs[i].Default = !specs[i].Default
		}
	}
	return featureflags.NewRegistry(specs)
}

func TestLoadAttentionFlags_ShouldUseInjectedDefaults_WhenRegistryInjected(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	reg := flippedDefaults(attentionFlagNames...)

	assert.Equal(t, AttentionFlags{AckBypassFix: true}, LoadAttentionFlags(testFlagRegistry()), "default registry: only AckBypassFix defaults on")
	assert.Equal(t, AttentionFlags{
		ProseQuestionDetection: true, SessionEventLog: true,
		IdleClockOutput: true, ProseQuestionLLMTiebreak: true, InboxConsolidation: true,
	}, LoadAttentionFlags(reg), "injected registry's defaults, not the global's")
}

func TestFlagReader_ShouldResolveDecisionPointsAgainstInjectedRegistry(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	reg := flippedDefaults(NotificationInlineActionsFlagName, AskUserQuestionStructuredFlagName,
		AgentWakeTriggersFlagName)
	injected, global := FlagReader{Reg: reg}, FlagReader{testFlagRegistry()}

	cases := []struct{ name, dp string }{
		{NotificationInlineActionsFlagName, ScopePrefixAction + "APPROVE"},
		{AskUserQuestionStructuredFlagName, ScopeStageRender},
		{AgentWakeTriggersFlagName, "dispatcher"},
		{AgentWakeTriggersFlagName, ScopePrefixTrigger + "at"},
	}
	for _, c := range cases {
		t.Run(c.name+"/"+c.dp, func(t *testing.T) {
			assert.False(t, global.EffectiveFlag(c.name, c.dp), "global registry default is off")
			assert.True(t, injected.EffectiveFlag(c.name, c.dp), "injected registry default is on")
		})
	}

	// hidden_session_reply defaults ON in the process registry, so the injected one (flipped OFF) must close the route.
	cfg := matrixCfg(NotificationInlineActionsFlagName, tv(true), map[string]tri{ScopePrefixAction + ActionKindAnswerQuestion: tv(true)})
	assert.True(t, FlagReader{testFlagRegistry()}.ResolveAnswerQuestion(cfg, CompiledFlagEvidence, RouteLiveKeystrokeHidden))
	assert.False(t, FlagReader{Reg: flippedDefaults(featureflags.HiddenSessionReplyFlagName)}.ResolveAnswerQuestion(cfg, CompiledFlagEvidence, RouteLiveKeystrokeHidden))
}

func TestWireNotificationActionFlags_ShouldSeedControllersFromInjectedRegistry(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	ff := NewFeatureFlagServiceWithRegistry(flippedDefaults(BackgroundActivityFeedFlagName, AgentWakeTriggersFlagName))
	WireNotificationActionFlags(ff)

	bg, wake := ff.featureControllers[BackgroundActivityFeedFlagName], ff.featureControllers[AgentWakeTriggersFlagName]
	require.NotNil(t, bg)
	require.NotNil(t, wake)
	assert.False(t, bg.IsEnabled(), "global default on, injected default off")
	assert.True(t, wake.IsEnabled(), "global default off, injected default on")
}
