package session

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
)

// TriageRespawner can automatically re-trigger triage for a backlog item whose
// most recent triage-role ItemSession orphaned (StuckReasonOrphanedTriage —
// reconcileOrphanedTriageItems found the session still open long after it
// should have finished, tombstoned it, and marked the item stuck). Before this
// existed, orphaned_triage was detection-and-notify only, exactly like
// StuckReasonAbandonedReview before ReviewRespawner: a human had to notice the
// one-time notification and manually re-trigger triage (confirmed live
// 2026-07-27, docs/tasks/backlog-feature-improvement.md — items 4f03de7b and
// 505fb733 sat in "idea" for 2 days this way). Implemented outside this
// package (BacklogService owns TriggerTriage, the RPC-shaped entry point that
// already knows how to tombstone/re-trigger triage safely) and wired via
// SetTriageRespawner, mirroring StaleWorkRemediator/ReviewRespawner exactly.
type TriageRespawner interface {
	// AutoRespawnTriage re-triggers triage for itemID. No-op (nil error) if
	// the item already moved off "idea" by the time this runs (e.g. a human
	// already re-triggered triage manually, or the item was otherwise
	// resolved) — mirrors AutoRespawnReview's identical staleness guard.
	AutoRespawnTriage(ctx context.Context, itemID string) error

	// IsTriageLive reports whether the implementer (BacklogService) itself still
	// has a headless triage call genuinely in flight for itemID. Added for
	// reconcileOrphanedTriageItems' shape-1 staleness gate (BUG-055): a headless
	// triage session has no live tmux instance to query, so before this existed
	// that gate had no way to tell a session still open past
	// maxHeadlessTriageSessionStaleness because it's genuinely still running
	// apart from one that's actually dead — it assumed the latter unconditionally.
	IsTriageLive(itemID string) bool
}

// headlessTriageSessionUUIDPrefix mirrors server/services/backlog_service_triage.go's
// headlessTriageUUIDPrefix constant (duplicated here rather than imported: server/services
// imports this package, so the reverse import would cycle). Headless triage sessions have
// no live in-memory Instance to check liveness against — per that file's
// tombstoneOrphanTriageSessions, an "open" (EndedAt nil) row found later means the call
// that would have closed it on completion already finished or crashed, not that it's
// genuinely still running — so they warrant a much shorter staleness threshold than the
// general 2h ceiling below.
const headlessTriageSessionUUIDPrefix = "headless-triage-"

// maxHeadlessTriageSessionStaleness bounds how long an open headless-triage session is
// trusted before reconcileOrphanedTriageItems flags it as orphaned. MUST stay strictly
// greater than server/services.triageCallBudget (the real per-call LLM budget, currently
// 30m) with real margin — confirmed live 2026-08-01 (BUG-055) that headless triage calls
// routinely run right up to that full 30m budget (27m41s, 27m53s, 30m38s observed across
// distinct items in one incident), not the 7-15 minutes this constant was originally tuned
// against. At exactly 30m (this constant's prior value, matching triageCallBudget with zero
// margin), this sweep's periodic tick raced the call's own natural
// completion/timeout on every slow call. IsTriageLive (checked by the shape-1 branch below)
// is the structural fix for that race — this margin is a defense-in-depth belt-and-suspenders
// measure on top of it, not a substitute: even with a real liveness check, there's no reason
// to court the race in the first place when a full call is still plausibly finishing.
const maxHeadlessTriageSessionStaleness = 35 * time.Minute

// latestTriageSession returns the most recent triage-role ItemSession (by
// CreatedAt), regardless of whether it has ended yet, or nil if none exists.
// Shared by reconcileOrphanedTriageItems (which needs both the open-and-stale
// and already-ended cases) and reconcilePlanNotApprovedItems (which only
// needs to check whether the latest attempt left a usable result behind).
func latestTriageSession(sessions []ItemSessionSummary) *ItemSessionSummary {
	var latest *ItemSessionSummary
	for i := range sessions {
		if sessions[i].Role != SessionRoleTriage {
			continue
		}
		if latest == nil || sessions[i].CreatedAt.After(latest.CreatedAt) {
			latest = &sessions[i]
		}
	}
	return latest
}

