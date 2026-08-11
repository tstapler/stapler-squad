package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestReconcileOrphanedTmuxSessions_UsesIsolatedSocket is the regression guard for the
// incident where ReconcileOrphanedTmuxSessions issued raw `tmux list-sessions` /
// `show-environment` / `kill-session` with no socket argument, always targeting the
// shared, machine-wide default tmux socket. That meant any test process calling
// BuildDependencies() on the same machine as a running production instance would
// enumerate and kill *that* instance's sessions. This replaces the tmux binary with a
// fake script that records its argv and asserts every invocation is scoped to the
// resolved per-process isolated socket rather than the shared default.
func TestReconcileOrphanedTmuxSessions_UsesIsolatedSocket(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "argv.log")
	fakeTmux := filepath.Join(dir, "tmux")

	script := `#!/bin/sh
echo "$@" >> "` + logPath + `"
case "$*" in
  *list-sessions*) echo "staplersquad_orphan-test"; exit 0 ;;
  *show-environment*) exit 1 ;;
  *kill-session*) exit 0 ;;
esac
exit 1
`
	require.NoError(t, os.WriteFile(fakeTmux, []byte(script), 0o755))
	t.Setenv("TMUX_BIN", fakeTmux)

	// No known instances -- the fake "staplersquad_orphan-test" session is an orphan,
	// which drives all three call sites (list-sessions, show-environment, kill-session).
	ReconcileOrphanedTmuxSessions(nil)

	logBytes, err := os.ReadFile(logPath)
	require.NoError(t, err, "expected the fake tmux binary to have been invoked")

	lines := strings.Split(strings.TrimSpace(string(logBytes)), "\n")
	require.GreaterOrEqual(t, len(lines), 3, "expected list-sessions, show-environment, and kill-session calls")

	for _, line := range lines {
		require.True(t, strings.HasPrefix(line, "-L "), "every tmux invocation must be socket-scoped, got: %q", line)
		fields := strings.Fields(line)
		require.GreaterOrEqual(t, len(fields), 2)
		socket := fields[1]
		require.NotEmpty(t, socket, "socket must never resolve to empty and fall through to the shared default")
		require.Contains(t, socket, "test-isolated-", "socket must be the per-process isolated socket, not the shared default")
	}
}

// TestReconcileOrphanedTmuxSessions_PreservesLiveShellSessions is the regression guard
// for the bug where shell sibling tmux sessions (staplersquad_{parent}_shell_{shellID})
// were unconditionally treated as orphans: the sweep only ever checked instance-level
// tmux names/UUIDs and never looked at an instance's shells, so a freshly-spawned shell
// with no Instance-level identity of its own was killed on the very next sweep/restart.
func TestReconcileOrphanedTmuxSessions_PreservesLiveShellSessions(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "argv.log")
	fakeTmux := filepath.Join(dir, "tmux")

	script := `#!/bin/sh
echo "$@" >> "` + logPath + `"
case "$*" in
  *list-sessions*) echo "staplersquad_parent_shell_shell-1"; exit 0 ;;
  *show-environment*) exit 1 ;;
  *kill-session*) exit 0 ;;
esac
exit 1
`
	require.NoError(t, os.WriteFile(fakeTmux, []byte(script), 0o755))
	t.Setenv("TMUX_BIN", fakeTmux)

	inst := &Instance{}
	inst.initShellRegistry()
	inst.shells.AddStopped(&Shell{
		ID:              "shell-1",
		TmuxSessionName: "staplersquad_parent_shell_shell-1",
	})

	ReconcileOrphanedTmuxSessions([]*Instance{inst})

	logBytes, err := os.ReadFile(logPath)
	require.NoError(t, err)

	logStr := string(logBytes)
	require.Contains(t, logStr, "list-sessions", "expected list-sessions to have been called")
	require.NotContains(t, logStr, "kill-session", "the known shell session must not be killed")
}
