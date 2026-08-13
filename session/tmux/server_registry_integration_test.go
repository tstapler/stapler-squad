//go:build integration

package tmux_test

import (
	"context"
	"fmt"
	"hash/fnv"
	"math/rand"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/tmux"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

// newIsolatedSocket returns a unique tmux socket name for the test and registers
// cleanup that kills the isolated server when the test finishes.
func newIsolatedSocket(t *testing.T) string {
	t.Helper()
	// Embed PID (so TestMain's watchdog can still reap this socket by its
	// "integration_" prefix on SIGKILL) plus a random suffix: t.Name() alone
	// collides across repeated runs of the same test in one process (-count=N,
	// or any other reason the same test body executes twice), racing one
	// run's teardown against the next run's setup on the same socket path --
	// observed as an intermittent "server exited unexpectedly" failure.
	//
	// t.Name() is hashed (fnv32a, 8 hex chars) rather than embedded verbatim:
	// AF_UNIX's sun_path is capped at ~107 usable bytes, and
	// "/tmp/tmux-<uid>/integration_<pid>_<name>_<rand>" overflows that limit
	// for long test names (e.g.
	// TestTmuxServerRegistry_PaneExitDetectedDespiteElevatedBackoff, 63
	// chars) -- confirmed via a deterministic "File name too long" failure
	// from `tmux new-session`, not a flake. extractTestSocketPID
	// (testutil/tmuxreap) only requires a "_"-delimited numeric PID segment
	// within pidMax after the "integration_" prefix, which this format still
	// provides.
	h := fnv.New32a()
	_, _ = h.Write([]byte(t.Name()))
	socket := fmt.Sprintf("integration_%d_%08x_%d", os.Getpid(), h.Sum32(), rand.Int31())
	t.Cleanup(func() {
		exec.Command(tmux.Binary(), "-L", socket, "kill-server").Run() //nolint:errcheck
	})
	return socket
}

// newSessionWithRetry runs `tmux -L socket -f /dev/null new-session <args...>`,
// retrying up to 3 times with short backoff. -f /dev/null is load-bearing when
// this call is the one that starts the isolated server (always true for the
// keepalive session, the first command issued against a fresh socket): tmux
// reads ~/.tmux.conf (and /etc/tmux.conf) on server startup regardless of -L,
// so without -f every "isolated" test server was silently inheriting this
// developer's real interactive config -- including `run
// '~/.tmux/plugins/tpm/tpm'`, which forks several `tmux <subcommand>` and
// helper-script child processes against the brand-new server as part of
// config load. That contended with this test suite's own near-immediate
// list-sessions/attach-session calls on the same fresh server and was
// root-caused (via temporary stderr capture on syncSessionsLocked, see git
// history) as the actual source of the "server exited unexpectedly" /
// "no server running" crashes this comment used to attribute to generic
// "heavy concurrent tmux usage" -- confirmed by the fact that count=40 loops
// stopped reproducing the crash once config loading was suppressed here.
// The retry loop is kept as defense-in-depth for genuine host-load
// transients (matching EnsureServerRunning's recovery pattern in
// session/tmux/tmux.go), not because -f alone was expected to leave any
// residual flakiness to retry away.
func newSessionWithRetry(t *testing.T, socket string, args ...string) {
	t.Helper()
	fullArgs := append([]string{"-L", socket, "-f", "/dev/null", "new-session"}, args...)
	const maxAttempts = 3
	var lastErr error
	var lastOut []byte
	for attempt := 0; attempt < maxAttempts; attempt++ {
		out, err := exec.Command(tmux.Binary(), fullArgs...).CombinedOutput()
		if err == nil {
			return
		}
		lastErr, lastOut = err, out
		if attempt < maxAttempts-1 {
			time.Sleep(time.Duration(100*(1<<uint(attempt))) * time.Millisecond)
		}
	}
	t.Fatalf("new-session failed after %d attempts: %v (%s)", maxAttempts, lastErr, lastOut)
}

// startIsolatedRegistry starts a tmux server on an isolated socket, creates the
// keepalive session (required by TmuxServerRegistry's control-mode attach), and
// returns a running registry whose context is cancelled by t.Cleanup.
func startIsolatedRegistry(t *testing.T) (*tmux.TmuxServerRegistry, string) {
	t.Helper()
	socket := newIsolatedSocket(t)

	// Create the keepalive session atomically with the server start. Using a
	// single new-session command avoids a race where the server starts with
	// exit-empty=on and then exits before the separate new-session arrives.
	// TmuxPrefix+"keepalive" is the name that TmuxServerRegistry.startControlMode
	// attaches to. "sleep 300" keeps the session alive for the test duration.
	keepaliveName := tmux.TmuxPrefix + "keepalive"
	newSessionWithRetry(t, socket, "-d", "-s", keepaliveName, "sleep 300")

	registry := tmux.NewTmuxServerRegistry(socket)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	if err := registry.Start(ctx); err != nil {
		t.Fatalf("registry.Start: %v", err)
	}

	// Wait for the control-mode client to actually attach before returning.
	// Root-caused via tmux's own -v server log (see git history): a session
	// created by the caller immediately after startIsolatedRegistry returns
	// can land in the gap between the registry's control-mode attach-session
	// (fully processed server-side only once IsHealthy() goes true) and its
	// post-connect syncSessions() snapshot. A session created in that gap
	// predates the control client's subscription, so tmux never emits
	// %session-created/%sessions-changed for it to this client, and no
	// further resync is guaranteed if the connection then stays healthy
	// indefinitely (no debounce trigger, no reconnect) -- the session is
	// permanently invisible to the registry, not just delayed. Confirmed
	// directly: reproduced as "session not visible in registry before
	// subscribing" timing out at the full registryPollTimeout with zero
	// reconnect/backoff log lines in between, i.e. the control-mode
	// connection never dropped and simply never re-synced. Waiting for
	// IsHealthy() here (set only after the post-connect sync succeeds, which
	// requires a live round trip to a server that has therefore also already
	// processed the earlier-submitted attach-session) closes the gap: any
	// session a test creates after this point is guaranteed to be created
	// after the control client has subscribed.
	pollUntil(t, registryPollTimeout, "registry did not become healthy initially", registry.IsHealthy)

	return registry, socket
}

// registryPollTimeout bounds every timing assertion in this file that
// depends on the registry's control-mode connection being up and synced.
// Sized at 12s (not the nominal ~3s of backoff) to cover -race slowdown and
// tmux-fork contention; kept above 8s even after the Makefile's -p 1
// serialization (the main fix for cross-package contention) since isolated
// runs still showed cycles occasionally exceeding that.
const registryPollTimeout = 12 * time.Second

// pollUntil polls fn until it returns true or the timeout expires.
// It calls t.Fatal with msg if the timeout is exceeded.
func pollUntil(t *testing.T, timeout time.Duration, msg string, fn func() bool) {
	t.Helper()
	if err := wait.WaitForCondition(fn, wait.WaitConfig{Timeout: timeout, PollInterval: 5 * time.Millisecond, Description: msg}); err != nil {
		t.Fatal(msg)
	}
}

// waitForFastRecheckWaitStart blocks until reconnectLoop reports (via
// SetFastRecheckWaitStartHook) that it has begun a backoff wait >= minBackoff.
// It waits on that real signal rather than an estimated wall-clock offset
// derived from the nominal backoff formula, since an estimate can land after
// that cycle's own fast-recheck window already ran and found nothing,
// deferring detection to the next, much longer cycle.
func waitForFastRecheckWaitStart(t *testing.T, registry *tmux.TmuxServerRegistry, minBackoff time.Duration, timeout time.Duration) {
	t.Helper()

	started := make(chan time.Duration, 1)
	registry.SetFastRecheckWaitStartHook(func(backoff time.Duration) {
		if backoff < minBackoff {
			return
		}
		select {
		case started <- backoff:
		default:
			// A qualifying wait was already reported; later ones (e.g. this
			// test's own kill-session hasn't landed yet and backoff keeps
			// climbing) are irrelevant once the caller has what it needs.
		}
	})

	select {
	case <-started:
		return
	case <-time.After(timeout):
		t.Fatalf("backoff never reached a fast-recheck-eligible wait >= %s within %s", minBackoff, timeout)
	}
}

// Test 1: Registry starts and becomes healthy within 2 seconds.
// IsHealthy is set in the reconnectLoop goroutine for a brief window on each
// cycle. We spin-wait with runtime.Gosched() to cooperatively yield and catch
// the healthy window without a fixed sleep interval.
func TestTmuxServerRegistry_StartsHealthy(t *testing.T) {
	registry, _ := startIsolatedRegistry(t)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if registry.IsHealthy() {
			return
		}
		runtime.Gosched()
	}
	t.Fatal("registry did not become healthy within 2 seconds")
}