// triageEndReasonOrUnknown formats a persisted ItemSession.EndReason (the
// errType bucket TriggerTriage's classifyHeadlessCallError writes via
// UpdateItemSessionEndedWithReason — server/services/backlog_service_triage.go)
// for a human-facing stuck-reason message. Falls back to "unknown" rather than
// rendering an empty parenthetical: a session can also end via the plain
// UpdateItemSessionEnded path (no errType classification recorded — e.g. a
// legacy row predating classifyHeadlessCallError, or the shutdown-respawn
// carve-out having already routed the "shutdown" bucket away before this is
// ever reached), and "ended () without..." would read as a rendering bug
// rather than a genuinely uncategorized failure.
func triageEndReasonOrUnknown(endReason string) string {
	if endReason == "" {
		return "unknown"
	}
	return endReason
}

// reconcileOrphanedTriageItems flags items gated on plan approval (no
// SkipPlanning, no PlanApproved) whose most recent triage-role ItemSession
// never left a usable plan behind. Originally scoped to idea-status items
// only; generalized 2026-08-03 (docs/tasks/backlog-feature-improvement.md)
// after item be676dab sat 22h+ stuck with a null triageResult: its triage
// session ran 8h52m and produced nothing usable, but the item had already
// advanced from idea to queued (via the WIP cap) before that mattered, which
// put it entirely outside this detector's old status==idea-only scope —
// reconcilePlanNotApprovedItems flagged it too, but treated it identically to
// the normal "plan generated, awaiting your review" wait, with no
// distinction and no automated retry path. The key generalization: this
// detector now keys off "item lacks an approved/skippable plan and lacks a
// usable triage result" rather than "item status == idea" — a superset that
// still covers the original idea-status shapes unchanged.
//
// Deliberately NOT generalized to "ready" status: TriggerTriage only ever
// transitions idea->ready immediately after a successful *parse* of the
// headless call's output (see its cleanupCtx block) — that transition is
// NOT additionally gated on the subsequent TriageResult persist write also
// succeeding (persistFailures there only drives a one-time notification, not
// a rollback), so a transient persist failure can in principle still leave a
// ready item with an empty TriageResult on its latest session. That's a
// real, pre-existing gap (nothing today detects "ready" items at all), but
// one step further down the pipeline than this generalization's scope —
// tracked separately rather than folded in here. idea and queued are the two
// statuses this detector understands today.
//
// Three shapes share this one detector and StuckReason:
//
//  1. Still open and stale (idea only) — the triage process crashed, was
//     killed, or a server restart happened mid-triage before the completion
//     goroutine ever ran. Previously this class of failure was only caught by
//     tombstoneOrphanTriageSessions (same package, server/services/
//     backlog_service_triage.go), and only when a human manually re-triggered
//     triage on the item; this is the standing-sweep equivalent. Pure staleness
//     gate — no liveness checker — matching reconcileStaleWorkSessions' established
//     pattern for the closest analogous detector in this file: a headless triage
//     call routinely runs 7-15 minutes, so per-tick liveness signals are noisy
//     here; staleness alone is the reliable signal. Headless-triage sessions (the
//     common case) get the much shorter maxHeadlessTriageSessionStaleness (30m)
//     rather than the general-purpose maxWorkSessionStaleness (2h): an open
//     headless row found later reliably means dead, not slow (see that constant's
//     doc comment). Not generalized beyond idea: nothing in this codebase creates
//     a new triage-role session while an item is queued, so an open session found
//     on a queued item would be an unmodeled anomaly, not this shape.
//  2. Already ended, idea-status item never left idea — the headless call errored,
//     or returned output ParseHeadlessTriageResult rejected (e.g. a premature-
//     completion status message instead of the final JSON block — see
//     docs/tasks/backlog-feature-improvement.md's 2026-07-30 entry for the live
//     incident, item 04089969, this shape was added for). Unlike shape 1, no
//     staleness wait is needed: TriggerTriage always attempts the idea->ready
//     transition immediately after a successful parse, so a triage session with
//     EndedAt set while the item is still in idea is an unambiguous "triage did
//     not succeed" signal, not a race with an in-flight write.
//  3. Already ended, item advanced past idea (queued) while still gated (no
//     SkipPlanning, no PlanApproved) and the ended session left no usable
//     TriageResult — the 2026-08-03 generalized shape. Unlike shape 2, "ended"
//     alone isn't the signal (a queued item legitimately has an ended,
//     SUCCESSFUL triage session behind it in the common case — that's a normal,
//     working-as-designed wait for human plan approval, not a failure): this
//     shape additionally requires the latest session's TriageResult be empty.
//
// Best-effort: query/notify failures are logged, never returned.
//
// Pre-existing complexity relocated verbatim by the backlog_lifecycle.go split
// (session/backlog_lifecycle_triage.go); not introduced by that split.
//
//nolint:gocognit,gocyclo // see above
func (l *BacklogLifecycleListener) reconcileOrphanedTriageItems(ctx context.Context, er *EntRepository) {
	items, err := l.storage.ListBacklogItems(ctx, BacklogItemFilter{
		Statuses: []string{string(BacklogStatusIdea), string(BacklogStatusQueued)},
	})
	if err != nil {
		log.WarningLog.Printf("[BacklogLifecycle] reconcileOrphanedTriageItems list error: %v", err)
		return
	}

	for _, item := range items {
		sessions, sessErr := l.storage.ListItemSessions(ctx, item.ID)
		if sessErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] reconcileOrphanedTriageItems ListItemSessions item=%s: %v", item.ID, sessErr)
			continue
		}
		// Find the most recent triage-role session regardless of whether it has
		// ended yet — shape 1 above needs the open-and-stale case, shapes 2/3 need
		// the already-ended case, and all three only ever care about the single
		// latest attempt (an older, already-superseded session should never
		// re-trigger this detector).
		latestTriage := latestTriageSession(sessions)
		if latestTriage == nil {
			continue // no triage session has ever run for this item
		}

		isIdea := item.Status == string(BacklogStatusIdea)

		var reasonDetail string
		if latestTriage.EndedAt == nil {
			// Shape 1: still open. Staleness gate as before — idea only, see doc
			// comment above for why this isn't generalized to queued.
			if !isIdea {
				continue
			}
			isHeadless := strings.HasPrefix(latestTriage.SessionUUID, headlessTriageSessionUUIDPrefix)
			staleness := maxWorkSessionStaleness
			if isHeadless {
				staleness = maxHeadlessTriageSessionStaleness
			}
			if time.Since(latestTriage.CreatedAt) <= staleness {
				continue // still plausibly running
			}

			// Past staleness is no longer sufficient on its own for a headless session
			// (BUG-055): consult IsTriageLive, the same liveness record
			// tombstoneOrphanTriageSessions already trusts, before tombstoning a call
			// that may genuinely still be running (the staleness margin above is
			// defense-in-depth, not a substitute — see that constant's doc comment).
			// No equivalent check exists for a non-headless (tmux-backed) session; that
			// gap is unchanged from before this fix.
			if isHeadless {
				if respawner := l.getTriageRespawner(); respawner != nil && respawner.IsTriageLive(item.ID) {
					continue // genuinely still running past staleness; don't tombstone a live call
				}
			}

			// Tombstone the dead row now rather than leaving it open until a human
			// manually re-triggers triage (the only other path that closes it, via
			// tombstoneOrphanTriageSessions in server/services). Past staleness with no
			// live record IS the confirmed-dead signal for this detector.
			if endErr := l.storage.UpdateItemSessionEnded(ctx, latestTriage.ID, time.Now()); endErr != nil { //nolint:silenttransition best-effort tombstone; MarkStuck+notify below runs unconditionally regardless of this write's outcome, so the item is surfaced either way
				log.WarningLog.Printf("[BacklogLifecycle] reconcileOrphanedTriageItems UpdateItemSessionEnded item=%s session=%s: %v", item.ID, latestTriage.ID, endErr)
			}
			reasonDetail = fmt.Sprintf("triage session %s still open after %s", latestTriage.SessionUUID, staleness)
		} else {
			// Already ended. The shutdown carve-out applies to both shape 2 (idea)
			// and shape 3 (queued) identically — a self-inflicted, zero-evidence
			// event either way.
			if latestTriage.EndReason == "shutdown" { // must match classifyHeadlessCallError's bucket name (server/services/backlog_service_triage.go)
				// The prior attempt was killed by our OWN graceful shutdown (a routine
				// deploy restart cancelling s.shutdownCtx mid-call, not a failure of
				// triage itself — see classifyHeadlessCallError). That carries zero
				// evidence retrying would fail, so treat it as "never happened" rather
				// than feeding it into MarkStuck/RemediationDue's exponential backoff
				// (30m/2h/8h/.../72h, sized for OOM-crash bursts): respawn immediately,
				// silently, with no remediation-attempt penalty and no user-facing
				// "may be stuck" notification for what is an expected, self-inflicted event.
				respawner := l.getTriageRespawner()
				if respawner != nil {
					log.InfoLog.Printf("[BacklogLifecycle] item %s triage session %s orphaned by graceful shutdown, respawning immediately with no penalty", item.ID, latestTriage.SessionUUID)
					go func(itemID string) {
						if err := respawner.AutoRespawnTriage(l.shutdownCtx, itemID); err != nil {
							log.WarningLog.Printf("[BacklogLifecycle] reconcileOrphanedTriageItems shutdown-respawn item=%s: %v", itemID, err)
						}
					}(item.ID)
				}
				continue
			}

			if isIdea {
				// Shape 2: already ended, item still in idea. Nothing to tombstone —
				// TriggerTriage's own goroutine already called UpdateItemSessionEnded.
				// EndReason carries classifyHeadlessCallError's bucket
				// (server/services/backlog_service_triage.go) — surface it so the
				// operator (and any future automated remediation) sees the actual
				// failure category instead of a generic "ended" message with no
				// diagnostic value. See triageEndReasonOrUnknown's doc comment for
				// why an empty EndReason still renders instead of being omitted.
				reasonDetail = fmt.Sprintf("triage session %s ended (%s) without moving the item out of idea",
					latestTriage.SessionUUID, triageEndReasonOrUnknown(latestTriage.EndReason))
			} else {
				// Shape 3 (generalized): item advanced past idea (queued) but is
				// still gated on plan approval, and its most recent triage session
				// left no usable plan. An item that IS gated but DOES have a real
				// plan (or has SkipPlanning/PlanApproved set) is
				// reconcilePlanNotApprovedItems' normal "awaiting human review"
				// case, not this detector's concern.
				if item.SkipPlanning || item.PlanApproved || latestTriage.TriageResult != "" {
					continue
				}
				reasonDetail = fmt.Sprintf("triage session %s ended (%s) with no usable plan while item was gated on plan approval (status=%s)",
					latestTriage.SessionUUID, triageEndReasonOrUnknown(latestTriage.EndReason), item.Status)
			}
		}

		applied, markErr := er.MarkStuck(ctx, item.ID, domain.StuckReasonOrphanedTriage, BacklogStatus(item.Status), reasonDetail)
		if markErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] reconcileOrphanedTriageItems MarkStuck item=%s: %v", item.ID, markErr)
			continue
		}
		if !applied {
			continue
		}
		rows, findErr := er.FindOpenStuckStates(ctx)
		if findErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] reconcileOrphanedTriageItems FindOpenStuckStates item=%s: %v", item.ID, findErr)
			continue
		}
		row, ok := findOpenStuckStateFor(rows, item.ID, domain.StuckReasonOrphanedTriage)
		if !ok || row.NotifiedAt != nil {
			continue
		}

		log.WarningLog.Printf("[BacklogLifecycle] item %s triage session %s orphaned (%s)", item.ID, latestTriage.SessionUUID, reasonDetail)
		l.notify(item.ID,
			"Triage may be stuck",
			fmt.Sprintf("%s — its triage session ended without producing a usable plan and nothing is running. Re-trigger triage or investigate.", item.Title),
			8, // sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING
			2, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM
		)
		if _, notifyErr := er.MarkStuckNotified(ctx, item.ID, domain.StuckReasonOrphanedTriage); notifyErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] reconcileOrphanedTriageItems MarkStuckNotified item=%s: %v", item.ID, notifyErr)
		}
	}
	// No resolve pass needed here: selfHealStuck (status-anchored) clears this
	// reason once the item leaves idea/queued — i.e. once triage is
	// re-triggered and succeeds (idea->ready), or the item is otherwise resolved.
}

