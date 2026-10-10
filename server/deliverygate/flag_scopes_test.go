package deliverygate

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/envtest"
)

func TestScopableKinds_ShouldExcludeTriageAndAcceptOnlyTheClosedSet_WhenParsingScopes(t *testing.T) {
	t.Parallel()
	for scope, want := range map[string]bool{
		"kind:review": true, "kind:diagnose": true, "kind:other": true,
		"kind:triage": false, "kind:bogus": false, "kind:": false, "global": false, "review": false,
	} {
		_, ok := ScopableKindFromScope(scope)
		assert.Equal(t, want, ok, scope)
	}
	assert.Equal(t, "kind:review", ScopeOf(KindReview))
}

func TestConfigFlagLoader_ShouldReadKindOverridesAndIgnoreUnknownScopes_WhenConfigHasFeatureFlagScopes(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	cfg := config.LoadConfig()
	require.NoError(t, cfg.SetFeatureFlag(config.HiddenSessionGateFeatureFlag, false))
	require.NoError(t, cfg.SetFeatureFlagScope(config.HiddenSessionGateFeatureFlag, "kind:review", true))
	require.NoError(t, cfg.SetFeatureFlagScope(config.HiddenSessionGateFeatureFlag, "kind:triage", true)) // hand edit
	require.NoError(t, cfg.SetFeatureFlagScope("some_other_flag", "kind:diagnose", true))

	s, err := ConfigFlagLoader()
	require.NoError(t, err)
	assert.False(t, s.Global)
	assert.Equal(t, map[HiddenKind]bool{KindReview: true}, s.KindOverrides)
}

// The flag history must say which mutation was applied: a reset or a clear is
// not a set false (plan.md PR 2a-2 record, deviation 5).
func TestStats_ShouldRecordTheMutationInFlagHistory_WhenOperatorResetsGlobalOrClearsScope(t *testing.T) {
	t.Parallel()
	e := newStatsEnv(t, true)
	flags := e.flags

	flags.setSettings(FlagSettings{Global: true, KindOverrides: map[HiddenKind]bool{KindReview: false}})
	e.g.Flags().OnFlagMutation(config.HiddenSessionGateFeatureFlag, "kind:review", "SET_DISABLED")
	flags.setSettings(FlagSettings{Global: true})
	e.g.Flags().OnFlagMutation(config.HiddenSessionGateFeatureFlag, "kind:review", "CLEAR_SCOPE")
	flags.setSettings(FlagSettings{Global: false}) // explicit true deleted, registry default off
	e.g.Flags().OnFlagMutation(config.HiddenSessionGateFeatureFlag, "global", "RESET_GLOBAL")
	// Reset of an explicit false to a default-off flag: the effective value did not move.
	e.g.Flags().OnFlagMutation(config.HiddenSessionGateFeatureFlag, "global", "RESET_GLOBAL")

	var got []string
	for _, c := range e.snap().FlagHistory {
		got = append(got, c.Scope+"="+c.Mutation)
	}
	assert.Equal(t, []string{
		"kind:review=SET_DISABLED", "kind:review=CLEAR_SCOPE", "global=RESET_GLOBAL", "global=RESET_GLOBAL",
	}, got)
	for _, c := range e.snap().FlagHistory {
		if c.Mutation == "RESET_GLOBAL" || c.Mutation == "CLEAR_SCOPE" {
			assert.False(t, c.Value, "a cleared value is recorded as false")
		}
	}
}

func TestFlagCache_ShouldIgnoreMutationHintsOfOtherFlags_WhenObserverIsToldAboutThem(t *testing.T) {
	t.Parallel()
	lg, _ := newRecLogger()
	f := &staticFlags{}
	c := NewFlagCache(f.load, lg)
	f.set(true)
	c.OnFlagMutation("some_other_flag", "global", "SET_ENABLED")
	assert.False(t, c.Enabled(), "another flag's mutation must not reload this one")
	assert.Equal(t, 0, f.readCount())
}

// T-FL-05: global off, review override on: review is suppressed, diagnose
// (inherits) is delivered and only counted.
func TestGate_ShouldSuppressReviewAndShadowTheInheritingKind_WhenGlobalOffAndReviewOverrideOn(t *testing.T) {
	t.Parallel()
	g, _, _, flags := newTestGate(false, hiddenReview, hiddenDiagnose)
	flags.setSettings(FlagSettings{KindOverrides: map[HiddenKind]bool{KindReview: true}})
	g.Flags().Reload()
	f := g.PublishFilter()

	assert.False(t, f(notif("review:abc", tTaskComplete, nil)), "review override on: routine suppressed")
	assert.True(t, f(notif("diagnose:abc", tTaskComplete, nil)), "inheriting kind is delivered")
	assert.EqualValues(t, 1, g.Metrics().Total(CounterSuppressed))
	assert.EqualValues(t, 1, g.Metrics().Total(CounterWouldSuppress), "the inheriting kind is shadow-counted")
	assert.True(t, f(notif("review:abc", tError, nil)), "failure class still delivered while the review scope is on")
}

func TestGate_ShouldWarnOncePerExplicitFalseKind_WhenStartedWithKindOverrides(t *testing.T) {
	t.Parallel()
	g, _, recs, flags := newTestGate(true, hiddenReview)
	flags.setSettings(FlagSettings{Global: true, KindOverrides: map[HiddenKind]bool{
		KindReview: false, KindDiagnose: true, KindOther: false,
	}})
	g.Flags().Reload()
	g.WarnExplicitOffKinds()

	var scopes []any
	for _, r := range recs() {
		if r.Msg == "hidden_session_gate_explicit_false" {
			assert.Equal(t, slog.LevelWarn, r.Level)
			scopes = append(scopes, r.Attrs["scope"])
		}
	}
	assert.Equal(t, []any{"kind:review", "kind:other"}, scopes, "an explicit true is not warned about")
}
