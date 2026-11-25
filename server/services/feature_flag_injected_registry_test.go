package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/server/services/flagsvc"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
)

const (
	injDefaultOn  = "inj_default_on"
	injDefaultOff = "inj_default_off"
	injScope      = "kind:review"
)

// injectedRegistry shares no flag name with the default registry, so any read that still reaches
// the default registry misses these flags.
func injectedRegistry() *featureflags.Registry {
	return featureflags.NewRegistry([]featureflags.Spec{
		{Name: injDefaultOn, Description: "on", Default: true, Scopes: []string{injScope}},
		{Name: injDefaultOff, Description: "off"},
	})
}

func TestFeatureFlagService_ShouldListOnlyInjectedFlagsWithInjectedDefaults_WhenRegistryInjected(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	got := flagStates(t, flagsvc.NewFeatureFlagServiceWithRegistry(injectedRegistry()))
	assert.Equal(t, map[string]bool{injDefaultOn: true, injDefaultOff: false}, got)
}

func TestFeatureFlagService_ShouldUseDefaultFlagRegistry_WhenConstructedWithoutRegistry(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	got := flagStates(t, flagsvc.NewFeatureFlagService())
	assert.Len(t, got, len(testFlagRegistry().Specs()))
	assert.NotContains(t, got, injDefaultOn)
}

func TestUpdateFeatureFlag_ShouldRejectProcessRegistryFlag_WhenRegistryInjectedWithoutIt(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	svc := flagsvc.NewFeatureFlagServiceWithRegistry(injectedRegistry())
	_, err := mutateFlag(svc, gateFlag, setOn, "")
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	_, err = mutateFlag(svc, injDefaultOn, setOff, flagsvc.GlobalScopeLiteral)
	require.NoError(t, err)
}

func TestUpdateFeatureFlag_ShouldResetToInjectedDefault_WhenResetGlobal(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	svc := flagsvc.NewFeatureFlagServiceWithRegistry(injectedRegistry())

	off, err := mutateFlag(svc, injDefaultOn, setOff, flagsvc.GlobalScopeLiteral)
	require.NoError(t, err)
	assert.False(t, off.Enabled)

	reset, err := mutateFlag(svc, injDefaultOn, resetGlob, "")
	require.NoError(t, err)
	assert.True(t, reset.Enabled, "reset resolves to the injected spec's default")
	_, explicit := config.LoadConfig().GetFeatureFlagOverride(injDefaultOn)
	assert.False(t, explicit, "reset deletes the explicit key")
}

func TestUpdateFeatureFlag_ShouldFollowInjectedDefault_WhenScopedSetThenCleared(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	svc := flagsvc.NewFeatureFlagServiceWithRegistry(injectedRegistry())

	set, err := mutateFlag(svc, injDefaultOn, setOff, injScope)
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{injScope: false}, scopesOf(set))

	cleared, err := mutateFlag(svc, injDefaultOn, clearScope, injScope)
	require.NoError(t, err)
	assert.Empty(t, scopesOf(cleared))
	assert.True(t, cleared.Enabled, "global value unset: the injected default (on) applies")
}

