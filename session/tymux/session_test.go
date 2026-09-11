package tymux

import (
	"context"
	"errors"
	"math"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"

	"github.com/tstapler/stapler-squad/session/tmux"
	"github.com/tstapler/stapler-squad/testutil/wait"
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
	sess := startedSession(t, &v1.PaneSnapshot{PaneId: "pane-1", Liveness: v1.Liveness_LIVENESS_LIVE})

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

func TestTymuxGRPCSession_GetPTY_ErrorsBeforeStart(t *testing.T) {
	sess := NewTymuxGRPCSession(&fakeTransport{})

	f, err := sess.GetPTY()

	assert.Nil(t, f)
	require.Error(t, err)
	assert.ErrorIs(t, err, errSessionNotStarted)
}

// TestTymuxGRPCSession_GetPTY_BridgesOutputAndInput exercises the
// socketpair adapter (pty_adapter.go) end to end against a fake standing
// stream: pane output pushed through the stream must show up on GetPTY()'s
// returned file, and bytes written to that file must arrive on the stream
// as AttachRequest_Input — the two directions ClaudeController/PTYAccess
// and CommandExecutor each depend on.
func TestTymuxGRPCSession_GetPTY_BridgesOutputAndInput(t *testing.T) {
	sess, stream, _ := startedSessionWithStream(t) // transport (3rd return) unused here

	f, err := sess.GetPTY()
	require.NoError(t, err)
	require.NotNil(t, f)

	// Second call must reuse the same adapter/file, not build a new one.
	f2, err := sess.GetPTY()
	require.NoError(t, err)
	assert.Same(t, f, f2)

	stream.push(&v1.AttachEvent{Payload: &v1.AttachEvent_Output{Output: []byte("hello from pane")}})

	require.NoError(t, f.SetReadDeadline(time.Now().Add(2*time.Second)))
	buf := make([]byte, 64)
	n, err := f.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, "hello from pane", string(buf[:n]))

	_, err = f.Write([]byte("echo hi\n"))
	require.NoError(t, err)

	wait.RequireEventually(t, func() bool {
		for _, req := range stream.sentRequests() {
			if in, ok := req.Payload.(*v1.AttachRequest_Input); ok && string(in.Input) == "echo hi\n" {
				return true
			}
		}
		return false
	}, time.Second, 10*time.Millisecond)
}

// TestTymuxGRPCSession_GetPTY_ClosedByClose confirms Close() tears the
// adapter down: the returned file must no longer be usable afterward, so a
// caller that kept a reference (PTYAccess does, for the controller's
// lifetime) can't read/write into a dead session.
func TestTymuxGRPCSession_GetPTY_ClosedByClose(t *testing.T) {
	sess, _, _ := startedSessionWithStream(t) // stream and transport (2nd/3rd returns) unused here

	f, err := sess.GetPTY()
	require.NoError(t, err)

	require.NoError(t, sess.Close())

	_, err = f.Write([]byte("x"))
	assert.Error(t, err)
}

// TestTymuxGRPCSession_GetPTY_CloseTerminatesPumpGoroutines proves Close()
// alone (no caller-supplied worktree cleanup, no re-entrant GetPTY() calls)
// is sufficient to unblock and terminate both of the adapter's pump
// goroutines — the property a code review flagged as merely coincidental
// (Close() happens to cancel the standing stream before calling
// pty.close(), rather than pty.close() being self-sufficient).
func TestTymuxGRPCSession_GetPTY_CloseTerminatesPumpGoroutines(t *testing.T) {
	baseline := goleak.IgnoreCurrent()
	sess, _, _ := startedSessionWithStream(t) // stream and transport (2nd/3rd returns) unused here

	_, err := sess.GetPTY()
	require.NoError(t, err)

	require.NoError(t, sess.Close())

	goleak.VerifyNone(t, baseline)
}

