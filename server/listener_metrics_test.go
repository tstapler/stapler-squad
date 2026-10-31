package server

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMeteredListener_should_TrackOpenConnectionsIncludingHijacked(t *testing.T) {
	t.Parallel()
	raw, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	ln := withListenerMetrics("test-open", raw).(*meteredListener)

	hijacked := make(chan struct{})
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up := websocket.Upgrader{}
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		close(hijacked)
		_, _, _ = c.ReadMessage() // hold the hijacked conn open until the client closes
		_ = c.Close()
	}))
	srv.Listener = ln
	srv.Start()
	defer srv.Close()

	ws, _, err := websocket.DefaultDialer.Dial("ws"+srv.URL[len("http"):], nil)
	require.NoError(t, err)
	<-hijacked
	assert.Equal(t, int64(1), ln.open.Load(), "a hijacked WebSocket stays counted; ConnState would have dropped it")

	require.NoError(t, ws.WriteMessage(websocket.TextMessage, []byte("bye")))
	require.Eventually(t, func() bool { return ln.open.Load() == 0 }, 5*time.Second, 5*time.Millisecond)
}

func TestAcceptErrorKind_should_BucketErrors(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "emfile", acceptErrorKind(syscall.EMFILE))
	assert.Equal(t, "emfile", acceptErrorKind(&net.OpError{Op: "accept", Err: syscall.ENFILE}))
	assert.Equal(t, "other", acceptErrorKind(syscall.ECONNABORTED))
}

// recordingConn remembers the last read deadline net/http or gorilla set on it.
type recordingConn struct {
	net.Conn
	lastRead chan time.Time
}

func (c *recordingConn) SetReadDeadline(t time.Time) error {
	select {
	case c.lastRead <- t:
	default:
		<-c.lastRead
		c.lastRead <- t
	}
	return c.Conn.SetReadDeadline(t)
}

func (c *recordingConn) SetDeadline(t time.Time) error {
	_ = c.SetReadDeadline(t)
	return c.Conn.SetWriteDeadline(t)
}

type recordingListener struct {
	net.Listener
	conns chan *recordingConn
}

func (l *recordingListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	rc := &recordingConn{Conn: c, lastRead: make(chan time.Time, 1)}
	l.conns <- rc
	return rc, nil
}

// The servers set ReadTimeout=15s; a hijacked WebSocket must not inherit that deadline,
// or every idle terminal stream would die with "i/o timeout".
func TestWebSocketUpgrade_should_ClearHTTPServerReadDeadline(t *testing.T) {
	t.Parallel()
	raw, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	rl := &recordingListener{Listener: raw, conns: make(chan *recordingConn, 1)}

	upgraded := make(chan struct{})
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		close(upgraded)
		_, _, _ = c.ReadMessage()
		_ = c.Close()
	}))
	srv.Listener = rl
	srv.Config.ReadTimeout = 15 * time.Second
	srv.Start()
	defer srv.Close()

	ws, _, err := websocket.DefaultDialer.Dial("ws"+srv.URL[len("http"):], nil)
	require.NoError(t, err)
	defer ws.Close()
	<-upgraded

	rc := <-rl.conns
	last := <-rc.lastRead
	assert.True(t, last.IsZero(), "last read deadline after Upgrade must be cleared, got %v", last)
}

func TestTLSHandshakeLog_should_PassThroughWithoutError(t *testing.T) {
	t.Parallel()
	w := tlsHandshakeLog{listener: "test"}
	n, err := w.Write([]byte("http: TLS handshake error from 127.0.0.1:1: EOF\n"))
	require.NoError(t, err)
	assert.Positive(t, n)
	assert.NotNil(t, tlsHandshakeCounter, "counter is created on the first handshake error")
}

// Wrapping must not hide *net.TCPConn's ReadFrom (sendfile path) or CloseWrite from net/http.
func TestMeteredConn_should_ForwardReadFromAndCloseWrite(t *testing.T) {
	t.Parallel()
	raw, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	ln := withListenerMetrics("test-fwd", raw)
	defer ln.Close()

	got := make(chan []byte, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		mc := c.(*meteredConn)
		n, err := mc.ReadFrom(strings.NewReader("hello"))
		if err != nil || n != 5 {
			got <- nil
			return
		}
		_ = mc.CloseWrite()
	}()

	cl, err := net.Dial("tcp", raw.Addr().String())
	require.NoError(t, err)
	defer cl.Close()
	b, err := io.ReadAll(cl) // returns only because CloseWrite half-closed the stream
	require.NoError(t, err)
	assert.Equal(t, "hello", string(b))
	select {
	case v := <-got:
		t.Fatalf("server side failed: %v", v)
	default:
	}

	var _ io.ReaderFrom = (*meteredConn)(nil)
}
