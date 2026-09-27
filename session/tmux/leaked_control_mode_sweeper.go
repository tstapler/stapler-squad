package tmux

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// leakedControlModeSweepInterval matches session.orphanTmuxSweepInterval -- this
// class of leak is at least as rare, so there is no reason for a tighter cadence.
const leakedControlModeSweepInterval = 5 * time.Minute

// StartLeakedControlModeSweeper periodically kills tmux control-mode ("-C")
// clients attached to serverSocket that this process did not itself spawn.
// Blocks until ctx is cancelled; run it in its own goroutine.
//
// KillOrphanedControlModeClients (called once at startup, right before session
// restore) is only correct at that one moment: a freshly-started process cannot
// yet have spawned any control-mode client of its own, so anything already
// attached is provably a leftover from a prior instance. Once the process has
// been running a while, that assumption no longer holds -- most attached
// control-mode clients by then are this process's own, legitimate, in-use
// connections, so blindly killing every attached client (as the startup sweep
// does) is not safe here.
//
// This sweeper instead cross-checks the spawn registry (TrackChildPID /
// LookupChildPID, see fork_metrics.go): a control-mode client whose PID this
// process never spawned is unconditionally an orphan, regardless of how long
// the process has been up, since every control-mode client this process
// spawns is tracked there at cmd.Start() time (session/tmux/control_mode.go's
// StartControlMode, and this package's own registry keepalive connection).
//
// This closes the gap behind BUG-042's 2026-09-25 recurrence
// (docs/bugs/fixed/BUG-042-orphaned-control-mode-clients-overload-tmux-server.md):
// the boot-time-only cleanup left ~50 clients attached to the keepalive
// sentinel, accumulated across many restarts, because by the time several of
// those restarts ran their own startup sweep the tmux server was often already
// too degraded for that sweep's own list-clients call to succeed (see
// KillOrphanedControlModeClients's doc comment on that failure). A periodic
// sweep run against an (at that point) still-healthy server prevents the count
// from ever climbing high enough to reach that degraded state in the first
// place, rather than trying to recover from it after the fact.
func StartLeakedControlModeSweeper(ctx context.Context, serverSocket string) {
	log.Info("[tmux] leaked control-mode client sweeper started", "interval", leakedControlModeSweepInterval)

	sweepLeakedControlModeClients(serverSocket)

	ticker := time.NewTicker(leakedControlModeSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Info("[tmux] leaked control-mode client sweeper stopped")
			return
		case <-ticker.C:
			sweepLeakedControlModeClients(serverSocket)
		}
	}
}

func sweepLeakedControlModeClients(serverSocket string) {
	killed, attached, err := killUntrackedControlModeClients(serverSocket)
	if err != nil {
		log.Warn("[tmux] leaked control-mode client sweep failed", "err", err)
		return
	}
	if killed > 0 {
		// Warn, not Info: a healthy steady state kills zero of these every
		// tick. Any nonzero count means something outside this sweeper's
		// normal startup-time cleanup is leaking control-mode clients during
		// live operation and deserves attention, not just quiet reconciliation.
		log.Warn("[tmux] killed leaked control-mode clients not owned by this process",
			"killed", killed, "attached", attached)
	}
}

// killUntrackedControlModeClients lists every control-mode client attached to
// serverSocket and kills any whose PID is not in this process's own spawn
// registry (LookupChildPID) -- i.e. a client this process did not itself
// start. Returns the number killed and the total number of control-mode
// clients seen (killed <= attached).
func killUntrackedControlModeClients(serverSocket string) (killed int, attached int, err error) {
	args := prependSocket(serverSocket, []string{"list-clients", "-F", "#{client_pid} #{client_control_mode}"})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, runErr := (LocalRunner{}).Run(ctx, "", ResolveClientForSocket(serverSocket), args...)
	if runErr != nil {
		// No server running yet, or no clients at all -- nothing to clean up.
		return 0, 0, nil
	}

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[1] != "1" {
			continue // not a control-mode client
		}
		attached++
		pid, convErr := strconv.Atoi(fields[0])
		if convErr != nil {
			continue
		}
		if _, _, tracked := LookupChildPID(pid); tracked {
			continue // this process's own, legitimate connection
		}
		proc, findErr := os.FindProcess(pid)
		if findErr != nil {
			continue
		}
		if killErr := proc.Kill(); killErr == nil {
			killed++
		}
	}
	return killed, attached, nil
}
