package session

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/mux"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

// fakeConn is a minimal net.Conn whose Read is fully test-controlled: each
// call blocks until the test sends an error on readErrs (mirroring a real
// mux.DecodeMessage read failure) or stopCh is closed (test cleanup).
// readLoop only ever calls Read, Close, and SetReadDeadline on the
// connection it holds, so every other method is a no-op.
type fakeConn struct {
	readErrs  chan error
	stopCh    chan struct{}
	readCalls int32
	closed    atomic.Bool
}

func newFakeConn() *fakeConn {
	return &fakeConn{
		readErrs: make(chan error),
		stopCh:   make(chan struct{}),
	}
}

func (c *fakeConn) Read([]byte) (int, error) {
	atomic.AddInt32(&c.readCalls, 1)
	select {
	case err := <-c.readErrs:
		return 0, err
	case <-c.stopCh:
		return 0, io.EOF
	}
}

func (c *fakeConn) Write(p []byte) (int, error)      { return len(p), nil }
func (c *fakeConn) Close() error                     { c.closed.Store(true); return nil }
func (c *fakeConn) LocalAddr() net.Addr              { return nil }
func (c *fakeConn) RemoteAddr() net.Addr             { return nil }
func (c *fakeConn) SetDeadline(time.Time) error      { return nil }
func (c *fakeConn) SetReadDeadline(time.Time) error  { return nil }
func (c *fakeConn) SetWriteDeadline(time.Time) error { return nil }

var _ net.Conn = (*fakeConn)(nil)

// startReadLoopWithFakeConn wires s up as if already connected to a fakeConn
// and starts its readLoop, registering cleanup that cancels the loop's
// context, unblocks any pending fakeConn.Read, and waits for the goroutine
// to exit -- shared setup for both tests below.
func startReadLoopWithFakeConn(t *testing.T, s *ExternalStreamer) *fakeConn {
	t.Helper()
	s.ctx, s.cancel = context.WithCancel(context.Background())

	fc := newFakeConn()
	s.conn = fc
	s.connected = true

	s.wg.Add(1)
	go s.readLoop()

	t.Cleanup(func() {
		s.cancel()
		close(fc.stopCh)
		s.wg.Wait()
	})

	return fc
}

func TestExternalStreamer_ReadLoop_WrappedTimeoutError_KeepsPollingWithoutReconnect(t *testing.T) {
	s := NewExternalStreamer("/tmp/unused-for-this-test.sock", 0)
	fc := startReadLoopWithFakeConn(t, s)

	// The exact shape mux.DecodeMessage produces from the 1-second poll
	// deadline readLoop sets every iteration -- must classify benign and
	// keep polling, not tear the connection down.
	fc.readErrs <- fmt.Errorf("failed to read message header: %w", os.ErrDeadlineExceeded)

	// Wait for the loop to call Read again, proving it treated the error as
	// benign and continued rather than reconnecting.
	wait.RequireEventually(t, func() bool {
		return atomic.LoadInt32(&fc.readCalls) >= 2
	}, time.Second, time.Millisecond, "readLoop did not continue polling after a benign timeout")

	s.connMu.RLock()
	defer s.connMu.RUnlock()
	require.True(t, s.connected, "a benign timeout must not mark the connection disconnected")
	require.Same(t, fc, s.conn, "a benign timeout must not replace or clear the connection")
}

// serveOneMuxHandshake sends a valid metadata handshake to conn, then holds
// it open (rather than closing right after) so a client that reconnects to
// it doesn't immediately see a second, spurious EOF.
func serveOneMuxHandshake(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	msg, err := mux.NewMetadataMessage(&mux.SessionMetadata{Command: "test", PID: 1})
	if err != nil {
		return
	}
	if err := mux.WriteMessage(conn, msg); err != nil {
		return
	}
	_, _ = io.Copy(io.Discard, conn)
}

// serveMuxHandshakes accepts connections on ln until it's closed, handling
// each with serveOneMuxHandshake.
func serveMuxHandshakes(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go serveOneMuxHandshake(conn)
	}
}

func TestExternalStreamer_ReadLoop_RealDisconnectError_TriggersReconnect(t *testing.T) {
	// A real listener behind the streamer's socketPath so the reconnect this
	// test proves can actually succeed, rather than merely being attempted.
	socketPath := filepath.Join(t.TempDir(), "reconnect-test.sock")
	ln, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	go serveMuxHandshakes(ln)

	s := NewExternalStreamer(socketPath, 0)
	fc := startReadLoopWithFakeConn(t, s)

	// A genuine disconnect -- not any of IsBenignTimeout's typed shapes --
	// must mark the connection dead.
	fc.readErrs <- errors.New("connection reset by peer")

	wait.RequireEventually(t, func() bool {
		s.connMu.RLock()
		defer s.connMu.RUnlock()
		return !s.connected && s.conn == nil
	}, time.Second, time.Millisecond, "a real disconnect error must mark the connection dead")

	require.True(t, fc.closed.Load(), "the dead connection must be closed")

	// ...and the next readLoop iteration must invoke reconnect(): the only
	// way s.connected becomes true again is a fresh, successful connect().
	wait.RequireEventually(t, func() bool {
		return s.IsConnected()
	}, 3*time.Second, 10*time.Millisecond, "readLoop did not reconnect after the disconnect")
}
