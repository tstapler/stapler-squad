package services

import (
	"context"
	"crypto/rand"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/gorilla/websocket"
	"github.com/puzpuzpuz/xsync/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/server/protocol"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/scrollback"
	"github.com/tstapler/stapler-squad/session/streamhub"
	"github.com/tstapler/stapler-squad/session/tymux"
	"google.golang.org/protobuf/proto"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// paneFake is the tymux daemon as the handler sees it: a redraw (or a decline) and a text capture.
type paneFake struct {
	tymux.TymuxManager
	redraw       string
	redrawAlt    bool
	redrawOK     bool
	redrawErr    error
	text         string
	textErr      error
	dead         bool
	redrawCalls  atomic.Int32
	textCaptures atomic.Int32
}

func (p *paneFake) HasSession() bool                     { return true }
func (p *paneFake) IsAlive() bool                        { return !p.dead }
func (p *paneFake) FilterBanners(c string) (string, int) { return c, 0 }
func (p *paneFake) HasMeaningfulContent(string) bool     { return true }
func (p *paneFake) GetCursorPosition() (int, int, error) { return 0, 0, nil }
func (p *paneFake) CapturePaneContentRaw() (string, error) {
	p.textCaptures.Add(1)
	return p.text, p.textErr
}
func (p *paneFake) CapturePaneRedraw(context.Context) (string, bool, bool, error) {
	p.redrawCalls.Add(1)
	return p.redraw, p.redrawAlt, p.redrawOK, p.redrawErr
}

const (
	testRedraw = "\x1b[?1049h\x1b]2;vim\x1b\\\x1b[3;1Hhi"
	testText   = "plain text screen"
)

func firstPaintInstance(t *testing.T, pane *paneFake) *session.Instance {
	t.Helper()
	return session.NewStartedInstanceForTest(t, "first-paint", session.NewTymuxBackend(pane))
}

func readOutput(t *testing.T, conn *websocket.Conn) *sessionv1.TerminalOutput {
	t.Helper()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	env, _, err := protocol.ParseEnvelope(msg)
	require.NoError(t, err)
	var data sessionv1.TerminalData
	require.NoError(t, proto.Unmarshal(env.Data, &data))
	require.NotNil(t, data.GetOutput(), "expected an Output frame, got %T", data.Data)
	return data.GetOutput()
}

func newFirstPaintHandler() *ConnectRPCWebSocketHandler {
	return &ConnectRPCWebSocketHandler{snapshotCache: xsync.NewMap[string, sessionSnapshot](), firstPaintBudget: time.Millisecond}
}

// lastMeaningful is when the instance last recorded meaningful output (zero if never).
func lastMeaningful(inst *session.Instance) time.Time { return inst.LastMeaningfulOutputTime() }

func TestCaptureAndSendInitialSnapshot_should_SendRedrawVerbatim_AndNotCacheIt(t *testing.T) {
	stream, conn, cleanup := createTestWebSocketPair(t)
	defer cleanup()
	pane := &paneFake{redraw: testRedraw, redrawAlt: true, redrawOK: true, text: testText}
	inst := firstPaintInstance(t, pane)
	h := newFirstPaintHandler()

	fullState, err := h.captureAndSendInitialSnapshot(stream, inst, "s")
	require.NoError(t, err)
	_, err = h.captureAndSendInitialSnapshot(stream, inst, "s")
	require.NoError(t, err)

	assert.True(t, fullState)
	for i := 0; i < 2; i++ {
		out := readOutput(t, conn)
		assert.Equal(t, testRedraw, string(out.Data), "verbatim: no ansiSnapshotPrefix, prepare or cursor sync")
		assert.True(t, out.GetAltScreenActive(), "alt screen must be stamped on the frame")
	}
	assert.EqualValues(t, 2, pane.redrawCalls.Load(), "each attach must capture fresh")
	assert.EqualValues(t, 0, pane.textCaptures.Load())
	assert.False(t, lastMeaningful(inst).IsZero(), "the control-mode initial snapshot records output timestamps")
}

func TestCaptureAndSendInitialSnapshot_should_UseLegacyText_When_RedrawDeclinedOrFailed(t *testing.T) {
	cases := map[string]*paneFake{
		"older daemon":   {redrawOK: false, text: testText},
		"capture failed": {redrawOK: true, redrawErr: errors.New("daemon gone"), text: testText},
	}
	for name, pane := range cases {
		pane := pane
		t.Run(name, func(t *testing.T) {
			stream, conn, cleanup := createTestWebSocketPair(t)
			defer cleanup()
			inst := firstPaintInstance(t, pane)

			fullState, err := newFirstPaintHandler().captureAndSendInitialSnapshot(stream, inst, "s")
			require.NoError(t, err)

			assert.False(t, fullState)
			out := string(readOutput(t, conn).Data)
			assert.True(t, strings.HasPrefix(out, ansiSnapshotPrefix), "legacy path keeps its prefix, got %q", out)
			assert.Contains(t, out, testText)
		})
	}
}

func TestSendPostResizeSnapshot_should_SendRedrawVerbatim_WithoutTouchingTimestamps(t *testing.T) {
	stream, conn, cleanup := createTestWebSocketPair(t)
	defer cleanup()
	pane := &paneFake{redraw: testRedraw, redrawAlt: true, redrawOK: true, text: testText}
	inst := firstPaintInstance(t, pane)

	newFirstPaintHandler().sendPostResizeSnapshot(controlModeResizeCoalescerParams{stream: stream, instance: inst, sessionID: "s"}, resizeReq{})

	out := readOutput(t, conn)
	assert.Equal(t, testRedraw, string(out.Data))
	assert.True(t, out.GetAltScreenActive())
	assert.True(t, lastMeaningful(inst).IsZero(), "a resize must not reset LastMeaningfulOutput or fire SessionEventOutput")
}

func TestSendPostResizeSnapshot_should_UseLegacyText_When_RedrawDeclined(t *testing.T) {
	stream, conn, cleanup := createTestWebSocketPair(t)
	defer cleanup()
	inst := firstPaintInstance(t, &paneFake{redrawOK: false, text: testText})

	newFirstPaintHandler().sendPostResizeSnapshot(controlModeResizeCoalescerParams{stream: stream, instance: inst, sessionID: "s"}, resizeReq{})

	out := string(readOutput(t, conn).Data)
	assert.True(t, strings.HasPrefix(out, ansiSnapshotPrefix))
	assert.Contains(t, out, testText)
}

func attachFirstPaintSubscriber(t *testing.T, stream *connectWebSocketStream, hub *streamhub.StreamHub) streamhub.SubscriberID {
	t.Helper()
	transport := NewWebSocketTransport(stream, "s")
	id := hub.AttachSubscriber(transport, streamhub.SubscriberCapability{CanResize: true, ManualFirstPaint: true})
	transport.BindSubscriber(hub, id)
	t.Cleanup(func() { hub.DetachSubscriber(id) })
	return id
}

func TestSendHubInitialSnapshot_should_SendRedrawVerbatim_WithoutTouchingTimestamps(t *testing.T) {
	stream, conn, cleanup := createTestWebSocketPair(t)
	defer cleanup()
	pane := &paneFake{redraw: testRedraw, redrawAlt: true, redrawOK: true, text: testText}
	inst := firstPaintInstance(t, pane)
	hub := streamhub.NewStreamHub("s", nil, streamhub.WithTeardownGrace(time.Hour))
	id := attachFirstPaintSubscriber(t, stream, hub)

	require.NoError(t, newFirstPaintHandler().sendHubInitialSnapshot(stream, hub, id, inst, "s"))

	out := readOutput(t, conn)
	assert.Equal(t, testRedraw, string(out.Data))
	assert.True(t, out.GetAltScreenActive())
	assert.EqualValues(t, 0, pane.textCaptures.Load())
	assert.True(t, lastMeaningful(inst).IsZero(), "attaching to an idle session must not reset LastMeaningfulOutput")
}

func TestSendHubInitialSnapshot_should_UseLegacyText_When_RedrawDeclined(t *testing.T) {
	stream, conn, cleanup := createTestWebSocketPair(t)
	defer cleanup()
	inst := firstPaintInstance(t, &paneFake{redrawOK: false, text: testText})
	hub := streamhub.NewStreamHub("s", nil, streamhub.WithTeardownGrace(time.Hour))
	id := attachFirstPaintSubscriber(t, stream, hub)

	require.NoError(t, newFirstPaintHandler().sendHubInitialSnapshot(stream, hub, id, inst, "s"))

	out := string(readOutput(t, conn).Data)
	assert.True(t, strings.HasPrefix(out, ansiSnapshotPrefix))
	assert.Contains(t, out, testText)
}

func TestHandleCurrentPaneRequest_should_SendRedraw_ThroughRealInstance(t *testing.T) {
	pane := &paneFake{redraw: testRedraw, redrawAlt: true, redrawOK: true, text: testText}
	inst := firstPaintInstance(t, pane)

	out, err := handleCurrentPaneRequest(context.Background(), "s", inst, &sessionv1.CurrentPaneRequest{}, ResyncOptions{Writer: testWriter()})

	require.NoError(t, err)
	assert.Equal(t, testRedraw, string(out.Data))
	assert.True(t, out.GetAltScreenActive())
}

func TestPollFullStateFrame_should_TryOnce_When_DeadlineAlreadyPassed(t *testing.T) {
	t.Parallel()
	var calls int
	frame, ok := pollFullStateFrame(func() session.Status { return session.Active }, time.Now().Add(-time.Second), time.Millisecond,
		func(context.Context) (streamhub.FullStateSnapshot, bool, error) {
			calls++
			return streamhub.FullStateSnapshot{}, false, errors.New("resuming")
		})

	assert.False(t, ok)
	assert.Equal(t, snapshotFrame{}, frame)
	assert.Equal(t, 1, calls, "an expired shared budget must not buy a second wait")
}

func TestPollFullStateFrame_should_RetryWhileResumingThenSucceed(t *testing.T) {
	t.Parallel()
	var calls int
	frame, ok := pollFullStateFrame(func() session.Status { return session.Active }, time.Now().Add(5*time.Second), time.Millisecond,
		func(context.Context) (streamhub.FullStateSnapshot, bool, error) {
			calls++
			if calls < 3 {
				return streamhub.FullStateSnapshot{}, false, errors.New("resuming")
			}
			return streamhub.FullStateSnapshot{Data: "R", AltScreen: true}, true, nil
		})

	assert.True(t, ok)
	assert.Equal(t, snapshotFrame{data: "R", altScreen: true}, frame)
	assert.Equal(t, 3, calls)
}

func TestPollFullStateFrame_should_GiveUpImmediately_When_StatusIsTerminal(t *testing.T) {
	t.Parallel()
	var calls int
	_, ok := pollFullStateFrame(func() session.Status { return session.Paused }, time.Now().Add(time.Hour), time.Hour,
		func(context.Context) (streamhub.FullStateSnapshot, bool, error) {
			calls++
			return streamhub.FullStateSnapshot{}, false, streamhub.ErrSessionNotStarted
		})

	assert.False(t, ok)
	assert.Equal(t, 1, calls)
}

// The redraw attempt and the text fallback share one wait budget; a session that never answers
// must not cost the budget twice.
func TestCaptureAndSendInitialSnapshot_should_ShareOneWaitBudget_When_NothingCaptures(t *testing.T) {
	stream, conn, cleanup := createTestWebSocketPair(t)
	defer cleanup()
	pane := &paneFake{redrawOK: true, redrawErr: errors.New("resuming"), textErr: errors.New("resuming")}
	inst := firstPaintInstance(t, pane)
	h := newFirstPaintHandler() // 1ms budget

	start := time.Now()
	fullState, err := h.captureAndSendInitialSnapshot(stream, inst, "s")
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.False(t, fullState)
	assert.Less(t, elapsed, firstPaintMaxWait/2, "the fallback poll must reuse the spent budget, not start its own")
	assert.Contains(t, string(readOutput(t, conn).Data), "session stopped")
}

// withinBound fails t, instead of hanging the run, when fn does not return within d.
func withinBound(t *testing.T, d time.Duration, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { defer close(done); fn() }()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("%s did not return within %s: the write has no deadline", what, d)
	}
}

