package services

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/tymux"
)

// newIsolatedTymuxRolloutService creates a TymuxRolloutService backed by a
// fresh temporary directory, preventing config state from leaking between
// tests (same isolation pattern as newIsolatedStreamHubRolloutService).
func newIsolatedTymuxRolloutService(t *testing.T) *TymuxRolloutService {
	t.Helper()
	withIsolatedStateDir(t)
	s := NewTymuxRolloutService()
	// No real lsof/ss/exec probes in unit tests; version-specific tests override these.
	s.detectDaemon = func(context.Context, tymux.DaemonConfig) tymux.DaemonVersionInfo { return tymux.DaemonVersionInfo{} }
	s.replaceDaemon = func(context.Context, tymux.DaemonConfig, bool) (tymux.DaemonVersionInfo, error) {
		return tymux.DaemonVersionInfo{}, errors.New("replaceDaemon not stubbed")
	}
	return s
}

func TestGetTymuxRolloutStatus_ReportsDaemonVersions(t *testing.T) {
	s := newIsolatedTymuxRolloutService(t)
	s.detectDaemon = func(context.Context, tymux.DaemonConfig) tymux.DaemonVersionInfo {
		return tymux.DaemonVersionInfo{Running: true, PID: 99, RunningVersion: "1.3.0", InstalledVersion: "1.2.0", Direction: tymux.VersionDowngrade}
	}

	resp, err := s.GetTymuxRolloutStatus(context.Background(), connect.NewRequest(&sessionv1.GetTymuxRolloutStatusRequest{}))

	require.NoError(t, err)
	d := resp.Msg.GetDaemon()
	assert.True(t, d.GetRunning())
	assert.Equal(t, int32(99), d.GetPid())
	assert.Equal(t, "1.3.0", d.GetRunningVersion())
	assert.Equal(t, "1.2.0", d.GetInstalledVersion())
	assert.Equal(t, sessionv1.TymuxdVersionDirection_TYMUXD_VERSION_DIRECTION_DOWNGRADE, d.GetDirection())
}

func TestReplaceTymuxd_MapsOutcomes(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantCode  connect.Code
		wantError bool
	}{
		{"ok", nil, 0, false},
		{"live sessions", fmt.Errorf("%w (2 live)", tymux.ErrLiveSessionsWouldBeInterrupted), connect.CodeFailedPrecondition, true},
		{"other failure", errors.New("boom"), connect.CodeInternal, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newIsolatedTymuxRolloutService(t)
			var gotAllow bool
			s.replaceDaemon = func(_ context.Context, _ tymux.DaemonConfig, allow bool) (tymux.DaemonVersionInfo, error) {
				gotAllow = allow
				return tymux.DaemonVersionInfo{Running: true, RunningVersion: "1.3.0"}, tc.err
			}

			resp, err := s.ReplaceTymuxd(context.Background(), connect.NewRequest(&sessionv1.ReplaceTymuxdRequest{AllowInterruptLiveSessions: true}))

			assert.True(t, gotAllow, "the opt-in flag must reach the replace call")
			if tc.wantError {
				require.Error(t, err)
				assert.Equal(t, tc.wantCode, connect.CodeOf(err))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "1.3.0", resp.Msg.GetDaemon().GetRunningVersion())
		})
	}
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
	//nolint:staticcheck // SA1019: intentionally reading the deprecated field — this test's whole point is asserting it's frozen false.
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
