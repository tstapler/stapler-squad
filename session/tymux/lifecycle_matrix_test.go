package tymux

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
)

// Fault-injection matrix: daemon pane/session states x the operations stapler-squad runs against
// them. Each row states the documented expectation for that (state, operation) cell; the rules
// behind them:
//
//   - BACKOFF/RESTARTING are transient: the daemon is already restarting the pane, so the pane
//     counts as alive and stapler-squad must not revive or recreate it.
//   - FAILED/STOPPED are terminal until an operator acts: nothing may report them as revived, and
//     no new daemon session may be created in their place.
//   - A daemon that cannot be reached is not evidence of death: no revive, no create.

type matrixCalls struct{ create, revive, kill, list atomic.Int32 }

// paneState is one daemon-side pane condition, rendered as the answers the daemon would give.
type paneState struct {
	name string
	// state is what GetPaneStatus reports; PANE_STATE_UNSPECIFIED with lifecycle=false models a
	// daemon without the pane.lifecycle capability.
	state     v1.PaneState
	lifecycle bool
	liveness  v1.Liveness
	exitCode  *int32
}

func i32(v int32) *int32 { return &v }

var matrixStates = []paneState{
	{name: "LIVE", state: v1.PaneState_PANE_STATE_LIVE, lifecycle: true, liveness: v1.Liveness_LIVENESS_LIVE},
	{name: "EXITED_code0", state: v1.PaneState_PANE_STATE_EXITED, lifecycle: true, liveness: v1.Liveness_LIVENESS_DEAD, exitCode: i32(0)},
	{name: "EXITED_nonzero", state: v1.PaneState_PANE_STATE_EXITED, lifecycle: true, liveness: v1.Liveness_LIVENESS_DEAD, exitCode: i32(7)},
	{name: "BACKOFF", state: v1.PaneState_PANE_STATE_BACKOFF, lifecycle: true, liveness: v1.Liveness_LIVENESS_DEAD, exitCode: i32(1)},
	{name: "RESTARTING", state: v1.PaneState_PANE_STATE_RESTARTING, lifecycle: true, liveness: v1.Liveness_LIVENESS_DEAD},
	{name: "FAILED", state: v1.PaneState_PANE_STATE_FAILED, lifecycle: true, liveness: v1.Liveness_LIVENESS_DEAD, exitCode: i32(1)},
	{name: "STOPPED", state: v1.PaneState_PANE_STATE_STOPPED, lifecycle: true, liveness: v1.Liveness_LIVENESS_DEAD},
	{name: "EXITED_old_daemon", state: v1.PaneState_PANE_STATE_UNSPECIFIED, lifecycle: false, liveness: v1.Liveness_LIVENESS_DEAD, exitCode: i32(0)},
}

func (p paneState) restartPending() bool {
	return p.state == v1.PaneState_PANE_STATE_BACKOFF || p.state == v1.PaneState_PANE_STATE_RESTARTING
}

func (p paneState) terminal() bool {
	return p.state == v1.PaneState_PANE_STATE_FAILED || p.state == v1.PaneState_PANE_STATE_STOPPED
}

func (p paneState) snapshot(paneID string) *v1.PaneSnapshot {
	snap := &v1.PaneSnapshot{PaneId: paneID, Liveness: p.liveness}
	snap.ExitCode = p.exitCode
	return snap
}

