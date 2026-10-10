package services

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/envtest"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/deliverygate"
)

const gateFlag = config.HiddenSessionGateFeatureFlag

const (
	setOn      = sessionv1.FlagMutation_FLAG_MUTATION_SET_ENABLED
	setOff     = sessionv1.FlagMutation_FLAG_MUTATION_SET_DISABLED
	clearScope = sessionv1.FlagMutation_FLAG_MUTATION_CLEAR_SCOPE
	resetGlob  = sessionv1.FlagMutation_FLAG_MUTATION_RESET_GLOBAL
)

func mutateFlag(svc *FeatureFlagService, name string, m sessionv1.FlagMutation, scope string) (*sessionv1.FeatureFlag, error) {
	resp, err := svc.UpdateFeatureFlag(context.Background(), connect.NewRequest(&sessionv1.UpdateFeatureFlagRequest{
		Name: name, Mutation: m, Scope: scope,
	}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.Flag, nil
}

func scopesOf(f *sessionv1.FeatureFlag) map[string]bool {
	out := map[string]bool{}
	for _, s := range f.Scopes {
		out[s.Scope] = s.Enabled
	}
	return out
}

type fakeCache struct{ calls []string }

func (c *fakeCache) OnFlagChanged(name string) { c.calls = append(c.calls, "changed:"+name) }
func (c *fakeCache) OnFlagMutation(name, scope, mutation string) {
	c.calls = append(c.calls, "mutation:"+name+":"+scope+":"+mutation)
}

// T-FL-02 / T-FL-16 / T-FL-22: validation of scope and mutation.
func TestFlagMutation_ShouldRejectMalformedScopeAndMutationCombinations_AndPersistNothing(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	svc := NewFeatureFlagService()
	require.NoError(t, config.LoadConfig().SetFeatureFlag(gateFlag, true))

	cases := []struct {
		name  string
		flag  string
		m     sessionv1.FlagMutation
		scope string
	}{
		{"bogus kind", gateFlag, setOn, "kind:bogus"},
		{"triage is not reachable", gateFlag, setOn, "kind:triage"},
		{"scope on non-scopable flag", notificationTrayV2FlagName, setOn, "kind:review"},
		{"set with empty scope", gateFlag, setOn, ""},
		{"set disabled with empty scope", gateFlag, setOff, ""},
		{"clear with empty scope", gateFlag, clearScope, ""},
		{"clear with global literal", gateFlag, clearScope, "global"},
		{"clear on non-scopable flag", notificationTrayV2FlagName, clearScope, "kind:review"},
		{"reset with a scope", gateFlag, resetGlob, "kind:review"},
		{"reset with the global literal", gateFlag, resetGlob, "global"},
		{"unspecified with a scope", gateFlag, sessionv1.FlagMutation_FLAG_MUTATION_UNSPECIFIED, "kind:review"},
		{"unknown enum value", gateFlag, sessionv1.FlagMutation(99), "kind:review"},
		{"unknown enum value without scope", gateFlag, sessionv1.FlagMutation(99), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := mutateFlag(svc, c.flag, c.m, c.scope)
			require.Error(t, err)
			assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		})
	}
	cfg := config.LoadConfig()
	v, ok := cfg.GetFeatureFlagOverride(gateFlag)
	assert.True(t, ok && v, "the global value is byte-identical after every refused request")
	assert.Empty(t, cfg.FeatureFlagScopeOverrides(gateFlag))
}

// T-FL-03 / T-FL-13: scoped set, clear and global reset with read-back.
func TestUpdateFeatureFlag_ShouldReadBackScopesFromPersistedConfig_WhenScopedOverrideSetClearedAndGlobalReset(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	svc := NewFeatureFlagService()

	f, err := mutateFlag(svc, gateFlag, setOn, "kind:review")
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"kind:review": true}, scopesOf(f))
	assert.False(t, f.Enabled, "a kind override never changes the global value")
	_, hasGlobal := config.LoadConfig().GetFeatureFlagOverride(gateFlag)
	assert.False(t, hasGlobal, "no explicit global key was written")

	f, err = mutateFlag(svc, gateFlag, setOff, "kind:diagnose")
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"kind:review": true, "kind:diagnose": false}, scopesOf(f))

	f, err = mutateFlag(svc, gateFlag, setOn, "global")
	require.NoError(t, err)
	assert.True(t, f.Enabled)

	f, err = mutateFlag(svc, gateFlag, clearScope, "kind:review")
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"kind:diagnose": false}, scopesOf(f), "clear deletes only that scope")
	assert.True(t, f.Enabled, "global untouched by a clear")

	f, err = mutateFlag(svc, gateFlag, resetGlob, "")
	require.NoError(t, err)
	assert.False(t, f.Enabled, "reset reads back the registry default")
	_, hasGlobal = config.LoadConfig().GetFeatureFlagOverride(gateFlag)
	assert.False(t, hasGlobal, "the explicit global key is deleted, not written false")
	assert.Equal(t, map[string]bool{"kind:diagnose": false}, scopesOf(f), "reset leaves kind overrides alone")

	got, err := svc.GetFeatureFlags(context.Background(), connect.NewRequest(&sessionv1.GetFeatureFlagsRequest{}))
	require.NoError(t, err)
	for _, fl := range got.Msg.Flags {
		if fl.Name == gateFlag {
			assert.Equal(t, map[string]bool{"kind:diagnose": false}, scopesOf(fl))
		} else {
			assert.Empty(t, fl.Scopes, fl.Name)
		}
	}
}

