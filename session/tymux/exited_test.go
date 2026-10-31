package tymux

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
)

func deadSnapshot(exitCode *int32) *v1.PaneSnapshot {
	return &v1.PaneSnapshot{
		Liveness: v1.Liveness_LIVENESS_DEAD,
		ExitCode: exitCode,
		Grid:     []*v1.Row{{Cells: []*v1.Cell{{Text: "b"}, {Text: "y"}, {Text: "e"}}}},
	}
}

func TestCapturePaneContent_ShouldFailWithPaneExitedAndExitCode_WhenSnapshotIsDead(t *testing.T) {
	code := int32(3)
	sess := startedSession(t, deadSnapshot(&code))

	content, err := sess.CapturePaneContent()

	require.ErrorIs(t, err, ErrPaneExited)
	assert.Empty(t, content)
	var exited *PaneExitedError
	require.True(t, errors.As(err, &exited))
	require.NotNil(t, exited.ExitCode)
	assert.Equal(t, int32(3), *exited.ExitCode)
	assert.Contains(t, err.Error(), "exit code 3")
}

func TestCapturePaneContent_ShouldReportNoExitCode_WhenDaemonOmitsIt(t *testing.T) {
	sess := startedSession(t, deadSnapshot(nil))

	_, err := sess.CapturePaneContent()

	var exited *PaneExitedError
	require.True(t, errors.As(err, &exited))
	assert.Nil(t, exited.ExitCode)
}

func TestCapturePaneContent_ShouldSucceed_WhenSnapshotIsLiveOrUnspecified(t *testing.T) {
	for _, l := range []v1.Liveness{v1.Liveness_LIVENESS_LIVE, v1.Liveness_LIVENESS_UNSPECIFIED} {
		sess := startedSession(t, &v1.PaneSnapshot{Liveness: l, Grid: []*v1.Row{{Cells: []*v1.Cell{{Text: "x"}}}}})
		content, err := sess.CapturePaneContent()
		require.NoError(t, err, l.String())
		assert.Contains(t, content, "x")
	}
}

func TestExitedPane_ShouldStillYieldFinalGrid_ThroughRawWithOptionsViewportAndFinalScreen(t *testing.T) {
	sess := startedSession(t, deadSnapshot(nil))

	raw, err := sess.CapturePaneContentRaw()
	require.NoError(t, err)
	assert.Contains(t, raw, "bye")

	hist, err := sess.CapturePaneContentWithOptions("-", "-")
	require.NoError(t, err)
	assert.Equal(t, "bye", hist)

	view, err := sess.CaptureViewport(5)
	require.NoError(t, err)
	assert.Equal(t, "bye", view)

	final, err := FinalScreen(sess.CapturePaneContent())
	require.NoError(t, err)
	assert.Contains(t, final, "bye")
}

func TestExitedPane_ShouldReportNotAlive(t *testing.T) {
	sess := startedSession(t, deadSnapshot(nil))
	assert.False(t, sess.IsAlive())
}

func TestFinalScreen_ShouldPassOtherResultsThrough(t *testing.T) {
	boom := errors.New("boom")
	c, err := FinalScreen("keep", nil)
	assert.Equal(t, "keep", c)
	assert.NoError(t, err)
	c, err = FinalScreen("", boom)
	assert.Empty(t, c)
	assert.Same(t, boom, err)
}

func startedLifecycleSession(t *testing.T, snap *v1.PaneSnapshot, caps []string, status func() (*v1.PaneStatus, error)) TymuxManager {
	t.Helper()
	dir := t.TempDir()
	transport := &fakeTransport{
		capabilities: caps,
		createSessionFn: func(_ context.Context, _ *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
			return connect.NewResponse(fakeSession(testSessionID, testPaneID, dir, v1.Liveness_LIVENESS_LIVE)), nil
		},
		capturePaneFn: func(context.Context, *connect.Request[v1.CapturePaneRequest]) (*connect.Response[v1.PaneSnapshot], error) {
			return connect.NewResponse(snap), nil
		},
	}
	if status != nil {
		transport.paneStatusFn = func(_ context.Context, req *connect.Request[v1.GetPaneStatusRequest]) (*connect.Response[v1.PaneStatus], error) {
			assert.Equal(t, testPaneID, req.Msg.GetPaneId())
			st, err := status()
			if err != nil {
				return nil, err
			}
			return connect.NewResponse(st), nil
		}
	}
	sess := NewTymuxGRPCSession(transport)
	require.NoError(t, sess.Start(dir))
	return sess
}

func TestCapturePaneContent_RestartPending_FollowsDaemonPaneState(t *testing.T) {
	lifecycle := []string{capPaneLifecycle}
	statusOf := func(st v1.PaneState) func() (*v1.PaneStatus, error) {
		return func() (*v1.PaneStatus, error) { return &v1.PaneStatus{State: st}, nil }
	}
	cases := []struct {
		name    string
		caps    []string
		status  func() (*v1.PaneStatus, error)
		pending bool
	}{
		{"backoff", lifecycle, statusOf(v1.PaneState_PANE_STATE_BACKOFF), true},
		{"restarting", lifecycle, statusOf(v1.PaneState_PANE_STATE_RESTARTING), true},
		{"exited", lifecycle, statusOf(v1.PaneState_PANE_STATE_EXITED), false},
		{"failed", lifecycle, statusOf(v1.PaneState_PANE_STATE_FAILED), false},
		{"stopped", lifecycle, statusOf(v1.PaneState_PANE_STATE_STOPPED), false},
		{"status call fails", lifecycle, func() (*v1.PaneStatus, error) { return nil, errors.New("boom") }, false},
		{"daemon without pane.lifecycle", nil, func() (*v1.PaneStatus, error) {
			t.Error("GetPaneStatus must not be called without the capability")
			return nil, errors.New("unreachable")
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sess := startedLifecycleSession(t, deadSnapshot(nil), tc.caps, tc.status)

			_, err := sess.CapturePaneContent()

			require.ErrorIs(t, err, ErrPaneExited)
			var exited *PaneExitedError
			require.True(t, errors.As(err, &exited))
			assert.Equal(t, tc.pending, exited.RestartPending)
			assert.Equal(t, !tc.pending, IsFinalExit(err))
		})
	}
}

func TestIsFinalExit(t *testing.T) {
	assert.True(t, IsFinalExit(&PaneExitedError{}))
	assert.True(t, IsFinalExit(fmt.Errorf("wrapped: %w", &PaneExitedError{})))
	assert.False(t, IsFinalExit(&PaneExitedError{RestartPending: true}))
	assert.False(t, IsFinalExit(errors.New("other")))
	assert.False(t, IsFinalExit(nil))
}