func shortFirstPaintTimeout(t *testing.T) {
	t.Helper()
	old := firstPaintWriteTimeout
	firstPaintWriteTimeout = 200 * time.Millisecond
	t.Cleanup(func() { firstPaintWriteTimeout = old })
}

func incompressible(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return string(b)
}

// A client that never reads must not hold a first-paint write (and the hub's resize lock) forever.
func TestWriteMessageTimeout_should_FailWithinDeadline_When_PeerNeverReads(t *testing.T) {
	stream, _, cleanup := createTestWebSocketPair(t) // the peer conn is never read
	defer cleanup()
	chunk := make([]byte, 1<<20)
	var err error
	withinBound(t, 20*time.Second, "WriteMessageTimeout", func() {
		for i := 0; i < 4096 && err == nil; i++ { // socket buffers fill long before 4 GiB
			err = stream.WriteMessageTimeout(websocket.BinaryMessage, chunk, 200*time.Millisecond)
		}
	})
	require.Error(t, err)
}

// deadlineRecordingConn records every SetWriteDeadline on the server side of a test socket.
type deadlineRecordingConn struct {
	net.Conn
	mu        sync.Mutex
	deadlines []time.Time
}

func (c *deadlineRecordingConn) SetWriteDeadline(t time.Time) error {
	c.mu.Lock()
	c.deadlines = append(c.deadlines, t)
	c.mu.Unlock()
	return c.Conn.SetWriteDeadline(t)
}