func TestUpdateFeatureFlag_ShouldRejectScope_WhenInjectedSpecHasNone(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	svc := flagsvc.NewFeatureFlagServiceWithRegistry(injectedRegistry())
	_, err := mutateFlag(svc, injDefaultOff, setOn, injScope)
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

type recordingController struct {
	enables, disables int
	scopeCalls        []bool
}

func (c *recordingController) Enable(context.Context) error { c.enables++; return nil }
func (c *recordingController) Disable() error               { c.disables++; return nil }
func (c *recordingController) IsEnabled() bool              { return c.enables > c.disables }
func (c *recordingController) ApplyScope(_ string, enabled bool) error {
	c.scopeCalls = append(c.scopeCalls, enabled)
	return nil
}

func TestUpdateFeatureFlag_ShouldEnableControllerToInjectedDefault_WhenResetGlobal(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	svc := flagsvc.NewFeatureFlagServiceWithRegistry(injectedRegistry())
	ctrl := &recordingController{}
	svc.SetFeatureController(injDefaultOn, ctrl)

	_, err := mutateFlag(svc, injDefaultOn, setOff, flagsvc.GlobalScopeLiteral)
	require.NoError(t, err)
	_, err = mutateFlag(svc, injDefaultOn, resetGlob, "")
	require.NoError(t, err)
	assert.Equal(t, 1, ctrl.disables)
	assert.Equal(t, 1, ctrl.enables, "reset drives the controller to the injected default (on)")
}

func TestUpdateFeatureFlag_ShouldApplyInjectedDefaultToScopedController_WhenScopeCleared(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	svc := flagsvc.NewFeatureFlagServiceWithRegistry(injectedRegistry())
	ctrl := &recordingController{}
	svc.SetFeatureController(injDefaultOn, ctrl)

	_, err := mutateFlag(svc, injDefaultOn, setOff, injScope)
	require.NoError(t, err)
	_, err = mutateFlag(svc, injDefaultOn, clearScope, injScope)
	require.NoError(t, err)
	assert.Equal(t, []bool{false, true}, ctrl.scopeCalls, "clearing falls back to the injected default (on)")
}

func TestUpdateFeatureFlag_ShouldRunEnableGuard_WhenClearingFalseScopeOverInjectedDefaultOn(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	svc := flagsvc.NewFeatureFlagServiceWithRegistry(injectedRegistry())
	svc.SetEnableGuard(injDefaultOn, func() string { return "blocked" })

	_, err := mutateFlag(svc, injDefaultOn, setOff, injScope)
	require.NoError(t, err)
	_, err = mutateFlag(svc, injDefaultOn, clearScope, injScope)
	require.Error(t, err, "clearing turns the kind on (default on), so the enable guard applies")
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
}

func TestFlagChangeAudit_ShouldRecordInjectedDefaultAsPrevious_WhenFlipOfUnsetFlagApplies(t *testing.T) {
	e := newAuditEnv(t)
	e.svc = flagsvc.NewFeatureFlagServiceWithRegistry(injectedRegistry())
	e.svc.SetAudit(e.sink, map[string]featureflags.FlagAuditPolicy{
		injDefaultOn: {Loosening: func(enabled bool) bool { return !enabled }},
	})
	require.NoError(t, e.flip(injDefaultOn, false))
	e.sink.Close()

	got := e.lines(t)
	require.Len(t, got, 2)
	require.NotNil(t, got[1].Previous)
	assert.True(t, *got[1].Previous, "no persisted key: previous is the injected default (on)")
}

func TestFeatureFlagService_ShouldReportInjectedDefaultInNudgeStatusDetail_WhenFlagUnset(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	name := config.DiagnoseNudgeFeatureFlag
	injected := !testFlagRegistry().Default(name)
	want := map[bool]string{true: "Default: On", false: "Default: Off"}[injected]
	svc := flagsvc.NewFeatureFlagServiceWithRegistry(featureflags.NewRegistry([]featureflags.Spec{{Name: name, Default: injected}}))

	resp, err := svc.GetFeatureFlags(context.Background(), connect.NewRequest(&sessionv1.GetFeatureFlagsRequest{}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Flags, 1)
	assert.Contains(t, resp.Msg.Flags[0].StatusDetail, want)
}

func TestFlagChangeAudit_ShouldRecordInjectedDefaultAsPrevious_WhenScopedFlipOfUnsetFlagApplies(t *testing.T) {
	e := newAuditEnv(t)
	injected := !testFlagRegistry().Default(gateFlag)
	e.svc = flagsvc.NewFeatureFlagServiceWithRegistry(featureflags.NewRegistry([]featureflags.Spec{
		{Name: gateFlag, Default: injected, Scopes: []string{injScope}},
	}))
	e.svc.SetAudit(e.sink, map[string]featureflags.FlagAuditPolicy{gateFlag: {}})
	_, err := mutateFlag(e.svc, gateFlag, setOn, injScope)
	require.NoError(t, err)
	e.sink.Close()

	got := e.lines(t)
	require.Len(t, got, 1)
	require.NotNil(t, got[0].Previous)
	assert.Equal(t, injected, *got[0].Previous, "no persisted key: previous is the injected default, not the process registry's")
}