// An old client (no scope, no mutation) never deletes a persisted scope.
func TestUpdateFeatureFlag_ShouldKeepEveryScopeKey_WhenLegacyRequestOmitsScope(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	svc := NewFeatureFlagService()
	require.NoError(t, config.LoadConfig().SetFeatureFlagScope(gateFlag, "kind:review", true))

	require.NoError(t, flagUpdate(svc, gateFlag, true))
	cfg := config.LoadConfig()
	v, ok := cfg.GetFeatureFlagOverride(gateFlag)
	assert.True(t, ok && v)
	assert.Equal(t, map[string]bool{"kind:review": true}, cfg.FeatureFlagScopeOverrides(gateFlag), "an old client never deletes a scope")

	require.NoError(t, flagUpdate(svc, gateFlag, false))
	v, ok = config.LoadConfig().GetFeatureFlagOverride(gateFlag)
	assert.True(t, ok && !v, "legacy disable writes an explicit false as before")
}

// T-FL-04: every persisted change reaches the cache with its scope and mutation.
func TestUpdateFeatureFlag_ShouldTellTheCacheTheScopeAndMutation_WhenFlagIsMutated(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	svc := NewFeatureFlagService()
	cache := &fakeCache{}
	svc.SetFlagObserver(cache)

	_, err := mutateFlag(svc, gateFlag, setOn, "kind:review")
	require.NoError(t, err)
	_, err = mutateFlag(svc, gateFlag, clearScope, "kind:review")
	require.NoError(t, err)
	_, err = mutateFlag(svc, gateFlag, resetGlob, "")
	require.NoError(t, err)
	require.NoError(t, flagUpdate(svc, gateFlag, true))

	assert.Equal(t, []string{
		"mutation:hidden_session_gate:kind:review:SET_ENABLED",
		"mutation:hidden_session_gate:kind:review:CLEAR_SCOPE",
		"mutation:hidden_session_gate:global:RESET_GLOBAL",
		"mutation:hidden_session_gate:global:SET_ENABLED",
	}, cache.calls)
}

type scopedController struct {
	fakeFeatureController
	failScope error
	applied   []string
}

func (c *scopedController) ApplyScope(scope string, enabled bool) error {
	c.applied = append(c.applied, scope)
	return c.failScope
}

// T-FL-15 (scoped half): a failed scoped apply of a key that was absent deletes
// it instead of writing false; a key that existed is restored.
func TestUpdateFeatureFlag_ShouldRollBackByDeleteNotExplicitFalse_WhenScopedControllerFailsAndTheKeyWasAbsent(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	svc := NewFeatureFlagService()
	ctrl := &scopedController{failScope: errors.New("boom")}
	svc.SetFeatureController(gateFlag, ctrl)

	_, err := mutateFlag(svc, gateFlag, setOff, "kind:review")
	require.Error(t, err)
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	_, ok := config.LoadConfig().GetFeatureFlagScopedOverride(gateFlag, "kind:review")
	assert.False(t, ok, "no key after rollback of an absent scope key")

	require.NoError(t, config.LoadConfig().SetFeatureFlagScope(gateFlag, "kind:review", true))
	_, err = mutateFlag(svc, gateFlag, setOff, "kind:review")
	require.Error(t, err)
	v, ok := config.LoadConfig().GetFeatureFlagScopedOverride(gateFlag, "kind:review")
	assert.True(t, ok && v, "an existing key is restored to its previous value")
}

