package server

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/jules"
	"github.com/tstapler/stapler-squad/session/tmux"
	"go.uber.org/goleak"
)

// syncBuffer wraps bytes.Buffer with a mutex so it's safe to read via
// String() from the test goroutine while background goroutines the server
// under test spawned (SessionRetentionSweeper, MemoryPressureNotifier, etc.)
// are still writing to it via the default slog logger — a plain
// bytes.Buffer is not safe for that, and Server.Shutdown() does not
// guarantee every such goroutine has exited by the time it returns.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// tempDirTolerantOfStragglers behaves like t.TempDir() but replaces its
// cleanup with a few retries on ENOTEMPTY. wireDepsIntoServer starts several
// background goroutines (SessionRetentionSweeper, StaleSessionNotifier,
// MemoryPressureNotifier, the hibernation sweeper) as bare `go X.Start(ctx)`
// calls never registered with Server.backgroundTasksWG, so Shutdown() cannot
// guarantee they've stopped touching files under this directory before it
// returns — a pre-existing gap (backgroundTasksWG's own doc comment lists
// only "fork-pressure logger, zombie watcher, zombie reaper" as joined), not
// something introduced by the Jules tests using this helper. Properly fixing
// it means auditing and rewiring four unrelated services' shutdown
// registration, out of scope here — this local retry keeps that pre-existing
// gap from flaking these two Jules-specific tests.
func tempDirTolerantOfStragglers(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", strings.ReplaceAll(t.Name(), "/", "_"))
	require.NoError(t, err)
	t.Cleanup(func() {
		var lastErr error
		for i := 0; i < 5; i++ {
			if lastErr = os.RemoveAll(dir); lastErr == nil {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Logf("tempDirTolerantOfStragglers: cleanup of %s did not converge: %v", dir, lastErr)
	})
	return dir
}

// captureDefaultLog swaps slog's default handler for one that writes to a
// buffer, restoring the original on test cleanup. Mirrors
// log/package_level_external_test.go's TestInfo_AttributesToCallerNotLogPackage
// setup — the only existing precedent in this codebase for asserting on
// log.Info/Debug output.
func captureDefaultLog(t *testing.T) *syncBuffer {
	t.Helper()
	buf := &syncBuffer{}
	handler := slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	origDefault := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(origDefault) })
	return buf
}

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
// bounded by a timeout, not a bare Wait(). Coverage of the underlying
// timeout-bounded join primitive itself now lives in
// internal/syncutil/syncutil_test.go (WaitWithTimeout), shared with
// session/pty_discovery.go instead of duplicated here.

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

// goleakElidedStackPanicMarker is the exact, and only, panic message goleak
// v1.3.0 (confirmed the latest published release via
// `go list -m -versions go.uber.org/goleak`) produces when its stack-trace
// parser can't handle a line the Go runtime itself truncated with a
// "...N frames elided..." marker (go.uber.org/goleak@v1.3.0's
// internal/stack/stacks.go:90, panicking rather than erroring, per that
// package's own doc comment: "Well-formed stack traces should never fail to
// parse... Panic so we can fix it"). Elision is Go's own runtime behavior
// (runtime/traceback.go's tracebackInnerFrames + tracebackOuterFrames = 100
// total frames per goroutine) — not something this codebase's ticker
// goroutines can avoid by staying shallow, since any sufficiently deep
// goroutine present anywhere in the process at capture time (e.g. under a
// forked `ps` subprocess) triggers it for the whole capture.
//
// See backlog item 5d164328-c8b4-4e96-ae6e-79c9a6b3dc4e for full root cause.
const goleakElidedStackPanicMarker = "Failed to parse stack trace"

