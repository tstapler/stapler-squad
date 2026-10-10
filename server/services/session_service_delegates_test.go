package services

import (
	"context"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// T-CT-08: contract-PR stubs fail loudly instead of ignoring new fields.
func TestContractStubs_ShouldReturnUnimplementedOrReject_WhenScopeMutationStatsOrReplyCalledBeforeTheirStories(t *testing.T) {
	cases := []struct {
		name string
		req  *sessionv1.UpdateFeatureFlagRequest
	}{
		{"scope_set", &sessionv1.UpdateFeatureFlagRequest{Name: "backlog", Enabled: true, Scope: "kind:review"}},
		{"global_literal_scope", &sessionv1.UpdateFeatureFlagRequest{Name: "backlog", Scope: "global"}},
		{"mutation_set_enabled", &sessionv1.UpdateFeatureFlagRequest{Name: "backlog", Mutation: sessionv1.FlagMutation_FLAG_MUTATION_SET_ENABLED}},
		{"mutation_set_disabled", &sessionv1.UpdateFeatureFlagRequest{Name: "backlog", Mutation: sessionv1.FlagMutation_FLAG_MUTATION_SET_DISABLED}},
		{"mutation_clear_scope", &sessionv1.UpdateFeatureFlagRequest{Name: "backlog", Mutation: sessionv1.FlagMutation_FLAG_MUTATION_CLEAR_SCOPE}},
		{"mutation_reset_global", &sessionv1.UpdateFeatureFlagRequest{Name: "backlog", Mutation: sessionv1.FlagMutation_FLAG_MUTATION_RESET_GLOBAL}},
		{"unknown_enum_value", &sessionv1.UpdateFeatureFlagRequest{Name: "backlog", Mutation: sessionv1.FlagMutation(99)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newFeatureFlagService(t)
			before := config.LoadConfig().GetFeatureFlag(tc.req.Name)

			_, err := svc.UpdateFeatureFlag(context.Background(), connect.NewRequest(tc.req))
			require.Error(t, err)
			require.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))

			require.Equal(t, before, config.LoadConfig().GetFeatureFlag(tc.req.Name),
				"a stub must not fall through to the legacy global write")
		})
	}

	t.Run("stats_rpc", func(t *testing.T) {
		svc := &SessionService{notificationSvc: &NotificationService{}}
		_, err := svc.GetDeliveryGateStats(context.Background(),
			connect.NewRequest(&sessionv1.GetDeliveryGateStatsRequest{}))
		require.Error(t, err)
		require.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
	})
}

// T-FL-10: a request with neither new field behaves exactly as before.
func TestFeatureFlags_ShouldNotChangeBehavior_WhenRequestOmitsScope(t *testing.T) {
	svc := newFeatureFlagService(t)

	resp, err := svc.UpdateFeatureFlag(context.Background(), connect.NewRequest(
		&sessionv1.UpdateFeatureFlagRequest{Name: "backlog", Enabled: true}))
	require.NoError(t, err)
	require.True(t, resp.Msg.Flag.Enabled)
	require.True(t, config.LoadConfig().GetFeatureFlag("backlog"))
	require.Empty(t, resp.Msg.Flag.Scopes, "scopes stay empty until the per-kind story lands")

	resp, err = svc.UpdateFeatureFlag(context.Background(), connect.NewRequest(
		&sessionv1.UpdateFeatureFlagRequest{Name: "backlog", Enabled: false}))
	require.NoError(t, err)
	require.False(t, resp.Msg.Flag.Enabled)
	require.False(t, config.LoadConfig().GetFeatureFlag("backlog"))
}

// Wire values are frozen by Contract PR 2; renumbering breaks old clients.
func TestFlagMutation_ShouldKeepFrozenNumbersAndPrefixedNames(t *testing.T) {
	require.EqualValues(t, 0, sessionv1.FlagMutation_FLAG_MUTATION_UNSPECIFIED)
	require.EqualValues(t, 1, sessionv1.FlagMutation_FLAG_MUTATION_SET_ENABLED)
	require.EqualValues(t, 2, sessionv1.FlagMutation_FLAG_MUTATION_SET_DISABLED)
	require.EqualValues(t, 3, sessionv1.FlagMutation_FLAG_MUTATION_CLEAR_SCOPE)
	require.EqualValues(t, 4, sessionv1.FlagMutation_FLAG_MUTATION_RESET_GLOBAL)
}
