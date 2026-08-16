package server

import (
	"context"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
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

// Backlog item 81e82fee-9528-4dc9-a513-1040b4dee2ec, AC3: the join must be
// bounded by a timeout, not a bare Wait().

// Test_waitGroupWithTimeout_should_ReturnTrue_When_WaitGroupReachesZeroBeforeTimeout
// covers the success path: wg.Done() is called well within the timeout, so
// waitGroupWithTimeout must return true promptly rather than waiting out the
// full timeout duration.
func Test_waitGroupWithTimeout_should_ReturnTrue_When_WaitGroupReachesZeroBeforeTimeout(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		time.Sleep(10 * time.Millisecond)
		wg.Done()
	}()

	start := time.Now()
	ok := waitGroupWithTimeout(&wg, 5*time.Second)
	elapsed := time.Since(start)

	if !ok {
		t.Fatal("expected waitGroupWithTimeout to return true when the WaitGroup reaches zero before the timeout")
	}
	if elapsed > time.Second {
		t.Fatalf("waitGroupWithTimeout took %v to return after the WaitGroup reached zero; expected it to return promptly, not wait out the full timeout", elapsed)
	}
}

// Test_waitGroupWithTimeout_should_ReturnFalse_When_WaitGroupNeverReachesZero
// covers the timeout path: the WaitGroup counter never reaches zero (a
// goroutine that never calls Done()), so waitGroupWithTimeout must return
// false at approximately the timeout instead of blocking forever like a bare
// wg.Wait() would.
func Test_waitGroupWithTimeout_should_ReturnFalse_When_WaitGroupNeverReachesZero(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	// Deliberately never call wg.Done() — simulates a stuck background task.

	const timeout = 100 * time.Millisecond
	start := time.Now()
	ok := waitGroupWithTimeout(&wg, timeout)
	elapsed := time.Since(start)

	if ok {
		t.Fatal("expected waitGroupWithTimeout to return false when the WaitGroup never reaches zero")
	}
	if elapsed < timeout {
		t.Fatalf("waitGroupWithTimeout returned after %v, before the %v timeout elapsed", elapsed, timeout)
	}
	if elapsed > timeout+2*time.Second {
		t.Fatalf("waitGroupWithTimeout took %v to return; expected it to return at approximately the %v timeout", elapsed, timeout)
	}
}

// Test_Shutdown_should_BlockUntilBackgroundTasksExit_When_BackgroundTaskIsRunning
// is the integration-level proof for AC0-AC2: a test-only goroutine registered
// on srv.backgroundTasksWG (standing in for StartForkPressureLogger /
// StartZombieWatcher / StartZombieReaper, which all join the same shared
// WaitGroup) must have fully exited by the time Shutdown() returns, not just
// been signaled via connCtxCancel.
func Test_Shutdown_should_BlockUntilBackgroundTasksExit_When_BackgroundTaskIsRunning(t *testing.T) {
	srv := newTestServer("localhost:0")

	var exited atomic.Bool
	srv.backgroundTasksWG.Add(1)
	go func() {
		defer srv.backgroundTasksWG.Done()
		// Simulate a background task still mid-tick when shutdown begins.
		time.Sleep(50 * time.Millisecond)
		exited.Store(true)
	}()

	if err := srv.Shutdown(); err != nil {
		t.Fatalf("Shutdown() returned unexpected error: %v", err)
	}
	if !exited.Load() {
		t.Fatal("Shutdown() returned before the background task finished — expected it to block until the task joined")
	}
}

// Test_Shutdown_should_NotBlockPastTimeout_When_BackgroundTaskNeverExits proves
// AC3 end-to-end through Shutdown() itself: a background task that never
// observes cancellation (simulating a stuck fork-pressure logger/zombie
// watcher/zombie reaper) must not prevent Shutdown() from returning — it
// should proceed once backgroundTasksJoinTimeout elapses, not hang forever.
func Test_Shutdown_should_NotBlockPastTimeout_When_BackgroundTaskNeverExits(t *testing.T) {
	srv := newTestServer("localhost:0")

	srv.backgroundTasksWG.Add(1)
	// Deliberately never call Done() — simulates a task stuck past shutdown signal.

	start := time.Now()
	err := srv.Shutdown()
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Shutdown() returned unexpected error: %v", err)
	}
	if elapsed < backgroundTasksJoinTimeout {
		t.Fatalf("Shutdown() returned after %v, before the %v background-tasks join timeout elapsed", elapsed, backgroundTasksJoinTimeout)
	}
	if elapsed > backgroundTasksJoinTimeout+5*time.Second {
		t.Fatalf("Shutdown() took %v to return; expected it to proceed shortly after the %v join timeout, not hang", elapsed, backgroundTasksJoinTimeout)
	}
}

// Test_Shutdown_should_NotPanic_When_CalledTwice guards against a regression
// where joining backgroundTasksWG a second time (e.g. a double Shutdown()
// call during a racy restart) could panic or deadlock. sync.WaitGroup.Wait()
// on an already-zero counter returns immediately, and connCtxCancel is an
// idempotent context.CancelFunc, so no additional guard should be needed.
func Test_Shutdown_should_NotPanic_When_CalledTwice(t *testing.T) {
	srv := newTestServer("localhost:0")

	if err := srv.Shutdown(); err != nil {
		t.Fatalf("first Shutdown() returned unexpected error: %v", err)
	}
	if err := srv.Shutdown(); err != nil {
		t.Fatalf("second Shutdown() returned unexpected error: %v", err)
	}
}
