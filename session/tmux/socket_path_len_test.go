package tmux

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// maxHarnessSocketPath is the plan's budget (Task 4.5b); the kernel limit is 107 bytes plus NUL.
const maxHarnessSocketPath = 100

// bazelTestTmpdirLen is the measured length of a sandboxed $TEST_TMPDIR socket path (spike S9: 169 bytes).
const bazelTestTmpdirLen = 150

// tmuxSocketPath mirrors tmux's own rule: $TMUX_TMPDIR (else /tmp)/tmux-<uid>/<name>.
// TMPDIR and TEST_TMPDIR play no part, so only TMUX_TMPDIR can lengthen it.
func tmuxSocketPath(tmuxTmpdir string, uid int, name string) string {
	if tmuxTmpdir == "" {
		tmuxTmpdir = "/tmp"
	}
	return filepath.Join(tmuxTmpdir, fmt.Sprintf("tmux-%d", uid), name)
}

func TestSocketPathLen_LongestNameUnderDefaultDir(t *testing.T) {
	t.Parallel()
	longest := strings.Repeat("a", maxEnvSocketLen)
	if err := ValidateEnvSocket(longest); err != nil {
		t.Fatalf("longest accepted name must validate: %v", err)
	}
	const maxUID = 4294967295
	for _, name := range []string{longest, testSocketOnce(), "ssq-e2e-0123456789abcdef"} {
		p := tmuxSocketPath("", maxUID, name)
		if len(p)+1 > maxHarnessSocketPath {
			t.Errorf("socket path %q is %d bytes (+NUL), budget %d", p, len(p)+1, maxHarnessSocketPath)
		}
	}
}

// The harness environment must not point TMUX_TMPDIR under a Bazel-length TEST_TMPDIR.
func TestSocketPathLen_HarnessEnvironment(t *testing.T) {
	t.Parallel()
	p := tmuxSocketPath(os.Getenv("TMUX_TMPDIR"), os.Getuid(), strings.Repeat("a", maxEnvSocketLen))
	if len(p)+1 > maxHarnessSocketPath {
		t.Fatalf("with TMUX_TMPDIR=%q (TEST_TMPDIR=%q) the longest socket path is %d bytes (+NUL), budget %d",
			os.Getenv("TMUX_TMPDIR"), os.Getenv("TEST_TMPDIR"), len(p)+1, maxHarnessSocketPath)
	}
}

// Negative control: a Bazel-length TEST_TMPDIR used as TMUX_TMPDIR would break the budget,
// so the checks above are not vacuous.
func TestSocketPathLen_BazelLengthTmpdirWouldExceed(t *testing.T) {
	t.Parallel()
	base := "/" + strings.Repeat("d", bazelTestTmpdirLen-1)
	p := tmuxSocketPath(base, 1000, testSocketOnce())
	if len(p)+1 <= maxHarnessSocketPath {
		t.Fatalf("expected %d-byte base to exceed the budget, got %d bytes", bazelTestTmpdirLen, len(p)+1)
	}
}
