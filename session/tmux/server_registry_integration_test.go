//go:build integration

package tmux_test

import (
	"context"
	"fmt"
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
	socket := fmt.Sprintf("integration_%d_%s_%d", os.Getpid(), t.Name(), rand.Int63())
	// Replace characters that are invalid in tmux socket names.
	safeSocket := ""
	for _, c := range socket {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			safeSocket += string(c)
		} else {
			safeSocket += "_"
		}
	}
	t.Cleanup(func() {
		exec.Command("tmux", "-L", safeSocket, "kill-server").Run() //nolint:errcheck
	})
	return safeSocket
}

// newSessionWithRetry runs `tmux -L socket new-session <args...>`, retrying up
// to 3 times with short backoff. Under heavy concurrent tmux usage (many
// packages each running real tmux servers during `go test ./...`), a fresh
// server can transiently report "server exited unexpectedly" on its first
// session-creation attempt even though the socket is otherwise healthy --
// the same class of transient failure EnsureServerRunning recovers from in
// session/tmux/tmux.go. Fails the test if every attempt fails.
func newSessionWithRetry(t *testing.T, socket string, args ...string) {
	t.Helper()
	fullArgs := append([]string{"-L", socket, "new-session"}, args...)
	const maxAttempts = 3
	var lastErr error
	var lastOut []byte
	for attempt := 0; attempt < maxAttempts; attempt++ {
		out, err := exec.Command("tmux", fullArgs...).CombinedOutput()
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

	return registry, socket
}

// registryPollTimeout is used for every timing assertion in this file that
// depends on the registry's control-mode connection being up and synced.
// reconnectLoop's exponential backoff (100ms, 200ms, 400ms, 800ms, ...) means
// a connection that happens to reconnect once or twice right before an
// assertion can burn most of a 1s budget on the reconnect delay alone, before
// syncSessions even runs -- observed under heavy concurrent tmux load (many
// packages each running real tmux servers during `go test ./...`). 3s
// comfortably covers a few backoff cycles plus sync time.
const registryPollTimeout = 3 * time.Second

// pollUntil polls fn until it returns true or the timeout expires.
// It calls t.Fatal with msg if the timeout is exceeded.
func pollUntil(t *testing.T, timeout time.Duration, msg string, fn func() bool) {
	t.Helper()
	if err := wait.WaitForCondition(fn, wait.WaitConfig{Timeout: timeout, PollInterval: 5 * time.Millisecond, Description: msg}); err != nil {
		t.Fatal(msg)
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
		exec.Command("tmux", "-L", socket, "kill-session", "-t", sessionName).Run() //nolint:errcheck
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

	if out, err := exec.Command("tmux", "-L", socket, "kill-session", "-t", sessionName).CombinedOutput(); err != nil {
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

// Test 4: ListSessions returns the correct set after create/destroy cycles.
func TestTmuxServerRegistry_ListSessions(t *testing.T) {
	registry, socket := startIsolatedRegistry(t)

	for _, name := range []string{"foo", "bar"} {
		newSessionWithRetry(t, socket, "-d", "-s", name)
	}
	t.Cleanup(func() {
		exec.Command("tmux", "-L", socket, "kill-session", "-t", "bar").Run() //nolint:errcheck
	})

	// Wait for both sessions to appear.
	pollUntil(t, registryPollTimeout, "sessions 'foo' and 'bar' not both visible within 3s", func() bool {
		sessions := registry.ListSessions()
		return sessions["foo"] && sessions["bar"]
	})

	// Kill "foo" and verify only "bar" remains.
	if out, err := exec.Command("tmux", "-L", socket, "kill-session", "-t", "foo").CombinedOutput(); err != nil {
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
	if out, err := exec.Command("tmux", "-L", socket, "kill-session", "-t", sessionName).CombinedOutput(); err != nil {
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
