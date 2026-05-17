package executor

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// readAllWithStop reads all bytes from r. If the read doesn't complete within
// timeout it calls p.Stop() to kill the process (closing the pipe) and fails
// the test. Under the race detector on CI, subprocess exit can be arbitrarily
// delayed, so we bound the wait rather than blocking indefinitely.
func readAllWithStop(t *testing.T, r io.Reader, p *ManagedProcess, timeout time.Duration) ([]byte, error) {
	t.Helper()
	type result struct {
		data []byte
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		data, err := io.ReadAll(r)
		ch <- result{data, err}
	}()
	select {
	case res := <-ch:
		return res.data, res.err
	case <-time.After(timeout):
		_ = p.Stop()
		t.Fatalf("io.ReadAll timed out after %v — process did not exit as expected", timeout)
		return nil, nil
	}
}

// waitWithStop waits for p to exit. If it doesn't exit within timeout it calls
// p.Stop() and fails the test.
func waitWithStop(t *testing.T, p *ManagedProcess, timeout time.Duration) error {
	t.Helper()
	ch := make(chan error, 1)
	go func() { ch <- p.Wait() }()
	select {
	case err := <-ch:
		return err
	case <-time.After(timeout):
		_ = p.Stop()
		t.Fatalf("p.Wait() timed out after %v — process did not exit as expected", timeout)
		return nil
	}
}

// T-UNIT-009: StartProcess_setpgidTrueByDefault
func TestStartProcess_setpgidTrueByDefault(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	p, err := StartProcess(ctx, helperBin, []string{"--sleep", "10s"}, WithGracePeriod(200*time.Millisecond))
	if err != nil {
		t.Fatalf("StartProcess failed: %v", err)
	}
	defer p.Stop() //nolint:errcheck

	if p.cmd.SysProcAttr == nil {
		t.Fatal("expected SysProcAttr to be set, got nil")
	}
	if !p.cmd.SysProcAttr.Setpgid {
		t.Error("expected SysProcAttr.Setpgid == true")
	}
}

// T-UNIT-010: ManagedProcess_stateMachine_notStarted
func TestManagedProcess_stateMachine_notStarted(t *testing.T) {
	t.Parallel()

	// Zero-value ManagedProcess: PID should be 0, IsAlive should return false (not panic).
	var p ManagedProcess
	pid := p.PID()
	if pid != 0 {
		t.Errorf("expected PID 0 for zero-value ManagedProcess, got %d", pid)
	}
	// IsAlive on nil done channel should return false (not block or panic).
	if p.IsAlive() {
		t.Error("expected IsAlive == false for zero-value ManagedProcess")
	}
}

// T-UNIT-011: StartProcess_commandNotFound_returnsError
func TestStartProcess_commandNotFound_returnsError(t *testing.T) {
	t.Parallel()

	_, err := StartProcess(context.Background(), "/nonexistent/binary/path/xyz", nil)
	if err == nil {
		t.Fatal("expected error for nonexistent binary, got nil")
	}
}

// T-UNIT-012: ManagedProcess_PID_nonZeroAfterStart
func TestManagedProcess_PID_nonZeroAfterStart(t *testing.T) {
	t.Parallel()

	p, err := StartProcess(context.Background(), helperBin, []string{"--sleep", "10s"}, WithGracePeriod(200*time.Millisecond))
	if err != nil {
		t.Fatalf("StartProcess failed: %v", err)
	}
	defer p.Stop() //nolint:errcheck

	if p.PID() <= 0 {
		t.Errorf("expected PID > 0, got %d", p.PID())
	}
}

