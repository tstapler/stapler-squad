package tmux

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/executor"
	"github.com/tstapler/stapler-squad/executor/safeexec"
)

type MockPtyFactory struct {
	t *testing.T

	// Array of commands and the corresponding file handles representing PTYs.
	cmds  []*exec.Cmd
	files []*os.File
}

func (pt *MockPtyFactory) Start(cmd *exec.Cmd) (*os.File, *exec.Cmd, error) {
	// Use a safe test name for the file path - replace problematic characters
	safeName := strings.ReplaceAll(pt.t.Name(), "/", "_")
	safeName = strings.ReplaceAll(safeName, " ", "_")
	filePath := filepath.Join(pt.t.TempDir(), fmt.Sprintf("pty-%s-%d", safeName, rand.Int31()))
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR, 0644)
	if err == nil {
		pt.cmds = append(pt.cmds, cmd)
		pt.files = append(pt.files, f)
	}
	return f, cmd, err
}

func (pt *MockPtyFactory) StartWithSize(cmd *exec.Cmd, _ *pty.Winsize) (*os.File, *exec.Cmd, error) {
	return pt.Start(cmd)
}

func (pt *MockPtyFactory) Close() {}

func NewMockPtyFactory(t *testing.T) *MockPtyFactory {
	return &MockPtyFactory{
		t: t,
	}
}

func TestSanitizeName(t *testing.T) {
	t.Parallel()
	session := NewTmuxSession("asdf", "program")
	require.Equal(t, TmuxPrefix+"asdf", session.sanitizedName)

	session = NewTmuxSession("a sd f . . asdf", "program")
	require.Equal(t, TmuxPrefix+"asdf__asdf", session.sanitizedName)

	// Test colon sanitization - colons are special in tmux (session:window.pane)
	session = NewTmuxSession("Resumed: test-session", "program")
	require.Equal(t, TmuxPrefix+"Resumed_test-session", session.sanitizedName)

	// Test combined special characters
	session = NewTmuxSession("My: Session. Name", "program")
	require.Equal(t, TmuxPrefix+"My_Session_Name", session.sanitizedName)
}

func TestStartTmuxSession(t *testing.T) {
	t.Parallel()
	ptyFactory := NewMockPtyFactory(t)

	created := false
	cmdExec := MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "has-session") && !created {
				created = true
				return fmt.Errorf("session already exists")
			}
			if strings.Contains(cmd.String(), "new-session") {
				created = true
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			// Handle DoesSessionExist() polling which uses list-sessions
			if strings.Contains(cmd.String(), "list-sessions") && strings.Contains(cmd.String(), "#{session_name}") {
				if created {
					return []byte("staplersquad_test-session"), nil
				} else {
					return nil, fmt.Errorf("no server running")
				}
			}
			return []byte("output"), nil
		},
	}

	workdir := t.TempDir()
	// WithRegistry(nil) prevents DoesSessionExist() from using the global real-tmux
	// registry fast path — ensures the mock executor is used for session polling.
	session := newTmuxSessionWithSocket("test-session", "echo", ptyFactory, cmdExec, TmuxPrefix, "", WithRegistry(nil))

	err := session.Start(workdir)
	require.NoError(t, err)

	// Verify the session was marked as created (behavioral test)
	require.True(t, created, "Session should be marked as created after Start()")

	// The current implementation may not use PTY factories the same way,
	// so we focus on testing the behavioral contract rather than implementation details
}

func TestStartTmuxSession_IncludesTmuxStderrOnFailure(t *testing.T) {
	t.Parallel()
	ptyFactory := NewMockPtyFactory(t)

	const wantStderr = "tmux: unrecognized option '-e'"
	cmdExec := MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if strings.Contains(cmd.String(), "new-session") {
				if cmd.Stderr != nil {
					_, _ = cmd.Stderr.Write([]byte(wantStderr))
				}
				return fmt.Errorf("exit status 1")
			}
			return nil
		},
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			if strings.Contains(cmd.String(), "list-sessions") {
				return nil, fmt.Errorf("no server running")
			}
			return []byte("output"), nil
		},
	}

	workdir := t.TempDir()
	session := newTmuxSessionWithSocket("test-session-fail", "echo", ptyFactory, cmdExec, TmuxPrefix, "", WithRegistry(nil))

	err := session.Start(workdir)
	require.Error(t, err)
	require.Contains(t, err.Error(), wantStderr)
}

// --- serverNotRunning detection tests ---

func TestServerNotRunning(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		output   []byte
		expected bool
	}{
		{
			name:     "exact 'no server running' phrase",
			output:   []byte("no server running"),
			expected: true,
		},
		{
			name:     "uppercase variant",
			output:   []byte("No Server Running on /tmp/tmux-501/default"),
			expected: true,
		},
		{
			name:     "error connecting to variant",
			output:   []byte("error connecting to /tmp/tmux-501/default"),
			expected: true,
		},
		{
			name:     "normal session list output",
			output:   []byte("my-session: 1 windows (created Mon Jan  1 00:00:00 2025) [200x50]"),
			expected: false,
		},
		{
			name:     "empty output",
			output:   []byte(""),
			expected: false,
		},
		{
			name:     "stale socket: server exited unexpectedly",
			output:   []byte("server exited unexpectedly"),
			expected: true,
		},
		{
			name:     "stale socket: uppercase variant",
			output:   []byte("Server Exited Unexpectedly"),
			expected: true,
		},
		{
			name:     "unrelated error message",
			output:   []byte("session not found: my-session"),
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := serverNotRunning(tc.output)
			require.Equal(t, tc.expected, result)
		})
	}
}

// --- tmuxCircuitBreakerConfig IsFailure classifier tests ---

func TestTmuxCircuitBreakerConfig(t *testing.T) {
	t.Parallel()
	cfg := tmuxCircuitBreakerConfig()
	someErr := fmt.Errorf("exit status 1")

	t.Run("nil_error_is_never_a_failure", func(t *testing.T) {
		require.False(t, cfg.IsFailure("tmux-list-sessions", nil, nil))
		require.False(t, cfg.IsFailure("tmux-has-session", nil, nil))
	})

	t.Run("list_sessions_no_server_running_is_failure", func(t *testing.T) {
		output := []byte("no server running")
		require.True(t, cfg.IsFailure("tmux-list-sessions", output, someErr),
			"list-sessions with 'no server running' output should be a circuit breaker failure")
	})

	t.Run("list_sessions_error_connecting_is_failure", func(t *testing.T) {
		output := []byte("error connecting to /tmp/tmux-501/default")
		require.True(t, cfg.IsFailure("tmux-list-sessions", output, someErr),
			"list-sessions with 'error connecting to' output should be a circuit breaker failure")
	})

	t.Run("list_sessions_empty_output_is_not_failure", func(t *testing.T) {
		// Exit 1 with empty output means the server is running but has no sessions.
		// This is a normal transient state during session creation, not a real failure.
		require.False(t, cfg.IsFailure("tmux-list-sessions", []byte(""), someErr),
			"list-sessions with empty output (no sessions) should NOT trip the circuit breaker")
	})

	t.Run("non_list_sessions_server_down_is_failure", func(t *testing.T) {
		// Server-down output trips the breaker for any command class.
		serverDown := []byte("no server running on /tmp/tmux-501/default")
		require.True(t, cfg.IsFailure("tmux-has-session", serverDown, someErr))
		require.True(t, cfg.IsFailure("tmux-new-session", serverDown, someErr))
		require.True(t, cfg.IsFailure("tmux-capture-pane", serverDown, someErr))
	})

	t.Run("non_list_sessions_per_target_errors_not_failure", func(t *testing.T) {
		// Per-target errors (pane/session not found) must NOT trip the breaker —
		// the server is healthy; only that specific session/pane is missing.
		require.False(t, cfg.IsFailure("tmux-capture-pane", []byte("can't find pane: staplersquad_mysession"), someErr))
		require.False(t, cfg.IsFailure("tmux-display-message", []byte("session not found: staplersquad_mysession"), someErr))
		require.False(t, cfg.IsFailure("tmux-has-session", nil, someErr))
		require.False(t, cfg.IsFailure("tmux-new-session", []byte(""), someErr))
	})
}

