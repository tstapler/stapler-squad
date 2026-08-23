package tymux

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"

	"github.com/tstapler/stapler-squad/session/tmux"
)

// fakeTransport is a hand-driven rpcTransport substitute (Task 2.2.1d /
// 2.2.6c) — tymuxGRPCSession owns the generated Connect-Go client directly
// with rpcTransport as the one seam beneath it, so these tests exercise
// Start/Close/IsAlive/RestoreWithWorkDir/capture/error-classification logic
// without a live tymuxd daemon.
type fakeTransport struct {
	createSessionFn func(context.Context, *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error)
	killSessionFn   func(context.Context, *connect.Request[v1.KillSessionRequest]) (*connect.Response[v1.KillSessionResponse], error)
	listSessionsFn  func(context.Context, *connect.Request[v1.ListSessionsRequest]) (*connect.Response[v1.ListSessionsResponse], error)
	reviveSessionFn func(context.Context, *connect.Request[v1.ReviveSessionRequest]) (*connect.Response[v1.ReviveSessionResponse], error)
	capturePaneFn   func(context.Context, *connect.Request[v1.CapturePaneRequest]) (*connect.Response[v1.PaneSnapshot], error)

	// attachFn lets a test substitute a custom attachStream (e.g. one that
	// plays back a scripted sequence of AttachEvents); defaults to a
	// working fakeAttachStream (stream_test.go) so every pre-Epic-2.3 test
	// above, which never configured this, keeps passing against the
	// standing stream Start()/RestoreWithWorkDir() now open.
	attachFn func(context.Context) attachStream
	// attachCalls counts every Attach() call — Story 2.3.1's acceptance
	// test asserts exactly one standing stream is opened per session,
	// never one per SendKeys call.
	attachCalls int32
}

func (f *fakeTransport) CreateSession(ctx context.Context, req *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
	if f.createSessionFn == nil {
		return nil, errors.New("fakeTransport: CreateSession not configured")
	}
	return f.createSessionFn(ctx, req)
}

func (f *fakeTransport) KillSession(ctx context.Context, req *connect.Request[v1.KillSessionRequest]) (*connect.Response[v1.KillSessionResponse], error) {
	if f.killSessionFn == nil {
		return connect.NewResponse(&v1.KillSessionResponse{}), nil
	}
	return f.killSessionFn(ctx, req)
}

func (f *fakeTransport) ListSessions(ctx context.Context, req *connect.Request[v1.ListSessionsRequest]) (*connect.Response[v1.ListSessionsResponse], error) {
	if f.listSessionsFn == nil {
		return connect.NewResponse(&v1.ListSessionsResponse{}), nil
	}
	return f.listSessionsFn(ctx, req)
}

func (f *fakeTransport) ReviveSession(ctx context.Context, req *connect.Request[v1.ReviveSessionRequest]) (*connect.Response[v1.ReviveSessionResponse], error) {
	if f.reviveSessionFn == nil {
		return nil, errors.New("fakeTransport: ReviveSession not configured")
	}
	return f.reviveSessionFn(ctx, req)
}

func (f *fakeTransport) CapturePane(ctx context.Context, req *connect.Request[v1.CapturePaneRequest]) (*connect.Response[v1.PaneSnapshot], error) {
	if f.capturePaneFn == nil {
		return nil, errors.New("fakeTransport: CapturePane not configured")
	}
	return f.capturePaneFn(ctx, req)
}

func (f *fakeTransport) Attach(ctx context.Context) attachStream {
	atomic.AddInt32(&f.attachCalls, 1)
	if f.attachFn != nil {
		return f.attachFn(ctx)
	}
	return newFakeAttachStream(ctx)
}

var _ rpcTransport = (*fakeTransport)(nil)

// fakeSession builds a *v1.Session shaped like a real CreateSession/
// ReviveSession response: one window, one leaf pane.
func fakeSession(sessionID, paneID, cwd string, liveness v1.Liveness) *v1.Session {
	return &v1.Session{
		Id:       sessionID,
		Liveness: liveness,
		Windows: []*v1.Window{
			{
				Id: "window-1",
				Layout: &v1.Layout{
					Node: &v1.Layout_Pane{
						Pane: &v1.Pane{
							Id:       paneID,
							Liveness: liveness,
							Cwd:      cwd,
						},
					},
				},
			},
		},
	}
}

