package tmux

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/shirou/gopsutil/v4/process"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
)

// resolvedClientBinaries memoizes ResolveClientForSocket's answer per socket
// (LoadOrStore key: the resolved socket string). The answer cannot change
// without a server restart, mirroring versionCheckedSockets in
// version_check.go -- ClearResolvedClientForSocket (called by
// RestartTmuxServer) invalidates it the same way.
var resolvedClientBinaries sync.Map // socket string -> string (binary path)

// ResolveClientForSocket returns the tmux client binary to use for commands
// targeting serverSocket, so this process's own client always speaks the
// same protocol version as whatever server it's actually about to talk to.
//
// TMUX_BIN, if set, always wins -- unchanged from Binary()'s own precedent,
// for tests and operators who want to pin a specific binary regardless of
// what's already running.
//
// Otherwise: if a tmux server is ALREADY running on serverSocket (e.g. a
// long-lived shared default-socket server this process didn't start), this
// looks up that server's own executable via the OS process table
// (findServerBinaryForSocket) and uses it -- an exact version match with no
// need to ask tmux itself. Asking tmux itself (a `display-message` etc.
// through the pinned/embedded binary) is NOT reliable here: version_check.go
// documents a narrow client/server gap (BUG-101) where ordinary commands
// still worked and only control mode broke, but a wider gap can break every
// command, not just control mode -- confirmed 2026-09-16 (titus-soaktest-followup
// incident: this process's pinned tmux 3.4 against a 20-day-old shared
// default-socket server running tmux 3.7b failed "server exited
// unexpectedly" on ordinary list-sessions/start-server calls, not just
// control-mode attach).
//
// Falls back to Binary() (the pinned/embedded binary) when no existing
// server is found on serverSocket, since this process will then be the one
// starting it -- no mismatch is possible for a server this process creates
// itself. Also falls back to Binary() when findSocketOwnerPID isn't
// implemented for this platform/build (see socket_owner_other.go) -- no
// subprocess is ever shelled out to as a substitute; the OS-native lookup
// (socket_owner_darwin.go's proc_pidinfo/proc_pidfdinfo syscalls,
// socket_owner_linux.go's /proc reads) is the only mechanism, and where it
// isn't available this simply degrades to the pre-existing behavior.
func ResolveClientForSocket(serverSocket string) string {
	if bin := os.Getenv("TMUX_BIN"); bin != "" {
		return bin
	}
	// Under test, every socket is a freshly minted, per-test/per-process
	// name (see ResolveSocket's own testSocketOnce branch) -- there is never
	// an already-running server this process didn't start, so the scenario
	// this function exists for cannot occur. Skipping the OS-level lookup
	// keeps the whole test suite from doing real process-table scans on
	// every uncached socket on every buildTmuxCommandContext call
	// (deterministic, fast) -- see the deterministic-fast-tests skill's
	// rationale for avoiding real-subprocess/real-syscall dependence in unit
	// tests wherever the code under test doesn't require it.
	if config.IsTestMode() {
		return Binary()
	}
	resolved := string(ResolveSocket(serverSocket))
	if cached, ok := resolvedClientBinaries.Load(resolved); ok {
		return cached.(string)
	}

	bin := Binary()
	if path, ok := findServerBinaryForSocket(resolved); ok {
		if path != bin {
			log.Info("tmux: matching client to already-running server's binary",
				"socket", resolved, "pinnedBinary", bin, "serverBinary", path)
		}
		bin = path
	}
	resolvedClientBinaries.Store(resolved, bin)
	return bin
}

// ClearResolvedClientForSocket forgets a previously memoized
// ResolveClientForSocket answer for serverSocket, so the next call
// re-resolves against whatever server is running (or gets started) next.
// Called by RestartTmuxServer alongside its own version-check cache clear.
func ClearResolvedClientForSocket(serverSocket string) {
	resolvedClientBinaries.Delete(string(ResolveSocket(serverSocket)))
}

// findServerBinaryForSocket asks the OS -- not tmux -- which executable is
// listening on the unix socket backing serverSocket. Returns false (falling
// back to Binary() at the call site) on any failure: no server running yet,
// no platform-specific findSocketOwnerPID implementation, or the owning
// process's executable can't be resolved. Never a hard error -- this is a
// best-effort optimization, not a correctness requirement.
func findServerBinaryForSocket(socket string) (string, bool) {
	sockPath, err := tmuxSocketFilePath(socket)
	if err != nil {
		return "", false
	}
	// The OS reports a socket's bound path fully resolved (e.g.
	// "/private/tmp/..." on macOS, where "/tmp" is itself a symlink to
	// "/private/tmp") -- comparing against the unresolved conventional path
	// would silently never match. EvalSymlinks fails when the socket doesn't
	// exist yet (no server running), which findSocketOwnerPID would also
	// correctly report as "not found", so falling back to the unresolved
	// path here changes nothing observable in that case.
	if resolved, err := filepath.EvalSymlinks(sockPath); err == nil {
		sockPath = resolved
	}
	pid, ok := findSocketOwnerPID(sockPath)
	if !ok {
		return "", false
	}
	proc, err := process.NewProcess(pid)
	if err != nil {
		return "", false
	}
	exePath, err := proc.Exe()
	if err != nil || filepath.Base(exePath) != "tmux" {
		return "", false
	}
	return exePath, true
}

// tmuxSocketFilePath reproduces tmux's own default socket-path convention
// (TMUX_TMPDIR, else the hardcoded "/tmp" tmux itself uses -- NOT
// os.TempDir(), which honors $TMPDIR and would resolve to the wrong
// directory on macOS, where $TMPDIR is a per-user /var/folders path distinct
// from tmux's own hardcoded /tmp). Mirrors
// testutil/tmuxreap.go's reapLeakedSocketFiles, the only other place in this
// codebase that already needs to compute this path.
func tmuxSocketFilePath(socket string) (string, error) {
	name := socket
	if name == "" {
		name = "default"
	}
	dir := os.Getenv("TMUX_TMPDIR")
	if dir == "" {
		dir = "/tmp"
	}
	return filepath.Join(dir, fmt.Sprintf("tmux-%d", os.Getuid()), name), nil
}