// --- Package-level tmux server function integration tests ---

// createSessionWithRetry creates a detached tmux session on serverSocket, retrying
// with backoff if the attempt fails or is killed by its own per-attempt timeout.
//
// Root cause this works around: `tmux new-session` forks and execs the real tmux
// server process, and on a loaded machine that fork/exec is measurably slow and
// highly variable -- a manual timing check in this environment found single
// invocations routinely taking 1.5-4s versus the sub-100ms typical on an idle
// box (see also the sessionCreateTimeout doc comment in tmux.go, which documents
// the same class of slowdown for a different fixed-budget call). A single
// un-retried attempt bounded by a fixed context timeout intermittently gets
// killed under exactly this load, which is what made
// TestEnsureServerRunning_NoOp flaky: the timeout wasn't guarding a logic race,
// it was too tight for this environment's real subprocess latency. This mirrors
// ensureServerRunningWithRetry in tmux.go, which fixes the identical failure
// shape (a real-tmux-subprocess call killed by a fixed timeout under load) in
// production code -- attempts/backoff values match serverStartAttempts/
// serverStartBackoffStart/serverStartBackoffMax there.
func createSessionWithRetry(t *testing.T, serverSocket, sessionName string, extraNewSessionArgs ...string) {
	t.Helper()
	hasArgs := prependSocket(serverSocket, []string{"has-session", "-t", sessionName})
	newArgs := prependSocket(serverSocket, append([]string{"new-session", "-d", "-s", sessionName}, extraNewSessionArgs...))

	const attempts = 8
	backoff := 100 * time.Millisecond
	const backoffMax = 3 * time.Second

	var lastErr error
	for i := 0; i < attempts; i++ {
		hasCtx, hasCancel := context.WithTimeout(context.Background(), 5*time.Second)
		hasErr := safeexec.CommandContext(hasCtx, Binary(), hasArgs...).Run()
		hasCancel()
		if hasErr == nil {
			return // a previous attempt's tmux process actually succeeded despite the local error
		}

		newCtx, newCancel := context.WithTimeout(context.Background(), 10*time.Second)
		lastErr = safeexec.CommandContext(newCtx, Binary(), newArgs...).Run()
		newCancel()
		if lastErr == nil {
			return
		}
		if i < attempts-1 {
			time.Sleep(backoff)
			backoff *= 2
			if backoff > backoffMax {
				backoff = backoffMax
			}
		}
	}
	require.NoError(t, lastErr, "failed to create tmux session %q on socket %q after %d attempts", sessionName, serverSocket, attempts)
}

// TestEnsureServerRunning_NoOp verifies that EnsureServerRunning is a no-op
// when the tmux server is already running.
func TestEnsureServerRunning_NoOp(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available")
	}
	socketName := fmt.Sprintf("test_ensure_noop_%d_%d", os.Getpid(), rand.Int63())
	t.Cleanup(func() {
		killCtx, killCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer killCancel()
		_ = safeexec.CommandContext(killCtx, Binary(), "-L", socketName, "kill-server").Run()
	})

	// Start the isolated server and keep it alive with a detached session.
	// Without a session, tmux exits immediately (exit-empty=on by default), causing
	// the follow-up check in EnsureServerRunning to falsely report the server as dead.
	createSessionWithRetry(t, socketName, "keepalive")

	// With the server running and a live session, EnsureServerRunning should be a no-op.
	_, err := EnsureServerRunning(socketName)
	require.NoError(t, err, "EnsureServerRunning should be a no-op when server is already running")
}

// TestEnsureServerRunning_StartsServer verifies that EnsureServerRunning actually
// starts the tmux server when it is not running.
func TestEnsureServerRunning_StartsServer(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real tmux test in short mode")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available")
	}
	socketName := fmt.Sprintf("test_ensure_start_%d_%d", os.Getpid(), rand.Int63())
	t.Cleanup(func() {
		killCtx2, killCancel2 := context.WithTimeout(context.Background(), 10*time.Second)
		defer killCancel2()
		_ = safeexec.CommandContext(killCtx2, Binary(), "-L", socketName, "kill-server").Run()
	})

	// Confirm no server is running on this socket yet.
	require.True(t, checkServerNotRunning(socketName),
		"server should not be running before the test starts")

	// EnsureServerRunning should start the server.
	_, err := EnsureServerRunning(socketName)
	require.NoError(t, err)

	// On macOS, tmux's default exit-empty=on causes the server to exit immediately
	// when there are no sessions. Create a session to keep the server alive and
	// verify the server is functional by confirming the session can be created.
	createCmd := safeexec.CommandContext(context.Background(), Binary(), "-L", socketName, "new-session", "-d", "-s", "verify-alive")
	require.NoError(t, createCmd.Run(),
		"should be able to create a session on the newly started server — server must be running")
}

// TestStartServerSucceededDespiteError covers the flaky-under-load scenario behind
// TestEnsureServerRunning_NoOp's original failure mode: under heavy concurrent tmux
// usage, checkServerNotRunning's list-sessions call can itself transiently report
// "server exited unexpectedly" against a socket that actually has a live server,
// which sends EnsureServerRunning down the start-server path even though a server
// is already running -- and that start-server call then hits the same transient
// failure. Rather than reproduce that real timing race (system-load dependent,
// not deterministic), this tests the recovery decision in isolation via an
// injected checker.
func TestStartServerSucceededDespiteError(t *testing.T) {
	t.Parallel()
	t.Run("recovers when a recheck shows the server is actually running", func(t *testing.T) {
		got := startServerSucceededDespiteError(func() bool { return false }) // false = is running
		require.True(t, got, "a start-server error should be swallowed when the server is actually up")
	})

	t.Run("does not recover when the server genuinely is not running", func(t *testing.T) {
		got := startServerSucceededDespiteError(func() bool { return true }) // true = not running
		require.False(t, got, "a start-server error must still surface when the server really isn't running")
	})
}

// TestServerStartAttempt covers the single-attempt decision: did start-server
// actually succeed, or did it fail but a recheck shows the server running
// anyway (the check-race startServerSucceededDespiteError recovers from)?
func TestServerStartAttempt(t *testing.T) {
	t.Parallel()
	t.Run("succeeds when start-server itself succeeds", func(t *testing.T) {
		calls := 0
		startServer := func() ([]byte, error) {
			calls++
			return []byte("ok"), nil
		}
		isNotRunning := func() bool { t.Fatal("should not need a recheck when start-server succeeded"); return true }
		ok, out, err := serverStartAttempt(startServer, isNotRunning)
		require.True(t, ok)
		require.NoError(t, err)
		require.Equal(t, []byte("ok"), out)
		require.Equal(t, 1, calls)
	})

	t.Run("recovers when start-server errors but a recheck shows the server running", func(t *testing.T) {
		startServer := func() ([]byte, error) { return []byte("server exited unexpectedly"), errors.New("exit status 1") }
		isNotRunning := func() bool { return false } // running
		ok, _, err := serverStartAttempt(startServer, isNotRunning)
		require.True(t, ok, "a start-server error should be swallowed when the server is actually up")
		require.Error(t, err, "the underlying error is still returned for logging even on recovery")
	})

	t.Run("fails when start-server errors and the server genuinely is not running", func(t *testing.T) {
		startServer := func() ([]byte, error) { return nil, errors.New("exit status 1") }
		isNotRunning := func() bool { return true } // not running
		ok, _, err := serverStartAttempt(startServer, isNotRunning)
		require.False(t, ok)
		require.Error(t, err)
	})
}

