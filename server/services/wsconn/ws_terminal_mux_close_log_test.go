package wsconn

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/services"
)

type muxLogLine struct {
	level slog.Level
	msg   string
	attrs map[string]any
}

// muxLogRecorder collects the bridge's own log lines; injected per bridge so tests stay parallel.
type muxLogRecorder struct {
	mu    sync.Mutex
	lines []muxLogLine
	ch    chan struct{}
}

func newMuxLogRecorder() *muxLogRecorder { return &muxLogRecorder{ch: make(chan struct{}, 16)} }

func (r *muxLogRecorder) log(level slog.Level, msg string, args ...any) {
	attrs := map[string]any{}
	for i := 0; i+1 < len(args); i += 2 {
		attrs[args[i].(string)] = args[i+1]
	}
	r.mu.Lock()
	r.lines = append(r.lines, muxLogLine{level, msg, attrs})
	r.mu.Unlock()
	r.ch <- struct{}{}
}

// awaitClosed waits for the "terminal mux socket closed" line and asserts it is the only one.
func (r *muxLogRecorder) awaitClosed(t *testing.T) muxLogLine {
	t.Helper()
	select {
	case <-r.ch:
	case <-time.After(muxTestWait):
		t.Fatal("no 'terminal mux socket closed' line was logged")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	require.Len(t, r.lines, 1)
	assert.Equal(t, "terminal mux socket closed", r.lines[0].msg)
	return r.lines[0]
}

type muxCloseHarness struct {
	rec    *muxLogRecorder
	srv    *httptest.Server
	conn   *websocket.Conn
	cancel context.CancelFunc
}

// newMuxCloseHarness dials a bridge whose idle limit, clock and logger are injected.
func newMuxCloseHarness(t *testing.T, d UnaryDispatchers, idle time.Duration, now func() time.Time, serve muxServeFunc) *muxCloseHarness {
	t.Helper()
	if serve == nil {
		serve = echoServe
	}
	h := &muxCloseHarness{rec: newMuxLogRecorder()}
	bridge := NewTerminalMuxBridge(serve, 1, WithTerminalMuxDispatchers(d), withTerminalMuxLogger(h.rec.log))
	bridge.readIdle = idle
	if now != nil {
		bridge.now = now
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	t.Cleanup(cancel)
	h.srv = httptest.NewUnstartedServer(services.WithRequestRecord(services.ListenerLocal, false)(bridge))
	h.srv.Config.BaseContext = func(net.Listener) context.Context { return ctx }
	h.srv.Start()
	t.Cleanup(h.srv.Close)
	hdr := http.Header{"User-Agent": {"mux-close-test/1"}}
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(h.srv.URL, "http"), hdr)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	h.conn = conn
	return h
}

func TestMuxSocketCloseLog_should_LogIdleTimeout_When_PeerGoesSilent(t *testing.T) {
	t.Parallel()
	h := newMuxCloseHarness(t, allowAllDispatcher, 50*time.Millisecond, nil, nil)
	line := h.rec.awaitClosed(t)
	assert.Equal(t, slog.LevelWarn, line.level)
	assert.Equal(t, "idle_timeout", line.attrs["reason"])
	assert.NotNil(t, line.attrs["err"])
	assert.Equal(t, "mux-close-test/1", line.attrs["user_agent"])
	assert.Equal(t, services.ListenerLocal, line.attrs["listener"])
	assert.NotEmpty(t, line.attrs["remote_addr"])
}

func TestMuxSocketCloseLog_should_LogReadError_When_TCPClosesAbruptly(t *testing.T) {
	t.Parallel()
	h := newMuxCloseHarness(t, allowAllDispatcher, time.Minute, nil, nil)
	require.NoError(t, h.conn.UnderlyingConn().Close())
	line := h.rec.awaitClosed(t)
	assert.Equal(t, slog.LevelWarn, line.level)
	assert.Equal(t, "read_error", line.attrs["reason"])
	assert.NotNil(t, line.attrs["err"]) // close_code is present only when the transport error is a CloseError (EOF) rather than a reset
}

func TestMuxSocketCloseLog_should_LogPeerClosedAtInfo_When_CloseFrameArrives(t *testing.T) {
	t.Parallel()
	h := newMuxCloseHarness(t, allowAllDispatcher, time.Minute, nil, nil)
	msg := websocket.FormatCloseMessage(websocket.CloseGoingAway, "bye")
	require.NoError(t, h.conn.WriteControl(websocket.CloseMessage, msg, time.Now().Add(muxTestWait)))
	line := h.rec.awaitClosed(t)
	assert.Equal(t, slog.LevelInfo, line.level)
	assert.Equal(t, "peer_closed", line.attrs["reason"])
	assert.Equal(t, websocket.CloseGoingAway, line.attrs["close_code"])
	assert.Equal(t, "bye", line.attrs["close_text"])
}

func TestMuxSocketCloseLog_should_LogMalformed_When_FrameIsGarbage(t *testing.T) {
	t.Parallel()
	for name, send := range map[string]func(*websocket.Conn) error{
		"text message": func(c *websocket.Conn) error { return c.WriteMessage(websocket.TextMessage, []byte("hi")) },
		"bad binary":   func(c *websocket.Conn) error { return c.WriteMessage(websocket.BinaryMessage, []byte{0xff}) },
		"empty binary": func(c *websocket.Conn) error { return c.WriteMessage(websocket.BinaryMessage, nil) },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newMuxCloseHarness(t, allowAllDispatcher, time.Minute, nil, nil)
			require.NoError(t, send(h.conn))
			line := h.rec.awaitClosed(t)
			assert.Equal(t, slog.LevelWarn, line.level)
			assert.Equal(t, "malformed", line.attrs["reason"])
			assert.NotNil(t, line.attrs["err"])
		})
	}
}