// runGoleakTolerant runs check (expected to invoke goleak.VerifyNone) and
// converts goleak's known elided-stack-trace parser panic
// (goleakElidedStackPanicMarker) into a t.Skip instead of letting it crash
// the whole test binary — see backlog item
// 5d164328-c8b4-4e96-ae6e-79c9a6b3dc4e. Any other panic is re-panicked
// unchanged, so unrelated test failures are never swallowed.
//
// KNOWN GAP: goleak's parser panics before its leak-filtering logic runs, so
// this cannot distinguish "a harmless goroutine happened to be deep enough to
// trigger elision" from "a genuinely leaked goroutine that also happens to be
// deep enough to trigger elision" — both hit the identical panic path. This
// wrapper accepts that small risk of masking a real leak in the rare case it
// coincides with elision; it is not a claim that the check remains fully
// leak-safe in that case.
func runGoleakTolerant(t *testing.T, check func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		msg := fmt.Sprint(r)
		if !strings.Contains(msg, goleakElidedStackPanicMarker) {
			// Not goleak's known elision-parser panic — propagate unchanged.
			panic(r)
		}
		// Diagnostic capture (companion to the workaround, not a fix): the
		// panic message already contains the full stack dump goleak
		// captured, so logging it here — rather than letting the panic
		// crash the binary before anyone sees it — gives a future
		// root-causing attempt real data instead of another blind
		// bisection. See backlog item 5d164328-c8b4-4e96-ae6e-79c9a6b3dc4e.
		t.Logf("goleak hit its known elided-stack parser panic; full diagnostic dump for future root-causing (backlog 5d164328-c8b4-4e96-ae6e-79c9a6b3dc4e):\n%s", msg)
		t.Skip("skipping: goleak's stack parser panicked on an elided (\"...N frames elided...\") stack trace — known goleak v1.3.0 limitation, see backlog 5d164328-c8b4-4e96-ae6e-79c9a6b3dc4e and the diagnostic dump logged above")
	}()
	check()
}

// verifyNoLeaksTolerant is goleak.VerifyNone wrapped by runGoleakTolerant.
// Use this in place of a bare goleak.VerifyNone(t, baseline) call wherever a
// test exercises goroutines whose stacks could plausibly grow deep enough to
// trigger Go's own stack-trace elision (e.g. tests that fork subprocesses,
// like tmux.StartZombieWatcher's `ps` calls). See
// goleakElidedStackPanicMarker and runGoleakTolerant for the full rationale.
func verifyNoLeaksTolerant(t *testing.T, baseline goleak.Option) {
	t.Helper()
	runGoleakTolerant(t, func() { goleak.VerifyNone(t, baseline) })
}

// ignoreCurrentTolerant is goleak.IgnoreCurrent wrapped in the same
// elision-panic recovery as runGoleakTolerant. goleak.IgnoreCurrent snapshots
// every goroutine alive anywhere in the process at call time (not just this
// test's own goroutines) via the identical stack.All() -> getStackBuffer()
// path goleak.VerifyNone uses, so it is exactly as exposed to the "...N
// frames elided..." parser panic — a straggling deep-stack goroutine left
// over from an earlier test can crash baseline capture before
// runGoleakTolerant's recover() around the later VerifyNone call is ever
// reached. CI hit this directly: the panic's own stack trace showed
// goleak.IgnoreCurrent() itself as the panicking frame. See backlog item
// 5d164328-c8b4-4e96-ae6e-79c9a6b3dc4e.
func ignoreCurrentTolerant(t *testing.T) goleak.Option {
	t.Helper()
	var opt goleak.Option
	func() {
		defer func() {
			r := recover()
			if r == nil {
				return
			}
			msg := fmt.Sprint(r)
			if !strings.Contains(msg, goleakElidedStackPanicMarker) {
				panic(r)
			}
			t.Logf("goleak hit its known elided-stack parser panic while capturing a baseline; full diagnostic dump for future root-causing (backlog 5d164328-c8b4-4e96-ae6e-79c9a6b3dc4e):\n%s", msg)
			t.Skip("skipping: goleak.IgnoreCurrent panicked on an elided (\"...N frames elided...\") stack trace before a baseline could be captured — known goleak v1.3.0 limitation, see backlog 5d164328-c8b4-4e96-ae6e-79c9a6b3dc4e and the diagnostic dump logged above")
		}()
		opt = goleak.IgnoreCurrent()
	}()
	return opt
}

// TestServer_Shutdown_JoinsBackgroundTickers pins the regression this fix
// addresses at the server level: the fork-pressure logger, zombie watcher,
// and zombie reaper goroutines used to be signaled via serverCtx cancellation
// but never joined, so Shutdown() could return while they were still running.
// Wiring them directly to a short interval and to srv.backgroundTasksWG
// (bypassing the full dependency graph, the same way newTestServer does)
// drives several ticks before Shutdown() is called; verifyNoLeaksTolerant
// afterward confirms all three have actually exited by the time Shutdown()
// returns — genuinely different coverage from the fake-goroutine test above,
// since this exercises the real tmux.Start* functions end to end.
//
// Uses verifyNoLeaksTolerant rather than a bare goleak.VerifyNone: this test
// exercises tmux.StartZombieWatcher, which forks a real `ps` subprocess, and
// CI has observed goleak's stack-trace parser hard-panic (not a normal test
// failure) when a goroutine's stack is deep enough to trigger Go's own
// "...N frames elided..." truncation. See backlog item
// 5d164328-c8b4-4e96-ae6e-79c9a6b3dc4e and goleakElidedStackPanicMarker's
// doc comment for the full root cause.
func TestServer_Shutdown_JoinsBackgroundTickers(t *testing.T) {
	baseline := ignoreCurrentTolerant(t)

	srv, serverCtx := newServerBase("localhost:0")

	noop := func(string, ...any) {}
	tmux.StartForkPressureLogger(serverCtx, time.Millisecond, noop, &srv.backgroundTasksWG)
	tmux.StartZombieWatcher(serverCtx, time.Millisecond, noop, &srv.backgroundTasksWG)
	tmux.StartZombieReaper(serverCtx, time.Millisecond, noop, &srv.backgroundTasksWG)

	time.Sleep(20 * time.Millisecond) // let several ticks fire

	if err := srv.Shutdown(); err != nil {
		t.Fatalf("Shutdown() returned unexpected error: %v", err)
	}

	verifyNoLeaksTolerant(t, baseline)
}

