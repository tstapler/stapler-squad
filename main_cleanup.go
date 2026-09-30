package main

import "github.com/tstapler/stapler-squad/log"

// cleanupOrphanedControlModeClients kills leftover control-mode clients on the default
// tmux socket. Isolated instances share that socket with the live instance and must not
// touch its clients (BUG-116).
func cleanupOrphanedControlModeClients(isolated bool, kill func(serverSocket string) (int, error)) {
	if isolated {
		log.Info("Skipping orphaned control-mode client cleanup for isolated instance (default tmux socket is shared)")
		return
	}
	if killed, err := kill(""); err != nil {
		log.Warn("Failed to clean up orphaned control-mode clients", "err", err)
	} else if killed > 0 {
		log.Info("Cleaned up orphaned control-mode clients left over from a prior process instance", "count", killed)
	}
}
