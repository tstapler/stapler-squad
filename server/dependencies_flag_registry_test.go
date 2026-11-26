package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/envtest"
	"github.com/tstapler/stapler-squad/session"
)

// The composition root builds one flag registry; the CoreDeps field and the flag
// service must be that same instance, or readers would disagree on defaults.
func TestBuildCoreDepsWithOptions_ShouldShareOneFlagRegistry_WhenBuilt(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	core, err := BuildCoreDepsWithOptions(BuildOptions{EntClient: session.NewTestEntRepository(t).GetEntClient()})
	require.NoError(t, err)

	require.NotNil(t, core.FlagRegistry)
	assert.Same(t, core.FlagRegistry, core.SessionService.FeatureFlagRegistry(), "flag service must resolve against the deps registry")
}

// The production path (no EntClient) opens storage from config; the registry
// built by the composition root must reach the flag service through it too.
func TestBuildCoreDepsWithOptions_ShouldShareOneFlagRegistry_WhenBuiltFromConfig(t *testing.T) {
	envtest.NewIsolatedStateDir(t)

	core, err := BuildCoreDepsWithOptions(BuildOptions{})
	require.NoError(t, err)
	require.NotNil(t, core.FlagRegistry)
	assert.Same(t, core.FlagRegistry, core.SessionService.FeatureFlagRegistry())
}