// TestEnsureServerRunningWithRetry covers the retry wrapper added around
// serverStartAttempt to survive start-server failures (transient check-races
// or genuine start failures) that outlast a single attempt under sustained
// heavy system load -- see TestEnsureServerRunning_NoOp's original failure
// and the doc comment above ensureServerRunningWithRetry in tmux.go.
func TestEnsureServerRunningWithRetry(t *testing.T) {
	t.Parallel()
	t.Run("succeeds on the first attempt", func(t *testing.T) {
		calls := 0
		startServer := func() ([]byte, error) { calls++; return nil, nil }
		isNotRunning := func() bool { return true }
		_, err := ensureServerRunningWithRetry(startServer, isNotRunning, 5, time.Millisecond, 4*time.Millisecond)
		require.NoError(t, err)
		require.Equal(t, 1, calls, "should not retry once the first attempt succeeds")
	})

	t.Run("recovers after a couple of failed start-server calls", func(t *testing.T) {
		calls := 0
		startServer := func() ([]byte, error) {
			calls++
			if calls < 3 {
				return nil, errors.New("exit status 1")
			}
			return nil, nil
		}
		isNotRunning := func() bool { return true } // never recovers via recheck alone
		_, err := ensureServerRunningWithRetry(startServer, isNotRunning, 5, time.Millisecond, 4*time.Millisecond)
		require.NoError(t, err, "should recover once a later attempt actually starts the server")
		require.Equal(t, 3, calls)
	})

	t.Run("gives up once the attempt budget is exhausted", func(t *testing.T) {
		calls := 0
		startServer := func() ([]byte, error) {
			calls++
			return []byte("server exited unexpectedly"), errors.New("exit status 1")
		}
		isNotRunning := func() bool { return true } // never running
		_, err := ensureServerRunningWithRetry(startServer, isNotRunning, 3, time.Millisecond, 4*time.Millisecond)
		require.Error(t, err, "must still surface the error if the server genuinely never comes up")
		require.Equal(t, 3, calls, "should attempt exactly the configured number of times")
	})
}

// TestCreateKeepaliveSession verifies that a keepalive session is created and
// that calling it again is idempotent.
func TestCreateKeepaliveSession(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real tmux test in short mode")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available")
	}
	socketName := fmt.Sprintf("test_keepalive_%d_%d", os.Getpid(), rand.Int63())
	t.Cleanup(func() {
		_ = safeexec.CommandContext(context.Background(), Binary(), "-L", socketName, "kill-server").Run()
	})

	// Start the server with an anchor session to keep it alive while we test.
	// (new-session -d is equivalent to start-server + create session atomically)
	require.NoError(t, safeexec.CommandContext(context.Background(), Binary(), "-L", socketName, "new-session", "-d", "-s", "anchor").Run())

	// Create keepalive session.
	err := CreateKeepaliveSession(socketName)
	require.NoError(t, err, "CreateKeepaliveSession should succeed")

	// Verify the keepalive session exists.
	keepaliveName := TmuxPrefix + "keepalive"
	out, err := safeexec.CommandContext(context.Background(), Binary(), "-L", socketName, "has-session", "-t", keepaliveName).CombinedOutput()
	require.NoError(t, err, "keepalive session should exist after CreateKeepaliveSession; output: %s", out)

	// Calling it again should be idempotent (no error).
	err = CreateKeepaliveSession(socketName)
	require.NoError(t, err, "CreateKeepaliveSession should be idempotent")
}

// TestSetExitEmpty verifies that SetExitEmpty sets the tmux server option correctly.
func TestSetExitEmpty(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real tmux test in short mode")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available")
	}
	socketName := fmt.Sprintf("test_exit_empty_%d_%d", os.Getpid(), rand.Int63())
	t.Cleanup(func() {
		_ = safeexec.CommandContext(context.Background(), Binary(), "-L", socketName, "kill-server").Run()
	})

	// Start the server WITH a detached session to prevent the server from exiting
	// immediately due to exit-empty=on (the default). Using new-session -d starts
	// both the server and an anchor session in one step.
	require.NoError(t, safeexec.CommandContext(context.Background(), Binary(), "-L", socketName, "new-session", "-d", "-s", "anchor").Run())

	// Set exit-empty off.
	err := SetExitEmpty(socketName, false)
	require.NoError(t, err, "SetExitEmpty(false) should succeed")

	// Verify the option was set.
	out, err := safeexec.CommandContext(context.Background(), Binary(), "-L", socketName, "show-options", "-g", "exit-empty").CombinedOutput()
	require.NoError(t, err)
	require.Contains(t, strings.ToLower(string(out)), "off",
		"exit-empty should be off after SetExitEmpty(false)")

	// Set exit-empty on.
	err = SetExitEmpty(socketName, true)
	require.NoError(t, err, "SetExitEmpty(true) should succeed")

	out, err = safeexec.CommandContext(context.Background(), Binary(), "-L", socketName, "show-options", "-g", "exit-empty").CombinedOutput()
	require.NoError(t, err)
	require.Contains(t, strings.ToLower(string(out)), "on",
		"exit-empty should be on after SetExitEmpty(true)")
}

// --- recoverFromServerFailure correctness tests ---

// TestRecoverFromServerFailure_ConcurrentGuard verifies that when recoveryInFlight is
// already true, concurrent callers of recoverFromServerFailure return immediately
// without attempting another recovery. This tests the recoveryMu + recoveryInFlight
// guard that prevents N sessions from all calling EnsureServerRunning simultaneously
// when the tmux server goes down.
func TestRecoverFromServerFailure_ConcurrentGuard(t *testing.T) {
	// Pre-set recoveryInFlight = true to simulate a recovery already running.
	recoveryMu.Lock()
	recoveryInFlight = true
	recoveryMu.Unlock()
	t.Cleanup(func() {
		recoveryMu.Lock()
		recoveryInFlight = false
		recoveryMu.Unlock()
	})

	ptyFactory := NewMockPtyFactory(t)
	cmdExec := MockCmdExec{
		RunFunc:            func(cmd *exec.Cmd) error { return nil },
		OutputFunc:         func(cmd *exec.Cmd) ([]byte, error) { return []byte(""), nil },
		CombinedOutputFunc: func(cmd *exec.Cmd) ([]byte, error) { return []byte(""), nil },
	}
	session := newTmuxSession("guard-test", "echo", ptyFactory, cmdExec, TmuxPrefix)

	// recoverFromServerFailure should detect recoveryInFlight=true and return immediately
	// without calling EnsureServerRunning (which would try to exec real tmux).
	const numGoroutines = 5
	var wg sync.WaitGroup
	done := make(chan struct{}, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			recoverFromServerFailure(session.serverSocket, "TestRecoverFromServerFailure_ConcurrentGuard")
			done <- struct{}{}
		}()
	}

	// All goroutines should finish quickly since the guard should short-circuit them.
	completedInTime := make(chan struct{})
	go func() {
		wg.Wait()
		close(completedInTime)
	}()

	select {
	case <-completedInTime:
		// All goroutines returned without blocking — guard is working.
	case <-time.After(500 * time.Millisecond):
		t.Fatal("recoverFromServerFailure did not return quickly with recoveryInFlight=true; possible deadlock or missing guard")
	}

	require.Equal(t, numGoroutines, len(done),
		"all goroutines should have completed and sent to done channel")
}