// matrixTransport serves state to every RPC, counting the mutating ones.
func matrixTransport(dir string, p paneState, calls *matrixCalls) *fakeTransport {
	listed := func() *v1.Session {
		s := fakeSession(testSessionID, testPaneID, dir, p.liveness)
		s.Name = dir
		return s
	}
	ft := &fakeTransport{
		createSessionFn: func(context.Context, *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
			calls.create.Add(1)
			s := fakeSession(testSessionID, testPaneID, dir, v1.Liveness_LIVENESS_LIVE)
			s.Name = dir
			return connect.NewResponse(s), nil
		},
		killSessionFn: func(context.Context, *connect.Request[v1.KillSessionRequest]) (*connect.Response[v1.KillSessionResponse], error) {
			calls.kill.Add(1)
			return connect.NewResponse(&v1.KillSessionResponse{}), nil
		},
		listSessionsFn: func(context.Context, *connect.Request[v1.ListSessionsRequest]) (*connect.Response[v1.ListSessionsResponse], error) {
			calls.list.Add(1)
			return connect.NewResponse(&v1.ListSessionsResponse{Sessions: []*v1.Session{listed()}}), nil
		},
		reviveSessionFn: func(context.Context, *connect.Request[v1.ReviveSessionRequest]) (*connect.Response[v1.ReviveSessionResponse], error) {
			calls.revive.Add(1)
			// tymuxd v1.12: revive respawns EXITED panes and skips STOPPED/FAILED.
			if p.state == v1.PaneState_PANE_STATE_EXITED || !p.lifecycle {
				s := fakeSession(testSessionID, testPaneID, dir, v1.Liveness_LIVENESS_LIVE)
				return connect.NewResponse(&v1.ReviveSessionResponse{PaneCount: 1, Session: s}), nil
			}
			return connect.NewResponse(&v1.ReviveSessionResponse{Session: listed()}), nil
		},
		capturePaneFn: func(_ context.Context, req *connect.Request[v1.CapturePaneRequest]) (*connect.Response[v1.PaneSnapshot], error) {
			return connect.NewResponse(p.snapshot(req.Msg.GetPaneId())), nil
		},
		paneStatusFn: func(_ context.Context, req *connect.Request[v1.GetPaneStatusRequest]) (*connect.Response[v1.PaneStatus], error) {
			return connect.NewResponse(&v1.PaneStatus{PaneId: req.Msg.GetPaneId(), State: p.state}), nil
		},
	}
	if p.lifecycle {
		ft.capabilities = []string{capPaneLifecycle}
	}
	return ft
}

// startedSession is a session Start()ed against a LIVE daemon; the caller then swaps in the state
// under test via the returned transport's capture/status fns (already state-driven).
func startedMatrixSession(t *testing.T, dir string, ft *fakeTransport) TymuxManager {
	t.Helper()
	resetClaims(t)
	sess := NewTymuxGRPCSession(ft)
	require.NoError(t, sess.Start(dir))
	t.Cleanup(func() { _ = sess.Close() })
	return sess
}

func TestLifecycleMatrix_IsAlive(t *testing.T) {
	for _, p := range matrixStates {
		t.Run(p.name, func(t *testing.T) {
			dir := t.TempDir()
			sess := startedMatrixSession(t, dir, matrixTransport(dir, p, &matrixCalls{}))
			want := p.liveness == v1.Liveness_LIVENESS_LIVE || p.restartPending()
			assert.Equal(t, want, sess.IsAlive())
			assert.True(t, sess.HasSession(), "HasSession reports initialisation, not liveness")
		})
	}
}

func TestLifecycleMatrix_CapturePaneContent(t *testing.T) {
	for _, p := range matrixStates {
		t.Run(p.name, func(t *testing.T) {
			dir := t.TempDir()
			sess := startedMatrixSession(t, dir, matrixTransport(dir, p, &matrixCalls{}))
			_, err := sess.CapturePaneContent()
			if p.liveness == v1.Liveness_LIVENESS_LIVE {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, ErrPaneExited, "a dead pane must not read as a current screen")
			var exited *PaneExitedError
			require.True(t, errors.As(err, &exited))
			assert.Equal(t, p.restartPending(), exited.RestartPending)
			assert.Equal(t, !p.restartPending(), IsFinalExit(err))
			if p.exitCode != nil {
				require.NotNil(t, exited.ExitCode)
				assert.Equal(t, *p.exitCode, *exited.ExitCode)
			}
			// The raw capture deliberately keeps returning the final grid.
			_, rawErr := sess.CapturePaneContentRaw()
			assert.NoError(t, rawErr)
		})
	}
}

func TestLifecycleMatrix_Restore(t *testing.T) {
	for _, p := range matrixStates {
		t.Run(p.name, func(t *testing.T) {
			resetClaims(t)
			dir := t.TempDir()
			calls := &matrixCalls{}
			sess := NewTymuxGRPCSession(matrixTransport(dir, p, calls))
			t.Cleanup(func() { _ = sess.Close() })

			err := sess.RestoreWithWorkDir(dir)

			assert.Zero(t, calls.create.Load(), "restore must adopt the listed session, never create a duplicate")
			switch {
			case p.terminal():
				require.ErrorIs(t, err, ErrPaneNotRevivable)
				assert.Zero(t, calls.revive.Load(), "FAILED/STOPPED panes are not the daemon's to respawn")
			case p.restartPending():
				require.NoError(t, err)
				assert.Zero(t, calls.revive.Load(), "tymuxd is already restarting this pane; reviving would bypass its backoff")
			case p.liveness == v1.Liveness_LIVENESS_LIVE:
				require.NoError(t, err)
				assert.Zero(t, calls.revive.Load())
			default:
				require.NoError(t, err)
				assert.Equal(t, int32(1), calls.revive.Load())
			}
		})
	}
}