func TestMuxSocketCloseLog_should_LogUnauthorized_When_OpensKeepBeingDenied(t *testing.T) {
	t.Parallel()
	h := newMuxCloseHarness(t, dispatcherReturning(http.StatusUnauthorized), time.Minute, nil, nil)
	for id := uint32(1); id <= muxMaxOpenDenials; id++ {
		require.NoError(t, h.conn.WriteMessage(websocket.BinaryMessage, encodeMuxEmpty(muxFrameOpen, id)))
	}
	line := h.rec.awaitClosed(t)
	assert.Equal(t, slog.LevelWarn, line.level)
	assert.Equal(t, "unauthorized", line.attrs["reason"])
}

func TestMuxSocketCloseLog_should_LogShutdownAtInfo_When_ServerContextCancelled(t *testing.T) {
	t.Parallel()
	h := newMuxCloseHarness(t, allowAllDispatcher, time.Minute, nil, nil)
	h.cancel()
	line := h.rec.awaitClosed(t)
	assert.Equal(t, slog.LevelInfo, line.level)
	assert.Equal(t, "shutdown", line.attrs["reason"])
}

func TestMuxSocketCloseLog_should_PreferServerTeardownCause_When_WriterFailedFirst(t *testing.T) {
	t.Parallel()
	b := NewTerminalMuxBridge(echoServe, 1)
	s := newMuxSocket(b, nil, httptest.NewRequest(http.MethodGet, "/", nil), services.ListenerLocal)
	s.failTeardown(muxCloseWriteError)
	s.failTeardown(muxCloseControlOverflow) // first cause wins
	got := s.classifyReadError(context.Background(), assert.AnError)
	assert.Equal(t, muxCloseWriteError, got.reason)
}

func TestMuxSocketCloseLog_should_ReportInjectedClockAndStreamCount_When_SocketCloses(t *testing.T) {
	t.Parallel()
	var tick atomic.Int64
	base := time.Unix(1_700_000_000, 0)
	// Every clock read advances one second, so age and since_last_frame are exact multiples.
	now := func() time.Time { return base.Add(time.Duration(tick.Add(1)) * time.Second) }
	opened := make(chan struct{}, 2)
	serve := func(conn streamConn, _ string) {
		opened <- struct{}{}
		_, _, _ = conn.ReadMessage()
	}
	h := newMuxCloseHarness(t, allowAllDispatcher, time.Minute, now, serve)
	h.conn.WriteMessage(websocket.BinaryMessage, encodeMuxEmpty(muxFrameOpen, 1))
	h.conn.WriteMessage(websocket.BinaryMessage, encodeMuxEmpty(muxFrameOpen, 2))
	<-opened
	<-opened
	require.NoError(t, h.conn.UnderlyingConn().Close())
	line := h.rec.awaitClosed(t)
	assert.Equal(t, 2, line.attrs["streams"])
	for _, k := range []string{"age", "since_last_frame"} {
		d, err := time.ParseDuration(line.attrs[k].(string))
		require.NoError(t, err, k)
		assert.Zero(t, d%time.Second, "%s=%v should be whole injected seconds", k, d)
	}
}

func TestMuxStreamErrors_should_ClassifyMuxOriginatedClosesAsSentinel(t *testing.T) {
	t.Parallel()
	assert.ErrorIs(t, errMuxSocketClosed, errMuxSocketClosed)
	var ce *websocket.CloseError
	require.ErrorAs(t, errMuxSocketClosed, &ce)
	assert.Equal(t, websocket.CloseAbnormalClosure, ce.Code, "wire behaviour is unchanged: still a 1006")
	assert.NotErrorIs(t, &websocket.CloseError{Code: websocket.CloseAbnormalClosure, Text: "mux socket closed"}, errMuxSocketClosed,
		"a real 1006 from a plain socket must not match the sentinel")
}

