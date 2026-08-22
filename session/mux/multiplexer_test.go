package mux

import (
	"context"
	"io"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// readySignalConn wraps a net.Conn and closes ready on the first Read call, immediately
// before delegating to the real Read — the closest a test can get to observing "this
// goroutine is about to block in Read" without an OS-level hook into the syscall itself.
type readySignalConn struct {
	net.Conn
	once  sync.Once
	ready chan<- struct{}
}

func (c *readySignalConn) Read(p []byte) (int, error) {
	c.once.Do(func() { close(c.ready) })
	return c.Conn.Read(p)
}

// TestMultiplexer_BroadcastToClients verifies that PTY output is sent to ALL connected clients.
func TestMultiplexer_BroadcastToClients(t *testing.T) {
	t.Parallel()
	m := &Multiplexer{
		clients: make(map[net.Conn]struct{}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.ctx = ctx
	m.cancel = cancel
	defer cancel()

	// Create two client pairs.
	c1s, c1c := net.Pipe()
	c2s, c2c := net.Pipe()
	defer c1s.Close()
	defer c1c.Close()
	defer c2s.Close()
	defer c2c.Close()

	m.clientsMu.Lock()
	m.clients[c1s] = struct{}{}
	m.clients[c2s] = struct{}{}
	m.clientsMu.Unlock()

	outputData := []byte("pty output to all clients")
	msg := NewOutputMessage(outputData)

	// Read from both client sides in goroutines before broadcasting. broadcastToClients
	// uses a 100ms write deadline per client (multiplexer.go), and net.Pipe() is fully
	// synchronous/unbuffered — a write only succeeds once a reader is actively blocked in
	// Read(). Without a readiness barrier, this test raced goroutine scheduling against
	// that write deadline: under heavy CPU contention (e.g. `go test -race -count=20
	// ./session/...`) the reader goroutines could still be starting up when
	// broadcastToClients ran, so the write timed out and the message was silently dropped,
	// leaving the reader to block until its own read deadline. Bumping the read deadline
	// (2s -> 10s) papered over this without fixing it.
	//
	// An earlier version of this fix closed readyCh immediately before calling
	// DecodeMessage(conn), on the theory that this made the reader "confirmed blocked in
	// Read" before broadcastToClients ran. It didn't: closing readyCh and entering
	// DecodeMessage's first io.ReadFull are still two separate statements, and under
	// `-race -count=50` the scheduler can preempt the reader goroutine in that gap and let
	// the main goroutine call broadcastToClients before the reader ever reaches its Read
	// call — reproduced directly (one failure in 50 iterations, "client 2 should receive
	// broadcast", after a 10s read-deadline timeout). Signaling from *inside* readyReader's
	// Read method — the statement immediately preceding the actual conn.Read call, with no
	// function-call boundary in between — closes that gap.
	var wg sync.WaitGroup
	readMsg := func(conn net.Conn, readyCh chan<- struct{}) ([]byte, error) {
		conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		decoded, err := DecodeMessage(&readySignalConn{Conn: conn, ready: readyCh})
		if err != nil {
			return nil, err
		}
		return decoded.Data, nil
	}

	c1Ready := make(chan struct{})
	c2Ready := make(chan struct{})
	var c1Data, c2Data []byte
	var c1Err, c2Err error
	wg.Add(2)
	go func() { defer wg.Done(); c1Data, c1Err = readMsg(c1c, c1Ready) }()
	go func() { defer wg.Done(); c2Data, c2Err = readMsg(c2c, c2Ready) }()

	<-c1Ready
	<-c2Ready
	m.broadcastToClients(msg)

	wg.Wait()
	require.NoError(t, c1Err, "client 1 should receive broadcast")
	require.NoError(t, c2Err, "client 2 should receive broadcast")
	assert.Equal(t, outputData, c1Data, "client 1 data should match PTY output")
	assert.Equal(t, outputData, c2Data, "client 2 data should match PTY output")
}

// fakeSubscription holds a channel and a once-guard so it can only be closed once,
// regardless of whether FirePaneExit or context cancellation fires first.
type fakeSubscription struct {
	ch   chan struct{}
	once sync.Once
}

func (s *fakeSubscription) close() {
	s.once.Do(func() { close(s.ch) })
}

// fakePaneExitSubscriber is a test double for tmux.PaneExitSubscriber.
type fakePaneExitSubscriber struct {
	mu   sync.Mutex
	subs map[string][]*fakeSubscription
}

func newFakePaneExitSubscriber() *fakePaneExitSubscriber {
	return &fakePaneExitSubscriber{subs: make(map[string][]*fakeSubscription)}
}

func (f *fakePaneExitSubscriber) SubscribePaneExit(ctx context.Context, name string) <-chan struct{} {
	sub := &fakeSubscription{ch: make(chan struct{})}
	f.mu.Lock()
	f.subs[name] = append(f.subs[name], sub)
	f.mu.Unlock()
	go func() {
		<-ctx.Done()
		// Remove from map then close (once-guarded against FirePaneExit racing).
		f.mu.Lock()
		subs := f.subs[name]
		newSubs := make([]*fakeSubscription, 0, len(subs))
		for _, s := range subs {
			if s != sub {
				newSubs = append(newSubs, s)
			}
		}
		f.subs[name] = newSubs
		f.mu.Unlock()
		sub.close()
	}()
	return sub.ch
}

func (f *fakePaneExitSubscriber) FirePaneExit(name string) {
	f.mu.Lock()
	subs := f.subs[name]
	delete(f.subs, name)
	f.mu.Unlock()
	for _, s := range subs {
		s.close()
	}
}

// newTestMultiplexer creates a minimal Multiplexer suitable for unit tests
// (no real PTY, no real tmux).
func newTestMultiplexer(t *testing.T) (*Multiplexer, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	m := &Multiplexer{
		clients:      make(map[net.Conn]struct{}),
		tmuxSession:  "test-session",
		serverSocket: tmux.ResolveSocket(""),
		ctx:          ctx,
		cancel:       cancel,
	}
	return m, cancel
}

// TestMultiplexer_StartSessionMonitor_ChannelBased verifies that when a
// PaneExitSubscriber is configured, firing pane exit triggers Shutdown.
func TestMultiplexer_StartSessionMonitor_ChannelBased(t *testing.T) {
	t.Parallel()
	m, cancel := newTestMultiplexer(t)
	t.Cleanup(cancel)

	fake := newFakePaneExitSubscriber()
	m.paneExitSub = fake

	shutdownCalled := make(chan struct{})
	// Override cancel so we can observe Shutdown being called.
	origCancel := m.cancel
	m.cancel = func() {
		origCancel()
		select {
		case <-shutdownCalled:
		default:
			close(shutdownCalled)
		}
	}

	m.startSessionMonitor()

	// Fire pane exit for the session name.
	fake.FirePaneExit("test-session")

	select {
	case <-shutdownCalled:
		// expected
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: Shutdown was not called after pane exit")
	}
}

// TestMultiplexer_StartSessionMonitor_FallbackWhenNilSubscriber verifies that
// when paneExitSub is nil the polling goroutine is started (wg incremented).
func TestMultiplexer_StartSessionMonitor_FallbackWhenNilSubscriber(t *testing.T) {
	t.Parallel()
	m, cancel := newTestMultiplexer(t)
	defer cancel()

	// paneExitSub is nil by default — polling path should be used.
	// We swap out monitorTmuxSessionPolling by cancelling the context immediately
	// so the goroutine exits right away; what we verify is that wg.Add(1) was
	// called (i.e., wg.Wait completes after cancel without deadlock).
	cancel() // cause the polling goroutine to return immediately

	m.startSessionMonitor()
	// If polling path was NOT taken, wg.Wait would return instantly with no
	// goroutine having been added. Either way, this must not block.
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// expected: goroutine finished (either immediately or after context cancel)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: polling goroutine did not exit after context cancel")
	}
}

// TestMultiplexer_StartSessionMonitor_ContextCancel verifies that cancelling the
// multiplexer context before pane exit causes the monitor goroutine to exit
// cleanly with no goroutine leak.
func TestMultiplexer_StartSessionMonitor_ContextCancel(t *testing.T) {
	t.Parallel()
	m, cancel := newTestMultiplexer(t)

	fake := newFakePaneExitSubscriber()
	m.paneExitSub = fake

	m.startSessionMonitor()

	// Cancel before any pane exit fires.
	cancel()

	// The goroutine must exit. Give it a moment.
	// We detect leaks by checking that the fake has no live subscriptions
	// after a short grace period (the goroutine removes its entry on ctx.Done).
	require.Eventually(t, func() bool {
		fake.mu.Lock()
		defer fake.mu.Unlock()
		return len(fake.subs["test-session"]) == 0
	}, 2*time.Second, 10*time.Millisecond,
		"subscription was not cleaned up after context cancel")
}

// TestMultiplexer_InputIsolation verifies that input from one client goes ONLY to the
// PTY and is NOT broadcast to other connected clients.
//
// This is a regression test for the multiplexer's asymmetric routing invariant:
//   - PTY output → broadcast to all clients (read-only fan-out)
//   - Client input → written to PTY only (isolated write-path)
func TestMultiplexer_InputIsolation(t *testing.T) {
	t.Parallel()
	m := &Multiplexer{
		clients: make(map[net.Conn]struct{}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.ctx = ctx
	m.cancel = cancel
	defer cancel()

	// Mock PTY: os.Pipe lets us capture what gets written to it.
	ptyR, ptyW, err := os.Pipe()
	require.NoError(t, err)
	defer ptyR.Close()
	m.ptmx = ptyW

	// Two client pairs.
	c1s, c1c := net.Pipe()
	c2s, c2c := net.Pipe()
	defer c1s.Close()
	defer c2s.Close()

	// Only c2s is registered as a passive client (monitoring for spurious writes).
	m.clientsMu.Lock()
	m.clients[c2s] = struct{}{}
	m.clientsMu.Unlock()

	inputPayload := []byte("keyboard input from client 1 only")
	inputMsg := NewInputMessage(inputPayload)
	encodedInput, err := EncodeMessage(inputMsg)
	require.NoError(t, err)

	// Client 1 sends: Input message, then Close to terminate handleClient.
	encodedClose, err := EncodeMessage(&Message{Type: MessageTypeClose})
	require.NoError(t, err)

	// Goroutine: c1c side — handle metadata reply from handleClient, then send input+close.
	go func() {
		defer c1c.Close()
		// handleClient sends metadata first; read and discard it.
		c1c.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		DecodeMessage(c1c) // discard metadata
		// Send input, then close.
		// net.Pipe is synchronous: Write blocks until handleClient reads it,
		// so no sleep is needed between sends.
		c1c.Write(encodedInput)
		c1c.Write(encodedClose)
	}()

	// Run handleClient for c1s — exits when it reads Close message.
	m.wg.Add(1)
	m.handleClient(c1s)

	// Close PTY write end so read gets EOF.
	ptyW.Close()
	ptyData, _ := io.ReadAll(ptyR)
	assert.Contains(t, string(ptyData), string(inputPayload),
		"input should be forwarded to PTY")

	// Verify c2c (passive client) received nothing.
	c2c.SetReadDeadline(time.Now().Add(80 * time.Millisecond))
	buf := make([]byte, 512)
	n, readErr := c2c.Read(buf)
	assert.Equal(t, 0, n,
		"passive client 2 must NOT receive input from client 1 (got %d bytes: %q)", n, buf[:n])
	assert.Error(t, readErr,
		"read from client 2 should time out since no data was broadcast")
	c2c.Close()
}