// TestTymuxGRPCSession_GetPTY_AfterClose_DoesNotLeakANewAdapter is the
// regression test for a code-review-caught bug: Close() resets s.pty to nil
// but never resets s.stream, so GetPTY() called after Close() (a real race
// window — instance.go calls GetPTY() from several lifecycle branches that
// can run concurrently with a health-check/retry-triggered Close()) saw
// s.stream still non-nil, built a brand-new adapter, subscribed it to the
// fanout, and started two more pump goroutines that would never be torn
// down — a session-lifetime leak on every such race.
func TestTymuxGRPCSession_GetPTY_AfterClose_DoesNotLeakANewAdapter(t *testing.T) {
	baseline := goleak.IgnoreCurrent()
	sess, _, _ := startedSessionWithStream(t) // stream and transport (2nd/3rd returns) unused here

	require.NoError(t, sess.Close())

	f, err := sess.GetPTY()
	assert.Nil(t, f)
	require.Error(t, err)
	assert.ErrorIs(t, err, errSessionNotStarted)

	goleak.VerifyNone(t, baseline)
}

func TestTymuxGRPCSession_GetPanePID_ReturnsNotSupportedError(t *testing.T) {
	sess := NewTymuxGRPCSession(&fakeTransport{})

	pid, err := sess.GetPanePID()

	assert.Zero(t, pid)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotSupportedOnTymuxBackend)
}