// Test 2: Creating a tmux session reflects in SessionExists within registryPollTimeout.
// The registry syncs its session map on every reconnect cycle (~100ms), so even
// in headless environments where the control-mode connection is short-lived a
// new session becomes visible on the next syncSessions pass.
func TestTmuxServerRegistry_SessionCreated(t *testing.T) {
	registry, socket := startIsolatedRegistry(t)

	sessionName := "testcreated"
	newSessionWithRetry(t, socket, "-d", "-s", sessionName)
	t.Cleanup(func() {
		exec.Command(tmux.Binary(), "-L", socket, "kill-session", "-t", sessionName).Run() //nolint:errcheck
	})

	pollUntil(t, registryPollTimeout, "session not visible in registry within 3s", func() bool {
		return registry.SessionExists(sessionName)
	})
}

// Test 3: Killing a tmux session closes SubscribePaneExit channel within registryPollTimeout.
// In headless mode the registry detects the disappearance via syncSessions on
// the next reconnect cycle and fires firePaneExit for gone sessions.
func TestTmuxServerRegistry_PaneExitChannel(t *testing.T) {
	registry, socket := startIsolatedRegistry(t)

	sessionName := "testpaneexit"
	newSessionWithRetry(t, socket, "-d", "-s", sessionName)

	// Wait until the registry knows about the session so it can detect its removal.
	pollUntil(t, registryPollTimeout, "session not visible in registry before subscribing", func() bool {
		return registry.SessionExists(sessionName)
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	exitCh := registry.SubscribePaneExit(ctx, sessionName)

	if out, err := exec.Command(tmux.Binary(), "-L", socket, "kill-session", "-t", sessionName).CombinedOutput(); err != nil {
		t.Fatalf("kill-session: %v (%s)", err, out)
	}

	select {
	case <-exitCh:
		// channel closed as expected
	case <-time.After(3 * time.Second):
		// reconnectLoop's exponential backoff (100ms, 200ms, 400ms, 800ms, ...) means
		// a control-mode connection that happened to reconnect once or twice right
		// before kill-session can burn most of a 1s budget on the reconnect delay
		// alone, before syncSessions even runs -- observed under heavy concurrent
		// tmux load. 3s comfortably covers a few backoff cycles plus sync time.
		t.Fatal("SubscribePaneExit channel not closed within 3s after kill-session")
	}
}

// Regression test: pane-exit is still detected quickly even while
// reconnectLoop's backoff has grown large. Killing the isolated socket's
// keepalive session makes every attach-session attempt fail near-instantly,
// so backoff doubles every cycle without resetting. The test polls
// reconnectLoop's own hook-reported state (waitForFastRecheckWaitStart)
// rather than IsHealthy(), which stays false throughout and so can't confirm
// the target backoff was reached.
//
// Known gap: backoff is elevated via a clean control-mode outage, so the
// syncMu-contention path (fast-recheck skipped because syncSessions holds
// the lock) isn't exercised here -- verifying it needs access this external
// test package doesn't have (see requirements.md AC6).
func TestTmuxServerRegistry_PaneExitDetectedDespiteElevatedBackoff(t *testing.T) {
	registry, socket := startIsolatedRegistry(t)
	t.Cleanup(func() { registry.SetFastRecheckWaitStartHook(nil) })
	keepaliveName := tmux.TmuxPrefix + "keepalive"

	// sentinelName is never killed, so the isolated server always has at
	// least one session left -- without it, killing both the keepalive and
	// target sessions leaves zero sessions, and tmux's exit-empty behavior
	// tears the whole server down, failing every subsequent list-sessions
	// call instead of returning an empty list.
	sentinelName := "testpaneexit-elevated-backoff-sentinel"
	newSessionWithRetry(t, socket, "-d", "-s", sentinelName, "sleep 300")

	sessionName := "testpaneexit-elevated-backoff"
	newSessionWithRetry(t, socket, "-d", "-s", sessionName)
	pollUntil(t, registryPollTimeout, "session not visible before elevating backoff", func() bool {
		return registry.SessionExists(sessionName)
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	exitCh := registry.SubscribePaneExit(ctx, sessionName)

	// targetBackoff=3200ms (5 doublings: 100->200->400->800->1600->3200)
	// comfortably exceeds the 1.5s detection-assertion window below, so
	// unfixed code (which has no fast-recheck and is bound by backoff alone)
	// would very likely still be waiting when that window's deadline fires
	// -- not just "slower in principle."
	const targetBackoff = 3200 * time.Millisecond

	// Elevate reconnectLoop's backoff: kill the keepalive session so every
	// subsequent attach-session attempt fails near-instantly and backoff
	// climbs every cycle without ever resetting. reconnectLoop takes ~3.1s
	// of doublings (100+200+400+800+1600ms) to reach targetBackoff, so
	// registering the wait via waitForFastRecheckWaitStart after this call
	// still lands well before that wait begins.
	if out, err := exec.Command(tmux.Binary(), "-L", socket, "kill-session", "-t", keepaliveName).CombinedOutput(); err != nil {
		t.Fatalf("kill-session keepalive: %v (%s)", err, out)
	}

	// reconnectLoop just entered its 3200ms backoff wait and is about to run
	// its own fast-recheck attempts -- kill the target session right after
	// this returns so detection is exercised deterministically, rather than
	// estimated, from inside that window.
	waitForFastRecheckWaitStart(t, registry, targetBackoff, registryPollTimeout)

	if out, err := exec.Command(tmux.Binary(), "-L", socket, "kill-session", "-t", sessionName).CombinedOutput(); err != nil {
		t.Fatalf("kill-session %s: %v (%s)", sessionName, err, out)
	}

	select {
	case <-exitCh:
		// detected despite elevated backoff -- the fix is working
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("SubscribePaneExit channel not closed within 1.5s despite elevated backoff -- fast-recheck did not decouple detection from backoff")
	}
}

// Test 4: ListSessions returns the correct set after create/destroy cycles.
func TestTmuxServerRegistry_ListSessions(t *testing.T) {
	registry, socket := startIsolatedRegistry(t)

	for _, name := range []string{"foo", "bar"} {
		newSessionWithRetry(t, socket, "-d", "-s", name)
	}
	t.Cleanup(func() {
		exec.Command(tmux.Binary(), "-L", socket, "kill-session", "-t", "bar").Run() //nolint:errcheck
	})

	// Wait for both sessions to appear.
	pollUntil(t, registryPollTimeout, "sessions 'foo' and 'bar' not both visible within 3s", func() bool {
		sessions := registry.ListSessions()
		return sessions["foo"] && sessions["bar"]
	})

	// Kill "foo" and verify only "bar" remains.
	if out, err := exec.Command(tmux.Binary(), "-L", socket, "kill-session", "-t", "foo").CombinedOutput(); err != nil {
		t.Fatalf("kill-session foo: %v (%s)", err, out)
	}

	pollUntil(t, registryPollTimeout, "'foo' still visible in ListSessions after kill", func() bool {
		sessions := registry.ListSessions()
		return !sessions["foo"] && sessions["bar"]
	})
}

// Test 5: Concurrent subscription stress test — passes under the race detector.
func TestTmuxServerRegistry_ConcurrentSubscriptions(t *testing.T) {
	registry, socket := startIsolatedRegistry(t)

	sessionName := "concurrent-test"
	newSessionWithRetry(t, socket, "-d", "-s", sessionName)

	// Wait until the registry sees the session.
	pollUntil(t, registryPollTimeout, "session not visible before concurrent subscriptions", func() bool {
		return registry.SessionExists(sessionName)
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	const numGoroutines = 10
	channels := make([]<-chan struct{}, numGoroutines)
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			channels[idx] = registry.SubscribePaneExit(ctx, sessionName)
		}(i)
	}
	wg.Wait()

	// Kill the session; all subscriber channels must close.
	if out, err := exec.Command(tmux.Binary(), "-L", socket, "kill-session", "-t", sessionName).CombinedOutput(); err != nil {
		t.Fatalf("kill-session: %v (%s)", err, out)
	}

	timeout := time.After(registryPollTimeout)
	for i, ch := range channels {
		if ch == nil {
			t.Errorf("goroutine %d: channel is nil", i)
			continue
		}
		select {
		case <-ch:
			// closed as expected
		case <-timeout:
			t.Fatalf("goroutine %d: channel not closed within 1s after kill-session", i)
		}
	}
}
