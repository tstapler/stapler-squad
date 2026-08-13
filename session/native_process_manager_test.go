//go:build !windows

package session

import (
	"context"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// checkPTYAvailable skips the test when the OS does not permit PTY allocation with
// Setpgid=true (e.g., macOS sandbox environments where setpgid is disallowed).
// This is an environmental restriction, not a code defect.
func checkPTYAvailable(t *testing.T) {
	t.Helper()
	cmd := safeexec.CommandContext(context.Background(), "bash", "-c", "exit 0")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if _, err := pty.Start(cmd); err != nil {
		_ = cmd.Wait()
		t.Skipf("PTY+Setpgid not available in this environment: %v", err)
	}
	_ = cmd.Wait()
}

// T-COMPILE-3: compile-time check that *NativeProcessManager satisfies ProcessManager.
var _ ProcessManager = (*NativeProcessManager)(nil)

// TestNativeProcessManager_ImplementsProcessManager documents the compile-time check.
func TestNativeProcessManager_ImplementsProcessManager(t *testing.T) {
	// This test exists to document that the compile-time check above is intentional.
	// If NativeProcessManager were missing any ProcessManager method, this file
	// would not compile.
	t.Log("NativeProcessManager implements ProcessManager (verified at compile time)")
}

// T-UNIT-10: GetSessionIdentifier returns the stable session name from construction.
func TestNativeProcessManager_GetSessionIdentifier_IsStable(t *testing.T) {
	mgr := NewNativeProcessManager(ProcessManagerOptions{
		SessionName: "my-stable-session",
		Program:     "bash",
	})
	assert.Equal(t, "my-stable-session", mgr.GetSessionIdentifier())
	// Must return same value on repeated calls without Start().
	assert.Equal(t, "my-stable-session", mgr.GetSessionIdentifier())
}

// T-UNIT-7: Start() allocates a PTY — ptm field is non-nil after Start().
func TestNativeProcessManager_StartAllocatesPTY(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PTY allocation")
	}
	checkPTYAvailable(t)
	mgr := NewNativeProcessManager(ProcessManagerOptions{
		SessionName: "test-pty",
		Program:     "bash",
		Args:        []string{"-c", "sleep 60"},
	})
	err := mgr.Start(t.TempDir())
	require.NoError(t, err)
	defer func() { _ = mgr.Close() }()

	mgr.mu.Lock()
	ptm := mgr.ptm
	mgr.mu.Unlock()

	assert.NotNil(t, ptm, "PTY master fd must be allocated after Start()")
	assert.True(t, mgr.IsAlive(), "process should be alive after Start()")
}

// T-UNIT-9: Close() stops the restart loop — goroutine count returns to baseline.
func TestNativeProcessManager_CloseStopsRestartLoop(t *testing.T) {
	if testing.Short() {
		t.Skip("goroutine measurement; takes ~500ms")
	}
	checkPTYAvailable(t)
	baseline := runtime.NumGoroutine()

	mgr := NewNativeProcessManager(ProcessManagerOptions{
		SessionName: "test-close",
		Program:     "bash",
		Args:        []string{"-c", "sleep 60"},
	})
	err := mgr.Start(t.TempDir())
	require.NoError(t, err)

	err = mgr.Close()
	require.NoError(t, err)

	// Give goroutines time to exit.
	time.Sleep(500 * time.Millisecond)

	// Should be back to baseline (±2 for scheduler variance).
	assert.InDelta(t, baseline, runtime.NumGoroutine(), 3,
		"goroutines should return to baseline after Close()")
}

// T-UNIT-8: NativeProcessManager restarts after the supervised process is killed.
// Primary demonstration of crash-restart behavior (requirement R3).
func TestNativeProcessManagerRestartsAfterKill(t *testing.T) {
	if testing.Short() {
		t.Skip("requires process supervision; takes ~2s")
	}
	checkPTYAvailable(t)
	mgr := NewNativeProcessManager(ProcessManagerOptions{
		SessionName: "test-restart",
		Program:     "bash",
		Args:        []string{"-c", "echo alive; sleep 60"},
	})
	err := mgr.Start(t.TempDir())
	require.NoError(t, err)
	defer func() { _ = mgr.Close() }()

	mgr.mu.Lock()
	pid1 := mgr.cmd.Process.Pid
	mgr.mu.Unlock()
	require.Greater(t, pid1, 0, "PID must be positive after Start()")

	// Kill the supervised process.
	err = syscall.Kill(pid1, syscall.SIGKILL)
	require.NoError(t, err)

	// Wait for restart (supervisor has 500 ms backoff + launch time).
	time.Sleep(2 * time.Second)

	mgr.mu.Lock()
	pid2 := 0
	if mgr.cmd != nil && mgr.cmd.Process != nil {
		pid2 = mgr.cmd.Process.Pid
	}
	mgr.mu.Unlock()

	assert.NotEqual(t, pid1, pid2,
		"process should have restarted with a new PID after kill")
	assert.Greater(t, pid2, 0, "new PID must be positive")
}

