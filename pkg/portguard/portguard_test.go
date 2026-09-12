package portguard

import (
	"context"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// TestMain re-execs this same test binary as a disposable "stuck server"
// helper process when PORTGUARD_TEST_HELPER_PORT is set, instead of shelling
// out to nc/python — keeping the test hermetic and portable. This is the
// standard Go "helper process" pattern (see os/exec's own TestHelperProcess).
func TestMain(m *testing.M) {
	if portStr := os.Getenv("PORTGUARD_TEST_HELPER_PORT"); portStr != "" {
		runHelperProcess(portStr)
		return
	}
	os.Exit(m.Run())
}

// runHelperProcess binds the requested port and blocks. If
// PORTGUARD_TEST_HELPER_IGNORE_TERM is set it also ignores SIGTERM, standing
// in for a process whose graceful-shutdown path hangs — exactly the
// condition that made the pre-fix shell version "wait 10s, then proceed
// anyway" into a real incident.
func runHelperProcess(portStr string) {
	if os.Getenv("PORTGUARD_TEST_HELPER_IGNORE_TERM") == "1" {
		signal.Ignore(syscall.SIGTERM)
	}
	ln, err := net.Listen("tcp", ":"+portStr)
	if err != nil {
		os.Exit(2)
	}
	defer func() { _ = ln.Close() }()
	time.Sleep(30 * time.Second)
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to reserve a free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

func startHelper(t *testing.T, port int, ignoreTerm bool) *exec.Cmd {
	t.Helper()
	// -test.run matches no actual Test function (TestMain is special-cased
	// and always runs regardless of -run) — we only want TestMain's early
	// env-var check below to fire, not the rest of the test suite.
	cmd := safeexec.CommandContext(context.Background(), os.Args[0], "-test.run=^NoSuchTest$")
	cmd.Env = append(os.Environ(), "PORTGUARD_TEST_HELPER_PORT="+strconv.Itoa(port))
	if ignoreTerm {
		cmd.Env = append(cmd.Env, "PORTGUARD_TEST_HELPER_IGNORE_TERM=1")
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start helper process: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	for i := 0; i < 250 && PortFree(port); i++ {
		time.Sleep(20 * time.Millisecond)
	}
	if PortFree(port) {
		t.Fatalf("helper process never bound port %d", port)
	}
	return cmd
}

// TestEnsureReleased_KillsGracefulHolder reproduces the exact shape of the
// 2026-09-08 incident: a stale process is still bound to the target port
// when EnsureReleased is called. The pre-fix shell equivalent
// (wait_for_port_release) would wait 10s and then return success anyway with
// the port still bound — this test fails against that behavior because it
// asserts the port is actually free afterward, not just that the call
// returned.
func TestEnsureReleased_KillsGracefulHolder(t *testing.T) {
	port := freePort(t)
	cmd := startHelper(t, port, false)

	err := EnsureReleased(context.Background(), Options{
		Ports:               []int{port},
		ProcessNameContains: "portguard.test",
		Timeout:             500 * time.Millisecond,
		PollInterval:        20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("EnsureReleased returned error: %v", err)
	}
	if !PortFree(port) {
		t.Fatalf("port %d is still bound after EnsureReleased returned success", port)
	}
	_ = cmd.Wait()
}

// TestEnsureReleased_EscalatesToSIGKILL covers the holder that ignores
// SIGTERM entirely — EnsureReleased must escalate to SIGKILL rather than
// giving up, since "the process didn't exit gracefully" is exactly the
// real-world condition (a hung shutdown path) that produced the incident.
func TestEnsureReleased_EscalatesToSIGKILL(t *testing.T) {
	port := freePort(t)
	cmd := startHelper(t, port, true)

	err := EnsureReleased(context.Background(), Options{
		Ports:               []int{port},
		ProcessNameContains: "portguard.test",
		Timeout:             300 * time.Millisecond,
		PollInterval:        20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("EnsureReleased returned error: %v", err)
	}
	if !PortFree(port) {
		t.Fatalf("port %d is still bound after EnsureReleased returned success", port)
	}
	_ = cmd.Wait()
}

// TestEnsureReleased_NoHolderIsANoop confirms the common case — nothing is
// listening — returns immediately without touching unrelated processes.
func TestEnsureReleased_NoHolderIsANoop(t *testing.T) {
	port := freePort(t)
	start := time.Now()
	err := EnsureReleased(context.Background(), Options{
		Ports:        []int{port},
		Timeout:      2 * time.Second,
		PollInterval: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("EnsureReleased returned error for an already-free port: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 1*time.Second {
		t.Fatalf("EnsureReleased took %v for an already-free port; want near-instant", elapsed)
	}
}