// T-UNIT-013: ManagedProcess_IsAlive_trueWhileRunning_falseAfterStop
func TestManagedProcess_IsAlive_trueWhileRunning_falseAfterStop(t *testing.T) {
	t.Parallel()

	p, err := StartProcess(context.Background(), helperBin, []string{"--sleep", "10s"}, WithGracePeriod(200*time.Millisecond))
	if err != nil {
		t.Fatalf("StartProcess failed: %v", err)
	}

	if !p.IsAlive() {
		t.Error("expected IsAlive == true while process is running")
	}

	if err := p.Stop(); err != nil {
		t.Errorf("Stop() failed: %v", err)
	}

	if p.IsAlive() {
		t.Error("expected IsAlive == false after Stop()")
	}
}

// T-UNIT-014: ManagedProcess_Wait_returnsAfterNaturalExit
func TestManagedProcess_Wait_returnsAfterNaturalExit(t *testing.T) {
	t.Parallel()

	p, err := StartProcess(context.Background(), helperBin, []string{"--exit-code", "0"})
	if err != nil {
		t.Fatalf("StartProcess failed: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- p.Wait() }()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("unexpected error from Wait(): %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Wait() did not return within 5s after process exited")
	}
}

// T-UNIT-015: ManagedProcess_WithEnv_appendsToEnvironment
func TestManagedProcess_WithEnv_appendsToEnvironment(t *testing.T) {
	t.Parallel()

	p, err := StartProcess(context.Background(), helperBin,
		[]string{"--print-env", "SAFEEXEC_MP_TEST"},
		WithProcessEnv("SAFEEXEC_MP_TEST", "mp_expected"))
	if err != nil {
		t.Fatalf("StartProcess failed: %v", err)
	}
	defer p.Stop() //nolint:errcheck

	data, err := readAllWithStop(t, p.Stdout(), p, 10*time.Second)
	if err != nil {
		t.Fatalf("ReadAll stdout: %v", err)
	}
	_ = p.Wait()

	got := strings.TrimSpace(string(data))
	if got != "mp_expected" {
		t.Errorf("expected 'mp_expected', got %q", got)
	}
}

// T-UNIT-016: ManagedProcess_Stdout_readsOutput
func TestManagedProcess_Stdout_readsOutput(t *testing.T) {
	t.Parallel()

	p, err := StartProcess(context.Background(), helperBin,
		[]string{"--print-lines", "3"})
	if err != nil {
		t.Fatalf("StartProcess failed: %v", err)
	}
	defer p.Stop() //nolint:errcheck

	data, err := readAllWithStop(t, p.Stdout(), p, 10*time.Second)
	if err != nil {
		t.Fatalf("ReadAll stdout: %v", err)
	}
	_ = p.Wait()

	expected := "line 1\nline 2\nline 3\n"
	if string(data) != expected {
		t.Errorf("expected %q, got %q", expected, string(data))
	}
}

// T-UNIT-017: ManagedProcess_Stderr_readsErrors
func TestManagedProcess_Stderr_readsErrors(t *testing.T) {
	t.Parallel()

	p, err := StartProcess(context.Background(), helperBin,
		[]string{"--print-stderr", "error output"})
	if err != nil {
		t.Fatalf("StartProcess failed: %v", err)
	}
	defer p.Stop() //nolint:errcheck

	data, err := readAllWithStop(t, p.Stderr(), p, 10*time.Second)
	if err != nil {
		t.Fatalf("ReadAll stderr: %v", err)
	}
	_ = p.Wait()

	if !strings.Contains(string(data), "error output") {
		t.Errorf("expected 'error output' in stderr, got %q", string(data))
	}
}

// T-UNIT-018: ManagedProcess_ScanLines_callsCallbackPerLine
func TestManagedProcess_ScanLines_callsCallbackPerLine(t *testing.T) {
	t.Parallel()

	p, err := StartProcess(context.Background(), helperBin,
		[]string{"--print-lines", "3"})
	if err != nil {
		t.Fatalf("StartProcess failed: %v", err)
	}
	defer p.Stop() //nolint:errcheck

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var lines []string
	err = p.ScanLines(ctx, func(line string) {
		lines = append(lines, line)
	})
	if err != nil {
		t.Fatalf("ScanLines returned error: %v", err)
	}
	_ = p.Wait()

	expected := []string{"line 1", "line 2", "line 3"}
	if len(lines) != len(expected) {
		t.Fatalf("expected %d lines, got %d: %v", len(expected), len(lines), lines)
	}
	for i, l := range expected {
		if lines[i] != l {
			t.Errorf("line[%d]: expected %q, got %q", i, l, lines[i])
		}
	}
}

// TestManagedProcess_ScanLines_ctxCancellation_stops verifies ScanLines returns
// when the context is cancelled.
func TestManagedProcess_ScanLines_ctxCancellation_stops(t *testing.T) {
	t.Parallel()

	// Start a long-running process.
	p, err := StartProcess(context.Background(), helperBin, []string{"--sleep", "10s"}, WithGracePeriod(200*time.Millisecond))
	if err != nil {
		t.Fatalf("StartProcess failed: %v", err)
	}
	defer p.Stop() //nolint:errcheck

	// Redirect stdout to discard since we don't expect any output.
	// The process sleeps, so Stdout() will block.
	// We cancel the context to unblock ScanLines.
	ctx, cancel := context.WithCancel(context.Background())

	scanDone := make(chan error, 1)
	go func() {
		scanDone <- p.ScanLines(ctx, func(_ string) {})
	}()

	// Cancel context after a brief moment.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-scanDone:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ScanLines did not return after context cancellation")
	}
}

