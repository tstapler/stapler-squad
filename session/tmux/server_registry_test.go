package tmux

import "github.com/linkdata/deadlock"

import (
	"bufio"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

// newTestRegistry creates a registry whose event loop is driven by a fake pipe
// rather than a real tmux process. The caller writes lines to the returned
// strings.Builder via feedLine.
func newTestRegistry(t *testing.T) (*TmuxServerRegistry, func(line string)) {
	t.Helper()
	r := NewTmuxServerRegistry("") // no real socket needed
	// Mark healthy manually since we bypass startControlMode.
	r.healthMu.Lock()
	r.healthy = true
	r.healthMu.Unlock()

	pipeR, pipeW := newPipe()

	go func() {
		r.readLines(bufio.NewScanner(pipeR))
	}()

	t.Cleanup(func() {
		r.Stop()
		_ = pipeW.Close()
	})

	feed := func(line string) {
		_, _ = pipeW.Write([]byte(line + "\n"))
	}
	return r, feed
}

// newPipe returns a synchronised reader/writer pair backed by a strings.Builder
// that blocks reads until data is available.
type syncPipe struct {
	mu   deadlock.Mutex
	cond *sync.Cond
	buf  []byte
	done bool
}

func newPipe() (*syncPipe, *syncPipe) {
	sp := &syncPipe{}
	sp.cond = sync.NewCond(&sp.mu)
	return sp, sp
}

func (sp *syncPipe) Read(p []byte) (int, error) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	for len(sp.buf) == 0 && !sp.done {
		sp.cond.Wait()
	}
	if len(sp.buf) == 0 {
		return 0, nil // EOF
	}
	n := copy(p, sp.buf)
	sp.buf = sp.buf[n:]
	return n, nil
}

func (sp *syncPipe) Write(p []byte) (int, error) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	sp.buf = append(sp.buf, p...)
	sp.cond.Signal()
	return len(p), nil
}

func (sp *syncPipe) Close() error {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	sp.done = true
	sp.cond.Broadcast()
	return nil
}

// waitFor polls fn until it returns true or 2 seconds elapse.
func waitFor(t *testing.T, fn func() bool) {
	t.Helper()
	if err := wait.WaitForCondition(fn, wait.WaitConfig{Timeout: 2 * time.Second, PollInterval: 5 * time.Millisecond, Description: "condition"}); err != nil {
		t.Fatal("condition not met within deadline")
	}
}

// --- Tests ---

func TestRegistry_EventParsing_SessionCreated(t *testing.T) {
	r, feed := newTestRegistry(t)

	feed("%session-created $0 mysession")
	waitFor(t, func() bool { return r.SessionExists("mysession") })
	require.True(t, r.SessionExists("mysession"))
}

func TestRegistry_EventParsing_SessionClosed(t *testing.T) {
	r, feed := newTestRegistry(t)

	// First create the session.
	r.mu.Lock()
	r.sessions["mysession"] = true
	r.mu.Unlock()

	feed("%session-closed $0 mysession")
	waitFor(t, func() bool { return !r.SessionExists("mysession") })
	require.False(t, r.SessionExists("mysession"))
}

func TestRegistry_EventParsing_PaneExited_ClosesSubscriber(t *testing.T) {
	r, feed := newTestRegistry(t)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	ch := r.SubscribePaneExit(ctx, "mysession")

	feed("%pane-exited %1 -t mysession:0.0")
	select {
	case <-ch:
		// subscriber channel was closed as expected
	case <-time.After(2 * time.Second):
		t.Fatal("subscriber channel not closed after pane-exited event")
	}
}

func TestRegistry_EventParsing_UnknownEvents_NoPanel(t *testing.T) {
	r, feed := newTestRegistry(t)
	// These should be ignored without panic.
	feed("%begin 1234 1")
	feed("%end 1234 1")
	feed("%output $0 hello")
	feed("%unrecognised-event foo bar baz")
	// Wait for the event loop to process the lines; registry must remain healthy.
	waitFor(t, func() bool { return r.IsHealthy() })
	require.True(t, r.IsHealthy())
}

func TestRegistry_SubscribePaneExit_ContextCancel(t *testing.T) {
	r, _ := newTestRegistry(t)

	ctx, cancel := context.WithCancel(context.Background())
	ch := r.SubscribePaneExit(ctx, "sess-cancel")

	// Cancel before any event fires.
	cancel()

	select {
	case <-ch:
		// Channel closed immediately on ctx cancel — correct.
	case <-time.After(2 * time.Second):
		t.Fatal("subscriber channel not closed after context cancel")
	}
}

func TestRegistry_Stop_ClosesAllSubscribers(t *testing.T) {
	r := NewTmuxServerRegistry("")
	r.healthMu.Lock()
	r.healthy = true
	r.healthMu.Unlock()

	ctx := context.Background()
	ch1 := r.SubscribePaneExit(ctx, "sess-a")
	ch2 := r.SubscribePaneExit(ctx, "sess-b")

	r.Stop()

	timeout := time.After(2 * time.Second)
	for _, ch := range []<-chan struct{}{ch1, ch2} {
		select {
		case <-ch:
		case <-timeout:
			t.Fatal("subscriber channel not closed after Stop()")
		}
	}
}

func TestRegistry_ConcurrentSubscriptions(t *testing.T) {
	r, feed := newTestRegistry(t)

	const numGoroutines = 10
	var wg sync.WaitGroup
	channels := make([]<-chan struct{}, numGoroutines)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			channels[idx] = r.SubscribePaneExit(ctx, "concurrent-sess")
		}(i)
	}
	wg.Wait()

	// Fire pane exit; all channels must be closed without deadlock or double-close.
	feed("%pane-exited %1 -t concurrent-sess:0.0")

	timeout := time.After(3 * time.Second)
	for _, ch := range channels {
		if ch == nil {
			continue
		}
		select {
		case <-ch:
		case <-timeout:
			t.Fatal("not all concurrent subscriber channels were closed after pane exit event")
		}
	}
}

func TestRegistry_SessionClosed_FiresPaneExit(t *testing.T) {
	r, feed := newTestRegistry(t)

	r.mu.Lock()
	r.sessions["closedSess"] = true
	r.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	ch := r.SubscribePaneExit(ctx, "closedSess")
	feed("%session-closed $1 closedSess")

	select {
	case <-ch:
		// Pane exit fired on session-closed.
	case <-time.After(2 * time.Second):
		t.Fatal("pane exit not fired when session-closed received")
	}
	require.False(t, r.SessionExists("closedSess"))
}
