package mux

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestListStaplerSquadSessions_UsesIsolatedSocket and its siblings below are the
// regression guard for the ssq-mux enumerate-all call sites (list-sessions) that
// previously had no socket argument at all, always targeting the shared,
// machine-wide default tmux socket -- the same class of bug closed for the
// session package's list-sessions/kill-session call sites via tmux.ResolveSocket.
// They replace the tmux binary with a fake script that records its argv and assert
// the call is socket-scoped to the per-process isolated socket rather than the
// shared default.
func TestListStaplerSquadSessions_UsesIsolatedSocket(t *testing.T) {
	logPath := installFakeTmuxBinaryForMuxTest(t)

	_, _ = ListStaplerSquadSessions()

	assertMuxInvocationUsesIsolatedSocket(t, logPath, "list-sessions")
}

func TestListStaplerSquadSessionsWithInfo_UsesIsolatedSocket(t *testing.T) {
	logPath := installFakeTmuxBinaryForMuxTest(t)

	_, _ = ListStaplerSquadSessionsWithInfo()

	assertMuxInvocationUsesIsolatedSocket(t, logPath, "list-sessions")
}

func TestScanByUserOptions_UsesIsolatedSocket(t *testing.T) {
	logPath := installFakeTmuxBinaryForMuxTest(t)

	_, _ = ScanByUserOptions()

	assertMuxInvocationUsesIsolatedSocket(t, logPath, "list-sessions")
}

// installFakeTmuxBinaryForMuxTest points TMUX_BIN (auto-restored by t.Setenv) at a
// script that appends its argv to a log file and exits non-zero, so callers
// gracefully return empty results. Returns the log path.
func installFakeTmuxBinaryForMuxTest(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "argv.log")
	fakeTmux := filepath.Join(dir, "tmux")

	script := "#!/bin/sh\necho \"$@\" >> \"" + logPath + "\"\nexit 1\n"
	if err := os.WriteFile(fakeTmux, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to write fake tmux binary: %v", err)
	}
	t.Setenv("TMUX_BIN", fakeTmux)
	return logPath
}

// assertMuxInvocationUsesIsolatedSocket asserts at least one invocation containing
// wantSubcommand was logged, and that every invocation logged is socket-scoped to the
// per-process isolated socket rather than falling through to the shared default.
func assertMuxInvocationUsesIsolatedSocket(t *testing.T, logPath string, wantSubcommand string) {
	t.Helper()
	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("expected the fake tmux binary to have been invoked: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(logBytes)), "\n")
	if len(lines) == 0 {
		t.Fatalf("expected at least one tmux invocation, got none")
	}

	sawWantedSubcommand := false
	for _, line := range lines {
		if !strings.HasPrefix(line, "-L ") {
			t.Fatalf("expected every invocation to be socket-scoped with -L, got: %q", line)
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.Contains(fields[1], "test-isolated-") {
			t.Fatalf("expected the per-process isolated socket, not the shared default, got: %q", line)
		}
		if strings.Contains(line, wantSubcommand) {
			sawWantedSubcommand = true
		}
	}
	if !sawWantedSubcommand {
		t.Fatalf("expected an invocation containing %q, got: %v", wantSubcommand, lines)
	}
}