func TestTymuxGRPCSession_Start_CreatesSessionAndCachesIdentifiers(t *testing.T) {
	dir := t.TempDir()
	var gotReq *v1.CreateSessionRequest
	transport := &fakeTransport{
		createSessionFn: func(_ context.Context, req *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
			gotReq = req.Msg
			return connect.NewResponse(fakeSession("sess-1", "pane-1", dir, v1.Liveness_LIVENESS_LIVE)), nil
		},
	}
	sess := NewTymuxGRPCSession(transport)

	err := sess.Start(dir)

	require.NoError(t, err)
	require.NotNil(t, gotReq)
	assert.Equal(t, dir, gotReq.GetCwd())
	assert.Equal(t, "sess-1", sess.GetSessionIdentifier())
	assert.True(t, sess.HasSession())
}

func TestTymuxGRPCSession_Start_RejectsMissingWorkDir(t *testing.T) {
	sess := NewTymuxGRPCSession(&fakeTransport{})

	err := sess.Start("")

	require.Error(t, err)
	assert.ErrorIs(t, err, tmux.ErrWorkDirMissing)
	assert.False(t, sess.HasSession())
}

func TestTymuxGRPCSession_Start_RejectsNonexistentWorkDir(t *testing.T) {
	sess := NewTymuxGRPCSession(&fakeTransport{})

	err := sess.Start(filepath.Join(t.TempDir(), "does-not-exist"))

	require.Error(t, err)
	assert.ErrorIs(t, err, tmux.ErrWorkDirMissing)
}

func TestTymuxGRPCSession_StartThenIsAlive_ReturnsTrueImmediately(t *testing.T) {
	dir := t.TempDir()
	transport := &fakeTransport{
		createSessionFn: func(_ context.Context, _ *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
			return connect.NewResponse(fakeSession("sess-1", "pane-1", dir, v1.Liveness_LIVENESS_LIVE)), nil
		},
		capturePaneFn: func(_ context.Context, req *connect.Request[v1.CapturePaneRequest]) (*connect.Response[v1.PaneSnapshot], error) {
			assert.Equal(t, "pane-1", req.Msg.GetPaneId())
			return connect.NewResponse(&v1.PaneSnapshot{
				PaneId:   "pane-1",
				Liveness: v1.Liveness_LIVENESS_LIVE,
			}), nil
		},
	}
	sess := NewTymuxGRPCSession(transport)
	require.NoError(t, sess.Start(dir))

	assert.True(t, sess.IsAlive())
}

func TestTymuxGRPCSession_IsAlive_FalseBeforeStart(t *testing.T) {
	sess := NewTymuxGRPCSession(&fakeTransport{})
	assert.False(t, sess.IsAlive())
	assert.False(t, sess.HasSession())
}

func TestTymuxGRPCSession_Close_IssuesKillSessionAndIsAliveThenFalse(t *testing.T) {
	dir := t.TempDir()
	var killedID string
	transport := &fakeTransport{
		createSessionFn: func(_ context.Context, _ *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
			return connect.NewResponse(fakeSession("sess-1", "pane-1", dir, v1.Liveness_LIVENESS_LIVE)), nil
		},
		killSessionFn: func(_ context.Context, req *connect.Request[v1.KillSessionRequest]) (*connect.Response[v1.KillSessionResponse], error) {
			killedID = req.Msg.GetSessionId()
			return connect.NewResponse(&v1.KillSessionResponse{}), nil
		},
	}
	sess := NewTymuxGRPCSession(transport)
	require.NoError(t, sess.Start(dir))

	require.NoError(t, sess.Close())

	assert.Equal(t, "sess-1", killedID)
	assert.False(t, sess.IsAlive())
}

