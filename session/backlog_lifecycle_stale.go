package session

import (
	"context"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
)

// StaleWorkRemediator can clean up and respawn an in_progress backlog item
// whose active work session has gone stale (StuckReasonStaleWork — no
// progress reported for over maxWorkSessionStaleness) but is NOT a zombie:
// the underlying tmux session and pane process are still alive
// (Instance.TmuxAlive/PaneProcessDead), so the generic tmux health check
// never flags it — the agent inside simply finished its own work and is
// idle at an interactive prompt instead of properly closing out. Before this
// existed, reconcileStaleWorkSessions was detection-only (MarkStuck +
// notify), so such an item sat "in_progress" forever once the agent went
// idle (docs/tasks/backlog-stuck-item-auto-remediation.md Phase B; live
// repro 2026-07-20, item 9264efe7-b4c2-455a-9e2a-ab0196a63ecd, rework suffix
// -r14). Implemented outside this package (BacklogService owns the live
// Instance registry needed to kill the stale tmux pane) and wired via
// SetStaleWorkRemediator, same pattern as AutoReopenSpawner/PRFixSpawner.
type StaleWorkRemediator interface {
	// RemediateStaleWorkSession ends the item's current stale work session
	// (killing its tmux pane but keeping the worktree so uncommitted work
	// survives) and spawns a fresh one with a new turn budget. No-op (nil
	// error) if the item already moved off in_progress or its work session
	// already ended by the time this runs.
	RemediateStaleWorkSession(ctx context.Context, itemID string) error
}

// ReworkBlockStaleResolver re-checks whether a review-status item's open
// StuckReasonReworkBlockedStale row (server/services/backlog_service_triage.go's
// notifyIfActiveWorkSessionStale) should resolve — because the blocking work
// session has produced output again, has ended, or the item has left review —
// and clears the row via storage.ResolveStuck if so. Implemented outside this
// package (BacklogService owns the live SessionStopper needed to re-check
// liveness/staleness) and wired via SetReworkBlockStaleResolver, mirroring
// StaleWorkRemediator/SetStaleWorkRemediator exactly: session-package
// orchestration (reconcileReworkBlockedStaleResolution) needs a
// server/services-layer, liveness-aware action, and this narrow interface —
// not a direct sessionStopper-shaped dependency added to
// BacklogLifecycleListener — is this codebase's established pattern for that.
// No automated remediation counterpart exists for this reason (unlike
// StaleWorkRemediator's RemediateStaleWorkSession) — see
// StuckReasonReworkBlockedStale's doc comment (session/domain/backlog.go) for
// why that's intentional, not a gap.
type ReworkBlockStaleResolver interface {
	// ResolveReworkBlockedStaleIfRecovered no-ops (nil error) if the item
	// still has an open, still-stale blocking work session. Best-effort: a
	// storage error is logged by the caller, never returned to the reconcile
	// tick as a hard failure.
	ResolveReworkBlockedStaleIfRecovered(ctx context.Context, itemID string) error
}

// maxWorkSessionStaleness is the longest an in_progress work session can go without
// reporting progress before ReconcileStuck flags it as stale. Mirrors the order of
// magnitude of maxTriageSessionAge (server/services/backlog_service_triage.go).
const maxWorkSessionStaleness = 2 * time.Hour

