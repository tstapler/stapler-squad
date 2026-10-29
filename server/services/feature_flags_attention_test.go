package services

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var attentionFlagNames = []string{
	attentionProseQuestionDetectionFlagName,
	sessionEventLogFlagName,
	attentionAckBypassFixFlagName,
	attentionIdleClockOutputFlagName,
	attentionProseQuestionLLMTiebreakFlagName,
	attentionInboxConsolidationFlagName,
}

func TestGetFeatureFlag_Should_ReturnFalseWithRegistryEntry_When_FreshInstanceForEachOfSixFlags(t *testing.T) {
	withIsolatedStateDir(t)
	for _, name := range attentionFlagNames {
		t.Run(name, func(t *testing.T) {
			found := false
			for _, f := range knownFeatureFlags {
				if f.name == name {
					found = true
					require.False(t, f.defaultValue)
					require.NotEmpty(t, f.description)
				}
			}
			require.True(t, found, "registry entry missing")
			require.False(t, featureFlagDefault(name))
		})
	}
	require.Equal(t, AttentionFlags{}, LoadAttentionFlags())
}

func TestAttentionFlags_Should_NotReadEnvVars_When_ResolverBuilt(t *testing.T) {
	src, err := os.ReadFile("feature_flags_attention.go")
	require.NoError(t, err)
	for _, banned := range []string{"os.Getenv", "os.LookupEnv", "os.Environ"} {
		require.False(t, strings.Contains(string(src), banned), banned)
	}
}

func TestFlagFlip_Should_TakeEffectNextPollCycleWithoutRestart_When_SetLive(t *testing.T) {
	withIsolatedStateDir(t)
	svc := NewFeatureFlagService()
	before := LoadAttentionFlags()
	require.False(t, before.ProseQuestionDetection)

	require.NoError(t, flagUpdate(svc, attentionProseQuestionDetectionFlagName, true))

	require.True(t, LoadAttentionFlags().ProseQuestionDetection)
	require.False(t, before.ProseQuestionDetection, "a snapshot taken for a cycle must not change mid-cycle")
	require.False(t, LoadAttentionFlags().SessionEventLog)
}
