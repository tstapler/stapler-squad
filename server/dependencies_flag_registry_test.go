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
	envtest.NewIsolatedStateDir(t)
	core, err := BuildCoreDepsWithOptions(BuildOptions{EntClient: session.NewTestEntRepository(t).GetEntClient()})
	require.NoError(t, err)

	t.Cleanup(func() { featureflags.SetCurrent(core.FlagRegistry) })
	featureflags.SetCurrent(featureflags.NewRegistry(nil))

	assert.Same(t, core.FlagRegistry, core.SessionService.FeatureFlagRegistry())
}
