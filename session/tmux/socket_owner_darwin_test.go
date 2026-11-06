//go:build darwin && cgo

package tmux

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

// TestFindSocketOwnerPID_FindsSelf opens a real unix socket listener in this
// test process itself (no dependency on any pre-existing tmux server, so
// this stays deterministic in CI) and confirms findSocketOwnerPID reports
// this process's own PID for it -- exercising the full
// proc_pidinfo/proc_pidfdinfo(PROC_PIDFDSOCKETINFO) cgo path end-to-end,
// including the union-unwrapping unsafe.Pointer casts (see
// pidHasUnixSocketBoundTo's doc comment) against this machine's real SDK
// struct layout.
func TestFindSocketOwnerPID_FindsSelf(t *testing.T) {
	// A short, non-t.TempDir()-nested directory: sun_path has a ~104-byte
	// limit on macOS, and t.TempDir() nests under the full test name (see
	// session/integration_test.go's uniqueTestTmuxSocket doc comment for the
	// same constraint hitting tmux socket names directly).
	dir, err := os.MkdirTemp("", "sockowner")
	if err != nil {
		t.Fatalf("os.MkdirTemp() error = %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	sockPath := filepath.Join(dir, "t.sock")
	l, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("net.Listen(unix, %q) error = %v", sockPath, err)
	}
	defer l.Close()

	pid, ok := findSocketOwnerPID(sockPath)
	if !ok {
		t.Fatalf("findSocketOwnerPID(%q) = (_, false), want to find this process's own socket", sockPath)
	}
	if pid != int32(os.Getpid()) {
		t.Errorf("findSocketOwnerPID(%q) pid = %d, want this process's own pid %d", sockPath, pid, os.Getpid())
	}
}

// TestFindSocketOwnerPID_NoMatchingSocket confirms a clean "not found"
// (never an error) for a path nothing is listening on.
func TestFindSocketOwnerPID_NoMatchingSocket(t *testing.T) {
	dir, err := os.MkdirTemp("", "sockowner")
	if err != nil {
		t.Fatalf("os.MkdirTemp() error = %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sockPath := filepath.Join(dir, "nothing.sock")
	if _, ok := findSocketOwnerPID(sockPath); ok {
		t.Errorf("findSocketOwnerPID(%q) = (_, true), want false when nothing is listening", sockPath)
	}
}