func TestTymuxGRPCSession_RestoreWithWorkDir_RevivesDeadSession(t *testing.T) {
	dir := t.TempDir()
	var revivedID string
	// tymuxGRPCSession.Start's Name convention: RestoreWithWorkDir matches
	// ListSessions results by Session.Name == workDir.
	existing := fakeSession("sess-1", "pane-1", dir, v1.Liveness_LIVENESS_DEAD)
	existing.Name = dir
	transport := &fakeTransport{
		listSessionsFn: func(_ context.Context, _ *connect.Request[v1.ListSessionsRequest]) (*connect.Response[v1.ListSessionsResponse], error) {
			return connect.NewResponse(&v1.ListSessionsResponse{Sessions: []*v1.Session{existing}}), nil
		},
		reviveSessionFn: func(_ context.Context, req *connect.Request[v1.ReviveSessionRequest]) (*connect.Response[v1.ReviveSessionResponse], error) {
			revivedID = req.Msg.GetSessionId()
			return connect.NewResponse(&v1.ReviveSessionResponse{
				Session: fakeSession("sess-1", "pane-1", dir, v1.Liveness_LIVENESS_LIVE),
			}), nil
		},
	}
	sess := NewTymuxGRPCSession(transport)

	err := sess.RestoreWithWorkDir(dir)

	require.NoError(t, err)
	assert.Equal(t, "sess-1", revivedID)
	assert.Equal(t, "sess-1", sess.GetSessionIdentifier())
	assert.True(t, sess.HasSession())
}

func TestTymuxGRPCSession_RestoreWithWorkDir_NoMatchFallsBackToStart(t *testing.T) {
	dir := t.TempDir()
	var created bool
	transport := &fakeTransport{
		listSessionsFn: func(_ context.Context, _ *connect.Request[v1.ListSessionsRequest]) (*connect.Response[v1.ListSessionsResponse], error) {
			return connect.NewResponse(&v1.ListSessionsResponse{}), nil
		},
		createSessionFn: func(_ context.Context, _ *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
			created = true
			return connect.NewResponse(fakeSession("sess-new", "pane-new", dir, v1.Liveness_LIVENESS_LIVE)), nil
		},
	}
	sess := NewTymuxGRPCSession(transport)

	err := sess.RestoreWithWorkDir(dir)

	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, "sess-new", sess.GetSessionIdentifier())
}

func TestTymuxGRPCSession_GetCurrentWorkingDirectory_ReturnsCachedCwdWithNoExtraRPC(t *testing.T) {
	dir := t.TempDir()
	calls := 0
	transport := &fakeTransport{
		createSessionFn: func(_ context.Context, req *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
			calls++
			return connect.NewResponse(fakeSession("sess-1", "pane-1", req.Msg.GetCwd(), v1.Liveness_LIVENESS_LIVE)), nil
		},
	}
	sess := NewTymuxGRPCSession(transport)
	require.NoError(t, sess.Start(dir))

	cwd, err := sess.GetCurrentWorkingDirectory()
	require.NoError(t, err)
	assert.Equal(t, dir, cwd)

	cwd, err = sess.GetCurrentWorkingDirectory()
	require.NoError(t, err)
	assert.Equal(t, dir, cwd)
	assert.Equal(t, 1, calls, "GetCurrentWorkingDirectory must not issue any RPC beyond Start's own CreateSession")
}

func TestTymuxGRPCSession_GetCurrentWorkingDirectory_EmptyBeforeStart(t *testing.T) {
	sess := NewTymuxGRPCSession(&fakeTransport{})
	cwd, err := sess.GetCurrentWorkingDirectory()
	require.NoError(t, err)
	assert.Empty(t, cwd)
}

func startedSession(t *testing.T, snap *v1.PaneSnapshot) TymuxManager {
	t.Helper()
	dir := t.TempDir()
	transport := &fakeTransport{
		createSessionFn: func(_ context.Context, _ *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
			return connect.NewResponse(fakeSession("sess-1", "pane-1", dir, v1.Liveness_LIVENESS_LIVE)), nil
		},
		capturePaneFn: func(_ context.Context, req *connect.Request[v1.CapturePaneRequest]) (*connect.Response[v1.PaneSnapshot], error) {
			assert.Equal(t, "pane-1", req.Msg.GetPaneId())
			return connect.NewResponse(snap), nil
		},
	}
	sess := NewTymuxGRPCSession(transport)
	require.NoError(t, sess.Start(dir))
	return sess
}