// T-INTEGRATION-1: Native session IsAlive() after Start().
func TestNativeProcessManager_IsAliveAfterStart(t *testing.T) {
	if testing.Short() {
		t.Skip("integration: requires PTY")
	}
	checkPTYAvailable(t)
	mgr := NewNativeProcessManager(ProcessManagerOptions{
		SessionName: "integration-test",
		Program:     "bash",
		Args:        []string{"-i"},
	})
	err := mgr.Start(t.TempDir())
	require.NoError(t, err)
	defer func() { _ = mgr.Close() }()

	assert.True(t, mgr.IsAlive(), "session must be alive after Start()")
	assert.NotEmpty(t, mgr.GetSessionIdentifier())

	mgr.mu.Lock()
	ptm := mgr.ptm
	mgr.mu.Unlock()
	assert.NotNil(t, ptm, "PTY master must be allocated")
}

// T-UNIT-12: Start() after Close() resets the stop signal and produces a live process.
func TestNativeProcessManager_StartAfterClose_Restarts(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PTY allocation")
	}
	checkPTYAvailable(t)
	mgr := NewNativeProcessManager(ProcessManagerOptions{
		SessionName: "test-restart-after-close",
		Program:     "bash",
		Args:        []string{"-c", "sleep 60"},
	})
	dir := t.TempDir()
	require.NoError(t, mgr.Start(dir))
	require.NoError(t, mgr.Close())

	time.Sleep(100 * time.Millisecond)

	require.NoError(t, mgr.Start(dir))
	defer func() { _ = mgr.Close() }()

	assert.True(t, mgr.IsAlive(), "process must be alive after Start() following Close()")
}

// T-UNIT-13: GetCurrentWorkingDirectory returns the directory passed to Start().
func TestNativeProcessManager_GetCurrentWorkingDirectory_ReturnsStartDir(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PTY allocation")
	}
	checkPTYAvailable(t)
	mgr := NewNativeProcessManager(ProcessManagerOptions{
		SessionName: "test-cwd",
		Program:     "bash",
		Args:        []string{"-c", "sleep 60"},
	})
	dir := t.TempDir()
	require.NoError(t, mgr.Start(dir))
	defer func() { _ = mgr.Close() }()

	cwd, err := mgr.GetCurrentWorkingDirectory()
	require.NoError(t, err)
	assert.Equal(t, dir, cwd)
}

// T-UNIT-14: SubscribeToControlModeUpdates receives PTY output via fanOut.
func TestNativeProcessManager_FanOut_DeliversPTYOutput(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PTY allocation")
	}
	checkPTYAvailable(t)
	mgr := NewNativeProcessManager(ProcessManagerOptions{
		SessionName: "test-fanout",
		Program:     "bash",
		Args:        []string{"-i"},
	})
	require.NoError(t, mgr.Start(t.TempDir()))
	defer func() { _ = mgr.Close() }()

	_, ch := mgr.SubscribeToControlModeUpdates()

	_, err := mgr.SendKeys("echo hello-from-fanout\n")
	require.NoError(t, err)

	deadline := time.After(3 * time.Second)
	var received []byte
	for {
		select {
		case data, ok := <-ch:
			if !ok {
				t.Fatal("fanOut channel closed before receiving output")
			}
			received = append(received, data...)
			if strings.Contains(string(received), "hello-from-fanout") {
				return
			}
		case <-deadline:
			t.Fatalf("fanOut did not deliver PTY output within 3s; got: %q", string(received))
		}
	}
}

// T-UNIT-11: Factory routes "native" → *NativeProcessManager.
func TestNewProcessManager_ReturnsNativeProcessManager_WhenFlagIsNative(t *testing.T) {
	RegisterBackendProvider(BackendNative)
	defer RegisterBackendProvider(BackendTmux) // restore default for other tests

	pm := NewProcessManager(context.Background(), BackendNative, ProcessManagerOptions{
		SessionName: "test-native",
	})
	_, ok := pm.(*NativeProcessManager)
	assert.True(t, ok, "expected *NativeProcessManager when flag is 'native'")
}