func (c *deadlineRecordingConn) recorded() []time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]time.Time(nil), c.deadlines...)
}

type recordingListener struct {
	net.Listener
	conns chan *deadlineRecordingConn
}

func (l recordingListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	rc := &deadlineRecordingConn{Conn: c}
	l.conns <- rc
	return rc, nil
}

// The write deadline must be set d ahead of now for the write and then cleared, so a later plain
// write is not subject to a stale deadline. Recording the SetWriteDeadline calls proves that
// without waiting for a real deadline to pass.
func TestWriteMessageTimeout_should_SetThenClearDeadline_When_WriteSucceeds(t *testing.T) {
	rec := make(chan *deadlineRecordingConn, 8)
	streamCh := make(chan *connectWebSocketStream, 1)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		streamCh <- &connectWebSocketStream{conn: conn}
	}))
	srv.Listener = recordingListener{Listener: srv.Listener, conns: rec}
	srv.Start()
	defer srv.Close()
	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	require.NoError(t, err)
	defer client.Close()
	stream := <-streamCh
	defer stream.conn.Close()
	conn := <-rec

	const d = 100 * time.Millisecond
	before := time.Now()
	require.NoError(t, stream.WriteMessageTimeout(websocket.BinaryMessage, []byte("a"), d))
	after := time.Now()
	_, _, err = client.ReadMessage()
	require.NoError(t, err)

	// gorilla forwards a deadline to the net.Conn only when it writes, so the clear shows up on the
	// next write: [deadline for "a", zero for "b"].
	require.NoError(t, stream.WriteMessage(websocket.BinaryMessage, []byte("b")))
	got := conn.recorded()
	require.Len(t, got, 2)
	assert.False(t, got[0].Before(before.Add(d)) || got[0].After(after.Add(d)), "deadline %v not within [%v, %v]", got[0], before.Add(d), after.Add(d))
	assert.True(t, got[1].IsZero(), "the deadline must be cleared; a stale one would fail the later write")
}

