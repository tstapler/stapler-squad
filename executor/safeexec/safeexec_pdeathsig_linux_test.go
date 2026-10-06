//go:build linux

package safeexec

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// This test reproduces the exact failure mode that leaked ~460 orphaned tmux
// control-mode processes in production: a parent process spawns a long-running
// child via exec.CommandContext and relies on ctx cancellation to clean it up,
// but the parent is killed with SIGKILL — which Go cannot intercept, so ctx is
// never cancelled and the child is orphaned instead of dying with its parent.
//
// It uses the standard re-exec pattern (mirrors os/exec's own TestHelperProcess):
// this test binary re-execs itself as a "middle" process (env var gated, via
// TestMain) that spawns a "grandchild" with EnsurePdeathsig, then the test
// SIGKILLs the middle process and asserts the grandchild dies with it.

const pdeathsigHelperEnvVar = "SAFEEXEC_PDEATHSIG_HELPER"

func TestMain(m *testing.M) {
	if os.Getenv(pdeathsigHelperEnvVar) == "1" {
		runPdeathsigHelperProcess()
		return
	}
	if os.Getenv(sigkillHelperEnvVar) == "1" {
		runSigkillHelperProcess()
		return
	}
	os.Exit(m.Run())
}

// runPdeathsigHelperProcess is the "middle" process: it spawns a grandchild
// with EnsurePdeathsig set, prints the grandchild's PID, then blocks forever
// so the test can SIGKILL it and observe whether the grandchild dies too.
func runPdeathsigHelperProcess() {
	cmd := exec.Command("sleep", "30")
	EnsurePdeathsig(cmd)
	if err := cmd.Start(); err != nil {
		fmt.Println("ERR", err)
		os.Exit(1)
	}
	fmt.Println(cmd.Process.Pid)
	os.Stdout.Sync()
	time.Sleep(time.Hour) // block until SIGKILLed by the test; not a Go-level deadlock
}

func TestEnsurePdeathsig_GrandchildDiesWhenMiddleIsSigkilled(t *testing.T) {
	middle := exec.Command(os.Args[0], "-test.run=^$")
	middle.Env = append(os.Environ(), pdeathsigHelperEnvVar+"=1")
	stdout, err := middle.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe: %v", err)
	}
	if err := middle.Start(); err != nil {
		t.Fatalf("failed to start middle process: %v", err)
	}

	var grandchildPID int
	scanErrCh := make(chan error, 1)
	go func() {
		_, err := fmt.Fscan(bufio.NewReader(stdout), &grandchildPID)
		scanErrCh <- err
	}()
	select {
	case err := <-scanErrCh:
		if err != nil {
			_ = middle.Process.Kill()
			t.Fatalf("failed to read grandchild PID from helper: %v", err)
		}
	case <-time.After(5 * time.Second):
		_ = middle.Process.Kill()
		t.Fatal("timed out waiting for helper to report grandchild PID")
	}

	if !processAlive(grandchildPID) {
		t.Fatalf("grandchild pid %d not alive right after start (test setup is broken)", grandchildPID)
	}

	if err := middle.Process.Kill(); err != nil { // SIGKILL — uncatchable, no Go cleanup runs
		t.Fatalf("failed to kill middle process: %v", err)
	}
	_ = middle.Wait()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(grandchildPID) {
			return // kernel reaped it via Pdeathsig — this is the fix working
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Don't leave a real leak behind just because the assertion failed.
	_ = syscall.Kill(grandchildPID, syscall.SIGKILL)
	t.Fatalf("grandchild pid %d still alive 3s after its parent was SIGKILLed — Pdeathsig did not fire", grandchildPID)
}

// processAlive reports whether pid is still running. Signal-0 delivery alone
// can't tell a zombie (dead, awaiting reap by its ambient parent/subreaper)
// from a live process — the kernel keeps a zombie's PID entry until
// something calls wait() on it, so kill(pid,0) succeeds for both. Pdeathsig's
// contract is "the kernel terminates the child," which a zombie already
// satisfies; how promptly some other process gets around to reaping the
// corpse is an unrelated, unbounded-latency concern this test shouldn't be
// sensitive to.
func processAlive(pid int) bool {
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return false // already fully reaped
	}
	// Format: "pid (comm) state ...". comm may itself contain ')', so the
	// state field is the first byte after the *last* ')'.
	i := bytes.LastIndexByte(stat, ')')
	if i < 0 || i+2 >= len(stat) {
		return false
	}
	return stat[i+2] != 'Z'
}

// TestProcessAlive_ReturnsFalseForZombie is a regression guard for the exact
// ambiguity processAlive exists to close: kill(pid,0) alone reports a zombie
// (exited, awaiting reap) as "alive" indistinguishably from a genuinely
// running process, which would make the flake-hardening above a no-op.
func TestProcessAlive_ReturnsFalseForZombie(t *testing.T) {
	cmd := exec.Command("true")
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	pid := cmd.Process.Pid
	t.Cleanup(func() { _ = cmd.Wait() })

	deadline := time.Now().Add(2 * time.Second)
	for {
		stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
		if err == nil {
			if i := bytes.LastIndexByte(stat, ')'); i >= 0 && i+2 < len(stat) && stat[i+2] == 'Z' {
				break // process has exited but we haven't reaped it — genuine zombie
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("pid %d never reached zombie state before reaping", pid)
		}
		time.Sleep(2 * time.Millisecond)
	}

	if syscall.Kill(pid, 0) != nil {
		t.Fatalf("pid %d not observed as a zombie by kill(pid,0) — test setup is broken", pid)
	}
	if processAlive(pid) {
		t.Fatalf("processAlive(%d) reported a zombie as alive", pid)
	}
}
