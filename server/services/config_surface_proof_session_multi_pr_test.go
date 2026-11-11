package services

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
)

// Each proof writes the setting through FeatureFlagService.UpdateFeatureFlag and
// asks the production reader, which must answer differently.

func TestConfigSurfaceProof_SessionMultiPR_ChangesReader(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	svc := NewFeatureFlagService()
	svctest.AssertConfigChangesBehavior(t, func(setting any) any {
		smprSet(t, svc, config.SessionMultiPRScopePoller, setting.(bool))
		return config.SessionMultiPR(config.SessionMultiPRScopePoller)
	}, false, true)
}

// The refresher runs by default and stops when refresher_off is set; a global
// flip must not change it (the scope is read as an explicit override only).
func TestConfigSurfaceProof_SessionMultiPR_RefresherOff_StopsRefresher(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	svc := NewFeatureFlagService()
	svctest.AssertConfigChangesBehavior(t, func(setting any) any {
		off := setting.(bool) // the value under test is "refresher runs"
		mutation := sessionv1.FlagMutation_FLAG_MUTATION_CLEAR_SCOPE
		if !off {
			mutation = sessionv1.FlagMutation_FLAG_MUTATION_SET_ENABLED
		}
		require.NoError(t, smprMutate(svc, config.SessionMultiPRScopeRefresherOff, mutation))
		return config.SessionMultiPRRefresher()
	}, false, true)

	before := config.SessionMultiPRRefresher()
	require.NoError(t, flagUpdate(svc, config.SessionMultiPRFlag, true))
	require.Equal(t, before, config.SessionMultiPRRefresher(), "a global flip must not change the refresher")
}

func TestConfigSurfaceProof_ArtifactScanV2_ChangesReader(t *testing.T) {
	set := bkfFlagWriter(t)
	svctest.AssertConfigChangesBehavior(t, func(setting any) any {
		set(config.ArtifactScanV2Flag, setting.(bool))
		return config.LoadConfig().ArtifactScanV2Enabled()
	}, false, true)
}
