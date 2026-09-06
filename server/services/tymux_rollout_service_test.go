package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// newIsolatedTymuxRolloutService creates a TymuxRolloutService backed by a
// fresh temporary directory, preventing config state from leaking between
// tests (same isolation pattern as newIsolatedStreamHubRolloutService).
func newIsolatedTymuxRolloutService(t *testing.T) *TymuxRolloutService {
	t.Helper()
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	return NewTymuxRolloutService()
}

// TestGetTymuxRolloutStatus_GlobalEnvVarAlwaysFalse is a regression guard:
// STAPLER_SQUAD_USE_TYMUX was removed in favor of the "tymux" feature flag
// (see GlobalOverride), so GlobalEnvVarSet must stay false even if that env
// var happens to be set in the environment.
func TestGetTymuxRolloutStatus_GlobalEnvVarAlwaysFalse(t *testing.T) {
	s := newIsolatedTymuxRolloutService(t)
	t.Setenv("STAPLER_SQUAD_USE_TYMUX", "true")

	resp, err := s.GetTymuxRolloutStatus(context.Background(), connect.NewRequest(&sessionv1.GetTymuxRolloutStatusRequest{}))
	require.NoError(t, err)
	assert.False(t, resp.Msg.GlobalEnvVarSet)
	assert.Nil(t, resp.Msg.RollbackRehearsalCompletedAt)
	assert.Empty(t, resp.Msg.SessionOverrides)
}

func TestGetTymuxRolloutStatus_ReportsRehearsalTimestamp(t *testing.T) {
	s := newIsolatedTymuxRolloutService(t)

	_, err := s.CompleteTymuxRollbackRehearsal(context.Background(), connect.NewRequest(&sessionv1.CompleteTymuxRollbackRehearsalRequest{}))
	require.NoError(t, err)
	resp, err := s.GetTymuxRolloutStatus(context.Background(), connect.NewRequest(&sessionv1.GetTymuxRolloutStatusRequest{}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.RollbackRehearsalCompletedAt)
	assert.False(t, resp.Msg.RollbackRehearsalCompletedAt.AsTime().IsZero())
}

func TestSetTymuxGlobalOverride_SetsAndClears(t *testing.T) {
	s := newIsolatedTymuxRolloutService(t)
	forceFalse := false

	resp, err := s.SetTymuxGlobalOverride(context.Background(), connect.NewRequest(&sessionv1.SetTymuxGlobalOverrideRequest{
		ForceTymux: &forceFalse,
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.GlobalOverride)
	assert.False(t, *resp.Msg.GlobalOverride)

	// Unset ForceTymux clears the override (config.SetTymuxGlobalOverride's
	// nil-means-clear convention).
	resp, err = s.SetTymuxGlobalOverride(context.Background(), connect.NewRequest(&sessionv1.SetTymuxGlobalOverrideRequest{}))
	require.NoError(t, err)
	assert.Nil(t, resp.Msg.GlobalOverride)
}

func TestGetTymuxRolloutStatus_ReportsGlobalOverride(t *testing.T) {
	s := newIsolatedTymuxRolloutService(t)
	forceTrue := true

	_, err := s.SetTymuxGlobalOverride(context.Background(), connect.NewRequest(&sessionv1.SetTymuxGlobalOverrideRequest{
		ForceTymux: &forceTrue,
	}))
	require.NoError(t, err)

	resp, err := s.GetTymuxRolloutStatus(context.Background(), connect.NewRequest(&sessionv1.GetTymuxRolloutStatusRequest{}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.GlobalOverride)
	assert.True(t, *resp.Msg.GlobalOverride)
}

func TestCompleteTymuxRollbackRehearsal_PersistsTimestamp(t *testing.T) {
	s := newIsolatedTymuxRolloutService(t)

	resp, err := s.CompleteTymuxRollbackRehearsal(context.Background(), connect.NewRequest(&sessionv1.CompleteTymuxRollbackRehearsalRequest{}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.RollbackRehearsalCompletedAt)
	assert.False(t, resp.Msg.RollbackRehearsalCompletedAt.AsTime().IsZero())

	// A fresh service instance backed by the same on-disk config sees the
	// persisted timestamp — confirms SaveConfig actually wrote it, not just
	// in-memory state on this *TymuxRolloutService.
	s2 := NewTymuxRolloutService()
	resp2, err := s2.GetTymuxRolloutStatus(context.Background(), connect.NewRequest(&sessionv1.GetTymuxRolloutStatusRequest{}))
	require.NoError(t, err)
	require.NotNil(t, resp2.Msg.RollbackRehearsalCompletedAt)
	assert.Equal(t, resp.Msg.RollbackRehearsalCompletedAt.AsTime(), resp2.Msg.RollbackRehearsalCompletedAt.AsTime())
}

// TestSetTymuxSessionOverride_RoundTrips confirms a set override is both
// returned directly and visible via a subsequent GetTymuxRolloutStatus call,
// and that clearing it (an unset ForceTymux) removes it from both — Phase
// 4's config.SetTymuxSessionOverride/TymuxSessionOverrides accessors now
// back this handler for real (see stream_hub_rollout_service_test.go's
// TestSetStreamHubSessionOverride_SetsAndClears for the precedent this
// mirrors).
func TestSetTymuxSessionOverride_RoundTrips(t *testing.T) {
	s := newIsolatedTymuxRolloutService(t)
	forceTrue := true

	resp, err := s.SetTymuxSessionOverride(context.Background(), connect.NewRequest(&sessionv1.SetTymuxSessionOverrideRequest{
		SessionName: "canary-session",
		ForceTymux:  &forceTrue,
	}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.SessionOverrides, 1)
	assert.Equal(t, "canary-session", resp.Msg.SessionOverrides[0].SessionName)
	assert.True(t, resp.Msg.SessionOverrides[0].ForceTymux)

	// The override is visible via a fresh GetTymuxRolloutStatus call too, not
	// just in the mutating RPC's own response.
	statusResp, err := s.GetTymuxRolloutStatus(context.Background(), connect.NewRequest(&sessionv1.GetTymuxRolloutStatusRequest{}))
	require.NoError(t, err)
	require.Len(t, statusResp.Msg.SessionOverrides, 1)
	assert.Equal(t, "canary-session", statusResp.Msg.SessionOverrides[0].SessionName)
	assert.True(t, statusResp.Msg.SessionOverrides[0].ForceTymux)

	// Unset ForceTymux clears the override (config.SetTymuxSessionOverride's
	// nil-means-clear convention).
	resp, err = s.SetTymuxSessionOverride(context.Background(), connect.NewRequest(&sessionv1.SetTymuxSessionOverrideRequest{
		SessionName: "canary-session",
	}))
	require.NoError(t, err)
	assert.Empty(t, resp.Msg.SessionOverrides)

	statusResp, err = s.GetTymuxRolloutStatus(context.Background(), connect.NewRequest(&sessionv1.GetTymuxRolloutStatusRequest{}))
	require.NoError(t, err)
	assert.Empty(t, statusResp.Msg.SessionOverrides)
}
