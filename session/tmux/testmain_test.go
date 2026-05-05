//go:build !windows

package tmux

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// testSocketPrefixes lists every socket-name prefix created by tests in this
// package. TestMain uses them to reap servers left over from a previous run
// that was killed with SIGKILL (which prevents t.Cleanup from running).
var testSocketPrefixes = []string{
	"test_coldrestore_",
	"test_ensure_noop_",
	"test_ensure_start_",
	"test_exit_empty_",
	"test_keepalive_",
	"test_recovery_",
	"integration_",
}

func TestMain(m *testing.M) {
	reapLeakedTestServers()
	startWatchdog(os.Getpid())
	os.Exit(m.Run())
}

// reapLeakedTestServers kills tmux servers whose socket names match a known
// test prefix AND whose owner PID is no longer alive.  Sockets owned by a
// live PID (another concurrent test runner) are left alone.
func reapLeakedTestServers() {
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
// Convention: each generator embeds os.Getpid() as a numeric segment.
// PID range on this system is [2, 4194304); nanosecond timestamps and
// rand.Int63() values are always >> pidMax, so the check is unambiguous.
func extractTestSocketPID(name string) (int, bool) {
	const pidMax = 4194304 // /proc/sys/kernel/pid_max on this system
	for _, part := range strings.Split(name, "_") {
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

// startWatchdog spawns a detached shell process that polls ownerPID and kills
// all test-prefixed tmux sockets bearing that PID when the process exits.
// The watchdog runs in its own process group so it survives SIGKILL to the
// test binary, covering the case where go test -timeout fires.
func startWatchdog(ownerPID int) {
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
    for f in "$SOCKDIR"/test_* "$SOCKDIR"/integration_*; do
        [ -S "$f" ] || continue
        name=$(basename "$f")
        case "$name" in
            *_${PID}_*) tmux -L "$name" kill-server 2>/dev/null; true ;;
        esac
    done
fi
rm -f "$0"
`, ownerPID, uid, ownerPID)
	if err := os.WriteFile(scriptPath, []byte(script), 0700); err != nil {
		return // best-effort; normal t.Cleanup handles the happy path
	}
	cmd := exec.CommandContext(context.Background(), "sh", scriptPath) //nolint:norawexec long-running cmd.Start() process; lifecycle managed by caller
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // own process group → survives SIGKILL to test binary
	_ = cmd.Start()
}