// TestDoesSessionExist_LockReleasedBeforeRecovery verifies that DoesSessionExist
// releases existsCacheMutex before calling recoverFromServerFailure. If the mutex
// were still held during recovery, a subsequent DoesSessionExist call (which tries
// to acquire the write lock) would deadlock.
func TestDoesSessionExist_LockReleasedBeforeRecovery(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)
	cmdExec := MockCmdExec{
		CombinedOutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			if strings.Contains(cmd.String(), "list-sessions") {
				// Simulate the tmux server being down so recovery is triggered.
				return []byte("no server running"), fmt.Errorf("exit status 1")
			}
			return []byte(""), nil
		},
		RunFunc:    func(cmd *exec.Cmd) error { return nil },
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) { return []byte(""), nil },
	}
	// Use serverSocket="" so needsRecovery=true is set in DoesSessionExist, exercising
	// the unlock-before-recovery code path.
	session := newTmuxSession("lock-test", "echo", ptyFactory, cmdExec, TmuxPrefix)

	// First call: should detect "no server running", release existsCacheMutex,
	// attempt recovery (which calls real tmux — it will fail, but quickly), and
	// return false.
	result := session.DoesSessionExist()
	require.False(t, result, "DoesSessionExist should return false when server is not running")

	// Second call in a goroutine: if existsCacheMutex were still held from the first
	// call's recovery phase, this goroutine would deadlock indefinitely.
	done := make(chan bool, 1)
	go func() {
		// Invalidate the cache so the second call re-executes the check.
		session.invalidateExistsCache()
		done <- session.DoesSessionExist()
	}()

	select {
	case result2 := <-done:
		// Lock was released correctly — second call proceeded without blocking.
		require.False(t, result2, "second DoesSessionExist call should also return false")
	case <-time.After(2 * time.Second):
		t.Fatal("DoesSessionExist deadlocked on second call — existsCacheMutex was not released before recovery ran")
	}
}

// TestPrependSocket verifies that prependSocket prepends "-L <socket>" for an
// explicit socket, and — since this runs inside a `go test` binary — also
// prepends the per-process isolated socket even when called with an empty
// socket, via ResolveSocket. This is the regression guard for the incident
// where an unguarded raw tmux call with no socket argument enumerated and
// killed sessions on another, currently-running stapler-squad process's
// shared default socket. See ResolveSocket's doc comment for the incident.
func TestPrependSocket(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		socket   string
		args     []string
		expected []string
	}{
		{
			name:     "empty socket resolves to the isolated test socket, not the shared default",
			socket:   "",
			args:     []string{"list-sessions"},
			expected: []string{"-L", testSocketOnce(), "list-sessions"},
		},
		{
			name:     "non-empty socket prepends -L flag",
			socket:   "test-socket",
			args:     []string{"list-sessions"},
			expected: []string{"-L", "test-socket", "list-sessions"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := prependSocket(tc.socket, tc.args)
			require.Equal(t, tc.expected, result)
		})
	}
}

// TestResolveSocket verifies the canonical socket-resolution choke point:
// an explicit socket always passes through unchanged (production callers
// that already isolate explicitly must not be double-prefixed), while an
// empty socket resolves to a stable, non-empty per-process value whenever
// running inside a `go test` binary — it must never resolve to "" and fall
// through to the real shared default socket.
func TestResolveSocket(t *testing.T) {
	t.Parallel()
	t.Run("explicit socket passes through unchanged", func(t *testing.T) {
		require.Equal(t, Socket("explicit-socket"), ResolveSocket("explicit-socket"))
	})

	t.Run("empty socket resolves to a non-empty isolated value in test mode", func(t *testing.T) {
		resolved := ResolveSocket("")
		require.NotEmpty(t, resolved.String(), "empty socket must never resolve to the shared default inside a test binary")
		require.Contains(t, resolved.String(), "test-isolated-")
	})

	t.Run("repeated empty-socket calls return the same isolated value", func(t *testing.T) {
		first := ResolveSocket("")
		second := ResolveSocket("")
		require.Equal(t, first, second, "ResolveSocket must be stable across calls within the same process")
	})
}

// TestSocket_Args verifies the smart constructor's sole args-building method:
// the default (zero-value) Socket leaves args untouched, and a non-default
// Socket prepends "-L <socket>".
func TestSocket_Args(t *testing.T) {
	t.Parallel()
	t.Run("default socket returns args unchanged", func(t *testing.T) {
		var s Socket
		require.Equal(t, []string{"list-sessions"}, s.Args("list-sessions"))
	})

	t.Run("non-default socket prepends -L flag", func(t *testing.T) {
		s := Socket("my-socket")
		require.Equal(t, []string{"-L", "my-socket", "list-sessions"}, s.Args("list-sessions"))
	})

	t.Run("String returns the underlying socket name", func(t *testing.T) {
		require.Equal(t, "my-socket", Socket("my-socket").String())
		require.Equal(t, "", Socket("").String())
	})
}

// TestSetServerRecoveryCallback verifies that the callback registered via
// SetServerRecoveryCallback is called after a successful server recovery.
func TestSetServerRecoveryCallback(t *testing.T) {
	// Restore original callback after test.
	orig := onServerRecovered
	t.Cleanup(func() { onServerRecovered = orig })

	called := make(chan struct{}, 1)
	SetServerRecoveryCallback(func() { called <- struct{}{} })

	// Inject a succeeding ensureServerRunning so recoverFromServerFailure
	// takes the success branch and fires the callback.
	origEnsure := ensureServerRunning
	ensureServerRunning = func(_ string) (TmuxServerReady, error) { return TmuxServerReady{}, nil }
	t.Cleanup(func() { ensureServerRunning = origEnsure })

	// Ensure recoveryInFlight is clean before and after.
	recoveryMu.Lock()
	require.False(t, recoveryInFlight, "test isolation: recoveryInFlight must be false at start")
	recoveryMu.Unlock()
	t.Cleanup(func() {
		recoveryMu.Lock()
		recoveryInFlight = false
		recoveryMu.Unlock()
	})

	recoverFromServerFailure("", "TestSetServerRecoveryCallback")

	select {
	case <-called:
		// callback fired as expected
	case <-time.After(500 * time.Millisecond):
		t.Fatal("recovery callback was not called after successful recovery")
	}
}

// TestRegistryKeyUnregisteredOnClose verifies that Close() unregisters the session's
// circuit breaker executor from the global registry. This prevents stale entries from
// accumulating in ResetAll() calls across long-lived processes.
func TestRegistryKeyUnregisteredOnClose(t *testing.T) {
	t.Parallel()
	ptyFactory := NewMockPtyFactory(t)

	// Build a mock cmdExec that makes DoesSessionExist return false (no kill-session needed).
	cmdExec := MockCmdExec{
		CombinedOutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte(""), nil // empty sessions list → session doesn't exist → skip kill
		},
		RunFunc:    func(cmd *exec.Cmd) error { return nil },
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) { return []byte(""), nil },
	}

	session := newTmuxSession("reg-close-test", "echo", ptyFactory, cmdExec, TmuxPrefix)

	// Register a CircuitBreakerExecutor in the global registry under a unique key,
	// mirroring what NewTmuxSession does at construction time.
	key := "tmux-reg-close-test-" + t.Name()
	session.registryKey = key

	// Use a failing delegate so we can trip the breaker and confirm its presence via AllBreakers.
	failingDelegate := MockCmdExec{
		RunFunc:    func(cmd *exec.Cmd) error { return fmt.Errorf("simulated failure") },
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) { return nil, fmt.Errorf("simulated failure") },
	}
	cbExec := executor.NewCircuitBreakerExecutor(failingDelegate, executor.CircuitBreakerConfig{
		FailureThreshold: 1,
		RecoveryTimeout:  30 * time.Second,
	})
	executor.GetGlobalRegistry().Register(key, cbExec)
	t.Cleanup(func() {
		// Defensive cleanup in case Close() doesn't run.
		executor.GetGlobalRegistry().Unregister(key)
	})

	// Trip the breaker so AllBreakers returns a non-empty snapshot for this executor.
	//nolint:norawexec executor-mediated; TimeoutExecutor wraps and sets WaitDelay internally
	_ = cbExec.Run(exec.CommandContext(context.Background(), "tmux", "list-sessions"))
	breakersBefore := executor.GetGlobalRegistry().AllBreakers()
	found := false
	for k := range breakersBefore {
		if strings.HasPrefix(k, key+"/") {
			found = true
			break
		}
	}
	require.True(t, found, "registry should contain executor key %q before Close()", key)

	// Close() should call GetGlobalRegistry().Unregister(registryKey).
	err := session.Close()
	require.NoError(t, err)

	// Verify the key is absent after Close().
	breakersAfter := executor.GetGlobalRegistry().AllBreakers()
	for k := range breakersAfter {
		require.False(t, strings.HasPrefix(k, key+"/"),
			"registry should not contain executor key %q after Close()", key)
	}
}

