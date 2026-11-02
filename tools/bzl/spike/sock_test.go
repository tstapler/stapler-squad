// Throwaway S9 probe: binds a unix socket under os.TempDir() the way the repo's mux/tymux/tmux
// tests do. sun_path holds at most 107 bytes plus NUL, so a long TMPDIR makes the bind fail.
package spike

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestUnixSocketUnderTempDir(t *testing.T) {
	sock := filepath.Join(os.TempDir(), "ssq-1234.sock")
	t.Logf("TMPDIR=%q TEST_TMPDIR=%q socket=%q len(bytes)=%d", os.Getenv("TMPDIR"), os.Getenv("TEST_TMPDIR"), sock, len(sock)+1)
	_ = os.Remove(sock)
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("bind %s (%d bytes incl. newline-free path+NUL): %v", sock, len(sock)+1, err)
	}
	defer l.Close()
	defer os.Remove(sock)
}