// T-FL-23: only enabling is refused by the precondition.
func TestUpdateFeatureFlag_ShouldNeverRefuseDisablingClearScopeOrResetGlobal_WhenStatsWriterNotRunning(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	svc := NewFeatureFlagService()
	svc.SetEnableGuard(gateFlag, func() string { return statsWriterNotRunning })
	require.NoError(t, config.LoadConfig().SetFeatureFlag(gateFlag, true))
	require.NoError(t, config.LoadConfig().SetFeatureFlagScope(gateFlag, "kind:review", true))

	_, err := mutateFlag(svc, gateFlag, setOn, "kind:diagnose")
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err), "enabling a kind is guarded too")

	for _, step := range []struct {
		m     sessionv1.FlagMutation
		scope string
	}{{setOff, "kind:review"}, {clearScope, "kind:review"}, {setOff, "global"}, {resetGlob, ""}} {
		_, err := mutateFlag(svc, gateFlag, step.m, step.scope)
		require.NoError(t, err, "%v %q", step.m, step.scope)
	}
}

// T-FL-21 (per-kind half) / T-FL-09: a persisted false kind override is
// reported through the composite provider and as explicit_off_scopes.
func TestStatusDetail_ShouldReportEachFalseKindOverrideAndStayEmpty_WhenNoneIsFalse(t *testing.T) {
	svc := newGatedFlagService(t)
	gate := svc.DeliveryGate()
	gate.Stats().SetWriterRunning(true)
	assert.Empty(t, statusDetailOf(t, svc.featureFlagSvc, gateFlag))
	assert.Empty(t, explicitOffScopes())

	_, err := mutateFlag(svc.featureFlagSvc, gateFlag, setOn, "kind:diagnose")
	require.NoError(t, err)
	assert.Empty(t, statusDetailOf(t, svc.featureFlagSvc, gateFlag), "an explicit true is not an off line")

	_, err = mutateFlag(svc.featureFlagSvc, gateFlag, setOff, "kind:review")
	require.NoError(t, err)
	assert.Equal(t, "Gate is OFF for kind review: hidden review sessions deliver everything",
		statusDetailOf(t, svc.featureFlagSvc, gateFlag))
	assert.Equal(t, []string{"kind:review"}, explicitOffScopes())

	gate.Stats().SetWriterRunning(false)
	assert.Equal(t, statsWriterNotRunning+"; Gate is OFF for kind review: hidden review sessions deliver everything",
		statusDetailOf(t, svc.featureFlagSvc, gateFlag), "contributions join in registration order")

	_, err = mutateFlag(svc.featureFlagSvc, gateFlag, clearScope, "kind:review")
	require.NoError(t, err)
	assert.Equal(t, statsWriterNotRunning, statusDetailOf(t, svc.featureFlagSvc, gateFlag))
}

// The flag cache sees the kind override at once and the gate follows it.
func TestUpdateFeatureFlag_ShouldReloadTheGateCacheWithKindOverrides_WhenScopedSetThroughTheService(t *testing.T) {
	svc := newGatedFlagService(t)
	gate := svc.DeliveryGate()
	gate.Stats().SetWriterRunning(true)

	_, err := mutateFlag(svc.featureFlagSvc, gateFlag, setOn, "kind:review")
	require.NoError(t, err)
	assert.True(t, gate.Flags().EnabledFor(deliverygate.KindReview))
	assert.False(t, gate.Flags().EnabledFor(deliverygate.KindDiagnose))
	assert.False(t, gate.Flags().Enabled())

	_, err = mutateFlag(svc.featureFlagSvc, gateFlag, clearScope, "kind:review")
	require.NoError(t, err)
	assert.False(t, gate.Flags().EnabledFor(deliverygate.KindReview))

	var last string
	for _, c := range gate.StatsSnapshot().FlagHistory {
		last = c.Scope + "=" + c.Mutation
	}
	assert.Equal(t, "kind:review=CLEAR_SCOPE", last, "flag_history carries the mutation")
}
