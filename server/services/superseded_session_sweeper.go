package services

import (
	"context"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

// supersededSessionSweeperCheckInterval mirrors StaleCreationSweeper's cadence
// -- this is a correctness backstop, not a hot path, so 60s is fine-grained
// enough without meaningful I/O overhead.
const supersededSessionSweeperCheckInterval = 60 * time.Second

// supersededSessionStore is the narrow storage surface SupersededSessionSweeper
// needs -- ListBacklogItems to find every item whose rework rounds could have
// accumulated a superseded session, ListItemSessions to read each item's round
// history.
type supersededSessionStore interface {
	ListBacklogItems(ctx context.Context, filter session.BacklogItemFilter) ([]session.BacklogItemData, error)
	ListItemSessions(ctx context.Context, itemID string) ([]session.ItemSessionSummary, error)
}

// SupersededSessionSweeper is a ticker-driven defense-in-depth sweeper
// (modeled directly on StaleCreationSweeper) that archives any work/review
// ItemSession left behind by an older rework round once a newer round for the
// same item+role exists.
//
// It closes two gaps the spawn-time archive-on-supersede calls
// (TriggerReReview/spawnSessionAfterGates's archiveItemWorkSessions calls)
// cannot close on their own: (1) archiveItemWorkSessions is explicitly
// best-effort -- a transient failure there leaves an orphaned session with no
// retry, and this sweeper is the self-heal; (2) it converges on rows already
// persisted as stale, not just ones created after this sweeper exists.
//
// "Which round is current" is decided by findSupersededSessions, which
// reuses findMostRecentSessions' latest-CreatedAt-wins tie-break -- the same
// helper TriggerReReview itself already calls to find the prior review round
// to archive -- rather than this sweeper re-deriving its own, third
// definition of "current" (see findSupersededSessions' doc comment for why
// that's a distinct question from spawnSessionAfterGates' EndedAt/IsSessionLive-based
// "is a work session already open" guards, and why archiveIfNotLive's own
// liveness check, not tie-break agreement, is what protects a genuinely-live
// session from an incorrect archive).
//
// Never hard-kills: a superseded session that is still confirmed-live (a user
// may be mid-conversation or steering it) is skipped for this tick rather
// than archived out from under them -- see pitfalls.md #2. It re-evaluates on
// every tick, so a session that goes idle later is picked up on a later pass.
type SupersededSessionSweeper struct {
	storage supersededSessionStore
	stopper SessionStopper
}

// NewSupersededSessionSweeper constructs a sweeper. Both parameters are
// consulted every tick and may be nil during startup wiring -- sweep() is a
// no-op if either is unset, mirroring SessionStopper's own nil-safety
// convention used throughout BacklogService.
func NewSupersededSessionSweeper(storage supersededSessionStore, stopper SessionStopper) *SupersededSessionSweeper {
	return &SupersededSessionSweeper{storage: storage, stopper: stopper}
}

// Start runs the periodic sweep loop. Blocks until ctx is cancelled. Mirrors
// StaleCreationSweeper.Start's shape: run once immediately (so pre-existing
// stale rows are cleaned up on the very next restart, not just prevented
// going forward), then on every tick.
func (s *SupersededSessionSweeper) Start(ctx context.Context) {
	ticker := time.NewTicker(supersededSessionSweeperCheckInterval)
	defer ticker.Stop()

	log.Info("superseded session sweeper started", "check_interval", supersededSessionSweeperCheckInterval)

	s.sweep(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Info("superseded session sweeper stopped")
			return
		case <-ticker.C:
			s.sweep(ctx)
		}
	}
}

// sweep scans every in_progress/review backlog item's session history and
// archives any tmux-backed session findSupersededSessions identifies as not
// the current round for its role, skipping any that are still confirmed-live.
func (s *SupersededSessionSweeper) sweep(ctx context.Context) {
	if s.storage == nil || s.stopper == nil {
		return
	}

	items, err := s.storage.ListBacklogItems(ctx, session.BacklogItemFilter{
		Statuses: []string{string(session.BacklogStatusInProgress), string(session.BacklogStatusReview)},
	})
	if err != nil {
		log.Warn("superseded session sweeper: ListBacklogItems failed", "err", err)
		return
	}

	for _, item := range items {
		sessions, err := s.storage.ListItemSessions(ctx, item.ID)
		if err != nil {
			log.Warn("superseded session sweeper: ListItemSessions failed", "item", item.ID, "err", err)
			continue
		}
		for _, is := range findSupersededSessions(sessions) {
			s.archiveIfNotLive(ctx, is)
		}
	}
}

// archiveIfNotLive soft-archives is unless it is still confirmed-live, in
// which case it's left alone for this tick (pitfalls.md #2: never hard-kill a
// session a user may be actively viewing or steering). ArchiveSessionByUUID
// is itself the actor-setter-routed, no-op-if-already-archived call
// (SetArchivedAtIfNilAndStop or its storage-only fallback) -- see its doc
// comment; this never performs a raw Status write.
func (s *SupersededSessionSweeper) archiveIfNotLive(ctx context.Context, is session.ItemSessionSummary) {
	if is.SessionUUID == "" {
		return
	}
	if s.stopper.IsSessionLive(is.SessionUUID) {
		return
	}
	if err := s.stopper.ArchiveSessionByUUID(ctx, is.SessionUUID); err != nil {
		log.Warn("superseded session sweeper: archive failed", "session", is.SessionUUID, "err", err)
	}
}
