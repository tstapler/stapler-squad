package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/envtest"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/session"
)

// The composition root builds one flag registry; the CoreDeps field, the process
// registry and the flag service must all be that same instance, or unmigrated
// readers of featureflags.Current() would disagree with the injected ones.
func TestBuildCoreDepsWithOptions_ShouldShareOneFlagRegistry_WhenBuilt(t *testing.T) {
	restoreGlobalFlagRegistry(t)
	envtest.NewIsolatedStateDir(t)
	core, err := BuildCoreDepsWithOptions(BuildOptions{EntClient: session.NewTestEntRepository(t).GetEntClient()})
	require.NoError(t, err)

	require.NotNil(t, core.FlagRegistry)
	assert.Same(t, core.FlagRegistry, featureflags.Current(), "SetCurrent must install the deps registry")
	assert.Same(t, core.FlagRegistry, core.SessionService.FeatureFlagRegistry(), "flag service must resolve against the deps registry")
}

// The flag service holds the injected registry itself rather than falling back to
// the process registry: replacing the global afterwards must not redirect it.
func TestBuildCoreDepsWithOptions_ShouldKeepInjectedFlagRegistry_WhenGlobalReplaced(t *testing.T) {
	restoreGlobalFlagRegistry(t)
	envtest.NewIsolatedStateDir(t)
	core, err := BuildCoreDepsWithOptions(BuildOptions{EntClient: session.NewTestEntRepository(t).GetEntClient()})
	require.NoError(t, err)

	featureflags.SetCurrent(featureflags.NewRegistry(nil))

	assert.Same(t, core.FlagRegistry, core.SessionService.FeatureFlagRegistry())
}

// The production path (no EntClient) opens storage from config; the registry
// built by the composition root must reach the flag service through it too.
func TestBuildCoreDepsWithOptions_ShouldKeepInjectedFlagRegistry_WhenBuiltFromConfig(t *testing.T) {
	restoreGlobalFlagRegistry(t)
	envtest.NewIsolatedStateDir(t)

	core, err := BuildCoreDepsWithOptions(BuildOptions{})
	require.NoError(t, err)
	require.NotNil(t, core.FlagRegistry)

	featureflags.SetCurrent(featureflags.NewRegistry(nil))
	assert.Same(t, core.FlagRegistry, core.SessionService.FeatureFlagRegistry())
}

// restoreGlobalFlagRegistry puts back whatever registry was installed before the
// test (nil when none was; Current panics in that case, hence the recover).
func restoreGlobalFlagRegistry(t *testing.T) {
	t.Helper()
	prev := func() (r *featureflags.Registry) {
		defer func() { _ = recover() }()
		return featureflags.Current()
	}()
	t.Cleanup(func() { featureflags.SetCurrent(prev) })
}