func TestTymuxGRPCSession_CapturePaneContentRaw_JoinsRowCellText(t *testing.T) {
	snap := &v1.PaneSnapshot{
		Grid: []*v1.Row{
			{Cells: []*v1.Cell{{Text: "h"}, {Text: "e"}, {Text: "l"}, {Text: "l"}, {Text: "o"}}},
			{Cells: []*v1.Cell{{Text: "!"}}},
		},
	}
	sess := startedSession(t, snap)

	content, err := sess.CapturePaneContentRaw()

	require.NoError(t, err)
	assert.Contains(t, content, "hello")
	assert.Equal(t, "hello\n!", content)
}

func TestTymuxGRPCSession_CaptureViewport_TailsToRequestedLines(t *testing.T) {
	snap := &v1.PaneSnapshot{
		Grid: []*v1.Row{
			{Cells: []*v1.Cell{{Text: "one"}}},
			{Cells: []*v1.Cell{{Text: "two"}}},
			{Cells: []*v1.Cell{{Text: "three"}}},
		},
	}
	sess := startedSession(t, snap)

	content, err := sess.CaptureViewport(2)

	require.NoError(t, err)
	assert.Equal(t, "two\nthree", content)
}

func TestTymuxGRPCSession_GetCursorPosition_ReturnsColRowFromSnapshot(t *testing.T) {
	snap := &v1.PaneSnapshot{CursorRow: 5, CursorCol: 2}
	sess := startedSession(t, snap)

	x, y, err := sess.GetCursorPosition()

	require.NoError(t, err)
	assert.Equal(t, 2, x)
	assert.Equal(t, 5, y)
}

func TestTymuxGRPCSession_GetPaneDimensions_ReturnsColsRowsFromSnapshot(t *testing.T) {
	snap := &v1.PaneSnapshot{Rows: 24, Cols: 80}
	sess := startedSession(t, snap)

	width, height, err := sess.GetPaneDimensions()

	require.NoError(t, err)
	assert.Equal(t, 80, width)
	assert.Equal(t, 24, height)
}

func TestTymuxGRPCSession_CaptureMethods_ErrorBeforeStart(t *testing.T) {
	sess := NewTymuxGRPCSession(&fakeTransport{})

	_, err := sess.CapturePaneContentRaw()
	assert.Error(t, err)

	_, _, err = sess.GetCursorPosition()
	assert.Error(t, err)

	_, _, err = sess.GetPaneDimensions()
	assert.Error(t, err)
}

func TestTymuxGRPCSession_GetPTY_ReturnsNotSupportedError(t *testing.T) {
	sess := NewTymuxGRPCSession(&fakeTransport{})

	f, err := sess.GetPTY()

	assert.Nil(t, f)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotSupportedOnTymuxBackend)
}

func TestTymuxGRPCSession_GetPanePID_ReturnsNotSupportedError(t *testing.T) {
	sess := NewTymuxGRPCSession(&fakeTransport{})

	pid, err := sess.GetPanePID()

	assert.Zero(t, pid)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotSupportedOnTymuxBackend)
}

// --- Story 2.2.6: ErrTymuxdUnreachable classification ---

func TestTymuxGRPCSession_Start_TransportUnavailable_ClassifiesAsErrTymuxdUnreachable(t *testing.T) {
	dir := t.TempDir()
	underlying := connect.NewError(connect.CodeUnavailable, errors.New("dial tcp: connection refused"))
	transport := &fakeTransport{
		createSessionFn: func(_ context.Context, _ *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
			return nil, underlying
		},
	}
	sess := NewTymuxGRPCSession(transport)

	err := sess.Start(dir)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTymuxdUnreachable)
	assert.ErrorIs(t, err, underlying)
	assert.Contains(t, err.Error(), "connection refused")
}

func TestTymuxGRPCSession_Start_OrdinaryRPCError_NotClassifiedAsUnreachable(t *testing.T) {
	dir := t.TempDir()
	underlying := connect.NewError(connect.CodeInvalidArgument, errors.New("bad request"))
	transport := &fakeTransport{
		createSessionFn: func(_ context.Context, _ *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
			return nil, underlying
		},
	}
	sess := NewTymuxGRPCSession(transport)

	err := sess.Start(dir)

	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrTymuxdUnreachable), "an invalid-argument rejection from a reachable daemon must not classify as ErrTymuxdUnreachable")
	assert.ErrorIs(t, err, underlying)
}

func TestClassifyRPCError_NilIsNil(t *testing.T) {
	assert.NoError(t, classifyRPCError("op", nil))
}
