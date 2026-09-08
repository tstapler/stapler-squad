package tmux

import (
	"context"
	"strings"
	"sync"

	"github.com/tstapler/stapler-squad/log"
)

// versionCheckedSockets tracks sockets whose client/server tmux version has
// already been compared once (LoadOrStore key: string(Socket)). The answer
// cannot change without a server restart, and --tmux-keep-server deliberately
// keeps the server alive across this process's own restarts, so re-checking
// on every StartControlMode call would be pure waste.
var versionCheckedSockets sync.Map

// controlModeDisabledSockets holds every socket where a client/server tmux
// version mismatch was detected. StartControlMode consults this to skip
// spawning a doomed control-mode client for the rest of this process's
// lifetime, rather than re-discovering the same mismatch (and paying its
// ctx-timeout cost) on every single command.
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

	controlModeDisabledSockets.Store(socketKey, struct{}{})
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
