package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
)

// flippedDefaults is a registry shaped like the process one with every named flag's default inverted, so
// a read that reaches featureflags.Current() disagrees with it on exactly those flags.
func flippedDefaults(names ...string) *featureflags.Registry {
	flip := map[string]bool{}
	for _, n := range names {
		flip[n] = true
	}
	specs := featureflags.Current().Specs()
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

	assert.Equal(t, AttentionFlags{}, LoadAttentionFlags(nil), "process registry: all six default off")
	assert.Equal(t, AttentionFlags{
		ProseQuestionDetection: true, SessionEventLog: true, AckBypassFix: true,
		IdleClockOutput: true, ProseQuestionLLMTiebreak: true, InboxConsolidation: true,
	}, LoadAttentionFlags(reg), "injected registry's defaults, not the global's")
}

func TestFlagReader_ShouldResolveDecisionPointsAgainstInjectedRegistry(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	reg := flippedDefaults(notificationInlineActionsFlagName, askUserQuestionStructuredFlagName,
		agentWakeTriggersFlagName)
	injected, global := flagReader{reg: reg}, flagReader{}

	cases := []struct{ name, dp string }{
		{notificationInlineActionsFlagName, scopePrefixAction + "APPROVE"},
		{askUserQuestionStructuredFlagName, scopeStageRender},
		{agentWakeTriggersFlagName, "dispatcher"},
		{agentWakeTriggersFlagName, scopePrefixTrigger + "at"},
	}
	for _, c := range cases {
		t.Run(c.name+"/"+c.dp, func(t *testing.T) {
			assert.False(t, global.effectiveFlag(c.name, c.dp), "global registry default is off")
			assert.True(t, injected.effectiveFlag(c.name, c.dp), "injected registry default is on")
		})
	}

	// hidden_session_reply defaults ON in the process registry, so the injected one (flipped OFF) must close the route.
	cfg := matrixCfg(notificationInlineActionsFlagName, tv(true), map[string]tri{scopePrefixAction + actionKindAnswerQuestion: tv(true)})
	assert.True(t, flagReader{}.resolveAnswerQuestion(cfg, compiledFlagEvidence, routeLiveKeystrokeHidden))
	assert.False(t, flagReader{reg: flippedDefaults(hiddenSessionReplyFlagName)}.resolveAnswerQuestion(cfg, compiledFlagEvidence, routeLiveKeystrokeHidden))
}

func TestWireNotificationActionFlags_ShouldSeedControllersFromInjectedRegistry(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	ff := NewFeatureFlagServiceWithRegistry(flippedDefaults(backgroundActivityFeedFlagName, agentWakeTriggersFlagName))
	wireNotificationActionFlags(ff)

	bg, wake := ff.featureControllers[backgroundActivityFeedFlagName], ff.featureControllers[agentWakeTriggersFlagName]
	require.NotNil(t, bg)
	require.NotNil(t, wake)
	assert.False(t, bg.IsEnabled(), "global default on, injected default off")
	assert.True(t, wake.IsEnabled(), "global default off, injected default on")
}