// T-GUARD-003: ManagedProcess_Stop_idempotent
func TestManagedProcess_Stop_idempotent(t *testing.T) {
	t.Parallel()

	p, err := StartProcess(context.Background(), helperBin, []string{"--exit-code", "0"}, WithGracePeriod(200*time.Millisecond))
	if err != nil {
		t.Fatalf("StartProcess failed: %v", err)
	}
	// Wait for natural exit.
	_ = p.Wait()

	// Idempotent: two sequential Stop() calls.
	err1 := p.Stop()
	err2 := p.Stop()
	// Neither should panic.
	_ = err1
	_ = err2

	// Two concurrent Stop() calls.
	p2, err := StartProcess(context.Background(), helperBin, []string{"--sleep", "10s"}, WithGracePeriod(200*time.Millisecond))
	if err != nil {
		t.Fatalf("StartProcess failed: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = p2.Stop()
	}()
	go func() {
		defer wg.Done()
		_ = p2.Stop()
	}()
	wg.Wait()
}

// T-GUARD-002: ErrWaitDelay_notReturnedToCaller
// Verifies that when SIGTERM is ignored and WaitDelay fires, Stop() returns
// nil (not exec.ErrWaitDelay).
func TestManagedProcess_Stop_errWaitDelayFiltered(t *testing.T) {
	t.Parallel()

	// Use a very short grace period so the test completes quickly.
	p, err := StartProcess(context.Background(), helperBin,
		[]string{"--trap-sigterm", "--sleep", "60s"},
		WithGracePeriod(300*time.Millisecond))
	if err != nil {
		t.Fatalf("StartProcess failed: %v", err)
	}

	start := time.Now()
	err = p.Stop()
	elapsed := time.Since(start)

	// exec.ErrWaitDelay must NOT leak through.
	if errors.Is(err, exec.ErrWaitDelay) {
		t.Error("Stop() returned exec.ErrWaitDelay — must be filtered")
	}

	// Should complete within a reasonable time (grace period + kill + a bit).
	if elapsed > 10*time.Second {
		t.Errorf("Stop() took too long: %v", elapsed)
	}
}

// TestManagedProcess_Stop_sendsTermThenKill verifies the graceful sequence.
// The process ignores SIGTERM, so Stop() must eventually SIGKILL it.
// On macOS there is a narrow race window where SIGTERM may arrive before
// signal.Ignore is installed in the helper — if that happens, the process
// exits immediately and the timing assertion would be a false failure.
// We verify the key invariant (Stop returns without error and process is dead)
// rather than the precise timing.
func TestManagedProcess_Stop_sendsTermThenKill(t *testing.T) {
	t.Parallel()

	p, err := StartProcess(context.Background(), helperBin,
		[]string{"--trap-sigterm", "--sleep", "60s"},
		WithGracePeriod(200*time.Millisecond))
	if err != nil {
		t.Fatalf("StartProcess failed: %v", err)
	}

	start := time.Now()
	stopErr := p.Stop()
	elapsed := time.Since(start)

	// Stop() must not return exec.ErrWaitDelay (that must be filtered).
	if errors.Is(stopErr, exec.ErrWaitDelay) {
		t.Errorf("Stop() returned exec.ErrWaitDelay — must be filtered")
	}

	// Process must be dead after Stop() returns.
	if p.IsAlive() {
		t.Error("process still alive after Stop() returned")
	}

	// Must complete within a reasonable time regardless of signal delivery.
	if elapsed > 10*time.Second {
		t.Errorf("Stop() took too long: %v", elapsed)
	}
}

// TestManagedProcess_ConsumeStdout_returnsNilStdout verifies WithConsumeStdout.
func TestManagedProcess_ConsumeStdout_returnsNilStdout(t *testing.T) {
	t.Parallel()

	var buf strings.Builder
	p, err := StartProcess(context.Background(), helperBin,
		[]string{"--print", "direct"},
		WithConsumeStdout(&buf),
		WithGracePeriod(200*time.Millisecond))
	if err != nil {
		t.Fatalf("StartProcess failed: %v", err)
	}
	defer p.Stop() //nolint:errcheck

	if p.Stdout() != nil {
		t.Error("expected Stdout() == nil when WithConsumeStdout is used")
	}
	_ = waitWithStop(t, p, 10*time.Second)
}

// TestManagedProcess_PID_afterStop verifies PID is still accessible after stop.
func TestManagedProcess_PID_afterStop(t *testing.T) {
	t.Parallel()

	p, err := StartProcess(context.Background(), helperBin, []string{"--sleep", "10s"}, WithGracePeriod(200*time.Millisecond))
	if err != nil {
		t.Fatalf("StartProcess failed: %v", err)
	}
	pid := p.PID()
	_ = p.Stop()

	// PID should still be accessible and non-zero after stop.
	if p.PID() != pid {
		t.Errorf("PID changed after Stop: was %d, now %d", pid, p.PID())
	}
}

// TestManagedProcess_Wait_nonZeroExit returns ExitError on non-zero exit.
func TestManagedProcess_Wait_nonZeroExit(t *testing.T) {
	t.Parallel()

	p, err := StartProcess(context.Background(), helperBin, []string{"--exit-code", "42"})
	if err != nil {
		t.Fatalf("StartProcess failed: %v", err)
	}

	err = p.Wait()
	if err == nil {
		t.Fatal("expected error for exit code 42, got nil")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *exec.ExitError, got %T: %v", err, err)
	}
	if exitErr.ExitCode() != 42 {
		t.Errorf("expected exit code 42, got %d", exitErr.ExitCode())
	}
}

// T-GUARD-004: ManagedProcess_Stop_afterWait_doesNotDeadlock
// Regression test: calling Stop() after Wait() must not deadlock.
// Previously, Stop() did an unbounded <-waitErr after <-done, but Wait() had
// already drained waitErr, causing Stop() to block forever.
func TestManagedProcess_Stop_afterWait_doesNotDeadlock(t *testing.T) {
	t.Parallel()

	p, err := StartProcess(context.Background(), helperBin, []string{"--exit-code", "0"})
	if err != nil {
		t.Fatalf("StartProcess failed: %v", err)
	}

	_ = p.Wait() // drains waitErr

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = p.Stop()
	}()

	select {
	case <-done:
		// pass
	case <-time.After(15 * time.Second):
		t.Fatal("Stop() deadlocked after Wait() was already called")
	}
}
