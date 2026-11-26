package gatestats

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
)

func TestExplicitOffScopes_ShouldListGlobalFirstThenFalseKinds_WhenOverridesPersisted(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	flag := config.HiddenSessionGateFeatureFlag
	assert.Empty(t, ExplicitOffScopes())

	require.NoError(t, config.LoadConfig().SetFeatureFlag(flag, true))
	assert.Empty(t, ExplicitOffScopes(), "an explicit true global is not an off scope")

	require.NoError(t, config.LoadConfig().SetFeatureFlagScope(flag, "kind:review", false))
	assert.Equal(t, []string{"kind:review"}, ExplicitOffScopes())

	require.NoError(t, config.LoadConfig().SetFeatureFlag(flag, false))
	assert.Equal(t, []string{"global", "kind:review"}, ExplicitOffScopes())
}