type recordedSlog struct {
	mu    sync.Mutex
	lines []muxLogLine
}

func (r *recordedSlog) Enabled(context.Context, slog.Level) bool { return true }
func (r *recordedSlog) WithAttrs([]slog.Attr) slog.Handler       { return r }
func (r *recordedSlog) WithGroup(string) slog.Handler            { return r }
func (r *recordedSlog) Handle(_ context.Context, rec slog.Record) error {
	attrs := map[string]any{}
	rec.Attrs(func(a slog.Attr) bool { attrs[a.Key] = a.Value.Any(); return true })
	r.mu.Lock()
	r.lines = append(r.lines, muxLogLine{rec.Level, rec.Message, attrs})
	r.mu.Unlock()
	return nil
}

// Not parallel: it swaps the process-wide slog default (log.SetSlogDefaultForTest), and parallel
// tests only resume after every serial test has finished.
func TestMuxBridge_should_LogOnceNotPerPane_When_SocketDropsWithManyStreams(t *testing.T) {
	const panes = 5
	global := &recordedSlog{}
	prev := log.SetSlogDefaultForTest(slog.New(global))
	t.Cleanup(func() { log.SetSlogDefaultForTest(prev) })

	var wg sync.WaitGroup
	var started atomic.Int32
	wg.Add(panes)
	serve := func(conn streamConn, _ string) {
		defer wg.Done()
		started.Add(1)
		stream := &connectWebSocketStream{conn: conn}
		errChan := make(chan error, 2)
		readOneInputFrame(inputReadLoopParams{stream: stream, errChan: errChan, sessionID: "mux-noise-test"})
		err := <-errChan
		assert.ErrorIs(t, err, errMuxSocketClosed)
		sendEndStreamError(stream, err)
	}
	h := newMuxCloseHarness(t, allowAllDispatcher, time.Minute, nil, serve)
	for id := uint32(1); id <= panes; id++ {
		require.NoError(t, h.conn.WriteMessage(websocket.BinaryMessage, encodeMuxEmpty(muxFrameOpen, id)))
	}
	require.Eventually(t, func() bool { return started.Load() == panes }, muxTestWait, 5*time.Millisecond)
	require.NoError(t, h.conn.UnderlyingConn().Close())
	line := h.rec.awaitClosed(t)
	assert.Equal(t, panes, line.attrs["streams"])
	wg.Wait()

	global.mu.Lock()
	defer global.mu.Unlock()
	perPane := 0
	for _, l := range global.lines {
		if strings.Contains(l.msg, "WebSocket read") || strings.Contains(l.msg, "StreamTerminal") || strings.Contains(l.msg, "EndStreamError") {
			perPane++
			assert.Equal(t, slog.LevelDebug, l.level, "per-pane line %q must be Debug", l.msg)
		}
	}
	assert.GreaterOrEqual(t, perPane, panes, "the per-pane read lines should still exist, at Debug")
}

func TestMuxSocketClose_should_ClassifyWriteFailuresDeterministically(t *testing.T) {
	t.Parallel()
	newSock := func() *muxSocket {
		return newMuxSocket(NewTerminalMuxBridge(echoServe, 1), nil, httptest.NewRequest(http.MethodGet, "/", nil), services.ListenerLocal)
	}
	peerErrs := map[string]error{
		"reset":       &net.OpError{Op: "write", Err: syscall.ECONNRESET},
		"broken pipe": &net.OpError{Op: "write", Err: syscall.EPIPE},
		"closed":      net.ErrClosed,
		"eof":         io.EOF,
	}
	for name, werr := range peerErrs {
		assert.True(t, isPeerCausedWriteError(werr), name)
	}
	assert.False(t, isPeerCausedWriteError(os.ErrDeadlineExceeded), "a write timeout is a server-side write failure")
	assert.False(t, isPeerCausedWriteError(errors.New("disk on fire")))

	// A genuine write failure wins over the read error it provokes.
	s := newSock()
	s.recordWriteFailure(os.ErrDeadlineExceeded)
	assert.Equal(t, muxCloseWriteError, s.classifyReadError(context.Background(), errors.New("use of closed")).reason)

	// A peer-caused write failure records nothing, so the read side decides.
	s = newSock()
	s.recordWriteFailure(&net.OpError{Op: "write", Err: syscall.EPIPE})
	assert.Equal(t, muxCloseReadError, s.classifyReadError(context.Background(), &net.OpError{Op: "read", Err: syscall.ECONNRESET}).reason)

	// A clean peer close frame is not overridden by a later write failure.
	s = newSock()
	s.recordWriteFailure(os.ErrDeadlineExceeded)
	closeErr := &websocket.CloseError{Code: websocket.CloseNormalClosure}
	assert.Equal(t, muxClosePeerClosed, s.classifyReadError(context.Background(), closeErr).reason)
}
