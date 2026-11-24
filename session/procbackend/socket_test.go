package procbackend

import (
	"fmt"
	"hash/fnv"
	"os"
	"testing"
)

// testTmuxSocket returns a per-test private tmux socket name. t.Name() is hashed
// (not embedded) to keep the Unix socket path short, and the "test_" prefix is
// what testutil/tmuxreap sweeps.
func testTmuxSocket(t *testing.T) string {
	t.Helper()
	h := fnv.New32a()
	_, _ = h.Write([]byte(t.Name()))
	return fmt.Sprintf("test_ssq_%08x_%d", h.Sum32(), os.Getpid())
}