// TestTymuxGRPCSession_StartStopControlMode_AreNoOps is the regression test
// for a real production gap: these used to return ErrNotImplemented, which
// made streamhub.SessionController's contract-required no-op fail instead,
// aborting the browser terminal's WebSocket stream (server/services'
// streamViaHub/streamViaControlMode both call StartControlMode before
// attaching) for every tymux-backed session — the interactive terminal
// never rendered and typing had nowhere to go.
func TestTymuxGRPCSession_StartStopControlMode_AreNoOps(t *testing.T) {
	sess := NewTymuxGRPCSession(&fakeTransport{})

	assert.NoError(t, sess.StartControlMode())
	assert.NoError(t, sess.StopControlMode())
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

// --- Phase 6 idiom review, MUST FIX 2: IsAlive must not collapse a
// transport-level CapturePane failure and a genuinely-confirmed-dead pane
// into the same "false" answer — see errors.go's ErrTymuxdUnreachable doc
// comment / research/ux.md:218-224.

func TestTymuxGRPCSession_IsAlive_TransportUnreachable_FallsBackToCachedLiveness(t *testing.T) {
	dir := t.TempDir()
	unreachable := connect.NewError(connect.CodeUnavailable, errors.New("dial tcp: connection refused"))
	var captureCalls int32
	transport := &fakeTransport{
		createSessionFn: func(_ context.Context, _ *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
			return connect.NewResponse(fakeSession("sess-1", "pane-1", dir, v1.Liveness_LIVENESS_LIVE)), nil
		},
		capturePaneFn: func(_ context.Context, _ *connect.Request[v1.CapturePaneRequest]) (*connect.Response[v1.PaneSnapshot], error) {
			atomic.AddInt32(&captureCalls, 1)
			return nil, unreachable
		},
	}
	sess := NewTymuxGRPCSession(transport)
	require.NoError(t, sess.Start(dir))
	t.Cleanup(func() { _ = sess.Close() })

	// cacheFromSession seeded s.liveness = LIVENESS_LIVE; a transport-level
	// failure on the CapturePane read must fall back to that cached value
	// (true) rather than collapsing to false, since the daemon never
	// actually said the pane was dead — it was simply unreachable.
	alive := sess.IsAlive()

	assert.True(t, alive, "a transport-unreachable CapturePane failure must fall back to cached liveness, not report dead")
	assert.EqualValues(t, 1, captureCalls)
}

func TestTymuxGRPCSession_IsAlive_OrdinaryRPCError_FallsBackToCachedLiveness(t *testing.T) {
	dir := t.TempDir()
	ordinary := connect.NewError(connect.CodeInternal, errors.New("boom"))
	transport := &fakeTransport{
		createSessionFn: func(_ context.Context, _ *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
			return connect.NewResponse(fakeSession("sess-1", "pane-1", dir, v1.Liveness_LIVENESS_LIVE)), nil
		},
		capturePaneFn: func(_ context.Context, _ *connect.Request[v1.CapturePaneRequest]) (*connect.Response[v1.PaneSnapshot], error) {
			return nil, ordinary
		},
	}
	sess := NewTymuxGRPCSession(transport)
	require.NoError(t, sess.Start(dir))
	t.Cleanup(func() { _ = sess.Close() })

	// Even a non-ErrTymuxdUnreachable RPC failure is not the daemon
	// confirming death — an error response never carries that
	// confirmation, only a successful one with LIVENESS_DEAD does (see
	// TestTymuxGRPCSession_IsAlive_GenuineDeadResponse_ReturnsFalse).
	assert.True(t, sess.IsAlive(), "a non-transport CapturePane error must also fall back to cached liveness, not report dead")
}

func TestTymuxGRPCSession_IsAlive_GenuineDeadResponse_ReturnsFalse(t *testing.T) {
	sess := startedSession(t, &v1.PaneSnapshot{PaneId: "pane-1", Liveness: v1.Liveness_LIVENESS_DEAD})
	t.Cleanup(func() { _ = sess.Close() })

	// A real, error-free response reporting LIVENESS_DEAD is the one case
	// that should actually report false.
	assert.False(t, sess.IsAlive())
}

// --- Phase 6 idiom review, MUST FIX 1: SendInputViaControlMode's ctx must
// actually do something — a cancelled/expired context must short-circuit
// before ever reaching the underlying stream write.

func TestTymuxGRPCSession_SendInputViaControlMode_CancelledContext_ReturnsEarlyWithoutSending(t *testing.T) {
	// No Start() call: if SendInputViaControlMode ignored ctx and fell
	// through to sendOnStream as before the fix, it would return
	// errSessionNotStarted (no standing stream open yet) instead of the
	// context error — that's how this test distinguishes "checked ctx
	// first" from "ignored ctx."
	sess := NewTymuxGRPCSession(&fakeTransport{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := sess.SendInputViaControlMode(ctx, []byte("hello"))

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.NotErrorIs(t, err, errSessionNotStarted, "a cancelled context must short-circuit before reaching sendOnStream")
}

func TestTymuxGRPCSession_SendInputViaControlMode_ValidContext_SendsOnStream(t *testing.T) {
	sess, stream, _ := startedSessionWithStream(t) // transport (3rd return) unused here

	err := sess.SendInputViaControlMode(context.Background(), []byte("hello"))

	require.NoError(t, err)
	sent := stream.sentRequests()
	require.NotEmpty(t, sent)
	last := sent[len(sent)-1]
	assert.Equal(t, []byte("hello"), last.GetInput())
}

// --- Story 2.4.2: SetOnExitCallback / ResetExitOnce fire-once semantics ---

// TestSetOnExitCallback_ShouldFireExactlyOnce_WhenRegisteredBeforePaneExits
// is the happy path (validation.md REQ-2): a callback registered while the
// pane is still live must fire exactly once when the standing stream later
// delivers Exited.
func TestSetOnExitCallback_ShouldFireExactlyOnce_WhenRegisteredBeforePaneExits(t *testing.T) {
	sess, stream, _ := startedSessionWithStream(t) // transport (3rd return) unused here

	var calls int32
	reasons := make(chan string, 4)
	sess.SetOnExitCallback(func(reason string) {
		atomic.AddInt32(&calls, 1)
		reasons <- reason
	})

	code := int32(0)
	stream.push(&v1.AttachEvent{Payload: &v1.AttachEvent_Exited{Exited: &v1.ExitStatus{Code: &code}}})

	select {
	case reason := <-reasons:
		assert.Equal(t, "exited: code=0", reason)
	case <-time.After(time.Second):
		t.Fatal("exit callback never fired")
	}

	// Give any errant second delivery a chance to land before asserting
	// the count is exactly one (a pane only exits once in practice, but
	// this is the seam that would catch a fire-more-than-once regression).
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls), "callback must fire exactly once")
}

// TestSetOnExitCallback_ShouldFireExactlyOnce_WhenRegisteredAfterPaneAlreadyExited
// is the check-before-and-after-registration case (validation.md REQ-2,
// mirroring pane.rs:264-274/wait_exit's shape): a callback registered
// *after* the standing stream already delivered Exited must still fire
// exactly once — not zero times, which is what a naive
// store-only-for-future-events implementation would do.
func TestSetOnExitCallback_ShouldFireExactlyOnce_WhenRegisteredAfterPaneAlreadyExited(t *testing.T) {
	sess, stream, _ := startedSessionWithStream(t) // transport (3rd return) unused here
	concrete := sess.(*tymuxGRPCSession)

	code := int32(7)
	stream.push(&v1.AttachEvent{Payload: &v1.AttachEvent_Exited{Exited: &v1.ExitStatus{Code: &code}}})

	// Wait for readAttachLoop to actually observe the Exited event before
	// registering — this is the ordering the test exists to exercise.
	wait.RequireEventually(t, func() bool {
		concrete.mu.RLock()
		defer concrete.mu.RUnlock()
		return concrete.exited
	}, time.Second, time.Millisecond, "readAttachLoop never observed the Exited event")

	var calls int32
	reasons := make(chan string, 4)
	sess.SetOnExitCallback(func(reason string) {
		atomic.AddInt32(&calls, 1)
		reasons <- reason
	})

	select {
	case reason := <-reasons:
		assert.Equal(t, "exited: code=7", reason)
	case <-time.After(time.Second):
		t.Fatal("exit callback registered after exit must still fire once, not zero times")
	}

	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls), "callback must fire exactly once")
}