// Test_Shutdown_should_NotBlockPastTimeout_When_BackgroundTaskNeverExits proves
// AC3 end-to-end through Shutdown() itself: a background task that never
// observes cancellation (simulating a stuck fork-pressure logger/zombie
// watcher/zombie reaper) must not prevent Shutdown() from returning — it
// should proceed once the join timeout elapses, not hang forever.
//
// srv.backgroundTasksJoinTimeout is overridden to a short duration here so
// this test doesn't have to wait out the real 10s production default
// (defaultBackgroundTasksJoinTimeout) on every run.
func Test_Shutdown_should_NotBlockPastTimeout_When_BackgroundTaskNeverExits(t *testing.T) {
	srv := newTestServer("localhost:0")
	const testTimeout = 50 * time.Millisecond
	srv.backgroundTasksJoinTimeout = testTimeout

	srv.backgroundTasksWG.Add(1)
	// Deliberately never call Done() — simulates a task stuck past shutdown signal.

	start := time.Now()
	err := srv.Shutdown()
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Shutdown() returned unexpected error: %v", err)
	}
	if elapsed < testTimeout {
		t.Fatalf("Shutdown() returned after %v, before the %v background-tasks join timeout elapsed", elapsed, testTimeout)
	}
	if elapsed > testTimeout+5*time.Second {
		t.Fatalf("Shutdown() took %v to return; expected it to proceed shortly after the %v join timeout, not hang", elapsed, testTimeout)
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

// deepRecurse recurses depth times before invoking done, then blocks forever
// on block. Used to synthesize a goroutine stack deep enough to trigger Go's
// own "...N frames elided..." truncation (runtime/traceback.go's
// tracebackInnerFrames + tracebackOuterFrames = 100 total frames per
// goroutine) — the real trigger for goleak's parser panic, since no captured
// real CI trace is available to replay. See backlog item
// 5d164328-c8b4-4e96-ae6e-79c9a6b3dc4e.
func deepRecurse(depth int, done func(), block <-chan struct{}) {
	if depth <= 0 {
		done()
		<-block
		return
	}
	deepRecurse(depth-1, done, block)
}

// Test_runGoleakTolerant_should_SkipInsteadOfCrash_When_GoleakHitsElidedStackTrace
// covers AC0/AC1: a goroutine deep enough to trigger Go's own stack-trace
// elision must make runGoleakTolerant skip the (sub)test rather than crash
// the whole test binary via goleak's unrecovered parser panic.
func Test_runGoleakTolerant_should_SkipInsteadOfCrash_When_GoleakHitsElidedStackTrace(t *testing.T) {
	block := make(chan struct{})
	ready := make(chan struct{})
	exited := make(chan struct{})
	// 150 frames comfortably exceeds the 100-frame elision threshold.
	go func() {
		defer close(exited)
		deepRecurse(150, func() { close(ready) }, block)
	}()
	<-ready
	// Not load-bearing: deepRecurse's done() callback (which closes ready)
	// only fires once the goroutine has already reached full depth and is
	// about to block on <-block, so the happens-before edge from closing
	// ready already guarantees full depth here. This sleep is a defensive
	// margin, not a synchronization point.
	time.Sleep(10 * time.Millisecond)

	// Unblock and wait for the deep goroutine to fully exit before this test
	// returns — otherwise it can still be mid-unwind (a still-deep, still-live
	// goroutine) when a later test takes its own goleak.VerifyNone snapshot,
	// spuriously tripping that unrelated test's elision panic.
	defer func() {
		close(block)
		<-exited
	}()

	// t.Skip (called on the elision path) invokes runtime.Goexit, which only
	// unwinds correctly inside a goroutine started by testing's own tRunner —
	// a bare &testing.T{} isn't one, so the check must run inside a real
	// t.Run subtest rather than against a manually constructed *testing.T.
	// Goexit immediately abandons the rest of the closure's function body
	// (only deferred calls still run), so capturing st.Skipped()/st.Failed()
	// as a plain statement AFTER runGoleakTolerant returns never executes on
	// the skip path — it must be captured in a defer instead.
	var wasSkipped, wasFailed bool
	t.Run("inner", func(st *testing.T) {
		defer func() {
			wasSkipped = st.Skipped()
			wasFailed = st.Failed()
		}()
		runGoleakTolerant(st, func() { goleak.VerifyNone(st) })
	})

	if !wasSkipped {
		t.Error("expected runGoleakTolerant to skip the test when goleak hits its elided-stack parser panic, but it did not skip")
	}
	if wasFailed {
		t.Error("expected runGoleakTolerant's skip path to not also mark the test failed")
	}
}

// assertGenuineLeakStillFails is the shared body behind
// Test_runGoleakTolerant_should_StillFailOnGenuineLeak_When_NoElisionOccurs
// and Test_verifyNoLeaksTolerant_should_ForwardBaselineAndFailOnGenuineLeak:
// both prove a real leaked goroutine with a shallow (non-elided) stack is
// still reported as a genuine test failure when routed through goleak, not
// masked or silently skipped. calleeName only affects the failure message
// (naming whichever tolerant wrapper the calling test is documenting).
//
// It calls goleak.VerifyNone directly rather than runGoleakTolerant or
// verifyNoLeaksTolerant themselves: those wrappers' own elision path calls
// t.Skip, which invokes runtime.Goexit — legal only inside a goroutine
// started by testing's own tRunner, which sub (below) is not. CI hit exactly
// this: in a large test binary (hundreds of concurrently-running goroutines
// under -race) goleak's whole-process capture can itself hit the
// elided-stack parser panic (goleakElidedStackPanicMarker) regardless of how
// shallow this test's own deliberately-leaked goroutine is — that capture
// panic is recovered here directly, and skipped via the real t (safe: called
// at the top level of this test's own tRunner goroutine), never via sub.
func assertGenuineLeakStillFails(t *testing.T, calleeName string) {
	t.Helper()
	baseline := ignoreCurrentTolerant(t)

	block := make(chan struct{})
	// Deliberately never close block or wait for this goroutine — it leaks
	// past the end of the test on purpose, with a shallow (unelided) stack.
	go func() { <-block }()
	t.Cleanup(func() { close(block) })

	// goleak.VerifyNone's leak-found path only ever calls t.Error (never
	// t.Fatal/t.Skip), which — unlike the elision-skip test above — never
	// invokes runtime.Goexit, so a manually constructed *testing.T is safe
	// to receive that call. Deliberately NOT using t.Run: a failing subtest
	// also marks its parent failed (common.Fail's parent-chain propagation),
	// which would make this meta-test itself report as failed in `go test`
	// output even though observing the expected failure here is success.
	//
	// runGoleakTolerant itself is NOT used here (unlike the elision-skip test
	// above): its own elision path calls t.Skip, which invokes
	// runtime.Goexit — legal only inside a goroutine started by testing's own
	// tRunner, which sub is not. CI hit exactly this: in a large test binary
	// (hundreds of concurrently-running goroutines under -race) goleak's
	// whole-process capture can itself hit the elided-stack parser panic
	// (goleakElidedStackPanicMarker) regardless of how shallow this test's
	// own deliberately-leaked goroutine is — that capture panic is recovered
	// here directly, and skipped via the real t (safe: called at the top
	// level of this test's own tRunner goroutine), never via sub.
	sub := &testing.T{}
	elided := func() (elided bool) {
		defer func() {
			r := recover()
			if r == nil {
				return
			}
			if !strings.Contains(fmt.Sprint(r), goleakElidedStackPanicMarker) {
				panic(r)
			}
			elided = true
		}()
		goleak.VerifyNone(sub, baseline)
		return false
	}()
	if elided {
		t.Skip("skipping: goleak's whole-process capture hit the elided-stack parser panic before it could check for the genuine leak this test injects — inconclusive rather than a real pass/fail, see backlog 5d164328-c8b4-4e96-ae6e-79c9a6b3dc4e")
	}

	if !sub.Failed() {
		t.Errorf("expected %s to report a genuine, non-elided goroutine leak as a failure", calleeName)
	}
	if sub.Skipped() {
		t.Error("a genuine leak must fail the test, not be skipped")
	}
}

// Test_runGoleakTolerant_should_StillFailOnGenuineLeak_When_NoElisionOccurs
// covers AC1/AC3: a real leaked goroutine with a shallow (non-elided) stack
// must still be reported as a normal test failure, proving the tolerant
// wrapper doesn't mask genuine leaks in the common case.
func Test_runGoleakTolerant_should_StillFailOnGenuineLeak_When_NoElisionOccurs(t *testing.T) {
	assertGenuineLeakStillFails(t, "runGoleakTolerant")
}

// Test_verifyNoLeaksTolerant_should_ForwardBaselineAndFailOnGenuineLeak
// exercises verifyNoLeaksTolerant directly rather than only as a side effect
// of TestServer_Shutdown_JoinsBackgroundTickers: it proves the wrapper
// actually forwards baseline into goleak.VerifyNone (a leaked goroutine not
// present in baseline is reported) rather than, say, silently dropping it.
func Test_verifyNoLeaksTolerant_should_ForwardBaselineAndFailOnGenuineLeak(t *testing.T) {
	assertGenuineLeakStillFails(t, "verifyNoLeaksTolerant")
}

// Test_runGoleakTolerant_should_RepanicUnrelatedPanics covers AC2: a panic
// that doesn't match goleak's known elided-stack marker must propagate
// unchanged, so runGoleakTolerant never silently swallows an unrelated
// panic.
func Test_runGoleakTolerant_should_RepanicUnrelatedPanics(t *testing.T) {
	const unrelatedPanicMsg = "some unrelated panic, not goleak's elided-stack parser panic"

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected the unrelated panic to propagate out of runGoleakTolerant, but it did not panic")
		}
		if fmt.Sprint(r) != unrelatedPanicMsg {
			t.Fatalf("expected the original unrelated panic message to propagate unchanged, got: %v", r)
		}
	}()

	runGoleakTolerant(t, func() { panic(unrelatedPanicMsg) })
}