// --- GetPaneCurrentPath tests ---

func TestGetPaneCurrentPath_ReturnsTrimmedPath(t *testing.T) {
	t.Parallel()
	cmdExec := MockCmdExec{
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			if strings.Contains(cmd.String(), "pane_current_path") {
				return []byte("/home/user/project\n"), nil
			}
			return []byte(""), nil
		},
		RunFunc:            func(cmd *exec.Cmd) error { return nil },
		CombinedOutputFunc: func(cmd *exec.Cmd) ([]byte, error) { return []byte(""), nil },
	}
	session := newTmuxSession("capture-test", "echo", NewMockPtyFactory(t), cmdExec, TmuxPrefix)

	path, err := session.GetPaneCurrentPath()

	require.NoError(t, err)
	// Trailing newline is trimmed.
	require.Equal(t, "/home/user/project", path)
}

func TestGetPaneCurrentPath_ReturnsError(t *testing.T) {
	t.Parallel()
	cmdExec := MockCmdExec{
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return nil, fmt.Errorf("tmux server not running")
		},
		RunFunc:            func(cmd *exec.Cmd) error { return nil },
		CombinedOutputFunc: func(cmd *exec.Cmd) ([]byte, error) { return []byte(""), nil },
	}
	session := newTmuxSession("capture-err-test", "echo", NewMockPtyFactory(t), cmdExec, TmuxPrefix)

	path, err := session.GetPaneCurrentPath()

	require.Error(t, err)
	require.Empty(t, path)
	require.Contains(t, err.Error(), "failed to get pane path")
}

// --- T3: DoesSessionExist registry integration tests ---

// TestDoesSessionExist_UsesRegistry verifies that when the registry is healthy,
// DoesSessionExist returns the registry answer without executing any tmux subprocess.
func TestDoesSessionExist_UsesRegistry(t *testing.T) {
	t.Parallel()
	forkCount := 0
	cmdExec := MockCmdExec{
		CombinedOutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			forkCount++ // Any exec call increments this.
			return []byte(""), nil
		},
		RunFunc:    func(cmd *exec.Cmd) error { forkCount++; return nil },
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) { forkCount++; return []byte(""), nil },
	}

	reg := NewFakeTmuxRegistry()
	reg.SetHealthy(true)
	reg.SetSessions([]string{TmuxPrefix + "reg-test"})

	session := newTmuxSessionWithSocket("reg-test", "echo", NewMockPtyFactory(t), cmdExec, TmuxPrefix, "", WithRegistry(reg))

	result := session.DoesSessionExist()

	require.True(t, result, "DoesSessionExist should return true from healthy registry")
	require.Equal(t, 0, forkCount, "no exec forks should occur when registry is healthy")
}

// TestDoesSessionExist_FallsBackWhenRegistryUnhealthy verifies that when the
// registry reports unhealthy, DoesSessionExist falls back to the exec path.
func TestDoesSessionExist_FallsBackWhenRegistryUnhealthy(t *testing.T) {
	t.Parallel()
	execCalled := false
	cmdExec := MockCmdExec{
		CombinedOutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			if strings.Contains(cmd.String(), "list-sessions") {
				execCalled = true
				return []byte(TmuxPrefix + "fallback-test"), nil
			}
			return []byte(""), nil
		},
		RunFunc:    func(cmd *exec.Cmd) error { return nil },
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) { return []byte(""), nil },
	}

	reg := NewFakeTmuxRegistry()
	reg.SetHealthy(false) // Registry is unhealthy — must fall back.

	session := newTmuxSessionWithSocket("fallback-test", "echo", NewMockPtyFactory(t), cmdExec, TmuxPrefix, "", WithRegistry(reg))

	// The exec fallback returns the session name in the list-sessions output.
	result := session.DoesSessionExist()

	require.True(t, result, "DoesSessionExist should return true from exec fallback")
	require.True(t, execCalled, "exec list-sessions should be called when registry is unhealthy")
}

// TestDoesSessionExist_NilRegistry verifies that a nil registry falls back to exec.
func TestDoesSessionExist_NilRegistry(t *testing.T) {
	t.Parallel()
	execCalled := false
	cmdExec := MockCmdExec{
		CombinedOutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			if strings.Contains(cmd.String(), "list-sessions") {
				execCalled = true
				return []byte(TmuxPrefix + "nil-reg-test"), nil
			}
			return []byte(""), nil
		},
		RunFunc:    func(cmd *exec.Cmd) error { return nil },
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) { return []byte(""), nil },
	}

	session := newTmuxSessionWithSocket("nil-reg-test", "echo", NewMockPtyFactory(t), cmdExec, TmuxPrefix, "", WithRegistry(nil))

	result := session.DoesSessionExist()

	require.True(t, result)
	require.True(t, execCalled, "exec list-sessions should be called when registry is nil")
}

// TestDoesSessionExistNoCache_UsesRegistry verifies that when the registry is
// healthy and confirms the session, DoesSessionExistNoCache returns true
// without executing any tmux subprocess.
func TestDoesSessionExistNoCache_UsesRegistry(t *testing.T) {
	t.Parallel()
	forkCount := 0
	cmdExec := MockCmdExec{
		CombinedOutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			forkCount++
			return []byte(""), nil
		},
		RunFunc:    func(cmd *exec.Cmd) error { forkCount++; return nil },
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) { forkCount++; return []byte(""), nil },
	}

	reg := NewFakeTmuxRegistry()
	reg.SetHealthy(true)
	reg.SetSessions([]string{TmuxPrefix + "reg-nocache-test"})

	session := newTmuxSessionWithSocket("reg-nocache-test", "echo", NewMockPtyFactory(t), cmdExec, TmuxPrefix, "", WithRegistry(reg))

	result := session.DoesSessionExistNoCache()

	require.True(t, result, "DoesSessionExistNoCache should return true from healthy registry")
	require.Equal(t, 0, forkCount, "no exec forks should occur when registry is healthy and confirms existence")
}

// TestDoesSessionExistNoCache_FallsBackWhenRegistrySaysFalse verifies that a
// registry `false` is never trusted — DoesSessionExistNoCache always falls
// through to the subprocess path for an authoritative negative, preserving
// its "always fresh" contract.
func TestDoesSessionExistNoCache_FallsBackWhenRegistrySaysFalse(t *testing.T) {
	t.Parallel()
	execCalled := false
	cmdExec := MockCmdExec{
		CombinedOutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			if strings.Contains(cmd.String(), "list-sessions") {
				execCalled = true
				return []byte(TmuxPrefix + "reg-false-nocache-test"), nil
			}
			return []byte(""), nil
		},
		RunFunc:    func(cmd *exec.Cmd) error { return nil },
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) { return []byte(""), nil },
	}

	reg := NewFakeTmuxRegistry()
	reg.SetHealthy(true) // Healthy, but doesn't know about this session yet.

	session := newTmuxSessionWithSocket("reg-false-nocache-test", "echo", NewMockPtyFactory(t), cmdExec, TmuxPrefix, "", WithRegistry(reg))

	result := session.DoesSessionExistNoCache()

	require.True(t, result, "DoesSessionExistNoCache should fall back to exec and find the session")
	require.True(t, execCalled, "exec list-sessions should be called when registry reports false")
}