// reconcileOrphanedTriageRemediation retries triage for every open
// orphaned_triage stuck row still anchored at "idea" — the periodic
// counterpart to reconcileOrphanedTriageItems' detection-and-notify pass
// above, which only ever fires once per orphaned session (the triage
// ItemSession it tombstones never reopens, so the detector has nothing left
// to re-observe on later ticks). Without this, an orphaned_triage row sat
// open forever once its one notification went unnoticed — confirmed live
// 2026-07-27 (docs/tasks/backlog-feature-improvement.md): items 4f03de7b and
// 505fb733 sat in "idea" for 2 days, only recovering once a human manually
// re-triggered triage. Mirrors reconcilePushFailedItems' shape exactly
// (Phase B of docs/tasks/backlog-stuck-item-auto-remediation.md).
// Best-effort: query failures are logged, never returned.
func (l *BacklogLifecycleListener) reconcileOrphanedTriageRemediation(ctx context.Context, er *EntRepository) {
	open, err := er.FindOpenStuckStates(ctx)
	if err != nil {
		log.WarningLog.Printf("[BacklogLifecycle] reconcileOrphanedTriageRemediation FindOpenStuckStates error: %v", err)
		return
	}
	for _, row := range open {
		if row.Reason != domain.StuckReasonOrphanedTriage {
			continue
		}
		if row.ItemStatus != BacklogStatusIdea && row.ItemStatus != BacklogStatusQueued {
			continue // no longer applicable — selfHealStuck resolves it once the item leaves idea/queued
		}
		l.retryOrphanedTriageWithBackoffGate(ctx, row.ItemID, row.ItemTitle)
	}
}