func (l *BacklogLifecycleListener) reconcileStaleWorkSessions(ctx context.Context, er *EntRepository) {
	items, err := l.storage.ListBacklogItems(ctx, BacklogItemFilter{
		Statuses: []string{string(BacklogStatusInProgress)},
	})
	if err != nil {
		log.WarningLog.Printf("[BacklogLifecycle] reconcileStaleWorkSessions list error: %v", err)
		return
	}

	stillStale := make(map[string]bool)

	for _, item := range items {
		sessions, sessErr := l.storage.ListItemSessions(ctx, item.ID)
		if sessErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] reconcileStaleWorkSessions ListItemSessions item=%s: %v", item.ID, sessErr)
			continue
		}
		var active *ItemSessionSummary
		for i := range sessions {
			if sessions[i].Role == SessionRoleWork && sessions[i].EndedAt == nil {
				active = &sessions[i]
				break
			}
		}
		if active == nil {
			continue
		}
		lastProgress := active.CreatedAt
		if active.LastProgressAt != nil {
			lastProgress = *active.LastProgressAt
		}
		if !staleWork(lastProgress, time.Now()) {
			continue
		}
		stillStale[item.ID] = true

		applied, markErr := er.MarkStuck(ctx, item.ID, domain.StuckReasonStaleWork, BacklogStatusInProgress,
			fmt.Sprintf("no progress since %s", lastProgress))
		if markErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] reconcileStaleWorkSessions MarkStuck item=%s: %v", item.ID, markErr)
			continue
		}
		if !applied {
			// Status precondition mismatch (item moved off in_progress between
			// the ListBacklogItems read above and this write) — nothing to mark
			// or remediate this tick.
			continue
		}
		rows, findErr := er.FindOpenStuckStates(ctx)
		if findErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] reconcileStaleWorkSessions FindOpenStuckStates item=%s: %v", item.ID, findErr)
			continue
		}
		row, ok := findOpenStuckStateFor(rows, item.ID, domain.StuckReasonStaleWork)
		if !ok {
			continue
		}
		if row.NotifiedAt != nil {
			// Already notified on a prior tick — not the first sighting.
			// Notify-once semantics already covered the "give it a chance"
			// window on the tick that opened this row; from here on,
			// automated remediation takes over, itself gated by the shared
			// backoff schedule (RemediationDue), independent of the
			// per-item rework cap (docs/tasks/backlog-stuck-item-auto-
			// remediation.md Phase B — a live item with reworkCapOverride=0
			// (unlimited) had bounced through this exact stale-agent-idle
			// shape 14 times with nothing ever unsticking it).
			l.remediateStaleWorkWithBackoffGate(ctx, item.ID, item.Title)
			continue
		}

		log.WarningLog.Printf("[BacklogLifecycle] item %s work session %s stale (no progress since %s)", item.ID, active.SessionUUID, lastProgress)
		l.notify(item.ID,
			"Work session may be stuck",
			fmt.Sprintf("%s — no progress reported in over %s. It may be hung or working silently.", item.Title, maxWorkSessionStaleness),
			8, // sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING
			2, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM
		)
		if _, notifyErr := er.MarkStuckNotified(ctx, item.ID, domain.StuckReasonStaleWork); notifyErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] reconcileStaleWorkSessions MarkStuckNotified item=%s: %v", item.ID, notifyErr)
		}
		// AC4-shaped on_session_stale callback (webhook-triggers Phase 5): fired
		// exactly once per crossing, reusing MarkStuckNotified's own dedup above
		// (this branch is only reached on the first sighting — row.NotifiedAt was
		// nil) rather than adding a second, separate "have we fired" flag.
		er.dispatchCallback("session_stale", map[string]any{
			"event":       "session_stale",
			"item_id":     item.ID,
			"title":       item.Title,
			"session_id":  active.SessionUUID,
			"occurred_at": time.Now(),
		})
	}

	// Poll-shaped resolve (else-branch, pre-mortem F2): an in_progress item
	// with an open stale_work row whose session resumed reporting progress
	// must be resolved here — same-status clears are invisible to the
	// status-anchored self-heal sweep.
	open, openErr := er.FindOpenStuckStates(ctx)
	if openErr != nil {
		log.WarningLog.Printf("[BacklogLifecycle] reconcileStaleWorkSessions FindOpenStuckStates(resolve pass) error: %v", openErr)
		return
	}
	for _, row := range open {
		if row.Reason != domain.StuckReasonStaleWork {
			continue
		}
		if row.ItemStatus != BacklogStatusInProgress {
			continue // self-heal sweep handles it once status has moved on
		}
		if stillStale[row.ItemID] {
			continue // still stale this tick
		}
		l.resolveStuckLogged(ctx, er, row.ItemID, domain.StuckReasonStaleWork, "reconcileStaleWorkSessions")
	}
}

// reconcileReworkBlockedStaleResolution is the resolve-only counterpart to
// notifyIfActiveWorkSessionStale (server/services/backlog_service_triage.go),
// which marks StuckReasonReworkBlockedStale but has no periodic tick of its
// own to notice when the blocking session recovers, ends, or the item leaves
// review — MarkStuck only ever runs again from inside
// AutoReopenAfterFailedReview, which won't re-fire while the item sits
// untouched in review. Mirrors reconcileStaleWorkSessions' resolve half
// (FindOpenStuckStates -> filter-by-reason -> delegate), but contains no
// liveness-checking logic itself — that's ReworkBlockStaleResolver's job,
// implemented by BacklogService (server/services/backlog_service_triage.go's
// ResolveReworkBlockedStaleIfRecovered), which has the SessionStopper this
// package deliberately does not depend on directly (see
// ReworkBlockStaleResolver's doc comment). Best-effort: query/delegate errors
// are logged, never returned — one item's failure must not skip the rest.
func (l *BacklogLifecycleListener) reconcileReworkBlockedStaleResolution(ctx context.Context, er *EntRepository) {
	resolver := l.getReworkBlockStaleResolver()
	if resolver == nil {
		return
	}
	open, openErr := er.FindOpenStuckStates(ctx)
	if openErr != nil {
		log.WarningLog.Printf("[BacklogLifecycle] reconcileReworkBlockedStaleResolution FindOpenStuckStates error: %v", openErr)
		return
	}
	for _, row := range open {
		if row.Reason != domain.StuckReasonReworkBlockedStale {
			continue
		}
		if resolveErr := resolver.ResolveReworkBlockedStaleIfRecovered(ctx, row.ItemID); resolveErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] reconcileReworkBlockedStaleResolution ResolveReworkBlockedStaleIfRecovered item=%s: %v", row.ItemID, resolveErr)
		}
	}
}

