package tmux

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

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

func (pt *MockPtyFactory) Close() {}

func NewMockPtyFactory(t *testing.T) *MockPtyFactory {
	return &MockPtyFactory{
		t: t,
	}
}

func TestSanitizeName(t *testing.T) {
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

// --- serverNotRunning detection tests ---

func TestServerNotRunning(t *testing.T) {
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
		_ = safeexec.CommandContext(killCtx, "tmux", "-L", socketName, "kill-server").Run()
	})

	// Start the isolated server and keep it alive with a detached session.
	// Without a session, tmux exits immediately (exit-empty=on by default), causing
	// the follow-up check in EnsureServerRunning to falsely report the server as dead.
	newSessionCtx, newSessionCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer newSessionCancel()
	require.NoError(t, safeexec.CommandContext(newSessionCtx, "tmux", "-L", socketName, "new-session", "-d", "-s", "keepalive").Run())

	// With the server running and a live session, EnsureServerRunning should be a no-op.
	err := EnsureServerRunning(socketName)
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
		_ = safeexec.CommandContext(killCtx2, "tmux", "-L", socketName, "kill-server").Run()
	})

	// Confirm no server is running on this socket yet.
	require.True(t, checkServerNotRunning(socketName),
		"server should not be running before the test starts")

	// EnsureServerRunning should start the server.
	err := EnsureServerRunning(socketName)
	require.NoError(t, err)

	// On macOS, tmux's default exit-empty=on causes the server to exit immediately
	// when there are no sessions. Create a session to keep the server alive and
	// verify the server is functional by confirming the session can be created.
	createCmd := safeexec.CommandContext(context.Background(), "tmux", "-L", socketName, "new-session", "-d", "-s", "verify-alive")
	require.NoError(t, createCmd.Run(),
		"should be able to create a session on the newly started server — server must be running")
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
		_ = safeexec.CommandContext(context.Background(), "tmux", "-L", socketName, "kill-server").Run()
	})

	// Start the server with an anchor session to keep it alive while we test.
	// (new-session -d is equivalent to start-server + create session atomically)
	require.NoError(t, safeexec.CommandContext(context.Background(), "tmux", "-L", socketName, "new-session", "-d", "-s", "anchor").Run())

	// Create keepalive session.
	err := CreateKeepaliveSession(socketName)
	require.NoError(t, err, "CreateKeepaliveSession should succeed")

	// Verify the keepalive session exists.
	keepaliveName := TmuxPrefix + "keepalive"
	out, err := safeexec.CommandContext(context.Background(), "tmux", "-L", socketName, "has-session", "-t", keepaliveName).CombinedOutput()
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
		_ = safeexec.CommandContext(context.Background(), "tmux", "-L", socketName, "kill-server").Run()
	})

	// Start the server WITH a detached session to prevent the server from exiting
	// immediately due to exit-empty=on (the default). Using new-session -d starts
	// both the server and an anchor session in one step.
	require.NoError(t, safeexec.CommandContext(context.Background(), "tmux", "-L", socketName, "new-session", "-d", "-s", "anchor").Run())

	// Set exit-empty off.
	err := SetExitEmpty(socketName, false)
	require.NoError(t, err, "SetExitEmpty(false) should succeed")

	// Verify the option was set.
	out, err := safeexec.CommandContext(context.Background(), "tmux", "-L", socketName, "show-options", "-g", "exit-empty").CombinedOutput()
	require.NoError(t, err)
	require.Contains(t, strings.ToLower(string(out)), "off",
		"exit-empty should be off after SetExitEmpty(false)")

	// Set exit-empty on.
	err = SetExitEmpty(socketName, true)
	require.NoError(t, err, "SetExitEmpty(true) should succeed")

	out, err = safeexec.CommandContext(context.Background(), "tmux", "-L", socketName, "show-options", "-g", "exit-empty").CombinedOutput()
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

// TestPrependSocket verifies that prependSocket returns args unmodified when the socket
// is empty and prepends "-L <socket>" when the socket is non-empty.
func TestPrependSocket(t *testing.T) {
	tests := []struct {
		name     string
		socket   string
		args     []string
		expected []string
	}{
		{
			name:     "empty socket returns args unchanged",
			socket:   "",
			args:     []string{"list-sessions"},
			expected: []string{"list-sessions"},
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
	ensureServerRunning = func(_ string) error { return nil }
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
