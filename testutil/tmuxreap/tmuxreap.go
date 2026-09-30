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
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// reapOverallBudget bounds the total wall time ReapLeakedTestServers may
// spend killing leaked sockets, regardless of how many are found. Without
// this, a machine that has accumulated thousands of leaked sockets (each
// kill-server call costs one subprocess fork/exec, ~20-250ms even against a
// dead socket) can make the reap loop itself run for minutes, which SIGQUITs
// the test binary while it's still inside TestMain setup — before a single
// test ever runs. See docs/bugs or the flaky-test backlog item this fixes
// for the empirical timing that produced this number: 50 sockets took ~13s
// serially, i.e. thousands of leaked sockets (observed: 5,386 on one dev
// machine) would take tens of minutes serially. reapMaxConcurrent trades
// that out for parallelism instead of just capping coverage.
const reapOverallBudget = 20 * time.Second

// reapMaxConcurrent bounds how many kill-server subprocesses run at once,
// so a large backlog is reaped in parallel instead of one at a time.
const reapMaxConcurrent = 32

// testSocketPrefixes lists every socket-name prefix created by tests across
// the whole module. Exported via ReapLeakedTestServers/StartTestServerWatchdog
// so every package's TestMain shares one reaper instead of maintaining its
// own copy.
//
// This list used to enumerate each generator's exact prefix one at a time
// (e.g. "test_coldrestore_", "test_recovery_") and kept missing new ones the
// same way every time: a prior duplicate in session/integration_test.go only
// knew about "test_coldrestore_" and silently missed "test-isolated-" (the
// name testSocketOnce in session/tmux generates); later, both
// testutil.CreateIsolatedTmuxServer's "test_<TestName>_<pid>_<n>" and
// session/session_creation_test.go's local testTmuxSocket helper
// ("test_<TestName>_<pid>") went unreaped for the same reason — an arbitrary
// test name between "test_" and the PID means no fixed literal prefix can
// name every case (see BUG-105: 882 leaked sockets accumulated over 8+ days,
// exhausting the stapler-squad.service memory cgroup and causing deploy
// health-check timeouts/rollbacks). Broadened to the bare "test_" prefix so
// any future "test_..."-named generator is covered without another
// whack-a-mole patch; extractTestSocketPID + isProcessAlive below still gate
// every match on the owning PID actually being dead before anything is
// killed, so this stays safe against a real session (which never uses a
// "test_"-prefixed name — see the StillMatchesUnderscorePrefixes test).
var testSocketPrefixes = []string{
	"test_",
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
	reapLeakedSocketFiles(myPID)
	reapOrphanedTestProcesses(myPID)
}

// reapLeakedSocketFiles is ReapLeakedTestServers' file-based sweep: it scans
// the tmux socket directory and kills every leaked entry in parallel, bounded
// by reapOverallBudget/reapMaxConcurrent.
func reapLeakedSocketFiles(myPID int) {
	socketDir := fmt.Sprintf("/tmp/tmux-%d", os.Getuid())
	entries, err := os.ReadDir(socketDir)
	if err != nil {
		return
	}

	overallCtx, cancel := context.WithTimeout(context.Background(), reapOverallBudget)
	defer cancel()
	sem := make(chan struct{}, reapMaxConcurrent)
	var wg sync.WaitGroup
	skipped := 0
	for _, entry := range entries {
		name := entry.Name()
		if !isLeakedTestSocket(myPID, name) {
			continue
		}
		select {
		case <-overallCtx.Done():
			// Ran out of budget: leave the remaining sockets for the next
			// run's reap pass rather than blocking TestMain indefinitely.
			skipped++
			continue
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			defer func() { <-sem }()
			killLeakedSocketFile(overallCtx, name)
		}(name)
	}
	wg.Wait()

	if skipped > 0 {
		fmt.Fprintf(os.Stderr, "tmuxreap: hit %s budget with %d leaked socket(s) still unreaped; will retry on next run\n", reapOverallBudget, skipped)
	}
}

// isLeakedTestSocket reports whether name is a test-prefixed socket safe to
// reap: either it has no embedded PID to check liveness against, or the PID
// it does embed (myPID excepted) belongs to a process that's confirmed dead.
func isLeakedTestSocket(myPID int, name string) bool {
	if !isTestSocketName(name) {
		return false
	}
	ownerPID, ok := extractTestSocketPID(name)
	if !ok {
		return true
	}
	if ownerPID == myPID {
		return false // our own run (shouldn't exist at TestMain start, be safe)
	}
	return !isProcessAlive(ownerPID) // another live test runner — don't interfere
}

