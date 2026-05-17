package vnc

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

// DisplayAllocator manages X11 display number allocation using the standard
// X11 lock-file protocol. Each allocated display number gets an exclusive lock
// at /tmp/.X<N>-lock. The POSIX O_EXCL flag makes the allocation atomic.
type DisplayAllocator struct {
	mu          sync.Mutex
	allocated   map[int]string // display number → session ID
	base        int
	rangeMax    int
}

// NewDisplayAllocator creates a DisplayAllocator that searches display numbers
// in [base, base+rangeMax).
func NewDisplayAllocator(base, rangeMax int) *DisplayAllocator {
	return &DisplayAllocator{
		allocated: make(map[int]string),
		base:      base,
		rangeMax:  rangeMax,
	}
}

// lockPath returns the standard X11 lock file path for display number n.
func lockPath(n int) string {
	return fmt.Sprintf("/tmp/.X%d-lock", n)
}

// Allocate finds an unused display number, creates the X11 lock file with
// O_EXCL to claim it atomically, and records the allocation. Returns the
// allocated display number or an error if none are free.
func (d *DisplayAllocator) Allocate(sessionID string) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for i := 0; i < d.rangeMax; i++ {
		n := d.base + i
		if _, inUse := d.allocated[n]; inUse {
			continue
		}

		path := lockPath(n)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err != nil {
			// Lock file already exists (either we didn't create it, or another
			// process holds it). Skip this display number.
			continue
		}
		// Write our PID into the lock file so orphan cleanup can identify us.
		_, _ = fmt.Fprintf(f, "%10d\n", os.Getpid())
		_ = f.Close()

		d.allocated[n] = sessionID
		return n, nil
	}

	return 0, fmt.Errorf("vnc: no free display numbers in range [%d, %d)", d.base, d.base+d.rangeMax)
}

// Release removes the Go-level allocation record and deletes the X11 lock file
// for display number n. It is safe to call even if the display was never
// successfully allocated by this process (it is a no-op in that case).
func (d *DisplayAllocator) Release(n int) {
	d.mu.Lock()
	defer d.mu.Unlock()

	delete(d.allocated, n)
	_ = os.Remove(lockPath(n))
}

// CleanupStaleDisplays scans all display numbers in the allocator's range and
// removes lock files whose recorded PID is no longer alive. This should be
// called at startup before any allocations to reclaim displays left behind by
// crashed processes.
func (d *DisplayAllocator) CleanupStaleDisplays() {
	d.mu.Lock()
	defer d.mu.Unlock()

	for i := 0; i < d.rangeMax; i++ {
		n := d.base + i
		if _, ours := d.allocated[n]; ours {
			// Skip displays we own in this process.
			continue
		}

		path := lockPath(n)
		data, err := os.ReadFile(path)
		if err != nil {
			// File doesn't exist or unreadable — nothing to clean up.
			continue
		}

		pid, err := parseLockPID(data)
		if err != nil {
			// Malformed lock file — remove it.
			_ = os.Remove(path)
			continue
		}

		if !isPIDAlive(pid) {
			// Process is dead; reclaim the lock file.
			_ = os.Remove(path)
		}
	}
}

// parseLockPID extracts the PID from the content of an X11 lock file.
// The standard format is a right-justified 10-character PID followed by a newline.
func parseLockPID(data []byte) (int, error) {
	s := strings.TrimSpace(string(data))
	return strconv.Atoi(s)
}

// isPIDAlive returns true if the process with the given PID is still running.
// It uses os.FindProcess (always succeeds on Unix) and sends signal 0 (a no-op
// that merely checks whether the process exists and we have permission to signal it).
func isPIDAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal(0) is a POSIX probe: returns nil if the process exists and we can
	// signal it, ESRCH if it does not exist. EPERM means it exists but we lack
	// permission to signal it — the process is still alive, so treat as alive
	// to be conservative and avoid deleting a live process's lock file.
	err = p.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	return errors.Is(err, syscall.EPERM)
}
