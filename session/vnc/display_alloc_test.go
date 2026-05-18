package vnc

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// ---- parseLockPID tests --------------------------------------------------------

func TestParseLockPID_ValidContent(t *testing.T) {
	// Standard X11 lock file format: right-justified 10-char PID + newline.
	pid := os.Getpid()
	data := []byte(fmt.Sprintf("%10d\n", pid))
	got, err := parseLockPID(data)
	if err != nil {
		t.Fatalf("parseLockPID(%q) error = %v, want nil", data, err)
	}
	if got != pid {
		t.Errorf("parseLockPID(%q) = %d, want %d", data, got, pid)
	}
}

func TestParseLockPID_ValidContentNoNewline(t *testing.T) {
	data := []byte("      1234")
	got, err := parseLockPID(data)
	if err != nil {
		t.Fatalf("parseLockPID error = %v, want nil", err)
	}
	if got != 1234 {
		t.Errorf("parseLockPID = %d, want 1234", got)
	}
}

func TestParseLockPID_MalformedContent(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{"empty", []byte("")},
		{"whitespace_only", []byte("   \n")},
		{"non_numeric", []byte("not-a-pid\n")},
		{"float", []byte("3.14\n")},
		{"negative", []byte("-1\n")}, // parseLockPID itself should succeed, but isPIDAlive rejects it
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Only expect an error for truly non-numeric content.
			_, err := parseLockPID(tc.data)
			s := strings.TrimSpace(string(tc.data))
			_, expectErr := strconv.Atoi(s)
			if (err != nil) != (expectErr != nil) {
				t.Errorf("parseLockPID(%q): err=%v, expectErr=%v", tc.data, err, expectErr)
			}
		})
	}
}

func TestParseLockPID_ExplicitNonNumeric(t *testing.T) {
	_, err := parseLockPID([]byte("abc\n"))
	if err == nil {
		t.Error("parseLockPID(\"abc\\n\") expected error, got nil")
	}
}

// ---- isPIDAlive tests ----------------------------------------------------------

func TestIsPIDAlive_PID1_IsAlways(t *testing.T) {
	// PID 1 (init/systemd) is always alive on Linux.
	if !isPIDAlive(1) {
		t.Error("isPIDAlive(1) = false, want true (PID 1 is always alive)")
	}
}

func TestIsPIDAlive_CurrentProcess(t *testing.T) {
	// Our own PID must be alive.
	if !isPIDAlive(os.Getpid()) {
		t.Errorf("isPIDAlive(%d) = false, want true (self)", os.Getpid())
	}
}

func TestIsPIDAlive_ZeroOrNegative(t *testing.T) {
	for _, pid := range []int{0, -1, -100} {
		if isPIDAlive(pid) {
			t.Errorf("isPIDAlive(%d) = true, want false", pid)
		}
	}
}

func TestIsPIDAlive_KilledProcess(t *testing.T) {
	// Spawn a short-lived child and wait for it to exit, then verify it's dead.
	// Use safeexec.CommandContext to comply with the norawexec lint rule.
	cmd := safeexec.CommandContext(context.Background(), "true")
	if err := cmd.Start(); err != nil {
		t.Skipf("could not start child process: %v", err)
	}
	childPID := cmd.Process.Pid
	if err := cmd.Wait(); err != nil {
		t.Skipf("child exited with error: %v", err)
	}
	// The process has exited. Give the OS a moment to clean up (reaping is
	// synchronous after cmd.Wait() returns, so no sleep needed).
	if isPIDAlive(childPID) {
		t.Errorf("isPIDAlive(%d) = true for an exited process, want false", childPID)
	}
}

// ---- Allocate / Release round-trip tests ----------------------------------------

func TestAllocate_ReturnsDisplayInRange(t *testing.T) {
	const base = 200
	const rangeMax = 5
	alloc := NewDisplayAllocator(base, rangeMax)

	// Clean up any lock files we may create.
	for i := 0; i < rangeMax; i++ {
		defer os.Remove(lockPath(base + i))
	}

	n, err := alloc.Allocate("session-a")
	if err != nil {
		t.Fatalf("Allocate() error = %v, want nil", err)
	}
	if n < base || n >= base+rangeMax {
		t.Errorf("Allocate() = %d, want in [%d, %d)", n, base, base+rangeMax)
	}

	// Lock file must exist.
	if _, err := os.Stat(lockPath(n)); os.IsNotExist(err) {
		t.Errorf("lock file %s does not exist after Allocate()", lockPath(n))
	}

	alloc.Release(n)

	// Lock file must be gone after Release.
	if _, err := os.Stat(lockPath(n)); !os.IsNotExist(err) {
		t.Errorf("lock file %s still exists after Release()", lockPath(n))
	}
}