// killLeakedSocketFile stops the tmux server behind a leaked socket, if one
// is still running, and removes the socket file.
func killLeakedSocketFile(parent context.Context, name string) {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	_ = safeexec.CommandContext(ctx, tmuxBinary(), "-L", name, "kill-server").Run()
	// kill-server only unlinks the socket when it actually stops a live
	// server. Most leaked sockets here have no server behind them at all (the
	// owning tmux process already exited on its own) — kill-server reports
	// "no server running" and leaves the file in place, which is why this
	// directory accumulates indefinitely otherwise. Safe to remove
	// unconditionally: we've already confirmed the owning test PID is dead,
	// and any server kill-server did stop has already unlinked its own
	// socket.
	_ = os.Remove(filepath.Join(fmt.Sprintf("/tmp/tmux-%d", os.Getuid()), name))
}

// reapOrphanedTestProcesses kills tmux server *processes* matching a known
// test socket prefix even when no socket file exists for them anymore. The
// file-based sweep above is blind to this case: a server killed mid-shutdown
// (e.g. by the machine's own OOM killer, see BUG-105) can unlink its own
// socket and then hang before actually exiting, leaving nothing under
// socketDir to find. Uses `ps` rather than /proc so this works on both Linux
// and macOS. Same liveness rule as the file-based sweep: only kill a server
// whose owning test PID (embedded in its -L socket name) is confirmed dead.
func reapOrphanedTestProcesses(myPID int) {
	out, err := safeexec.CommandContext(context.Background(), "ps", "-eo", "pid=,args=").Output()
	if err != nil {
		return
	}

	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		fields := strings.SplitN(line, " ", 2)
		if len(fields) != 2 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil || pid == myPID {
			continue
		}

		socketName, ok := tmuxDashLSocketName(fields[1])
		if !ok || !isTestSocketName(socketName) {
			continue
		}

		if ownerPID, ok := extractTestSocketPID(socketName); ok {
			if ownerPID == myPID || isProcessAlive(ownerPID) {
				continue // owning test run is still alive — don't interfere
			}
		}

		if p, err := os.FindProcess(pid); err == nil {
			_ = p.Kill()
		}
	}
}

// tmuxDashLSocketName extracts the socket name from a tmux command line's
// "-L <name>" argument, e.g. "tmux -L test_Foo_123 start-server" -> "test_Foo_123".
func tmuxDashLSocketName(args string) (string, bool) {
	if !strings.Contains(args, "tmux") {
		return "", false
	}
	idx := strings.Index(args, "-L ")
	if idx == -1 {
		return "", false
	}
	rest := strings.Fields(args[idx+len("-L "):])
	if len(rest) == 0 {
		return "", false
	}
	return rest[0], true
}

// tmuxBinary mirrors session/tmux's Binary() env-var check (TMUX_BIN) without
// importing that package, which would reintroduce the import cycle this
// package exists to avoid. Keeps kill-server on the same TMUX_BIN-pinned
// binary that created the socket, consistent with the convention commit
// dccee742a applied to the rest of the test suite for the same reason
// (avoiding cross-version protocol mismatches against a pinned test build).
func tmuxBinary() string {
	if bin := os.Getenv("TMUX_BIN"); bin != "" {
		return bin
	}
	return "tmux"
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
            *_${PID}_*|*-${PID}) "${TMUX_BIN:-tmux}" -L "$name" kill-server 2>/dev/null; true ;;
        esac
    done
fi
rm -f "$0"
`, ownerPID, uid, ownerPID)
	if err := os.WriteFile(scriptPath, []byte(script), 0600); err != nil {
		return // best-effort; normal t.Cleanup handles the happy path
	}
	// #nosec G204 -- scriptPath is a fixed path this function just wrote itself above, not external input.
	cmd := exec.CommandContext(context.Background(), "sh", scriptPath) //nolint:norawexec long-running cmd.Start() process; lifecycle managed by caller
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}              // own process group → survives SIGKILL to test binary
	_ = cmd.Start()
}
