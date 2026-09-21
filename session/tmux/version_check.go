package tmux

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// versionCheckedSockets tracks sockets whose client/server tmux version has
// already been compared once (LoadOrStore key: string(Socket)). The answer
// cannot change without a server restart, and --tmux-keep-server deliberately
// keeps the server alive across this process's own restarts, so re-checking
// on every StartControlMode call would be pure waste. RestartTmuxServer
// deletes the entry for its socket so the next StartControlMode re-checks.
var versionCheckedSockets sync.Map

// VersionMismatch describes one socket where this process's tmux client
// version disagrees with the already-running server's version.
type VersionMismatch struct {
	ServerSocket  string
	ClientVersion string
	ServerVersion string
}

// controlModeDisabledSockets holds every socket where a client/server tmux
// version mismatch was detected (value type VersionMismatch). StartControlMode
// consults this to skip spawning a doomed control-mode client for the rest of
// this process's lifetime, rather than re-discovering the same mismatch (and
// paying its ctx-timeout cost) on every single command.
var controlModeDisabledSockets sync.Map

// normalizeTmuxVersion strips `tmux -V`'s "tmux " prefix (display-message's
// #{version} format variable never has one) and surrounding whitespace, so
// both sides of a client/server comparison land in the same shape.
func normalizeTmuxVersion(raw string) string {
	return strings.TrimPrefix(strings.TrimSpace(raw), "tmux ")
}

// checkControlModeVersionMatchOnce compares this process's resolved tmux
// client version (`Binary() -V`) against the ALREADY-RUNNING server's own
// reported version (`display-message -p '#{version}'`, targeted at this
// session -- it must already exist on t.serverSocket for the query to
// succeed). Memoized per socket via versionCheckedSockets.
//
// A mismatch here reproduces a confirmed, previously-invisible production
// incident (see docs/bugs/open/BUG-101-...): a newer tmux -C attach-session
// CLIENT against an older SERVER (or vice versa) completes the OS-level
// attach but the control-mode text protocol layer never exchanges a single
// %begin/%end -- every control-mode command silently times out at its ctx
// deadline for as long as that server process lives (confirmed live: 100%
// of control_mode commands timed out for over 24h of metric history, root
// cause was a tmux 3.6a client against a --tmux-keep-server-preserved tmux
// 3.4 server from an earlier embed_tmux deploy). Ordinary (non-control-mode)
// commands are unaffected -- a plain one-shot `display-message` succeeds
// across the same version gap -- which is why this check works at all, and
// why the existing subprocess fallbacks keep the terminal usable once
// control mode is disabled here.
func (t *TmuxSession) checkControlModeVersionMatchOnce(ctx context.Context) {
	socketKey := string(t.serverSocket)
	if _, already := versionCheckedSockets.LoadOrStore(socketKey, struct{}{}); already {
		return
	}

	clientCmd := t.buildTmuxCommandContext(ctx, "-V")
	clientOut, err := t.cmdExec.Output(clientCmd)
	if err != nil {
		log.Warn("tmux version check: failed to get client version, skipping mismatch check", "session", t.sanitizedName, "err", err)
		return
	}

	// A failure here almost always means no server is running yet on this
	// socket (this process is about to start a brand new one) -- the common
	// case, not worth alarming on.
	serverCmd := t.buildTmuxCommandContext(ctx, "display-message", "-p", "-t", t.sanitizedName, "#{version}")
	serverOut, err := t.cmdExec.Output(serverCmd)
	if err != nil {
		log.Debug("tmux version check: no existing server to compare against yet", "session", t.sanitizedName, "err", err)
		return
	}

	clientVer := normalizeTmuxVersion(string(clientOut))
	serverVer := normalizeTmuxVersion(string(serverOut))
	if clientVer == "" || serverVer == "" || clientVer == serverVer {
		return
	}

	controlModeDisabledSockets.Store(socketKey, VersionMismatch{
		ServerSocket:  socketKey,
		ClientVersion: clientVer,
		ServerVersion: serverVer,
	})
	log.Error("tmux control-mode client/server version mismatch detected -- control mode disabled for this server, falling back to slower per-command subprocess calls",
		"session", t.sanitizedName,
		"client_version", clientVer,
		"server_version", serverVer,
		"remediation", "restart the tmux server on a matching tmux version (this kills its sessions), or set TMUX_BIN to the binary path the running server was actually started with")
}

// controlModeDisabledForSocket reports whether checkControlModeVersionMatchOnce
// found a version mismatch for this session's server socket.
func (t *TmuxSession) controlModeDisabledForSocket() bool {
	_, disabled := controlModeDisabledSockets.Load(string(t.serverSocket))
	return disabled
}

// GetVersionMismatches returns every currently-known tmux client/server
// version mismatch, across every socket any TmuxSession in this process has
// checked.
func GetVersionMismatches() []VersionMismatch {
	var out []VersionMismatch
	controlModeDisabledSockets.Range(func(_, value any) bool {
		if vm, ok := value.(VersionMismatch); ok {
			out = append(out, vm)
		}
		return true
	})
	return out
}

// RestartTmuxServer kills the tmux server on serverSocket (killing every
// session currently attached to it) and clears this process's cached
// version-check/disabled state for that socket, so the next StartControlMode
// call re-checks against whatever server starts next -- expected to succeed
// since a fresh server will be started by this same process's own Binary().
//
// DESTRUCTIVE. Callers must obtain explicit user confirmation first; this
// function does not ask.
func RestartTmuxServer(serverSocket string) error {
	args := prependSocket(serverSocket, []string{"kill-server"})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := runGated(ctx, serverSocket, func() ([]byte, error) {
		return (LocalRunner{}).Run(ctx, "", Binary(), args...)
	})
	if err != nil {
		// Combine err+out the same way ListAllSessions does: LocalRunner.Run's
		// combined stdout+stderr (the actual "no server running" text) comes
		// back via out, not err.Error() -- checking err alone here always
		// missed this and treated "server already gone" as a real failure.
		combinedOutput := append([]byte(err.Error()), out...)
		if !serverNotRunning(combinedOutput) {
			return fmt.Errorf("failed to kill tmux server on socket %q: %w", serverSocket, err)
		}
	}
	versionCheckedSockets.Delete(serverSocket)
	controlModeDisabledSockets.Delete(serverSocket)
	return nil
}
