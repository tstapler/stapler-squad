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
// Unlike the startup-only KillOrphanedControlModeClients (safe to assume every
// attached client is a leftover only at the instant a fresh process boots),
// this cross-checks the spawn registry (TrackChildPID/LookupChildPID) so it
// can run continuously without killing this process's own live connections.
// Closes the gap behind BUG-042's 2026-09-25 recurrence: the startup sweep's
// own list-clients call fails once the server is already too degraded, so a
// periodic, lower-stakes sweep is needed to stop the count reaching that point.
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
		// Warn: a healthy steady state kills zero every tick, so any nonzero
		// count means something is leaking clients during live operation.
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
