//go:build integration

package tymux

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
	"github.com/tstapler/tymux/clients/go/gen/tymux/v1/tymuxv1connect"

	"github.com/tstapler/stapler-squad/envtest"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

// lifecycleDaemon starts a real tymuxd and returns a raw client for staging pane states the
// stapler-squad session API cannot create itself (restart policies, StopPane).
func lifecycleDaemon(t *testing.T) (addr string, raw tymuxv1connect.TymuxServiceClient) {
	t.Helper()
	bin := findTymuxdBinary(t)
	envtest.NewIsolatedStateDir(t)
	cfg, _, err := ensureDaemonOnFreshLoopbackPort(t, bin)
	t.Cleanup(func() { _ = stopTymuxdFn() })
	require.NoError(t, err)
	return cfg.Addr, newTymuxClient(newH2CClient(), cfg.Addr)
}

func createStagedSession(t *testing.T, raw tymuxv1connect.TymuxServiceClient, dir string, req *v1.CreateSessionRequest) *v1.Session {
	t.Helper()
	req.Name, req.Cwd = dir, dir
	resp, err := raw.CreateSession(context.Background(), connect.NewRequest(req))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = raw.KillSession(context.Background(), connect.NewRequest(&v1.KillSessionRequest{SessionId: resp.Msg.GetId()}))
	})
	return resp.Msg
}

func waitPaneState(t *testing.T, raw tymuxv1connect.TymuxServiceClient, paneID string, want v1.PaneState) {
	t.Helper()
	wait.RequireEventually(t, func() bool {
		resp, err := raw.GetPaneStatus(context.Background(), connect.NewRequest(&v1.GetPaneStatusRequest{PaneId: paneID}))
		return err == nil && resp.Msg.GetState() == want
	}, 10*time.Second, 50*time.Millisecond, "pane never reached %s", want)
}

func paneRestartCount(t *testing.T, raw tymuxv1connect.TymuxServiceClient, paneID string) uint32 {
	t.Helper()
	resp, err := raw.GetPaneStatus(context.Background(), connect.NewRequest(&v1.GetPaneStatusRequest{PaneId: paneID}))
	require.NoError(t, err)
	return resp.Msg.GetRestartCount()
}

// sessionCount counts daemon sessions named name; the daemon may hold unrelated persisted ones.
func sessionCount(t *testing.T, raw tymuxv1connect.TymuxServiceClient, name string) int {
	t.Helper()
	resp, err := raw.ListSessions(context.Background(), connect.NewRequest(&v1.ListSessionsRequest{}))
	require.NoError(t, err)
	n := 0
	for _, s := range resp.Msg.GetSessions() {
		if s.GetName() == name {
			n++
		}
	}
	return n
}

// A pane in BACKOFF is alive to stapler-squad and a restore adopts it without reviving it
// underneath the daemon's backoff or creating a second session.
func TestIntegration_Lifecycle_BackoffPaneIsAdoptedNotRevived(t *testing.T) {
	resetClaims(t)
	addr, raw := lifecycleDaemon(t)
	dir := t.TempDir()
	delay := uint64(600_000)
	maxDelay := delay
	sess := createStagedSession(t, raw, dir, &v1.CreateSessionRequest{
		Command: "/bin/sh", Args: []string{"-c", "exit 3"},
		RestartPolicy: &v1.RestartPolicy{Mode: v1.RestartMode_RESTART_MODE_ON_FAILURE, InitialDelayMs: &delay, MaxDelayMs: &maxDelay},
	})
	paneID := firstPane(sess).GetId()
	waitPaneState(t, raw, paneID, v1.PaneState_PANE_STATE_BACKOFF)
	before := paneRestartCount(t, raw, paneID)

	mgr := NewTymuxGRPCSession(NewRealTransport(addr))
	t.Cleanup(func() { _ = mgr.Close() })
	require.NoError(t, mgr.RestoreWithWorkDir(dir))

	assert.True(t, mgr.IsAlive(), "BACKOFF is transient, not dead")
	assert.Equal(t, 1, sessionCount(t, raw, dir), "no duplicate daemon session")
	assert.Equal(t, before, paneRestartCount(t, raw, paneID), "restore must not respawn the pane")
	_, err := mgr.CapturePaneContent()
	var exited *PaneExitedError
	require.ErrorAs(t, err, &exited)
	assert.True(t, exited.RestartPending)
}

// A pane an operator stopped is not revivable: restore must say so instead of reporting success.
func TestIntegration_Lifecycle_StoppedPaneIsNotRevived(t *testing.T) {
	resetClaims(t)
	addr, raw := lifecycleDaemon(t)
	dir := t.TempDir()
	sess := createStagedSession(t, raw, dir, &v1.CreateSessionRequest{
		Command: "/bin/sh", Args: []string{"-c", "sleep 300"},
		RestartPolicy: &v1.RestartPolicy{Mode: v1.RestartMode_RESTART_MODE_ON_FAILURE},
	})
	paneID := firstPane(sess).GetId()
	_, err := raw.StopPane(context.Background(), connect.NewRequest(&v1.StopPaneRequest{PaneId: paneID}))
	require.NoError(t, err)
	waitPaneState(t, raw, paneID, v1.PaneState_PANE_STATE_STOPPED)

	mgr := NewTymuxGRPCSession(NewRealTransport(addr))
	t.Cleanup(func() { _ = mgr.Close() })
	err = mgr.RestoreWithWorkDir(dir)

	require.ErrorIs(t, err, ErrPaneNotRevivable)
	assert.Equal(t, 1, sessionCount(t, raw, dir), "no duplicate daemon session")
	assert.False(t, mgr.IsAlive())
}
