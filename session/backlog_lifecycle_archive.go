package session

import (
	"context"
	"strconv"
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

// WorktreeCleaner removes git worktrees for a backlog item's work-role
// sessions and archives its work/review sessions, once the item has reached
// a terminal status. Implemented by server/services.BacklogService (it owns
// cleanupItemWorktrees/archiveItemWorkSessions — session/ cannot perform
// those git-worktree operations directly or import server/services/, per
// this codebase's package-cycle constraint); wired via SetWorktreeCleaner
// from server/dependencies.go. Used synchronously by internal transition
// paths (transitionBouncingItemToDone, the PR-merge-detected done path) right
// after a terminal transition, and periodically by the
// cleanup_terminal_worktrees detector (reconcileTerminalItemWorktrees) in
// ReconcileStuck as a safety net for items that reached done/archived
// without that synchronous call firing (pre-existing terminal items, a
// race/crash mid-transition).
type WorktreeCleaner interface {
	// CleanupTerminalItem is best-effort: internal failures are logged, never
	// returned, matching the RPC call site's existing contract.
	CleanupTerminalItem(ctx context.Context, itemID string)
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
		log.WarningLog().Printf("[BacklogLifecycle] archiveStaleDoneItems FindDoneItemsOlderThan error: %v", err)
		return
	}
	if len(items) == 0 {
		return
	}
	archived := 0
	for _, item := range items {
		precondition := &BacklogItemPrecondition{ExpectedStatus: string(BacklogStatusDone)}
		if _, transErr := l.storage.GatedTransition(ctx, TransitionRequest{
			Caller: CallerArchiveStaleDone, ItemID: item.ID, To: BacklogStatusArchived, Precondition: precondition, TriggeredBy: TriggeredBySystem,
		}); transErr != nil {
			// A denial is reported once by the gate; the item stays done and is retried next tick.
			logSystemTransitionFailure("archiveStaleDoneItems", item.ID, BacklogStatusArchived, transErr)
			continue
		}
		archived++
	}
	if archived > 0 {
		log.InfoLog().Printf("[BacklogLifecycle] archiveStaleDoneItems: auto-archived %d item(s) done for more than %s", archived, maxDoneAge)
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
		log.WarningLog().Printf("[BacklogLifecycle] reconcileTerminalItemSessions ListBacklogItems error: %v", err)
		return
	}
	processed := 0
	now := l.sweepNow()
	counts, countsOK := l.terminalSessionCounts(ctx)
	for _, item := range items {
		settleKey := terminalSweepKey("sessions", item, counts[item.ID])
		if countsOK && l.terminalSettled.isSettled(settleKey, now) {
			continue
		}
		sessions, sessErr := l.storage.ListItemSessions(ctx, item.ID)
		if sessErr != nil {
			log.WarningLog().Printf("[BacklogLifecycle] reconcileTerminalItemSessions ListItemSessions item=%s: %v", item.ID, sessErr)
			continue
		}
		clean := true
		for _, is := range sessions {
			if is.SessionUUID == "" || !IsTmuxBackedSessionRole(is.Role) {
				continue
			}
			if archErr := archiver.ArchiveSessionByUUID(ctx, is.SessionUUID); archErr != nil {
				log.WarningLog().Printf("[BacklogLifecycle] reconcileTerminalItemSessions failed to archive session=%s item=%s: %v", is.SessionUUID, item.ID, archErr)
				clean = false
				continue
			}
			if killErr := archiver.KillTmuxPaneOnly(ctx, is.SessionUUID); killErr != nil {
				log.WarningLog().Printf("[BacklogLifecycle] reconcileTerminalItemSessions failed to kill tmux pane session=%s item=%s: %v", is.SessionUUID, item.ID, killErr)
				clean = false
			}
			processed++
		}
		if clean && countsOK {
			l.terminalSettled.settle(settleKey, now, terminalSweepSettleTTL)
		}
	}
	if processed > 0 {
		log.InfoLog().Printf("[BacklogLifecycle] reconcileTerminalItemSessions: processed %d work/review session(s) across %d terminal item(s)", processed, len(items))
	}
}