// TestDoesSessionExistNoCache_FallsBackWhenRegistryUnhealthy verifies that
// when the registry reports unhealthy, DoesSessionExistNoCache falls back to
// the exec path.
func TestDoesSessionExistNoCache_FallsBackWhenRegistryUnhealthy(t *testing.T) {
	t.Parallel()
	execCalled := false
	cmdExec := MockCmdExec{
		CombinedOutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			if strings.Contains(cmd.String(), "list-sessions") {
				execCalled = true
				return []byte(TmuxPrefix + "fallback-nocache-test"), nil
			}
			return []byte(""), nil
		},
		RunFunc:    func(cmd *exec.Cmd) error { return nil },
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) { return []byte(""), nil },
	}

	reg := NewFakeTmuxRegistry()
	reg.SetHealthy(false) // Registry is unhealthy — must fall back.

	session := newTmuxSessionWithSocket("fallback-nocache-test", "echo", NewMockPtyFactory(t), cmdExec, TmuxPrefix, "", WithRegistry(reg))

	result := session.DoesSessionExistNoCache()

	require.True(t, result, "DoesSessionExistNoCache should return true from exec fallback")
	require.True(t, execCalled, "exec list-sessions should be called when registry is unhealthy")
}

// TestDoesSessionExistNoCache_NilRegistry verifies that a nil registry falls
// back to exec.
func TestDoesSessionExistNoCache_NilRegistry(t *testing.T) {
	t.Parallel()
	execCalled := false
	cmdExec := MockCmdExec{
		CombinedOutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			if strings.Contains(cmd.String(), "list-sessions") {
				execCalled = true
				return []byte(TmuxPrefix + "nil-reg-nocache-test"), nil
			}
			return []byte(""), nil
		},
		RunFunc:    func(cmd *exec.Cmd) error { return nil },
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) { return []byte(""), nil },
	}

	session := newTmuxSessionWithSocket("nil-reg-nocache-test", "echo", NewMockPtyFactory(t), cmdExec, TmuxPrefix, "", WithRegistry(nil))

	result := session.DoesSessionExistNoCache()

	require.True(t, result)
	require.True(t, execCalled, "exec list-sessions should be called when registry is nil")
}

// TestCapturePaneSemaphore verifies that the capturePaneSem semaphore caps
// concurrent CapturePaneContent subprocess executions to at most 8.
func TestCapturePaneSemaphore(t *testing.T) {
	t.Parallel()
	const goroutines = 20
	const maxConcurrent = 8

	var mu sync.Mutex
	inflight := 0
	maxSeen := 0

	cmdExec := MockCmdExec{
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			mu.Lock()
			inflight++
			if inflight > maxSeen {
				maxSeen = inflight
			}
			mu.Unlock()

			// Hold the "subprocess" briefly so concurrency builds up.
			time.Sleep(10 * time.Millisecond)

			mu.Lock()
			inflight--
			mu.Unlock()
			return []byte("pane content"), nil
		},
		RunFunc:            func(cmd *exec.Cmd) error { return nil },
		CombinedOutputFunc: func(cmd *exec.Cmd) ([]byte, error) { return []byte(""), nil },
	}

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			session := newTmuxSession(fmt.Sprintf("sem-test-%d", i), "echo", NewMockPtyFactory(t), cmdExec, TmuxPrefix)
			_, _ = session.CapturePaneContent()
		}(i)
	}
	wg.Wait()

	require.LessOrEqual(t, maxSeen, maxConcurrent,
		"concurrent capture-pane subprocesses (%d) exceeded semaphore limit (%d)", maxSeen, maxConcurrent)
	require.Greater(t, maxSeen, 0, "at least one subprocess should have executed")
}

// TestCapturePaneContentPriority_should_UseFastLaneGate_When_ExecGateFastLaneFlagOn proves
// CapturePaneContentPriority routes through the resync fast-lane gate pool (runGatedFastLane)
// rather than the default pool (runGated) that CapturePaneContent uses. It saturates only the
// fast-lane pool (size 1) and shows CapturePaneContentPriority blocks on it, while the default
// pool (size 8, left empty) would not have blocked -- proving the two calls consult different
// gate state for the same serverSocket.
func TestCapturePaneContentPriority_should_UseFastLaneGate_When_ExecGateFastLaneFlagOn(t *testing.T) {
	serverSocket := setupExecGateTestConfig(t, 8, 1)

	cmdExec := MockCmdExec{
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("captured"), nil
		},
		RunFunc:            func(cmd *exec.Cmd) error { return nil },
		CombinedOutputFunc: func(cmd *exec.Cmd) ([]byte, error) { return []byte(""), nil },
	}
	session := newTmuxSessionWithSocket("fast-lane-test", "echo", NewMockPtyFactory(t), cmdExec, TmuxPrefix, serverSocket)

	releaseFastLane, err := AcquireResyncExecSlot(context.Background(), serverSocket)
	require.NoError(t, err)

	// The default pool is unsaturated, so a plain CapturePaneContent must return immediately
	// even while the fast lane's single slot is held.
	start := time.Now()
	content, err := session.CapturePaneContent()
	elapsed := time.Since(start)
	require.NoError(t, err)
	assert.Equal(t, "captured", content)
	assert.Less(t, elapsed, 100*time.Millisecond, "CapturePaneContent should use the default pool and not be blocked by the held fast lane slot")

	done := make(chan struct{})
	go func() {
		defer close(done)
		time.Sleep(150 * time.Millisecond)
		releaseFastLane()
	}()

	start = time.Now()
	content, err = session.CapturePaneContentPriority()
	elapsed = time.Since(start)
	<-done

	require.NoError(t, err)
	assert.Equal(t, "captured", content)
	assert.GreaterOrEqual(t, elapsed, 100*time.Millisecond, "CapturePaneContentPriority should have waited for the saturated fast lane pool, proving it uses the fast lane gate rather than the (unsaturated) default pool")
}

// ---------------------------------------------------------------------------
// ptmxMu / PTY-triple synchronization (tmux-ptmx-race-fix)
// ---------------------------------------------------------------------------

// erroringPtyFactory always fails Start/StartWithSize, for testing PTY-attach error paths.
type erroringPtyFactory struct{}

func (erroringPtyFactory) Start(cmd *exec.Cmd) (*os.File, *exec.Cmd, error) {
	return nil, nil, fmt.Errorf("boom: pty start failed")
}

func (erroringPtyFactory) StartWithSize(cmd *exec.Cmd, _ *pty.Winsize) (*os.File, *exec.Cmd, error) {
	return nil, nil, fmt.Errorf("boom: pty start failed")
}

func (erroringPtyFactory) Close() {}

func TestLockedPTMX_ReturnsNil_BeforeAnyTripleSet(t *testing.T) {
	t.Parallel()
	session := newTmuxSession("locked-ptmx-nil-test", "echo", NewMockPtyFactory(t), MockCmdExec{}, TmuxPrefix)
	require.Nil(t, session.lockedPTMX())
}

