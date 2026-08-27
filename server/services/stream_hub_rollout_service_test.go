package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// newIsolatedStreamHubRolloutService creates a StreamHubRolloutService backed
// by a fresh temporary directory, preventing config state from leaking
// between tests (same isolation pattern as newIsolatedSlackConfigService).
func newIsolatedStreamHubRolloutService(t *testing.T) *StreamHubRolloutService {
	t.Helper()
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	return NewStreamHubRolloutService()
}

func TestGetStreamHubRolloutStatus_DefaultsToNoRehearsalNoOverrides(t *testing.T) {
	s := newIsolatedStreamHubRolloutService(t)

	resp, err := s.GetStreamHubRolloutStatus(context.Background(), connect.NewRequest(&sessionv1.GetStreamHubRolloutStatusRequest{}))
	require.NoError(t, err)
	assert.Nil(t, resp.Msg.RollbackRehearsalCompletedAt)
	assert.Empty(t, resp.Msg.SessionOverrides)
}

func TestGetStreamHubRolloutStatus_ReportsGlobalEnvVar(t *testing.T) {
	s := newIsolatedStreamHubRolloutService(t)
	t.Setenv("STAPLER_SQUAD_USE_STREAM_HUB", "true")

	resp, err := s.GetStreamHubRolloutStatus(context.Background(), connect.NewRequest(&sessionv1.GetStreamHubRolloutStatusRequest{}))
	require.NoError(t, err)
	assert.True(t, resp.Msg.GlobalEnvVarSet)
}

func TestCompleteStreamHubRollbackRehearsal_SetsTimestamp(t *testing.T) {
	s := newIsolatedStreamHubRolloutService(t)

	resp, err := s.CompleteStreamHubRollbackRehearsal(context.Background(), connect.NewRequest(&sessionv1.CompleteStreamHubRollbackRehearsalRequest{}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.RollbackRehearsalCompletedAt)
	assert.False(t, resp.Msg.RollbackRehearsalCompletedAt.AsTime().IsZero())
}

func TestSetStreamHubSessionOverride_SetsAndClears(t *testing.T) {
	s := newIsolatedStreamHubRolloutService(t)
	forceTrue := true

	resp, err := s.SetStreamHubSessionOverride(context.Background(), connect.NewRequest(&sessionv1.SetStreamHubSessionOverrideRequest{
		SessionName: "canary-session",
		ForceHub:    &forceTrue,
	}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.SessionOverrides, 1)
	assert.Equal(t, "canary-session", resp.Msg.SessionOverrides[0].SessionName)
	assert.True(t, resp.Msg.SessionOverrides[0].ForceHub)

	// Unset ForceHub clears the override (config.SetStreamHubSessionOverride's
	// nil-means-clear convention).
	resp, err = s.SetStreamHubSessionOverride(context.Background(), connect.NewRequest(&sessionv1.SetStreamHubSessionOverrideRequest{
		SessionName: "canary-session",
	}))
	require.NoError(t, err)
	assert.Empty(t, resp.Msg.SessionOverrides)
}

func TestSetStreamHubGlobalOverride_SetsAndClears(t *testing.T) {
	s := newIsolatedStreamHubRolloutService(t)
	forceFalse := false

	resp, err := s.SetStreamHubGlobalOverride(context.Background(), connect.NewRequest(&sessionv1.SetStreamHubGlobalOverrideRequest{
		ForceHub: &forceFalse,
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.GlobalOverride)
	assert.False(t, *resp.Msg.GlobalOverride)

	// Unset ForceHub clears the override (config.SetStreamHubGlobalOverride's
	// nil-means-clear convention).
	resp, err = s.SetStreamHubGlobalOverride(context.Background(), connect.NewRequest(&sessionv1.SetStreamHubGlobalOverrideRequest{}))
	require.NoError(t, err)
	assert.Nil(t, resp.Msg.GlobalOverride)
}

func TestGetStreamHubRolloutStatus_ReportsGlobalOverride(t *testing.T) {
	s := newIsolatedStreamHubRolloutService(t)
	forceTrue := true

	_, err := s.SetStreamHubGlobalOverride(context.Background(), connect.NewRequest(&sessionv1.SetStreamHubGlobalOverrideRequest{
		ForceHub: &forceTrue,
	}))
	require.NoError(t, err)

	resp, err := s.GetStreamHubRolloutStatus(context.Background(), connect.NewRequest(&sessionv1.GetStreamHubRolloutStatusRequest{}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.GlobalOverride)
	assert.True(t, *resp.Msg.GlobalOverride)
}