// reconcileBlockedByDependencyResolution is the resolve-only counterpart to
// notifyBlockedByDependency (server/services/backlog_service_triage.go),
// which marks StuckReasonBlockedByDependency but only runs from inside
// DequeueNextQueuedItems's claim-error path — an item that's already been
// skipped once this sweep won't be re-examined until the next sweep, and if
// nothing else ever queues past it, the stuck row would otherwise sit open
// forever even after every blocker reaches done/archived (AC2). Mirrors
// reconcileReworkBlockedStaleResolution's shape (FindOpenStuckStates ->
// filter-by-reason -> re-check -> resolveStuckLogged), but re-checks the
// blocking condition directly via UnresolvedBlockerItemIDs rather than
// delegating to an external resolver, since dependency resolution is pure
// storage state with no session liveness to consult. Best-effort:
// query failures are logged, never returned — one item's failure must not
// skip the rest.
func (l *BacklogLifecycleListener) reconcileBlockedByDependencyResolution(ctx context.Context, er *EntRepository) {
	open, openErr := er.FindOpenStuckStates(ctx)
	if openErr != nil {
		log.WarningLog.Printf("[BacklogLifecycle] reconcileBlockedByDependencyResolution FindOpenStuckStates error: %v", openErr)
		return
	}

	itemIDs := make([]string, 0, len(open))
	for _, row := range open {
		if row.Reason == domain.StuckReasonBlockedByDependency {
			itemIDs = append(itemIDs, row.ItemID)
		}
	}
	if len(itemIDs) == 0 {
		return
	}

	stillBlocked, err := er.UnresolvedBlockerItemIDs(ctx, itemIDs)
	if err != nil {
		log.WarningLog.Printf("[BacklogLifecycle] reconcileBlockedByDependencyResolution UnresolvedBlockerItemIDs error: %v", err)
		return
	}

	for _, itemID := range itemIDs {
		if stillBlocked[itemID] {
			continue
		}
		l.resolveStuckLogged(ctx, er, itemID, domain.StuckReasonBlockedByDependency, "reconcileBlockedByDependencyResolution")
	}
}

// remediateStaleWorkWithBackoffGate dispatches StaleWorkRemediator.
// RemediateStaleWorkSession through the shared remediation backoff gate
// (Storage.RemediationDue, session/backlog_remediation.go) — the
// "stale_work" reason's remediation action per
// docs/tasks/backlog-stuck-item-auto-remediation.md Phase B. Mirrors
// retryPushFailedWithBackoffGate's shape (bare goroutine, no semaphore — see
// that function's doc comment for why the reviewSem review-gate respawns
// share is not needed here either: ending a stale ItemSession and
// respawning a fresh one is fast compared to a live headless LLM call).
// Best-effort: gate query/write errors are logged, never returned, and fail
// OPEN (still attempts the remediation) rather than silently stranding the
// item — same rationale as autoReopenWithBackoffGate/
// retryPushFailedWithBackoffGate.
//
// Deliberately does NOT add a second liveness check before dispatching
// (e.g. re-querying Instance.TmuxAlive/PaneProcessDead here) — the caller
// (reconcileStaleWorkSessions) already reconfirmed staleWork() true this
// tick, and RemediationDue's own backoff (minimum 30 minutes after the
// first notification) has independently elapsed by the time due=true. A
// second, independently-computed liveness heuristic here could disagree
// with that detector and cause flapping; trust the one signal already
// gating this call.
func (l *BacklogLifecycleListener) remediateStaleWorkWithBackoffGate(ctx context.Context, itemID, itemTitle string) {
	remediator := l.getStaleWorkRemediator()
	if remediator == nil {
		return
	}

	due, justParked, gateErr := l.storage.RemediationDue(ctx, itemID, domain.StuckReasonStaleWork)
	if gateErr != nil {
		log.WarningLog.Printf("[BacklogLifecycle] remediateStaleWorkWithBackoffGate RemediationDue item=%s: %v", itemID, gateErr)
		due = true // fail open — see retryPushFailedWithBackoffGate's identical rationale
	}
	if justParked {
		l.notify(itemID,
			"Auto-rework paused",
			fmt.Sprintf("%s — automated stale-session recovery has been retried %d times over an extended period without resolving. It now needs manual attention; use Reset to try again automatically.", itemTitle, MaxRemediationAttempts),
			8, // sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING
			3, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH
		)
	}
	if !due {
		log.InfoLog.Printf("[BacklogLifecycle] remediateStaleWorkWithBackoffGate item=%s: stale_work remediation backoff not yet due, skipping", itemID)
		return
	}

	go func() {
		if err := remediator.RemediateStaleWorkSession(l.shutdownCtx, itemID); err != nil {
			log.ErrorLog.Printf("[BacklogLifecycle] remediateStaleWorkWithBackoffGate RemediateStaleWorkSession item=%s: %v", itemID, err)
		}
	}()
}