func TestSetPTYTriple_AssignsAllThreeFieldsTogether(t *testing.T) {
	t.Parallel()
	session := newTmuxSession("set-pty-triple-test", "echo", NewMockPtyFactory(t), MockCmdExec{}, TmuxPrefix)
	r, w, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = w.Close() })
	cmd := safeexec.CommandContext(context.Background(), "true")
	once := new(sync.Once)

	session.setPTYTriple(r, cmd, once)

	require.Equal(t, r, session.lockedPTMX())
	require.Equal(t, cmd, session.attachCmd)          // allow-direct-ptmx-access: same-package assertion, not concurrent with anything
	require.Equal(t, once, session.attachCmdWaitOnce) // allow-direct-ptmx-access: same-package assertion, not concurrent with anything
	require.NoError(t, r.Close())
}

func TestClearPTYTriple_CapturesThenNilsAllThreeFields(t *testing.T) {
	t.Parallel()
	session := newTmuxSession("clear-pty-triple-test", "echo", NewMockPtyFactory(t), MockCmdExec{}, TmuxPrefix)
	r, w, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = w.Close() })
	cmd := safeexec.CommandContext(context.Background(), "true")
	once := new(sync.Once)
	session.setPTYTriple(r, cmd, once)

	gotFile, gotCmd, gotOnce := session.clearPTYTriple()
	require.Equal(t, r, gotFile)
	require.Equal(t, cmd, gotCmd)
	require.Equal(t, once, gotOnce)
	require.Nil(t, session.lockedPTMX())

	// Idempotent: calling it again with nothing installed returns nils, not a panic.
	gotFile2, gotCmd2, gotOnce2 := session.clearPTYTriple()
	require.Nil(t, gotFile2)
	require.Nil(t, gotCmd2)
	require.Nil(t, gotOnce2)

	require.NoError(t, r.Close())
}

// TestPtmxMuDocComment_StatesLeafLockInvariant verifies the ptmxMu field declaration's
// doc comment documents the lock-order invariant (AC4), by reading tmux.go's own source
// rather than duplicating the lock list into a second, driftable source of truth.
func TestPtmxMuDocComment_StatesLeafLockInvariant(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("tmux.go")
	require.NoError(t, err)
	lines := strings.Split(string(data), "\n")

	fieldLine := -1
	for i, line := range lines {
		if strings.Contains(line, "ptmxMu deadlock.Mutex") {
			fieldLine = i
			break
		}
	}
	require.GreaterOrEqual(t, fieldLine, 0, "ptmxMu field declaration not found in tmux.go")

	// Walk upward from the field declaration, collecting only the contiguous run of
	// "//"-comment lines immediately preceding it -- this is that field's doc comment,
	// not a fixed byte lookback that could drift into an unrelated preceding comment
	// block if either grows or shrinks.
	var commentLines []string
	for i := fieldLine - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(trimmed, "//") {
			break
		}
		commentLines = append([]string{trimmed}, commentLines...)
	}
	docComment := strings.ToLower(strings.Join(commentLines, "\n"))
	require.NotEmpty(t, docComment, "ptmxMu field declaration has no preceding doc comment")

	require.Contains(t, docComment, "leaf lock")
	for _, lockName := range []string{"detachMutex", "controlModeSubMu", "controlModeStartMu", "cmdSendMu", "recoveryMu"} {
		require.Contains(t, docComment, strings.ToLower(lockName),
			"ptmxMu doc comment should name %s as part of its lock-order documentation", lockName)
	}
}

// TestGetPTY_ClosePTYAndAttachCmd_ConcurrentAccessIsSerialized forces the exact interleave
// from the original -race report (GetPTY() vs closePTYAndAttachCmd()) by holding ptmxMu
// before spawning either goroutine, so the fix's correctness is proven deterministically
// rather than left to incidental scheduler timing.
func TestGetPTY_ClosePTYAndAttachCmd_ConcurrentAccessIsSerialized(t *testing.T) {
	t.Parallel()
	session := newTmuxSession("ptmx-race-test", "echo", NewMockPtyFactory(t), MockCmdExec{}, TmuxPrefix)

	r, w, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close(); _ = w.Close() })
	session.ptmx = r // allow-direct-ptmx-access: pre-test setup, not concurrent with anything yet

	session.ptmxMu.Lock()
	var wg sync.WaitGroup
	wg.Add(2)
	var getErr error
	go func() {
		defer wg.Done()
		_, getErr = session.GetPTY()
	}()
	go func() {
		defer wg.Done()
		session.closePTYAndAttachCmd()
	}()
	session.ptmxMu.Unlock()

	completed := make(chan struct{})
	go func() {
		wg.Wait()
		close(completed)
	}()
	select {
	case <-completed:
	case <-time.After(2 * time.Second):
		t.Fatal("GetPTY/closePTYAndAttachCmd deadlocked under concurrent access — ptmxMu not released correctly")
	}

	if getErr != nil {
		require.Contains(t, getErr.Error(), "not initialized",
			"GetPTY's only valid error outcome when racing closePTYAndAttachCmd is the not-initialized error")
	}
}

// TestDetachSafely_ConcurrentWithGetPTY_NoDeadlock exercises the documented
// detachMutex (outer) -> ptmxMu (inner/leaf) lock order: DetachSafely holds detachMutex
// across a closePTYAndAttachCmd() call that internally acquires ptmxMu, while a concurrent
// goroutine repeatedly calls GetPTY() (ptmxMu only). Neither ordering should ever deadlock.
func TestDetachSafely_ConcurrentWithGetPTY_NoDeadlock(t *testing.T) {
	t.Parallel()
	session := newTmuxSession("detach-getpty-nodeadlock-test", "echo", NewMockPtyFactory(t), MockCmdExec{}, TmuxPrefix)

	const iterations = 20
	detachDone := make(chan struct{})
	var pipeErr error
	go func() {
		defer close(detachDone)
		for i := 0; i < iterations; i++ {
			r, w, err := os.Pipe()
			if err != nil {
				pipeErr = err
				return
			}
			session.setPTYTriple(r, nil, nil)
			session.attachCh = make(chan struct{})
			session.wg = &sync.WaitGroup{}
			_ = session.DetachSafely()
			_ = w.Close()
		}
	}()

	getDone := make(chan struct{})
	go func() {
		defer close(getDone)
		for i := 0; i < iterations; i++ {
			_, _ = session.GetPTY()
		}
	}()

	select {
	case <-detachDone:
	case <-time.After(5 * time.Second):
		t.Fatal("DetachSafely deadlocked against concurrent GetPTY — detachMutex/ptmxMu ordering broken")
	}
	require.NoError(t, pipeErr)
	select {
	case <-getDone:
	case <-time.After(5 * time.Second):
		t.Fatal("GetPTY deadlocked against concurrent DetachSafely — detachMutex/ptmxMu ordering broken")
	}
}

