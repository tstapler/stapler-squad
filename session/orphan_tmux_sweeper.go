package session

import (
	"context"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
)

// orphanTmuxSweepInterval is how often OrphanedTmuxSweeper re-scans the shared
// tmux socket. Matches HibernationSweeper's cadence — tmux orphans are rare
// enough that a tighter interval isn't warranted, and this keeps the two
// session-hygiene sweepers' CPU/subprocess cost on the same rough cycle.
const orphanTmuxSweepInterval = 5 * time.Minute

// orphanTmuxMinAge is the grace period passed to ReconcileOrphanedTmuxSessions.
// CreateSession's own wait-for-startup budget is 150s (see create_session MCP tool
// doc), so 5 minutes leaves comfortable margin against the registration race described
// on ReconcileOrphanedTmuxSessions's minAge parameter, even under load.
const orphanTmuxMinAge = 5 * time.Minute

// OrphanedTmuxSweeper periodically kills staplersquad_ tmux sessions that have no
// corresponding record in the current workspace DB — the live-operation counterpart
// to the one-time startup call in BuildRuntimeDeps (step 6d). That startup-only call
// leaves a gap: an orphan created mid-run (a crash between DeleteSession's DB delete
// and its tmux teardown, or a KillTmuxPaneOnly call in archiveItemWorkSessions that
// silently failed) accumulates a live claude process with its own MCP subprocess
// fleet until the next restart — which, for a long-uptime instance, can be days.
//
// Deliberately has NO storage-based fallback path when liveProvider is unset, unlike
// HibernationSweeper: a freshly-constructed Instance from storage.LoadInstances() has
// never gone through ReconcileShells (that only runs once, on the live Instance
// object, during startup adoption), so its in-memory shells map is always empty —
// every currently-open shell tab would look like an orphan and get killed. Only the
// live, continuously-adopted instances a LiveInstancesProvider (ReviewQueuePoller)
// tracks have an accurate shells map. Skipping the sweep entirely when unwired is the
// safe failure mode for a destructive operation; a stale/empty view is not.
type OrphanedTmuxSweeper struct {
	liveProvider LiveInstancesProvider
}

// NewOrphanedTmuxSweeper constructs a sweeper. Call SetLiveProvider before Start,
// or every sweep tick is skipped (see the type doc comment for why there is no
// fallback).
func NewOrphanedTmuxSweeper() *OrphanedTmuxSweeper {
	return &OrphanedTmuxSweeper{}
}

// SetLiveProvider wires the live instance source. Call this after constructing the
// ReviewQueuePoller, mirroring HibernationSweeper.SetLiveProvider.
func (s *OrphanedTmuxSweeper) SetLiveProvider(p LiveInstancesProvider) {
	s.liveProvider = p
}

// Start runs the periodic sweep loop. Blocks until ctx is cancelled. Skips entirely
// for an isolated instance (test/named/demo harness) — ReconcileOrphanedTmuxSessions
// always targets the shared default tmux socket regardless of this process's own
// isolated DB/config directory, so an isolated process running this loop would treat
// every real session on the machine's shared tmux server as an orphan. See
// config.IsIsolatedInstance's doc comment and BuildRuntimeDeps step 6d, which applies
// the identical guard to the one-time startup call.
func (s *OrphanedTmuxSweeper) Start(ctx context.Context) {
	if config.IsIsolatedInstance() {
		log.Info("orphan tmux sweeper: skipping — isolated instance")
		return
	}

	log.Info("orphan tmux sweeper started",
		"check_interval", orphanTmuxSweepInterval, "min_age", orphanTmuxMinAge)

	// Run immediately on start rather than waiting for the first tick.
	s.sweep()

	ticker := time.NewTicker(orphanTmuxSweepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("orphan tmux sweeper stopped")
			return
		case <-ticker.C:
			s.sweep()
		}
	}
}

// sweep reads the live instance set and delegates to ReconcileOrphanedTmuxSessions.
func (s *OrphanedTmuxSweeper) sweep() {
	if s.liveProvider == nil {
		log.Warn("orphan tmux sweeper: no live instances provider configured, skipping sweep")
		return
	}
	ReconcileOrphanedTmuxSessions(s.liveProvider.GetInstances(), orphanTmuxMinAge)
}
