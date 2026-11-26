package flagsvc

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
)

var attentionFlagNames = []string{
	AttentionProseQuestionDetectionFlagName,
	SessionEventLogFlagName,
	AttentionAckBypassFixFlagName,
	AttentionIdleClockOutputFlagName,
	AttentionProseQuestionLLMTiebreakFlagName,
	AttentionInboxConsolidationFlagName,
}

func TestGetFeatureFlag_Should_ReturnDefaultsWithRegistryEntry_When_FreshInstanceForEachOfSixFlags(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	for _, name := range attentionFlagNames {
		t.Run(name, func(t *testing.T) {
			found := false
			for _, f := range testFlagRegistry().Specs() {
				if f.Name == name {
					found = true
					require.Equal(t, name == AttentionAckBypassFixFlagName, f.Default)
					require.NotEmpty(t, f.Description)
				}
			}
			require.True(t, found, "registry entry missing")
			require.Equal(t, name == AttentionAckBypassFixFlagName, testFlagRegistry().Default(name))
		})
	}
	require.Equal(t, AttentionFlags{AckBypassFix: true}, LoadAttentionFlags(testFlagRegistry()))
}

func TestAttentionFlags_Should_NotReadEnvVars_When_ResolverBuilt(t *testing.T) {
	src, err := os.ReadFile("feature_flags_attention.go")
	require.NoError(t, err)
	for _, banned := range []string{"os.Getenv", "os.LookupEnv", "os.Environ"} {
		require.False(t, strings.Contains(string(src), banned), banned)
	}
}

func TestFlagFlip_Should_TakeEffectNextPollCycleWithoutRestart_When_SetLive(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	svc := NewFeatureFlagService()
	before := LoadAttentionFlags(testFlagRegistry())
	require.False(t, before.ProseQuestionDetection)

	require.NoError(t, flagUpdate(svc, AttentionProseQuestionDetectionFlagName, true))

	require.True(t, LoadAttentionFlags(testFlagRegistry()).ProseQuestionDetection)
	require.False(t, before.ProseQuestionDetection, "a snapshot taken for a cycle must not change mid-cycle")
	require.False(t, LoadAttentionFlags(testFlagRegistry()).SessionEventLog)
}