// TestDetach_ConcurrentWithGetPTY_NoDeadlock covers the specific nesting
// TestDetachSafely_ConcurrentWithGetPTY_NoDeadlock does not: Detach() (unlike
// DetachSafely) holds detachMutex across TWO separate ptmxMu acquisitions in
// sequence -- closePTYAndAttachCmd() first, then Restore() -> RestoreWithWorkDir(),
// which itself acquires ptmxMu via lockedPTMX()/setPTYTriple() in its retry loop.
// This is exactly the nesting the ptmxMu doc comment and ADR-001 cite by name as
// justification for treating ptmxMu as a safe leaf lock -- it needs its own
// deadlock-guard test, not just DetachSafely's simpler one-acquisition case.
func TestDetach_ConcurrentWithGetPTY_NoDeadlock(t *testing.T) {
	t.Parallel()
	registry := NewFakeTmuxRegistry()
	session := newTmuxSessionWithSocket("detach-restore-getpty-nodeadlock-test", "echo", NewMockPtyFactory(t), MockCmdExec{}, TmuxPrefix, "", WithRegistry(registry))
	registry.SetSessions([]string{session.sanitizedName})

	const iterations = 10
	detachDone := make(chan struct{})
	go func() {
		defer close(detachDone)
		for i := 0; i < iterations; i++ {
			r, w, err := os.Pipe()
			require.NoError(t, err)
			session.setPTYTriple(r, nil, nil)
			session.attachCh = make(chan struct{})
			session.wg = &sync.WaitGroup{}
			// Detach() panics on a fatal cleanup/restore error; the mocked PTY factory
			// and registry are wired so DoesSessionExist()/ptyFactory.StartWithSize()
			// always succeed, so no panic is expected -- this test's only concern is
			// deadlock-freedom, not Detach()'s success/failure semantics.
			session.Detach()
			_ = w.Close()
		}
	}()

	getDone := make(chan struct{})
	go func() {
		defer close(getDone)
		for i := 0; i < iterations; i++ {
			_, _ = session.GetPTY()
		}
	}()

	select {
	case <-detachDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Detach deadlocked against concurrent GetPTY — detachMutex/ptmxMu ordering broken across Restore()/RestoreWithWorkDir()")
	}
	select {
	case <-getDone:
	case <-time.After(5 * time.Second):
		t.Fatal("GetPTY deadlocked against concurrent Detach")
	}
}

// TestClosePTYAndAttachCmd_OnlyFirstConcurrentCallerPerformsCleanup covers the one accepted
// behavioral side effect documented in AC6: only the first caller to acquire ptmxMu inside
// clearPTYTriple() receives the non-nil snapshot, so a second concurrent caller is a fast
// no-op instead of racing Close()/Kill()/Wait() against the first.
func TestClosePTYAndAttachCmd_OnlyFirstConcurrentCallerPerformsCleanup(t *testing.T) {
	t.Parallel()
	session := newTmuxSession("close-serialize-test", "echo", NewMockPtyFactory(t), MockCmdExec{}, TmuxPrefix)
	r, w, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = w.Close() })

	cmd := safeexec.CommandContext(context.Background(), "sleep", "5")
	require.NoError(t, cmd.Start())
	session.setPTYTriple(r, cmd, new(sync.Once))

	var wg sync.WaitGroup
	results := make([][]error, 2)
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func(i int) {
			defer wg.Done()
			results[i] = session.closePTYAndAttachCmd()
		}(i)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("concurrent closePTYAndAttachCmd calls deadlocked")
	}

	require.Empty(t, results[0], "first concurrent close call should report no errors")
	require.Empty(t, results[1], "second concurrent close call should report no errors (fast no-op)")
	require.Nil(t, session.lockedPTMX(), "triple must be fully cleared after both calls complete")

	// The real process must have been killed and reaped exactly once by the winning
	// caller — a second Wait() (from the test, standing in for a hypothetical second
	// caller that reused the same *exec.Cmd) must error, proving Wait ran already.
	waitErr := cmd.Wait()
	require.Error(t, waitErr, "process should already be reaped by closePTYAndAttachCmd; a second Wait should error")
}

func TestAttachToExisting_ReturnsSameWrappedError_When_PtyFactoryStartFails(t *testing.T) {
	t.Parallel()
	registry := NewFakeTmuxRegistry()
	cmdExec := MockCmdExec{}
	session := newTmuxSessionWithSocket("attach-existing-error-test", "echo", erroringPtyFactory{}, cmdExec, TmuxPrefix, "", WithRegistry(registry))
	registry.SetSessions([]string{session.sanitizedName})

	err := session.AttachToExisting()
	require.Error(t, err)
	require.Contains(t, err.Error(), fmt.Sprintf("failed to attach PTY to session '%s':", session.sanitizedName))
	require.Contains(t, err.Error(), "boom: pty start failed")
}

func TestGetPTY_ReturnsNotInitializedError_When_TripleNeverSet(t *testing.T) {
	t.Parallel()
	session := newTmuxSession("getpty-not-initialized-test", "echo", NewMockPtyFactory(t), MockCmdExec{}, TmuxPrefix)
	file, err := session.GetPTY()
	require.Nil(t, file)
	require.EqualError(t, err, "PTY not initialized - session may not be started")
}

func TestTapEnter_TapDAndEnter_SendKeys_ReturnSameWrappedErrors_When_PTYNil(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		call          func(*TmuxSession) error
		wantMsgPrefix string // "" means the raw GetPTY error is returned unwrapped
	}{
		{
			name:          "TapEnter",
			call:          func(s *TmuxSession) error { return s.TapEnter() },
			wantMsgPrefix: "error sending enter keystroke to PTY:",
		},
		{
			name:          "TapDAndEnter",
			call:          func(s *TmuxSession) error { return s.TapDAndEnter() },
			wantMsgPrefix: "error sending enter keystroke to PTY:",
		},
		{
			name: "SendKeys",
			call: func(s *TmuxSession) error {
				n, err := s.SendKeys("hello")
				require.Equal(t, 0, n)
				return err
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			session := newTmuxSession("tap-sendkeys-nil-pty-test", "echo", NewMockPtyFactory(t), MockCmdExec{}, TmuxPrefix)
			err := tc.call(session)
			require.Error(t, err)
			if tc.wantMsgPrefix != "" {
				require.Contains(t, err.Error(), tc.wantMsgPrefix)
			}
			require.Contains(t, err.Error(), "PTY not initialized")
		})
	}
}

func TestUpdateWindowSize_ReturnsSameErrors_When_PTYNilOrFdInvalid(t *testing.T) {
	t.Parallel()
	t.Run("nil PTY", func(t *testing.T) {
		session := newTmuxSession("resize-nil-pty-test", "echo", NewMockPtyFactory(t), MockCmdExec{}, TmuxPrefix)
		err := session.updateWindowSize(80, 24)
		require.EqualError(t, err, "PTY is not initialized")
	})

	t.Run("closed fd", func(t *testing.T) {
		// os.File.Fd() returns -1 once the file is closed (its internal Sysfd is reset),
		// so a closed *os.File hits the fd<0 branch, not the /dev/fd stat branch below it.
		session := newTmuxSession("resize-closed-fd-test", "echo", NewMockPtyFactory(t), MockCmdExec{}, TmuxPrefix)
		r, w, err := os.Pipe()
		require.NoError(t, err)
		require.NoError(t, r.Close())
		t.Cleanup(func() { _ = w.Close() })
		session.setPTYTriple(r, nil, nil)

		resizeErr := session.updateWindowSize(80, 24)
		require.Error(t, resizeErr)
		require.Contains(t, resizeErr.Error(), "PTY file descriptor is invalid")
	})
}

// TestLockedPTMX_ReflectsNewestGeneration_When_SetPTYTripleSwapsMidLoop is a proxy for the
// Attach() stdin-forward goroutine's "re-snapshot every loop iteration" behavior: repeated
// lockedPTMX() calls interleaved with a setPTYTriple() swap always observe the newest
// generation, matching the pre-fix closure's implicit re-read semantics.
func TestLockedPTMX_ReflectsNewestGeneration_When_SetPTYTripleSwapsMidLoop(t *testing.T) {
	t.Parallel()
	session := newTmuxSession("locked-ptmx-newest-generation-test", "echo", NewMockPtyFactory(t), MockCmdExec{}, TmuxPrefix)

	r1, w1, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = w1.Close() })
	session.setPTYTriple(r1, nil, nil)
	require.Equal(t, r1, session.lockedPTMX())

	r2, w2, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = w2.Close() })
	session.setPTYTriple(r2, nil, nil)

	require.Equal(t, r2, session.lockedPTMX())
	require.NotEqual(t, r1, session.lockedPTMX())

	require.NoError(t, r1.Close())
	require.NoError(t, r2.Close())
}