// TestServer_should_LeaveJulesSessionPollerNil_When_JulesConfigDisabled covers
// google-jules-integration Story 2.4.4: with Jules disabled (the zero-value
// default), the server must start with deps.JulesSessionPoller nil and must
// never log a "jules poll tick" line (which only the poller's own tick()
// loop, never started, could produce).
func TestServer_should_LeaveJulesSessionPollerNil_When_JulesConfigDisabled(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", tempDirTolerantOfStragglers(t))
	buf := captureDefaultLog(t)

	deps, err := BuildDependencies()
	require.NoError(t, err)

	srv := NewServerWithDeps("localhost:0", deps)
	// Shut down before reading buf: wireDepsIntoServer starts background
	// goroutines (SessionRetentionSweeper, MemoryPressureNotifier, etc.) that
	// write to this same shared, unsynchronized bytes.Buffer via the default
	// slog logger. Reading buf.String() while they're still running (as a
	// deferred t.Cleanup would, since cleanups run after the test body) is a
	// data race under -race — Shutdown stops them first.
	require.NoError(t, srv.Shutdown())

	assert.Nil(t, deps.JulesSessionPoller)
	assert.NotContains(t, buf.String(), "jules poll tick")
}

// TestServer_should_LogJulesSessionPollerStarted_When_JulesEnabledAndKeyResolvable
// covers Story 2.4.4: with Jules enabled and a resolvable API key at
// startup, "JulesSessionPoller started" must be logged beside the existing
// "WorktreePRPoller started" line (server.go's wireDepsIntoServer).
func TestServer_should_LogJulesSessionPollerStarted_When_JulesEnabledAndKeyResolvable(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", tempDirTolerantOfStragglers(t))
	keyring.MockInit()
	require.NoError(t, jules.NewKeyringTokenSource().SetJulesAPIKey(context.Background(), "AIzaSyD-TEST"))

	cfg := config.LoadConfig()
	cfg.Jules.Enabled = true
	require.NoError(t, config.SaveConfig(cfg))

	buf := captureDefaultLog(t)

	deps, err := BuildDependencies()
	require.NoError(t, err)

	srv := NewServerWithDeps("localhost:0", deps)
	// Shut down before reading buf — see the sibling
	// TestServer_should_LeaveJulesSessionPollerNil_When_JulesConfigDisabled's
	// comment: background goroutines wireDepsIntoServer starts write to this
	// same shared, unsynchronized bytes.Buffer, so reading it via a deferred
	// t.Cleanup (which runs after the test body) races under -race.
	require.NoError(t, srv.Shutdown())

	require.NotNil(t, deps.JulesSessionPoller)
	assert.Contains(t, buf.String(), "JulesSessionPoller started")
	assert.Contains(t, buf.String(), "WorktreePRPoller started")
}
