package session

import (
	"context"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"
)

// openSilentPTY returns the master of a real pseudo-terminal opened exactly the
// way tmux attach PTYs are (creack/pty), with a slave that never writes. The
// master is the file type that rejects SetReadDeadline on darwin.
func openSilentPTY(t *testing.T) (master, slave *os.File) {
	t.Helper()
	master, slave, err := pty.Open()
	if err != nil {
		t.Skipf("cannot allocate a PTY on this host: %v", err)
	}
	// Closing the slave makes a blocked master read return EOF, so a failing
	// run does not leave its reader goroutine behind for the rest of the binary.
	t.Cleanup(func() { _ = slave.Close(); _ = master.Close() })
	return master, slave
}

// parkedStreamLoops counts streamLoop goroutines currently parked in a
// syscall or the netpoller, i.e. inside the PTY read/wait.
func parkedStreamLoops() int {
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, 2*len(buf)) // truncated: retry with room
	}
	count := 0
	for _, g := range strings.Split(string(buf), "\n\n") {
		header := strings.SplitN(g, "\n", 2)[0]
		// "[syscall": darwin (unpollable master, select wait). "[IO wait":
		// linux, where the netpoller serves the deadline-bounded read.
		parked := strings.Contains(header, "[syscall") || strings.Contains(header, "[IO wait")
		if parked && strings.Contains(g, "(*ResponseStream).streamLoop") {
			count++
		}
	}
	return count
}

// waitUntilStreamLoopParked yields (no sleeping) until this test's loop is
// parked, i.e. the parked count rises above baseline (loops leaked by earlier
// tests are already counted in it). Cancelling earlier would let the loop exit
// at its next ctx check and hide the hang this file exists to catch.
func waitUntilStreamLoopParked(t *testing.T, baseline int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if parkedStreamLoops() > baseline {
			return
		}
		runtime.Gosched()
	}
	t.Fatal("stream loop never parked in a PTY syscall")
}

// primeStream starts rs and feeds one chunk through it so the loop has
// provably entered its read cycle, then waits until it is parked again.
func primeStream(t *testing.T, rs *ResponseStream, ctx context.Context, slave *os.File) {
	t.Helper()
	baseline := parkedStreamLoops()
	got := make(chan struct{}, 1)
	rs.SetOnOutput(func() {
		select {
		case got <- struct{}{}:
		default:
		}
	})
	if err := rs.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := slave.Write([]byte("x")); err != nil {
		t.Fatalf("slave write: %v", err)
	}
	select {
	case <-got:
	case <-time.After(10 * time.Second):
		t.Fatal("output written to the PTY never reached the stream")
	}
	waitUntilStreamLoopParked(t, baseline)
}

// stopWithin runs rs.Stop and fails the test if it does not return in time.
// The bound only classifies a hang as a failure; a passing run returns as soon
// as the stream loop observes cancellation.
func stopWithin(t *testing.T, rs *ResponseStream, bound time.Duration) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- rs.Stop() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Stop() = %v", err)
		}
	case <-time.After(bound):
		t.Fatal("Stop() did not return: the stream loop is blocked in pty.Read and cannot observe cancellation until the PTY produces output")
	}
}

// Not parallel: waitUntilStreamLoopParked scans every goroutine.
// TestResponseStream_StopWithSilentPTY proves that cancelling the stream never
// depends on the PTY producing data or being closed. Stop() joins the stream
// goroutine, so a read that ignores cancellation hangs it forever.
func TestResponseStream_StopWithSilentPTY(t *testing.T) {
	master, slave := openSilentPTY(t)
	rs := NewResponseStream("silent-pty", NewPTYAccess("silent-pty", master, NewCircularBuffer(4096)))
	primeStream(t, rs, context.Background(), slave)
	stopWithin(t, rs, 5*time.Second)
}

// TestResponseStream_ParentContextCancelWithSilentPTY covers the other stop
// path: the context passed to Start is cancelled (service shutdown), nobody
// calls Stop(), and the loop must still exit on its own.
func TestResponseStream_ParentContextCancelWithSilentPTY(t *testing.T) {
	master, slave := openSilentPTY(t)
	rs := NewResponseStream("silent-pty-ctx", NewPTYAccess("silent-pty-ctx", master, NewCircularBuffer(4096)))
	ctx, cancel := context.WithCancel(context.Background())
	primeStream(t, rs, ctx, slave)
	cancel()
	joined := make(chan struct{})
	go func() { rs.wg.Wait(); close(joined) }()
	select {
	case <-joined:
	case <-time.After(5 * time.Second):
		t.Fatal("stream goroutines still running after the parent context was cancelled")
	}
}

// TestResponseStream_DeliversOutputAndEOF checks behaviour parity: bytes
// written to the slave reach the buffer, and closing the slave is reported
// through OnEOF.
func TestResponseStream_DeliversOutputAndEOF(t *testing.T) {
	t.Parallel()
	master, slave, err := pty.Open()
	if err != nil {
		t.Skipf("cannot allocate a PTY on this host: %v", err)
	}
	t.Cleanup(func() { _ = master.Close() })
	buf := NewCircularBuffer(4096)
	rs := NewResponseStream("deliver-pty", NewPTYAccess("deliver-pty", master, buf))
	got := make(chan struct{}, 1)
	rs.SetOnOutput(func() {
		select {
		case got <- struct{}{}:
		default:
		}
	})
	eof := make(chan struct{})
	rs.OnEOF = func() { close(eof) }
	if err := rs.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := slave.Write([]byte("hello\n")); err != nil {
		t.Fatalf("slave write: %v", err)
	}
	select {
	case <-got:
	case <-time.After(5 * time.Second):
		t.Fatal("output written to the PTY never reached the stream")
	}
	_ = slave.Close()
	select {
	case <-eof:
	case <-time.After(5 * time.Second):
		t.Fatal("OnEOF not called after the slave side closed")
	}
	// onOutput fires before the buffer write, so only after EOF (same goroutine,
	// later) is the delivered data guaranteed to be in the buffer.
	if out := string(buf.GetAll()); !strings.Contains(out, "hello") {
		t.Fatalf("circular buffer = %q, want it to contain %q", out, "hello")
	}
}

// TestResponseStream_StopWithUnreadInputQueued guards the darwin FIONREAD trap:
// bytes written through the master that the slave has not read sit in the
// slave's input queue, which a readiness probe must not mistake for pending
// output. Stop must still return promptly.
func TestResponseStream_StopWithUnreadInputQueued(t *testing.T) {
	master, slave := openSilentPTY(t)
	// Raw, so the typed bytes are neither echoed back as output nor line-buffered.
	if _, err := term.MakeRaw(int(slave.Fd())); err != nil {
		t.Fatalf("MakeRaw: %v", err)
	}
	rs := NewResponseStream("silent-pty-input", NewPTYAccess("silent-pty-input", master, NewCircularBuffer(4096)))
	primeStream(t, rs, context.Background(), slave)
	if _, err := master.Write([]byte("typed but never read by the slave\n")); err != nil {
		t.Fatalf("master write: %v", err)
	}
	stopWithin(t, rs, 5*time.Second)
}