// retryOrphanedTriageWithBackoffGate dispatches TriageRespawner.AutoRespawnTriage
// through the shared remediation backoff gate (Storage.RemediationDue,
// session/backlog_remediation.go) — the "orphaned_triage" reason's remediation
// action per docs/tasks/backlog-stuck-item-auto-remediation.md Phase B.
// Mirrors retryPushFailedWithBackoffGate's shape (bare goroutine, no
// semaphore): AutoRespawnTriage's underlying TriggerTriage call returns
// immediately after creating an ItemSession — the actual headless triage call
// runs inside TriggerTriage's own goroutine — so there is no live LLM call
// here to bound concurrency against, unlike markAbandonedReview's
// reviewSem-gated respawn (TriggerReReview blocks synchronously for the
// review call's duration). Best-effort: gate query/write errors are logged,
// never returned, and fail OPEN (still attempts the retry) rather than
// silently stranding the item — same rationale as
// retryPushFailedWithBackoffGate/remediateStaleWorkWithBackoffGate.
func (l *BacklogLifecycleListener) retryOrphanedTriageWithBackoffGate(ctx context.Context, itemID, itemTitle string) {
	respawner := l.getTriageRespawner()
	if respawner == nil {
		return
	}

	due, justParked, gateErr := l.storage.RemediationDue(ctx, itemID, domain.StuckReasonOrphanedTriage)
	if gateErr != nil {
		log.WarningLog.Printf("[BacklogLifecycle] retryOrphanedTriageWithBackoffGate RemediationDue item=%s: %v", itemID, gateErr)
		due = true // fail open — see retryPushFailedWithBackoffGate's identical rationale
	}
	if justParked {
		l.notify(itemID,
			"Auto-triage paused",
			fmt.Sprintf("%s — automated triage retry has been attempted %d times over an extended period without resolving. It now needs manual attention; use Reset to try again automatically.", itemTitle, MaxRemediationAttempts),
			8, // sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING
			3, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH
		)
	}
	if !due {
		log.InfoLog.Printf("[BacklogLifecycle] retryOrphanedTriageWithBackoffGate item=%s: orphaned_triage remediation backoff not yet due, skipping retry", itemID)
		return
	}

	go func() {
		if err := respawner.AutoRespawnTriage(l.shutdownCtx, itemID); err != nil {
			log.WarningLog.Printf("[BacklogLifecycle] retryOrphanedTriageWithBackoffGate AutoRespawnTriage item=%s: %v", itemID, err)
		}
	}()
}