func TestSendFullStateFrame_should_UseWriteDeadline(t *testing.T) {
	shortFirstPaintTimeout(t)
	stream, _, cleanup := createTestWebSocketPair(t)
	defer cleanup()
	var err error
	withinBound(t, 20*time.Second, "sendFullStateFrame", func() {
		err = sendFullStateFrame(stream, "s", snapshotFrame{data: incompressible(16 << 20)})
	})
	require.Error(t, err)
}

func TestWriteHubInitialSnapshot_should_UseWriteDeadline_OnLegacyText(t *testing.T) {
	shortFirstPaintTimeout(t)
	stream, _, cleanup := createTestWebSocketPair(t)
	defer cleanup()
	inst := firstPaintInstance(t, &paneFake{redrawOK: false, text: incompressible(16 << 20)})
	var err error
	withinBound(t, 30*time.Second, "writeHubInitialSnapshot", func() {
		err = newFirstPaintHandler().writeHubInitialSnapshot(stream, inst, "s")
	})
	require.Error(t, err)
}

// A capture that keeps failing must not end the connection: the client gets a blank paint and
// then live frames, instead of a reconnect loop on a session whose output works.
func TestSendHubInitialSnapshot_should_PaintBlankAndStreamLive_When_NoCaptureSucceeds(t *testing.T) {
	old := hubCaptureRetryWindow
	hubCaptureRetryWindow = time.Millisecond
	t.Cleanup(func() { hubCaptureRetryWindow = old })
	stream, conn, cleanup := createTestWebSocketPair(t)
	defer cleanup()
	boom := errors.New("daemon gone")
	inst := firstPaintInstance(t, &paneFake{redrawOK: true, redrawErr: boom, textErr: boom})
	hub := streamhub.NewStreamHub("s", nil, streamhub.WithTeardownGrace(time.Hour))
	id := attachFirstPaintSubscriber(t, stream, hub)

	require.NoError(t, newFirstPaintHandler().sendHubInitialSnapshot(stream, hub, id, inst, "s"))
	blank := string(readOutput(t, conn).Data)
	assert.True(t, strings.HasPrefix(blank, ansiSnapshotPrefix), "a blank paint must still clear the client's spinner")

	hub.Broadcast([]byte("live-frame"))
	assert.Contains(t, string(readOutput(t, conn).Data), "live-frame")
}