// A daemon without pane.lifecycle cannot be asked for the pane state; a revive that leaves the
// session dead is the only signal that the pane was skipped.
func TestLifecycleMatrix_Restore_ReviveLeavesSessionDead(t *testing.T) {
	resetClaims(t)
	dir := t.TempDir()
	calls := &matrixCalls{}
	p := paneState{name: "skipped", state: v1.PaneState_PANE_STATE_STOPPED, lifecycle: true, liveness: v1.Liveness_LIVENESS_DEAD}
	ft := matrixTransport(dir, p, calls)
	ft.capabilities = nil // pre-1.12 daemon: no GetServerInfo capability, so no state lookup
	sess := NewTymuxGRPCSession(ft)
	t.Cleanup(func() { _ = sess.Close() })

	err := sess.RestoreWithWorkDir(dir)

	require.ErrorIs(t, err, ErrPaneNotRevivable)
	assert.Equal(t, int32(1), calls.revive.Load())
	assert.Zero(t, calls.create.Load())
}

// A pane that went LIVE between ListSessions and ReviveSession reports already_live with
// pane_count=0 (also what a daemon restart with old semantics returns): that is success.
func TestLifecycleMatrix_Restore_ReviveAlreadyLive(t *testing.T) {
	resetClaims(t)
	dir := t.TempDir()
	calls := &matrixCalls{}
	p := paneState{state: v1.PaneState_PANE_STATE_EXITED, lifecycle: true, liveness: v1.Liveness_LIVENESS_DEAD}
	ft := matrixTransport(dir, p, calls)
	ft.reviveSessionFn = func(context.Context, *connect.Request[v1.ReviveSessionRequest]) (*connect.Response[v1.ReviveSessionResponse], error) {
		calls.revive.Add(1)
		s := fakeSession(testSessionID, testPaneID, dir, v1.Liveness_LIVENESS_LIVE)
		return connect.NewResponse(&v1.ReviveSessionResponse{AlreadyLive: true, Session: s}), nil
	}
	sess := NewTymuxGRPCSession(ft)
	t.Cleanup(func() { _ = sess.Close() })

	require.NoError(t, sess.RestoreWithWorkDir(dir))
	assert.Zero(t, calls.create.Load())
	assert.Equal(t, testSessionID, sess.GetSessionIdentifier())
}

