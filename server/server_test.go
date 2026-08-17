package server

import (
	"context"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/tmux"
	"go.uber.org/goleak"
)

// newTestServer builds a minimal Server (bypassing BuildDependencies),
// suitable for exercising Start()'s listener behavior in isolation from the
// rest of the dependency graph. Start() itself registers a "/health" route
// on s.mux and builds the middleware-wrapped handler, so no extra handler
// setup is needed here.
func newTestServer(addr string) *Server {
	srv, _ := newServerBase(addr)
	return srv
}

// Task 1.1.1a, REQ-1 test #1: binding an address ending in port 0 should
// resolve a real OS-assigned port and update s.addr before Start() returns
// control to the caller (i.e. before any request handling can occur).
func Test_Start_should_BindOSAssignedPort_And_UpdateAddr_When_ListenAddressEndsInZero(t *testing.T) {
	srv := newTestServer("localhost:0")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start(ctx)
	}()

	// Poll for the address to be updated away from the requested ":0" address.
	deadline := time.Now().Add(5 * time.Second)
	var addr string
	for time.Now().Before(deadline) {
		addr = srv.GetAddr()
		if addr != "localhost:0" && addr != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if addr == "localhost:0" || addr == "" {
		t.Fatalf("expected GetAddr() to reflect a real bound port, got %q", addr)
	}
	if strings.HasSuffix(addr, ":0") {
		t.Fatalf("expected a non-zero OS-assigned port, got %q", addr)
	}

	// The resolved address must actually be reachable.
	resp, err := http.Get("http://" + addr + "/health")
	if err != nil {
		t.Fatalf("expected resolved address to be reachable, got error: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from resolved address, got %d", resp.StatusCode)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Start() returned unexpected error after shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start() did not return after context cancellation")
	}
}

// Task 1.1.1a, REQ-1 test #2: an explicit, already-known port must be bound
// exactly as requested — no regression for tests/e2e/helpers/test-server.ts's
// explicit-port path.
func Test_Start_should_BindExplicitPort_When_PortIsAlreadyKnown(t *testing.T) {
	// Reserve a free port up front, then release it so the explicit address
	// is very likely free for the real bind below.
	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to reserve a free port: %v", err)
	}
	addr := ln.Addr().String()
	if cerr := ln.Close(); cerr != nil {
		t.Fatalf("failed to release reserved port: %v", cerr)
	}

	srv := newTestServer(addr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start(ctx)
	}()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, gerr := http.Get("http://" + addr + "/")
		if gerr == nil {
			resp.Body.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if got := srv.GetAddr(); got != addr {
		t.Fatalf("expected GetAddr() to remain %q, got %q", addr, got)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Start() returned unexpected error after shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start() did not return after context cancellation")
	}
}

// Task 1.1.1a, REQ-1 test #3: if the requested port is already bound by
// another listener, Start() must surface the net.Listen error rather than
// panicking or silently retrying.
func Test_Start_should_ReturnListenError_When_RequestedPortIsAlreadyBound(t *testing.T) {
	// Bind a real listener first to occupy the port for the duration of the test.
	occupied, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to bind occupying listener: %v", err)
	}
	defer occupied.Close()
	addr := occupied.Addr().String()

	srv := newTestServer(addr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start(ctx)
	}()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected Start() to return an error when the requested port is already bound")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start() did not return an error within the timeout")
	}
}

// TestWaitGroupWithTimeout pins waitGroupWithTimeout's two branches directly,
// mirroring session's TestWaitGroupWithTimeout for the same helper shape.
func TestWaitGroupWithTimeout(t *testing.T) {
	t.Run("returns true when the group finishes in time", func(t *testing.T) {
		var wg sync.WaitGroup
		wg.Add(1)
		go func() { defer wg.Done() }()
		if !waitGroupWithTimeout(&wg, time.Second) {
			t.Error("waitGroupWithTimeout returned false, want true")
		}
	})

	t.Run("returns false when the group doesn't finish in time", func(t *testing.T) {
		var wg sync.WaitGroup
		wg.Add(1) // deliberately never Done()
		if waitGroupWithTimeout(&wg, 10*time.Millisecond) {
			t.Error("waitGroupWithTimeout returned true, want false")
		}
	})
}

// TestServer_Shutdown_JoinsBackgroundTickers pins the regression this fix
// addresses at the server level: the fork-pressure logger, zombie watcher,
// and zombie reaper goroutines used to be signaled via serverCtx cancellation
// but never joined, so Shutdown() could return while they were still running.
// Wiring them directly to a short interval and to srv.bgTickersWG (bypassing
// the full dependency graph, the same way newTestServer does) drives several
// ticks before Shutdown() is called; goleak.VerifyNone afterward confirms all
// three have actually exited by the time Shutdown() returns.
func TestServer_Shutdown_JoinsBackgroundTickers(t *testing.T) {
	baseline := goleak.IgnoreCurrent()

	srv, serverCtx := newServerBase("localhost:0")

	noop := func(string, ...any) {}
	tmux.StartForkPressureLogger(serverCtx, time.Millisecond, noop, &srv.bgTickersWG)
	tmux.StartZombieWatcher(serverCtx, time.Millisecond, noop, &srv.bgTickersWG)
	tmux.StartZombieReaper(serverCtx, time.Millisecond, noop, &srv.bgTickersWG)

	time.Sleep(20 * time.Millisecond) // let several ticks fire

	if err := srv.Shutdown(); err != nil {
		t.Fatalf("Shutdown() returned unexpected error: %v", err)
	}

	goleak.VerifyNone(t, baseline)
}