// TestResetExitOnce_WithoutANewExit_DoesNotFireSpuriously covers plan.md
// Story 2.4.2's second acceptance criterion: since a pane can only exit
// once, the assertion ResetExitOnce is checked against is that calling it
// with no subsequent exit never spuriously invokes the callback again.
func TestResetExitOnce_WithoutANewExit_DoesNotFireSpuriously(t *testing.T) {
	sess, stream, _ := startedSessionWithStream(t) // transport (3rd return) unused here

	var calls int32
	sess.SetOnExitCallback(func(string) { atomic.AddInt32(&calls, 1) })

	code := int32(1)
	stream.push(&v1.AttachEvent{Payload: &v1.AttachEvent_Exited{Exited: &v1.ExitStatus{Code: &code}}})

	wait.RequireEventually(t, func() bool {
		return atomic.LoadInt32(&calls) == 1
	}, time.Second, time.Millisecond, "callback never fired for the original exit")

	sess.ResetExitOnce()
	time.Sleep(20 * time.Millisecond)

	assert.Equal(t, int32(1), atomic.LoadInt32(&calls), "ResetExitOnce alone (no new exit) must not re-fire the callback")
}

// --- REQ-6 (validation.md): fake-transport-driven unit-level happy path ---

// assertAgentPromptSentOverStream sends prompt through sess.SendKeys and
// checks it reached the standing stream as the second Send call (the first
// being the pane_id handshake startedSessionWithStream's caller already
// opened).
func assertAgentPromptSentOverStream(t *testing.T, sess TymuxManager, stream *fakeAttachStream, prompt string) {
	t.Helper()
	n, err := sess.SendKeys(prompt)
	require.NoError(t, err)
	assert.Equal(t, len(prompt), n)
	sent := stream.sentRequests()
	require.Len(t, sent, 2) // [0] = pane_id, [1] = this Input
	assert.Equal(t, []byte(prompt), sent[1].GetInput())
}

// agentShapedCapturePane returns a fakeTransport.capturePaneFn rendering a
// Claude-Code-shaped screen: a bold, 256-color braille spinner glyph
// ("thinking" indicator) on row 0, and a box-drawn full-screen redraw
// (alt-screen-toggle-shaped) border on row 1.
func agentShapedCapturePane(t *testing.T) func(context.Context, *connect.Request[v1.CapturePaneRequest]) (*connect.Response[v1.PaneSnapshot], error) {
	t.Helper()
	return func(_ context.Context, req *connect.Request[v1.CapturePaneRequest]) (*connect.Response[v1.PaneSnapshot], error) {
		assert.Equal(t, "pane-1", req.Msg.GetPaneId())
		return connect.NewResponse(&v1.PaneSnapshot{
			PaneId:   "pane-1",
			Liveness: v1.Liveness_LIVENESS_LIVE,
			Grid: []*v1.Row{
				row(
					cell("⠋", packIndexed(6), 0, attrBold),
					cell(" ", 0, 0, 0),
					cell("T", 0, 0, 0), cell("h", 0, 0, 0), cell("i", 0, 0, 0),
					cell("n", 0, 0, 0), cell("k", 0, 0, 0), cell("i", 0, 0, 0),
					cell("n", 0, 0, 0), cell("g", 0, 0, 0),
				),
				row(
					cell("╭", packRGB(80, 200, 255), 0, attrBold),
					cell("─", packRGB(80, 200, 255), 0, attrBold),
					cell("─", packRGB(80, 200, 255), 0, attrBold),
					cell("╮", packRGB(80, 200, 255), 0, attrBold),
				),
			},
		}), nil
	}
}