func TestLifecycleMatrix_Restore_DaemonFaults(t *testing.T) {
	unavailable := connect.NewError(connect.CodeUnavailable, errors.New("connection refused"))
	notFound := connect.NewError(connect.CodeNotFound, errors.New("no such session"))
	dead := paneState{state: v1.PaneState_PANE_STATE_EXITED, lifecycle: true, liveness: v1.Liveness_LIVENESS_DEAD}

	t.Run("list unavailable: no create, no revive", func(t *testing.T) {
		resetClaims(t)
		dir := t.TempDir()
		calls := &matrixCalls{}
		ft := matrixTransport(dir, dead, calls)
		ft.listSessionsFn = func(context.Context, *connect.Request[v1.ListSessionsRequest]) (*connect.Response[v1.ListSessionsResponse], error) {
			return nil, unavailable
		}
		err := NewTymuxGRPCSession(ft).RestoreWithWorkDir(dir)
		require.ErrorIs(t, err, ErrTymuxdUnreachable)
		assert.Zero(t, calls.create.Load())
		assert.Zero(t, calls.revive.Load())
	})
	t.Run("revive unavailable: error, claim released, no create", func(t *testing.T) {
		resetClaims(t)
		dir := t.TempDir()
		calls := &matrixCalls{}
		ft := matrixTransport(dir, dead, calls)
		ft.reviveSessionFn = func(context.Context, *connect.Request[v1.ReviveSessionRequest]) (*connect.Response[v1.ReviveSessionResponse], error) {
			calls.revive.Add(1)
			return nil, unavailable
		}
		require.ErrorIs(t, NewTymuxGRPCSession(ft).RestoreWithWorkDir(dir), ErrTymuxdUnreachable)
		assert.Zero(t, calls.create.Load())
		_, held := claimedSessions.Load(testSessionID)
		assert.False(t, held, "a failed restore must not leave the session claimed")
		// A second owner can now adopt it.
		ft2 := matrixTransport(dir, paneState{liveness: v1.Liveness_LIVENESS_LIVE, state: v1.PaneState_PANE_STATE_LIVE, lifecycle: true}, &matrixCalls{})
		s2 := NewTymuxGRPCSession(ft2)
		t.Cleanup(func() { _ = s2.Close() })
		require.NoError(t, s2.RestoreWithWorkDir(dir))
	})
	t.Run("revive not_found: error, no create", func(t *testing.T) {
		resetClaims(t)
		dir := t.TempDir()
		calls := &matrixCalls{}
		ft := matrixTransport(dir, dead, calls)
		ft.reviveSessionFn = func(context.Context, *connect.Request[v1.ReviveSessionRequest]) (*connect.Response[v1.ReviveSessionResponse], error) {
			calls.revive.Add(1)
			return nil, notFound
		}
		require.Error(t, NewTymuxGRPCSession(ft).RestoreWithWorkDir(dir))
		assert.Zero(t, calls.create.Load())
	})
	t.Run("session absent: one fresh create", func(t *testing.T) {
		resetClaims(t)
		dir := t.TempDir()
		calls := &matrixCalls{}
		ft := matrixTransport(dir, dead, calls)
		ft.listSessionsFn = func(context.Context, *connect.Request[v1.ListSessionsRequest]) (*connect.Response[v1.ListSessionsResponse], error) {
			return connect.NewResponse(&v1.ListSessionsResponse{}), nil
		}
		sess := NewTymuxGRPCSession(ft)
		t.Cleanup(func() { _ = sess.Close() })
		require.NoError(t, sess.RestoreWithWorkDir(dir))
		assert.Equal(t, int32(1), calls.create.Load())
		assert.Zero(t, calls.revive.Load())
	})
}

func TestLifecycleMatrix_IsAlive_DaemonFaults(t *testing.T) {
	live := paneState{state: v1.PaneState_PANE_STATE_LIVE, lifecycle: true, liveness: v1.Liveness_LIVENESS_LIVE}
	unavailable := connect.NewError(connect.CodeUnavailable, errors.New("connection refused"))
	notFound := connect.NewError(connect.CodeNotFound, errors.New("no such pane"))

	t.Run("daemon unreachable keeps the cached answer", func(t *testing.T) {
		dir := t.TempDir()
		ft := matrixTransport(dir, live, &matrixCalls{})
		sess := startedMatrixSession(t, dir, ft)
		ft.capturePaneFn = func(context.Context, *connect.Request[v1.CapturePaneRequest]) (*connect.Response[v1.PaneSnapshot], error) {
			return nil, unavailable
		}
		assert.True(t, sess.IsAlive())
		_, err := sess.CapturePaneContent()
		require.ErrorIs(t, err, ErrTymuxdUnreachable)
		assert.NotErrorIs(t, err, ErrPaneExited)
	})
	t.Run("pane not_found and session gone is dead", func(t *testing.T) {
		dir := t.TempDir()
		ft := matrixTransport(dir, live, &matrixCalls{})
		sess := startedMatrixSession(t, dir, ft)
		ft.capturePaneFn = func(context.Context, *connect.Request[v1.CapturePaneRequest]) (*connect.Response[v1.PaneSnapshot], error) {
			return nil, notFound
		}
		ft.listSessionsFn = func(context.Context, *connect.Request[v1.ListSessionsRequest]) (*connect.Response[v1.ListSessionsResponse], error) {
			return connect.NewResponse(&v1.ListSessionsResponse{}), nil
		}
		assert.False(t, sess.IsAlive())
		_, err := sess.CapturePaneContent()
		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrPaneExited)
	})
	t.Run("pane not_found but daemon lists it live is transient", func(t *testing.T) {
		dir := t.TempDir()
		ft := matrixTransport(dir, live, &matrixCalls{})
		sess := startedMatrixSession(t, dir, ft)
		ft.capturePaneFn = func(context.Context, *connect.Request[v1.CapturePaneRequest]) (*connect.Response[v1.PaneSnapshot], error) {
			return nil, notFound
		}
		assert.True(t, sess.IsAlive())
	})
	t.Run("pane status call failing on a dead pane means plain exit", func(t *testing.T) {
		dir := t.TempDir()
		p := matrixStates[3] // BACKOFF
		ft := matrixTransport(dir, p, &matrixCalls{})
		ft.paneStatusFn = func(context.Context, *connect.Request[v1.GetPaneStatusRequest]) (*connect.Response[v1.PaneStatus], error) {
			return nil, unavailable
		}
		sess := startedMatrixSession(t, dir, ft)
		assert.False(t, sess.IsAlive(), "an unanswerable state query must not be read as restart pending")
	})
}