func scrollbackHandler(t *testing.T) *ConnectRPCWebSocketHandler {
	t.Helper()
	cfg := scrollback.DefaultScrollbackConfig()
	cfg.StoragePath = t.TempDir()
	mgr := scrollback.NewScrollbackManager(cfg)
	t.Cleanup(func() { _ = mgr.Close() })
	require.NoError(t, mgr.AppendOutput("s", []byte("older line\r\n")))
	return &ConnectRPCWebSocketHandler{scrollbackManager: mgr}
}

func TestSendInitialScrollbackUnlessFullState_should_ReplayScrollback_When_LegacyPaint(t *testing.T) {
	stream, conn, cleanup := createTestWebSocketPair(t)
	defer cleanup()
	scrollbackHandler(t).sendInitialScrollbackUnlessFullState(stream, "s", false)

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	env, _, err := protocol.ParseEnvelope(msg)
	require.NoError(t, err)
	var data sessionv1.TerminalData
	require.NoError(t, proto.Unmarshal(env.Data, &data))
	require.NotNil(t, data.GetScrollbackResponse())
}

func TestSendInitialScrollbackUnlessFullState_should_SendNothing_When_FullStatePaint(t *testing.T) {
	stream, conn, cleanup := createTestWebSocketPair(t)
	defer cleanup()
	scrollbackHandler(t).sendInitialScrollbackUnlessFullState(stream, "s", true)

	require.NoError(t, conn.SetReadDeadline(time.Now().Add(300*time.Millisecond)))
	_, _, err := conn.ReadMessage()
	var ne net.Error
	require.ErrorAs(t, err, &ne)
	require.True(t, ne.Timeout(), "scrollback replay would clear the full-state paint: %v", err)
}

// Every first-paint capture failure against a tymux pane: only a NotFound the backend agrees is
// final ends the stream (as not_found); everything else paints blank and keeps streaming.
func TestWriteHubInitialSnapshot_should_EndOnlyWhenBackendConfirmsGone(t *testing.T) {
	old := hubCaptureRetryWindow
	hubCaptureRetryWindow = time.Millisecond
	t.Cleanup(func() { hubCaptureRetryWindow = old })
	notFound := connect.NewError(connect.CodeNotFound, errors.New("no such pane"))
	for _, tc := range []struct {
		name     string
		err      error
		dead     bool
		wantGone bool
	}{
		{"pane not_found, backend dead", notFound, true, true},
		{"pane not_found, backend alive (transient)", notFound, false, false},
		{"daemon unavailable, backend alive", connect.NewError(connect.CodeUnavailable, errors.New("restarting")), false, false},
		{"daemon unavailable, backend dead", connect.NewError(connect.CodeUnavailable, errors.New("restarting")), true, false},
		{"plain error", errors.New("boom"), true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stream, conn, cleanup := createTestWebSocketPair(t)
			defer cleanup()
			inst := firstPaintInstance(t, &paneFake{redrawOK: true, redrawErr: tc.err, textErr: tc.err, dead: tc.dead})

			err := newFirstPaintHandler().writeHubInitialSnapshot(stream, inst, "s")

			if tc.wantGone {
				require.ErrorIs(t, err, errSessionGone)
				assert.Equal(t, "not_found", endStreamErrorCode(err))
				return
			}
			require.NoError(t, err)
			assert.True(t, strings.HasPrefix(string(readOutput(t, conn).Data), ansiSnapshotPrefix), "blank paint clears the spinner")
		})
	}
}