// reconcileTerminalItemWorktrees is the cleanup_terminal_worktrees safety-net detector:
// it finds every backlog item already in done/archived status and re-runs
// cleanupTerminalItemSync for it. This exists because, unlike session archival (see
// reconcileTerminalItemSessions above), worktree cleanup previously had no periodic
// re-scan at all — WorktreeCleaner.CleanupTerminalItem was only ever invoked
// synchronously, on specific internal transition paths (transitionBouncingItemToDone,
// the PR-merge-detected done path, the manual archive RPC). An item that reached
// done/archived through any other path — a pre-existing terminal item from before
// those call sites existed, or a race/crash between the transition committing and the
// synchronous cleanup call running — would otherwise keep its worktree on disk forever.
//
// Deliberately a second, independent query over the same done/archived item set
// reconcileTerminalItemSessions already queries, rather than merging the two into one
// detector: keeps this change additive (no risk of altering reconcileTerminalItemSessions's
// existing behavior) at the cost of one extra ListBacklogItems call per tick, which is
// cheap relative to the git/filesystem work CleanupTerminalItem itself does.
//
// Idempotent and cheap to re-run every tick, by the same reasoning as
// reconcileTerminalItemSessions: cleanupTerminalItemSync no-ops when no WorktreeCleaner is
// wired, and CleanupTerminalItem's own git.Cleanup() call is itself a no-op (logged at
// Info, not Warn/Error — verified in session/git/native/worktree_remove.go's
// RemoveWorktree) when the worktree directory is already gone from disk, so repeat runs
// over an already-cleaned item do not escalate to warning-level log spam.
func (l *BacklogLifecycleListener) reconcileTerminalItemWorktrees(ctx context.Context) {
	if l.getWorktreeCleaner() == nil {
		return
	}
	items, err := l.storage.ListBacklogItems(ctx, BacklogItemFilter{
		Statuses: []string{string(BacklogStatusDone), string(BacklogStatusArchived)},
	})
	if err != nil {
		log.WarningLog().Printf("[BacklogLifecycle] reconcileTerminalItemWorktrees ListBacklogItems error: %v", err)
		return
	}
	now := l.sweepNow()
	counts, countsOK := l.terminalSessionCounts(ctx)
	for _, item := range items {
		settleKey := terminalSweepKey("worktrees", item, counts[item.ID])
		if countsOK && l.terminalSettled.isSettled(settleKey, now) {
			continue
		}
		l.cleanupTerminalItemSync(ctx, item.ID)
		// WorktreeCleaner is best-effort and reports nothing, so a failed cleanup is
		// retried after the TTL rather than on the next tick.
		if countsOK {
			l.terminalSettled.settle(settleKey, now, terminalSweepSettleTTL)
		}
	}
	if len(items) > 0 {
		log.InfoLog().Printf("[BacklogLifecycle] reconcileTerminalItemWorktrees: re-swept worktree cleanup for %d terminal item(s)", len(items))
	}
}

// sweepNow returns the sweep clock (overridable so tests can step the TTL).
func (l *BacklogLifecycleListener) sweepNow() time.Time {
	if l.sweepClock != nil {
		return l.sweepClock()
	}
	return time.Now()
}

// terminalSweepKey scopes a settled entry to one sweep and one version of the item.
// sessionCount is part of the key because attaching an item session does not bump
// the item's updated_at, and a session attached late must re-enter the sweep.
func terminalSweepKey(sweep string, item BacklogItemData, sessionCount int) string {
	return sweep + "/" + item.ID + "@" + strconv.FormatInt(item.UpdatedAt.UnixNano(), 10) + "#" + strconv.Itoa(sessionCount)
}

// terminalSessionCounts returns item id -> item_sessions row count from one query.
// ok is false when it failed; callers then bypass the settled cache entirely.
func (l *BacklogLifecycleListener) terminalSessionCounts(ctx context.Context) (counts map[string]int, ok bool) {
	entries, err := l.storage.GetAllItemSessionsWithBacklogInfo(ctx)
	if err != nil {
		log.WarningLog().Printf("[BacklogLifecycle] terminal sweep session counts: %v (settled cache bypassed this tick)", err)
		return nil, false
	}
	counts = make(map[string]int)
	for _, e := range entries {
		counts[e.ItemID]++
	}
	return counts, true
}