func TestAllocate_DistinctDisplaysForMultipleSessions(t *testing.T) {
	const base = 210
	const rangeMax = 5
	alloc := NewDisplayAllocator(base, rangeMax)
	defer func() {
		for i := 0; i < rangeMax; i++ {
			os.Remove(lockPath(base + i))
		}
	}()

	n1, err := alloc.Allocate("session-1")
	if err != nil {
		t.Fatalf("first Allocate() error = %v", err)
	}
	n2, err := alloc.Allocate("session-2")
	if err != nil {
		t.Fatalf("second Allocate() error = %v", err)
	}
	if n1 == n2 {
		t.Errorf("both sessions got the same display number %d", n1)
	}

	alloc.Release(n1)
	alloc.Release(n2)
}

func TestAllocate_FullRange_ReturnsError(t *testing.T) {
	const base = 220
	const rangeMax = 3
	alloc := NewDisplayAllocator(base, rangeMax)
	defer func() {
		for i := 0; i < rangeMax; i++ {
			os.Remove(lockPath(base + i))
		}
	}()

	// Pre-claim all displays with fake lock files so Allocate sees them as busy.
	for i := 0; i < rangeMax; i++ {
		path := lockPath(base + i)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err != nil {
			// File already exists (e.g. from a prior test run). That's fine.
			continue
		}
		_, _ = fmt.Fprintf(f, "%10d\n", os.Getpid())
		_ = f.Close()
	}

	_, err := alloc.Allocate("overflow")
	if err == nil {
		t.Error("Allocate() with full range expected error, got nil")
	}
}

func TestRelease_SafeToCallUnallocated(t *testing.T) {
	// Release of a display that was never allocated must not panic.
	alloc := NewDisplayAllocator(230, 5)
	alloc.Release(230) // should be a no-op
}

// ---- CleanupStaleDisplays tests -------------------------------------------------

func TestCleanupStaleDisplays_RemovesStaleFile(t *testing.T) {
	const base = 240
	const rangeMax = 3
	alloc := NewDisplayAllocator(base, rangeMax)
	defer func() {
		for i := 0; i < rangeMax; i++ {
			os.Remove(lockPath(base + i))
		}
	}()

	// Create a stale lock file using a PID that definitely does not exist.
	stalePID := 99999999
	stalePath := lockPath(base)
	f, err := os.OpenFile(stalePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		t.Fatalf("could not create stale lock file: %v", err)
	}
	_, _ = fmt.Fprintf(f, "%10d\n", stalePID)
	_ = f.Close()

	// Verify the stale PID is indeed dead (if 99999999 happens to be alive, skip).
	if isPIDAlive(stalePID) {
		t.Skipf("PID %d is alive on this host; cannot run stale-cleanup test", stalePID)
	}

	alloc.CleanupStaleDisplays()

	// The stale lock file must have been removed.
	if _, statErr := os.Stat(stalePath); !os.IsNotExist(statErr) {
		t.Errorf("stale lock file %s still exists after CleanupStaleDisplays()", stalePath)
	}
}

func TestCleanupStaleDisplays_PreservesLiveFile(t *testing.T) {
	const base = 250
	const rangeMax = 3
	alloc := NewDisplayAllocator(base, rangeMax)
	defer func() {
		for i := 0; i < rangeMax; i++ {
			os.Remove(lockPath(base + i))
		}
	}()

	// Create a lock file with the current (live) PID.
	livePath := lockPath(base)
	f, err := os.OpenFile(livePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		t.Fatalf("could not create live lock file: %v", err)
	}
	_, _ = fmt.Fprintf(f, "%10d\n", os.Getpid())
	_ = f.Close()

	alloc.CleanupStaleDisplays()

	// The live lock file must NOT have been removed.
	if _, statErr := os.Stat(livePath); os.IsNotExist(statErr) {
		t.Errorf("live lock file %s was incorrectly removed by CleanupStaleDisplays()", livePath)
	}
}

func TestCleanupStaleDisplays_MalformedLockFileRemoved(t *testing.T) {
	const base = 260
	const rangeMax = 3
	alloc := NewDisplayAllocator(base, rangeMax)
	defer func() {
		for i := 0; i < rangeMax; i++ {
			os.Remove(lockPath(base + i))
		}
	}()

	// Write a malformed (non-numeric) lock file.
	malformedPath := lockPath(base)
	if err := os.WriteFile(malformedPath, []byte("not-a-pid\n"), 0644); err != nil {
		t.Fatalf("could not create malformed lock file: %v", err)
	}

	alloc.CleanupStaleDisplays()

	// The malformed lock file should have been removed.
	if _, statErr := os.Stat(malformedPath); !os.IsNotExist(statErr) {
		t.Errorf("malformed lock file %s still exists after CleanupStaleDisplays()", malformedPath)
	}
}
