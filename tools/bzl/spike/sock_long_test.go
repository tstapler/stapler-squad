package spike

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

// Binds under TEST_TMPDIR, Bazel's long per-test directory: the "without the short private /tmp"
// case. It is expected to FAIL (tagged manual, so //... never runs it).
func TestUnixSocketUnderTestTmpdir(t *testing.T) {
	sock := filepath.Join(os.Getenv("TEST_TMPDIR"), "ssq-1234.sock")
	t.Logf("TEST_TMPDIR socket=%q len(bytes)=%d", sock, len(sock)+1)
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("bind %s (%d bytes): %v", sock, len(sock)+1, err)
	}
	l.Close()
}
