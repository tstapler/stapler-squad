package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

func smprMutate(svc *FeatureFlagService, scope string, mutation sessionv1.FlagMutation) error {
	_, err := svc.UpdateFeatureFlag(context.Background(), connect.NewRequest(&sessionv1.UpdateFeatureFlagRequest{
		Name: config.SessionMultiPRFlag, Scope: scope, Mutation: mutation,
	}))
	return err
}

func smprSet(t *testing.T, svc *FeatureFlagService, scope string, on bool) {
	t.Helper()
	m := sessionv1.FlagMutation_FLAG_MUTATION_SET_DISABLED
	if on {
		m = sessionv1.FlagMutation_FLAG_MUTATION_SET_ENABLED
	}
	require.NoError(t, smprMutate(svc, scope, m))
}

func TestSessionMultiPRFlag_ShouldRoundTripThroughUpdateFeatureFlag(t *testing.T) {
	withIsolatedStateDir(t)
	svc := NewFeatureFlagService()
	require.False(t, config.SessionMultiPR(config.SessionMultiPRScopePoller))

	smprSet(t, svc, config.SessionMultiPRScopePoller, true)

	assert.True(t, config.SessionMultiPR(config.SessionMultiPRScopePoller))
	assert.False(t, config.SessionMultiPR(config.SessionMultiPRScopeUI))
}

func TestSessionMultiPRFlag_ShouldStoreOutOfOrderScopeButResolveInOrder(t *testing.T) {
	withIsolatedStateDir(t)
	svc := NewFeatureFlagService()

	smprSet(t, svc, config.SessionMultiPRScopeUI, true)
	assert.False(t, config.SessionMultiPR(config.SessionMultiPRScopeUI), "stored, but poller/backlog/actions are off")
	assert.True(t, readbackScope(t, svc, config.SessionMultiPRScopeUI), "the readback shows the stored value")

	for _, s := range []string{config.SessionMultiPRScopePoller, config.SessionMultiPRScopeBacklog, config.SessionMultiPRScopeActions} {
		smprSet(t, svc, s, true)
		assert.True(t, config.SessionMultiPR(s), s)
	}
	assert.True(t, config.SessionMultiPR(config.SessionMultiPRScopeUI))

	smprSet(t, svc, config.SessionMultiPRScopePoller, false)
	for _, s := range []string{config.SessionMultiPRScopeBacklog, config.SessionMultiPRScopeActions, config.SessionMultiPRScopeUI} {
		assert.False(t, config.SessionMultiPR(s), "%s must follow the poller off immediately", s)
	}
	smprSet(t, svc, config.SessionMultiPRScopePoller, true)
	assert.True(t, config.SessionMultiPR(config.SessionMultiPRScopeUI), "re-enabling the poller restores the later scopes")
}

func TestSessionMultiPRFlag_RefresherOffKillSwitch(t *testing.T) {
	withIsolatedStateDir(t)
	svc := NewFeatureFlagService()
	require.True(t, config.SessionMultiPRRefresher(), "refresher runs by default")

	smprSet(t, svc, config.SessionMultiPRScopeRefresherOff, true)
	assert.False(t, config.SessionMultiPRRefresher(), "no other scope enabled or required")

	require.NoError(t, smprMutate(svc, config.SessionMultiPRScopeRefresherOff, sessionv1.FlagMutation_FLAG_MUTATION_CLEAR_SCOPE))
	assert.True(t, config.SessionMultiPRRefresher(), "CLEAR_SCOPE restores the refresher")

	smprSet(t, svc, config.SessionMultiPRScopeRefresherOff, true)
	require.NoError(t, flagUpdate(svc, config.SessionMultiPRFlag, true))
	assert.False(t, config.SessionMultiPRRefresher(), "turning the global flag on leaves the kill switch set")
	require.NoError(t, flagUpdate(svc, config.SessionMultiPRFlag, false))
	assert.False(t, config.SessionMultiPRRefresher())
	assert.False(t, config.SessionMultiPR(config.SessionMultiPRScopePoller), "the kill switch is not in the reader order")
}

func TestSessionMultiPRFlag_ShouldRejectUnknownScope(t *testing.T) {
	withIsolatedStateDir(t)
	svc := NewFeatureFlagService()
	err := smprMutate(svc, "reader:nope", sessionv1.FlagMutation_FLAG_MUTATION_SET_ENABLED)
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestArtifactScanV2Flag_ShouldRegisterGlobalOnlyDefaultOff(t *testing.T) {
	withIsolatedStateDir(t)
	svc := NewFeatureFlagService()
	assert.False(t, featureFlagDefault(config.ArtifactScanV2Flag))
	assert.False(t, config.LoadConfig().ArtifactScanV2Enabled())
	require.NoError(t, flagUpdate(svc, config.ArtifactScanV2Flag, true))
	assert.True(t, config.LoadConfig().ArtifactScanV2Enabled())
}

// readbackScope returns the stored value of a scope as the flag panel reports it.
func readbackScope(t *testing.T, svc *FeatureFlagService, scope string) bool {
	t.Helper()
	resp, err := svc.GetFeatureFlags(context.Background(), connect.NewRequest(&sessionv1.GetFeatureFlagsRequest{}))
	require.NoError(t, err)
	for _, f := range resp.Msg.Flags {
		if f.Name != config.SessionMultiPRFlag {
			continue
		}
		for _, s := range f.Scopes {
			if s.Scope == scope {
				return s.Enabled
			}
		}
	}
	t.Fatalf("scope %q not in the readback", scope)
	return false
}

func TestSessionMultiPRAudit_ShouldLetRefresherOffPersistWithFallback_WhenSinkIsDownButRefuseReaderScopes(t *testing.T) {
	e := newAuditEnv(t)
	e.svc.MergeAuditPolicies(sessionMultiPRFlagAuditPolicies())
	e.mfs.setFailWrite(errDiskFull)

	err := smprMutate(e.svc, config.SessionMultiPRScopePoller, sessionv1.FlagMutation_FLAG_MUTATION_SET_ENABLED)
	require.Error(t, err, "opening a reader stays refused while the sink is down")
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))

	require.NoError(t, smprMutate(e.svc, config.SessionMultiPRScopeRefresherOff, sessionv1.FlagMutation_FLAG_MUTATION_SET_ENABLED),
		"the kill switch is the escape hatch for a sink fault")
	assert.False(t, config.LoadConfig().SessionMultiPRRefresher())
	assert.GreaterOrEqual(t, e.degradedCount(auditModeFallbackLog), 1)
}