// assertAgentShapedCaptureRendered checks the ANSI-rendered capture output
// carries exactly the expected bold+256-color spinner and bold+truecolor
// box-drawing SGR escapes (real ANSI-aware assertions, not substring-only).
func assertAgentShapedCaptureRendered(t *testing.T, out string) {
	t.Helper()
	require.True(t, strings.HasPrefix(out, "\x1b[1;38;5;6m⠋"), "got %q", out)
	assert.Contains(t, out, "Thinking")
	assert.Contains(t, out, "╭──╮")
	seqs := parseSGRSequences(t, out)
	assert.Contains(t, seqs, []int{1, 38, 5, 6}, "expected the bold+256-color spinner SGR sequence")
	assert.Contains(t, seqs, []int{1, 38, 2, 80, 200, 255}, "expected the bold+truecolor box-drawing SGR sequence")
}

// assertCleanExitFires pushes a clean Exited{code:0} event onto stream and
// waits for sess's exit callback to fire with the expected reason.
func assertCleanExitFires(t *testing.T, sess TymuxManager, stream *fakeAttachStream) {
	t.Helper()
	reasons := make(chan string, 1)
	sess.SetOnExitCallback(func(reason string) { reasons <- reason })
	code := int32(0)
	stream.push(&v1.AttachEvent{Payload: &v1.AttachEvent_Exited{Exited: &v1.ExitStatus{Code: &code}}})

	select {
	case reason := <-reasons:
		assert.Equal(t, "exited: code=0", reason)
	case <-time.After(time.Second):
		t.Fatal("exit callback never fired for a clean agent exit")
	}
}

// TestBackendTymux_ShouldRoundTripStartSendKeysCapture_WhenDrivenWithAgentShapedByteSequences
// is validation.md's REQ-6 happy-path test: a fake-rpcTransport-driven UNIT
// test (no live tymuxd) exercising the full lifecycle — start, input,
// capture, clean exit — with content shaped like a real Claude-Code-style
// agent session (a braille spinner glyph with color/bold attributes typical
// of a "thinking" indicator, and a full-screen box-drawn redraw typical of
// an alt-screen toggle), complementing (not replacing)
// TestTymuxGRPCSession_LiveTymuxd_StartSendKeysCaptureClose's live-daemon
// integration coverage (integration_test.go), which used a plain echo
// marker and can't assert on rendered ANSI/SGR content the way this test
// does via a real ANSI-aware parser (parseSGRSequences, render_test.go).
func TestBackendTymux_ShouldRoundTripStartSendKeysCapture_WhenDrivenWithAgentShapedByteSequences(t *testing.T) {
	sess, stream, transport := startedSessionWithStream(t)

	assertAgentPromptSentOverStream(t, sess, stream, "explain the reconnect backoff schedule\n")

	transport.capturePaneFn = agentShapedCapturePane(t)
	out, err := sess.CapturePaneContent()
	require.NoError(t, err)
	assertAgentShapedCaptureRendered(t, out)

	assertCleanExitFires(t, sess, stream)
}

// TestParseScrollbackOffset covers parseScrollbackOffset end to end: the
// pre-existing non-numeric-input and lower-bound (n < 0) fallbacks to 0, plus
// the gosec G115 fix's new upper-bound clamp at math.MaxUint32 for a
// client-supplied capture-pane -S argument that would otherwise wrap through
// the uint32 conversion.
func TestParseScrollbackOffset(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want uint32
	}{
		{name: "not a number falls back to zero", in: "not-a-number", want: 0},
		{name: "empty string falls back to zero", in: "", want: 0},
		{name: "negative falls back to zero", in: "-1", want: 0},
		{name: "zero passes through", in: "0", want: 0},
		{name: "normal value passes through", in: "42", want: 42},
		{name: "MaxUint32 boundary passes through", in: "4294967295", want: math.MaxUint32},
		{name: "above MaxUint32 clamps to MaxUint32", in: "4294967296", want: math.MaxUint32},
		{name: "far above MaxUint32 clamps to MaxUint32", in: "99999999999", want: math.MaxUint32},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseScrollbackOffset(tt.in))
		})
	}
}
