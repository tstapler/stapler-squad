//go:build !windows

// Package tmuxreap reaps tmux servers left behind by test binaries that were
// SIGKILLed before their t.Cleanup handlers could run. It has no dependency
// on session/session-mux/session-tmux so their internal (same-package) test
// files can import it without an import cycle — testutil itself already
// imports all three, which internal test files of those packages cannot.
package tmuxreap

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// testSocketPrefixes lists every socket-name prefix created by tests across
// the whole module (session, session/mux, session/tmux). Exported via
// ReapLeakedTestServers/StartTestServerWatchdog so every package's TestMain
// shares one reaper instead of maintaining its own copy — a prior duplicate
// in session/integration_test.go only knew about "test_coldrestore_" and
// silently missed "test-isolated-" (the name testSocketOnce in session/tmux
// actually generates), letting orphaned isolated servers from a SIGKILLed
// test binary accumulate indefinitely instead of being reaped on the next
// run.
var testSocketPrefixes = []string{
	"test_coldrestore_",
	"test_ensure_noop_",
	"test_ensure_start_",
	"test_exit_empty_",
	"test_keepalive_",
	"test_recovery_",
	"integration_",
	"test-isolated-",
}

// ReapLeakedTestServers kills tmux servers whose socket names match a known
// test prefix AND whose owner PID is no longer alive. Sockets owned by a
// live PID (another concurrent test runner) are left alone. Call this from
// TestMain in any package whose tests create tmux servers via a test-prefixed
// socket name.
func ReapLeakedTestServers() {
	myPID := os.Getpid()
	socketDir := fmt.Sprintf("/tmp/tmux-%d", os.Getuid())
	entries, err := os.ReadDir(socketDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if !isTestSocketName(name) {
			continue
		}
		ownerPID, ok := extractTestSocketPID(name)
		if ok {
			if ownerPID == myPID {
				continue // our own run (shouldn't exist at TestMain start, be safe)
			}
			if isProcessAlive(ownerPID) {
				continue // another live test runner — don't interfere
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = safeexec.CommandContext(ctx, "tmux", "-L", name, "kill-server").Run()
		cancel()
	}
}

// extractTestSocketPID finds the PID embedded in a test socket name.
// Convention: each generator embeds os.Getpid() as a numeric segment,
// delimited by "_" (e.g. "test_recovery_1234_5678") or "-" (e.g.
// "test-isolated-1234"). PID range on this system is [2, 4194304);
// nanosecond timestamps and rand.Int63() values are always >> pidMax, so the
// check is unambiguous.
func extractTestSocketPID(name string) (int, bool) {
	const pidMax = 4194304 // /proc/sys/kernel/pid_max on this system
	for _, part := range strings.FieldsFunc(name, func(r rune) bool { return r == '_' || r == '-' }) {
		n, err := strconv.Atoi(part)
		if err == nil && n >= 2 && n < pidMax {
			return n, true
		}
	}
	return 0, false
}

func isProcessAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

func isTestSocketName(name string) bool {
	for _, prefix := range testSocketPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// StartTestServerWatchdog spawns a detached shell process that polls
// ownerPID and kills all test-prefixed tmux sockets bearing that PID when
// the process exits. The watchdog runs in its own process group so it
// survives SIGKILL to the test binary, covering the case where go test
// -timeout fires.
func StartTestServerWatchdog(ownerPID int) {
	uid := os.Getuid()
	scriptPath := fmt.Sprintf("/tmp/tmux-test-watchdog-%d.sh", ownerPID)
	script := fmt.Sprintf(`#!/bin/sh
# Watchdog: kills test tmux sockets for PID %d when that process exits.
SOCKDIR=/tmp/tmux-%d
PID=%d
while kill -0 "$PID" 2>/dev/null; do
    sleep 1
done
if [ -d "$SOCKDIR" ]; then
    for f in "$SOCKDIR"/test_* "$SOCKDIR"/integration_* "$SOCKDIR"/test-isolated-*; do
        [ -S "$f" ] || continue
        name=$(basename "$f")
        case "$name" in
            *_${PID}_*|*-${PID}) tmux -L "$name" kill-server 2>/dev/null; true ;;
        esac
    done
fi
rm -f "$0"
`, ownerPID, uid, ownerPID)
	if err := os.WriteFile(scriptPath, []byte(script), 0700); err != nil {
		return // best-effort; normal t.Cleanup handles the happy path
	}
	cmd := exec.CommandContext(context.Background(), "sh", scriptPath) //nolint:norawexec long-running cmd.Start() process; lifecycle managed by caller
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}              // own process group → survives SIGKILL to test binary
	_ = cmd.Start()
}
