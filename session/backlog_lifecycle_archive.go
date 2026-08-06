package session

import (
	"context"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// SessionArchiver soft-archives a session by UUID so it stops accumulating in the
// default session list, and can also kill its live tmux pane. Implemented by
// server/services.SessionService (it owns the live in-memory Instance registry both
// operations must go through — see ArchivedAt's doc comment on session.Instance);
// wired via SetSessionArchiver from server/dependencies.go, same pattern as
// SetNotifier/SetSessionCreator below.
// Used by the archive_terminal_sessions detector in ReconcileStuck as a periodic
// safety net for work sessions belonging to backlog items that reached done/archived
// without their sessions being archived/stopped by the (also newly added) transition
// hook — e.g. pre-existing terminal items from before this detector existed, or a
// race/crash mid-transition. Nil-safe: the detector no-ops when unset.
type SessionArchiver interface {
	// ArchiveSessionByUUID soft-archives the session, if found and not already
	// archived. No-op (not an error) if the session is not tracked.
	ArchiveSessionByUUID(ctx context.Context, sessionUUID string) error
	// KillTmuxPaneOnly closes the session's live tmux pane, if any, leaving its
	// worktree intact (worktree cleanup is handled separately — see
	// cleanupItemWorktreesExcept). No-op if the session isn't tracked live.
	// Without this, ArchiveSessionByUUID alone only hides a terminal item's work
	// session from the default list — the underlying tmux/claude process keeps
	// running indefinitely, accumulating memory across every completed backlog
	// item (root cause of the 2026-07-29 OOM: dozens of `done` items' work
	// sessions still live, each with its own MCP server subprocess fleet).
	KillTmuxPaneOnly(ctx context.Context, sessionUUID string) error
}

// maxDoneAge is how long a backlog item remains in "done" status before the
// auto_archive_done detector (see archiveStaleDoneItems) transitions it to
// "archived". A fixed constant rather than a Settings/Defaults config knob
// (unlike e.g. MaxConcurrentBacklogWorkItems) — this matches the literal
// requirement ("archive 3 days after done") without adding configuration
// surface nothing has asked for; promote to a per-deployment setting if a
// real need for tuning this ever shows up.
const maxDoneAge = 3 * 24 * time.Hour

// archiveStaleDoneItems is the auto_archive_done detector: it finds backlog
// items that have been in "done" status for longer than maxDoneAge (measured
// from the most recent transition into "done" — see FindDoneItemsOlderThan's
// doc comment for why UpdatedAt is not used) and transitions each to
// "archived". Registered before archive_terminal_sessions in ReconcileStuck
// so an item archived by this detector gets its work sessions swept by that
// detector in the very same tick, rather than waiting a full cycle.
//
// Idempotent by construction, not by precondition-failure suppression: an
// item only appears in FindDoneItemsOlderThan's result while its status is
// still "done", so a re-run after a successful archive naturally excludes it
// on the next tick — no double-transition, no error, on repeat runs.
func (l *BacklogLifecycleListener) archiveStaleDoneItems(ctx context.Context) {
	items, err := l.storage.FindDoneItemsOlderThan(ctx, time.Now().Add(-maxDoneAge))
	if err != nil {
		log.WarningLog.Printf("[BacklogLifecycle] archiveStaleDoneItems FindDoneItemsOlderThan error: %v", err)
		return
	}
	if len(items) == 0 {
		return
	}
	archived := 0
	for _, item := range items {
		precondition := &BacklogItemPrecondition{ExpectedStatus: string(BacklogStatusDone)}
		if _, transErr := l.storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusArchived, precondition, TriggeredBySystem); transErr != nil { //nolint:silenttransition idempotent by construction (see doc comment above) — item stays "done" and reappears in FindDoneItemsOlderThan's result on the next tick, so a failed archive here is retried, not silently dropped
			log.WarningLog.Printf("[BacklogLifecycle] archiveStaleDoneItems transition item=%s: %v", item.ID, transErr)
			continue
		}
		archived++
	}
	if archived > 0 {
		log.InfoLog.Printf("[BacklogLifecycle] archiveStaleDoneItems: auto-archived %d item(s) done for more than %s", archived, maxDoneAge)
	}
}

// reconcileTerminalItemSessions is the archive_terminal_sessions safety-net detector:
// it finds every backlog item already in done/archived status and archives any of its
// work- or review-role sessions that are not yet archived. This exists because
// TransitionBacklogItemStatus's archival hook only fires on a NEW transition into
// done/archived — items that were already terminal before that hook was added (or hit a
// race/crash mid-transition) would otherwise keep their sessions unarchived forever.
// Review-role sessions are included alongside work-role ones for the same reason
// archiveItemWorkSessions covers both (see its doc comment) — a review session left
// running after its item reached done/archived leaks a live claude process exactly
// like an orphaned work session does.
// Idempotent and cheap to re-run every tick: SessionArchiver.ArchiveSessionByUUID is a
// CAS no-op for sessions that are already archived or no longer tracked.
func (l *BacklogLifecycleListener) reconcileTerminalItemSessions(ctx context.Context) {
	archiver := l.getSessionArchiver()
	if archiver == nil {
		return
	}
	items, err := l.storage.ListBacklogItems(ctx, BacklogItemFilter{
		Statuses: []string{string(BacklogStatusDone), string(BacklogStatusArchived)},
	})
	if err != nil {
		log.WarningLog.Printf("[BacklogLifecycle] reconcileTerminalItemSessions ListBacklogItems error: %v", err)
		return
	}
	processed := 0
	for _, item := range items {
		sessions, sessErr := l.storage.ListItemSessions(ctx, item.ID)
		if sessErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] reconcileTerminalItemSessions ListItemSessions item=%s: %v", item.ID, sessErr)
			continue
		}
		for _, is := range sessions {
			if is.SessionUUID == "" || !IsTmuxBackedSessionRole(is.Role) {
				continue
			}
			if archErr := archiver.ArchiveSessionByUUID(ctx, is.SessionUUID); archErr != nil {
				log.WarningLog.Printf("[BacklogLifecycle] reconcileTerminalItemSessions failed to archive session=%s item=%s: %v", is.SessionUUID, item.ID, archErr)
				continue
			}
			if killErr := archiver.KillTmuxPaneOnly(ctx, is.SessionUUID); killErr != nil {
				log.WarningLog.Printf("[BacklogLifecycle] reconcileTerminalItemSessions failed to kill tmux pane session=%s item=%s: %v", is.SessionUUID, item.ID, killErr)
			}
			processed++
		}
	}
	if processed > 0 {
		log.InfoLog.Printf("[BacklogLifecycle] reconcileTerminalItemSessions: processed %d work session(s) across %d terminal item(s)", processed, len(items))
	}
}