func TestLifecycleMatrix_Close(t *testing.T) {
	for _, p := range matrixStates {
		t.Run(p.name, func(t *testing.T) {
			dir := t.TempDir()
			calls := &matrixCalls{}
			sess := startedMatrixSession(t, dir, matrixTransport(dir, p, calls))
			require.NoError(t, sess.Close())
			require.NoError(t, sess.Close(), "Close is idempotent")
			assert.Equal(t, int32(1), calls.kill.Load(), "exactly one KillSession across repeated Close")
			assert.False(t, sess.IsAlive(), "a closed session is never alive, whatever the daemon says")
		})
	}
}

func TestLifecycleMatrix_Close_DaemonFaults(t *testing.T) {
	live := paneState{state: v1.PaneState_PANE_STATE_LIVE, lifecycle: true, liveness: v1.Liveness_LIVENESS_LIVE}
	t.Run("kill not_found is success", func(t *testing.T) {
		dir := t.TempDir()
		ft := matrixTransport(dir, live, &matrixCalls{})
		sess := startedMatrixSession(t, dir, ft)
		ft.killSessionFn = func(context.Context, *connect.Request[v1.KillSessionRequest]) (*connect.Response[v1.KillSessionResponse], error) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("gone"))
		}
		assert.NoError(t, sess.Close())
	})
	t.Run("kill unavailable surfaces unreachable once", func(t *testing.T) {
		dir := t.TempDir()
		var kills atomic.Int32
		ft := matrixTransport(dir, live, &matrixCalls{})
		sess := startedMatrixSession(t, dir, ft)
		ft.killSessionFn = func(context.Context, *connect.Request[v1.KillSessionRequest]) (*connect.Response[v1.KillSessionResponse], error) {
			kills.Add(1)
			return nil, connect.NewError(connect.CodeUnavailable, errors.New("refused"))
		}
		require.ErrorIs(t, sess.Close(), ErrTymuxdUnreachable)
		_ = sess.Close()
		assert.Equal(t, int32(1), kills.Load(), "bounded: a failed kill is not retried by the next Close")
	})
}

func TestLifecycleMatrix_Input_DaemonFaults(t *testing.T) {
	live := paneState{state: v1.PaneState_PANE_STATE_LIVE, lifecycle: true, liveness: v1.Liveness_LIVENESS_LIVE}
	dir := t.TempDir()
	stream := newFakeAttachStream(context.Background())
	ft := matrixTransport(dir, live, &matrixCalls{})
	ft.attachFn = func(ctx context.Context) attachStream { stream.ctx = ctx; return stream }
	sess := startedMatrixSession(t, dir, ft)

	n, err := sess.SendKeys("x")
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	require.NoError(t, sess.SetWindowSize(80, 24))

	stream.sendFn = func(*v1.AttachRequest) error {
		return connect.NewError(connect.CodeUnavailable, errors.New("refused"))
	}
	_, err = sess.SendKeys("y")
	require.ErrorIs(t, err, ErrTymuxdUnreachable)
	require.ErrorIs(t, sess.SetWindowSize(80, 24), ErrTymuxdUnreachable)
}

func TestLifecycleMatrix_NotStarted(t *testing.T) {
	sess := NewTymuxGRPCSession(&fakeTransport{})
	assert.False(t, sess.IsAlive())
	assert.False(t, sess.HasSession())
	_, err := sess.SendKeys("x")
	require.Error(t, err)
	_, err = sess.CapturePaneContent()
	require.Error(t, err)
	assert.NoError(t, sess.Close(), "Close before Start is a no-op")
}
