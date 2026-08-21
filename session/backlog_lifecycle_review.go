package session

import (
	"context"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
)

// ReviewGateSpawner can create a short-lived review session for a backlog item.
// Deprecated: use headless.Pool via NewBacklogLifecycleListenerWithSpawner instead.
// Retained for backward compatibility with existing tests and callers.
type ReviewGateSpawner interface {
	// SpawnReviewSession creates a one-shot review session for item using prompt.
	// itemSessionID is the UUID of the work ItemSession being reviewed.
	SpawnReviewSession(ctx context.Context, item *BacklogItemData, itemSessionID string, prompt string) (*Instance, error)
}

// AutoReopenSpawner can automatically reopen a backlog item for rework after a
// failed review verdict (FAIL or PARTIAL). It transitions the item back to
// in_progress and spawns a new work session so the review→rework cycle is
// fully automated.
type AutoReopenSpawner interface {
	AutoReopenAfterFailedReview(ctx context.Context, itemID string) error
}

// ReviewRespawner can automatically re-trigger the review gate for a backlog
// item stuck in review with no active session in flight (the
// StuckReasonAbandonedReview condition — see markAbandonedReview). Before this
// existed, such items were detected and notified but nothing ever respawned
// work on them, so they sat forever until a human noticed (see
// docs/tasks/backlog-feature-improvement.md, 2026-07-17 update — 4 real items
// went stale this way, several with nearly all acceptance criteria already
// marked complete, just never actually re-reviewed).
type ReviewRespawner interface {
	AutoRespawnReview(ctx context.Context, itemID string) error
}

// maxConcurrentReviewGates is the maximum number of review gates that can run
// concurrently. This caps goroutine fan-out when many sessions exit simultaneously.
const maxConcurrentReviewGates = 8

// handleReviewSessionExited processes the outcome of a review session (Role ==
// SessionRoleReview) that has just exited. Review now always happens in a real,
// hidden session.Instance (see ReviewGateRunner.Run / SpawnReviewSession)
// instead of a synchronous in-process headless LLM call, so the verdict — if
// any — was submitted via the submit_review_verdict MCP tool while the review
// session was running (see server/mcp/tools_backlog.go) and is read back here
// from storage rather than computed inline.
//
// forcePush controls the PASS branch's behavior when the work session that
// earned the verdict is still alive (EndedAt nil): false (the normal,
// real-time exit-event path — see onSessionExited below) defers to that live
// session, which is expected to discover the PASS verdict on its own next
// poll and ship the PR itself via /backlog/ship (see taskProtocolBlock rules
// 8-9). true (used only by reconcileUnprocessedReviewVerdicts, the
// crash-recovery sweep for a review session that died before this function
// ever ran for it normally) routes to shipViaAgentOrFallback regardless of
// work-session liveness — that sweep cannot tell a genuinely-live,
// still-polling work session apart from a zombie that will never poll again,
// and its whole reason to exist is to make forward progress on a verdict
// nothing else is going to act on. See
// TestReconcileUnprocessedReviewVerdicts_should_applyPassVerdict_When_ReviewSessionDiedButWorkSessionStillAlive
// — that test's fixture has no worktree recorded at all, so it exercises
// shipViaAgentOrFallback -> pushAndCreatePR's pre-existing, unchanged
// fallbackToDone("no worktree") branch; this fix does not touch that branch.
func (l *BacklogLifecycleListener) handleReviewSessionExited(ctx context.Context, reviewIS ItemSessionSummary, forcePush bool) {
	item, err := l.storage.GetBacklogItem(ctx, reviewIS.BacklogItemID)
	if err != nil {
		log.ErrorLog.Printf("[BacklogLifecycle] handleReviewSessionExited GetBacklogItem item=%s: %v", reviewIS.BacklogItemID, err)
		return
	}

	// ListItemSessions (unlike GetItemSessionBySessionUUID, used by the caller)
	// eagerly loads the ReviewVerdict edge, which is what we need here.
	sessions, err := l.storage.ListItemSessions(ctx, item.ID)
	if err != nil {
		log.ErrorLog.Printf("[BacklogLifecycle] handleReviewSessionExited ListItemSessions item=%s: %v", item.ID, err)
		return
	}

	// Scan oldest-first: find the review ItemSession matching this exited
	// session, and keep overwriting workEntry so it ends up as the most recent
	// work session — the one whose worktree needs to be pushed on a PASS verdict.
	var reviewEntry *ItemSessionSummary
	var workEntry *ItemSessionSummary
	for i := range sessions {
		s := &sessions[i]
		if s.SessionUUID == reviewIS.SessionUUID && s.Role == SessionRoleReview {
			reviewEntry = s
		}
		if s.Role == SessionRoleWork {
			workEntry = s
		}
	}

	if reviewEntry == nil || reviewEntry.ReviewVerdict == nil {
		// The review session exited without ever calling submit_review_verdict —
		// crashed, killed, ran out of turns, etc. Treat it like a failed review so
		// the item doesn't sit stuck in "review" forever.
		//
		// BUG-046: when autoReopenWithBackoffGate's downstream "bouncing" gate is
		// already open/mid-backoff (a prior bounce cycle already surfaced this
		// exact condition), the item never leaves "review" — nothing transitions
		// it — so reconcileUnprocessedReviewVerdicts' sweep re-detects the SAME
		// dead session on every subsequent ~60s tick and would otherwise notify
		// and log a WARNING every time, forever, until the gate finally opens
		// (confirmed live: item 12981e9d reached occurrence_count 95 in ~94
		// minutes). RemediationBlocked is a read-only peek at that same gate — if
		// it's already blocking, this exact condition was already recorded on a
		// prior tick, so skip the redundant notify+log and only re-run
		// autoReopenWithBackoffGate (whose own gating logic is unchanged and
		// still correctly no-ops until the gate opens). Mirrors the idempotency
		// pattern BUG-043 established via the same RemediationBlocked primitive
		// for abandoned_review's attempt-budget guard. Fails open on a query
		// error (proceeds to notify) rather than silently going quiet.
		blocked, blockedErr := l.storage.RemediationBlocked(ctx, item.ID, domain.StuckReasonBouncing)
		if blockedErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] handleReviewSessionExited RemediationBlocked(bouncing) item=%s: %v", item.ID, blockedErr)
		}
		if !blocked {
			log.WarningLog.Printf("[BacklogLifecycle] handleReviewSessionExited item=%s review session %s exited without a verdict", item.ID, reviewIS.SessionUUID)
			l.notify(item.ID,
				"Review session ended without a verdict",
				fmt.Sprintf("%s — the review session exited without calling submit_review_verdict. Treating as a failed review.", item.Title),
				7, // sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR
				3, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH
			)
		}
		l.autoReopenWithBackoffGate(ctx, item.ID, item.Title, BacklogStatus(item.Status))
		return
	}

	verdict := reviewEntry.ReviewVerdict
	overall := ReviewOutcome(verdict.OverallOutcome)
	perCriterion, parseErr := parsePerCriterionVerdicts(verdict.PerCriterion)
	if parseErr != nil {
		log.WarningLog.Printf("[BacklogLifecycle] handleReviewSessionExited parsePerCriterionVerdicts item=%s: %v", item.ID, parseErr)
	}
	acSnapshot, _ := ParseAcCriteria(reviewIS.AcSnapshot)
	applyVerdictsToACs(ctx, l.storage, item, acSnapshot, perCriterion)

	log.InfoLog.Printf("[BacklogLifecycle] handleReviewSessionExited item=%s outcome=%s (review session %s)", item.ID, overall, reviewIS.SessionUUID)

	switch overall {
	case ReviewVerdictFail, ReviewVerdictPartial, ReviewVerdictUnverifiable:
		l.autoReopenWithBackoffGate(ctx, item.ID, item.Title, BacklogStatus(item.Status))
	case ReviewVerdictPass:
		if workEntry == nil {
			log.ErrorLog.Printf("[BacklogLifecycle] handleReviewSessionExited item=%s: PASS verdict but no work session found — cannot push", item.ID)
			return
		}
		if workEntry.EndedAt == nil && !forcePush {
			// The work session that produced this PASS is still alive — it stays
			// running and polls get_backlog_item/backlog status after request_review
			// (see taskProtocolBlock rules 8-9, session/backlog_context.go). Per those
			// rules it will discover this PASS verdict on its next poll and run
			// /backlog/ship itself, which drives /github:pr-ship end to end (local CI,
			// code review, remote CI, and — unlike the mechanical push below — actual
			// merge-conflict resolution and reaction to failing checks). Leave the item
			// in review and let the live agent drive shipping; do not race it with the
			// mechanical push path. Mirrors AutoReopenAfterFailedReview's identical
			// hasActiveWorkSession guard on the FAIL/PARTIAL side of this same loop.
			log.InfoLog.Printf("[BacklogLifecycle] handleReviewSessionExited item=%s: PASS verdict with a live work session (%s) — leaving PR creation to the agent via /backlog/ship instead of the mechanical push path", item.ID, workEntry.SessionUUID)
			return
		}
		// Reached when either the work session that earned this PASS already exited
		// (crashed, was killed, or hit a turn cap — nothing will ever run
		// /backlog/ship for this item on its own) or forcePush is set
		// (reconcileUnprocessedReviewVerdicts' crash-recovery sweep, which cannot
		// distinguish a genuinely-live work session from a zombie). Ship the PR —
		// see shipViaAgentOrFallback's doc comment for the agent-driven-first,
		// mechanical-push-as-backstop policy.
		l.shipViaAgentOrFallback(ctx, item, *workEntry)
	}
}

// autoReopenWithBackoffGate dispatches AutoReopenAfterFailedReview through the
// shared remediation backoff gate (Storage.RemediationDue,
// session/backlog_remediation.go) — the "bouncing" reason's remediation
// action per docs/tasks/backlog-stuck-item-auto-remediation.md Phase A.
// Called on every failed/verdict-less review exit, same trigger points as
// before this gate existed; the gate itself is what makes repeated calls in
// rapid succession (the exact 2026-07-19 incident shape) stop consuming a
// fresh attempt every few minutes once a "bouncing" BacklogStuckState row is
// open. When no such row exists yet (this reason hasn't been detected as
// stuck), RemediationDue reports due=true unconditionally — the first few
// reopen attempts, before reconcileBouncingItems' bounceThreshold trips,
// behave exactly as they did before this gate existed. Best-effort: gate
// query/write errors are logged, never returned, and fail OPEN (still
// attempts the reopen) rather than silently stranding the item.
//
// itemStatus must be the item's actual current status at the call site (NOT
// hardcoded) — it is passed straight through to MarkStuck's expectedStatus
// precondition below when the cap is hit while still bouncing (Signal 2, see
// plan.md Story 1.3.1). handleReviewSessionExited's item is in "review"
// status at both of its call sites, never "in_progress"; a hardcoded
// BacklogStatusInProgress would make that MarkStuck call silently no-op
// (applied=false) every time.
func (l *BacklogLifecycleListener) autoReopenWithBackoffGate(ctx context.Context, itemID, itemTitle string, itemStatus BacklogStatus) {
	reopener := l.getAutoReopener()
	if reopener == nil {
		return
	}

	due, justParked, gateErr := l.storage.RemediationDue(ctx, itemID, domain.StuckReasonBouncing)
	if gateErr != nil {
		log.WarningLog.Printf("[BacklogLifecycle] autoReopenWithBackoffGate RemediationDue item=%s: %v", itemID, gateErr)
		due = true // fail open — see doc comment above
	}
	if justParked {
		// Signal 2 (Epic 1.3, plan.md Story 1.3.1): the cap being hit while
		// "bouncing" is still open is itself evidence the retry loop isn't
		// converging, not just an ordinary single-reason park — mark a
		// durable, differentiated bounce_cap_exhausted row so it's
		// distinguishable in the Stuck Items UI and durable across restarts,
		// and upgrade this park's notification from the generic
		// WARNING/HIGH "Auto-rework paused" copy to ERROR/URGENT framing
		// naming the retry loop specifically. Best-effort throughout: log a
		// warning on error, never block the reopen attempt below on these
		// writes.
		applied, markErr := l.storage.MarkStuck(ctx, itemID, domain.StuckReasonBounceCapExhausted, itemStatus, "bouncing remediation cap exhausted while bouncing reason still open")
		if markErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] autoReopenWithBackoffGate MarkStuck(bounce_cap_exhausted) item=%s: %v", itemID, markErr)
		}
		// Only notify if the row was actually written. If MarkStuck's
		// expectedStatus precondition failed closed (itemStatus drifted from
		// the DB's live value between fetch and this call), applied is false
		// and there is no durable row backing this notification — firing it
		// anyway would be exactly the "one-time toast with nothing durable
		// behind it" ADR-001 chose synthetic StuckReason rows to avoid.
		// justParked is a one-shot signal (RemediationDue never returns it
		// true twice for the same park), so there is no later retry if this
		// tick is skipped — matches reconcileMultiReasonEscalation's and
		// reconcileBouncingItems' own `if !applied { continue }` guard. Note:
		// this only guards the notify/MarkStuckNotified below, NOT the
		// reopen-attempt logic further down this function — a failed mark
		// must not block the actual auto-reopen.
		if applied {
			log.WarningLog.Printf("[BacklogLifecycle] bounce cap exhausted while still bouncing item=%s", itemID)
			l.notify(itemID,
				"Bounce cap exhausted — retry loop not converging",
				fmt.Sprintf("%s — automated rework hit its retry cap (%d attempts) while still bouncing between in_progress and review. This is evidence the retry loop itself isn't converging, not a transient failure — a different approach may be needed before using Reset.", itemTitle, MaxRemediationAttempts),
				7, // sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR
				4, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_URGENT
			)
			if _, notifiedErr := l.storage.MarkStuckNotified(ctx, itemID, domain.StuckReasonBounceCapExhausted); notifiedErr != nil {
				log.WarningLog.Printf("[BacklogLifecycle] autoReopenWithBackoffGate MarkStuckNotified(bounce_cap_exhausted) item=%s: %v", itemID, notifiedErr)
			}
		}
	}
	if !due {
		log.InfoLog.Printf("[BacklogLifecycle] autoReopenWithBackoffGate item=%s: bouncing remediation backoff not yet due, skipping auto-reopen", itemID)
		return
	}

	go func() {
		if err := reopener.AutoReopenAfterFailedReview(ctx, itemID); err != nil {
			log.ErrorLog.Printf("[BacklogLifecycle] autoReopenWithBackoffGate AutoReopenAfterFailedReview item=%s: %v", itemID, err)
		}
	}()
}

// TriggerReviewForSession immediately spawns a review gate for the work session
// identified by workSessionUUID. Used by the autonomous driver to trigger review
// as soon as the driver signals DONE, rather than waiting for ReconcileStuck.
// No-op if the listener is disabled or no review mechanism is configured.
func (l *BacklogLifecycleListener) TriggerReviewForSession(workSessionUUID string) {
	if !l.enabled.Load() {
		return
	}
	if l.getSessionCreator() == nil {
		return
	}
	go func() {
		select {
		case l.reviewSem <- struct{}{}:
		case <-l.shutdownCtx.Done():
			return
		}
		defer func() { <-l.reviewSem }()

		ctx := l.shutdownCtx
		is, err := l.storage.GetItemSessionBySessionUUID(ctx, workSessionUUID)
		if err != nil {
			log.ErrorLog.Printf("[BacklogLifecycle] TriggerReviewForSession GetItemSessionBySessionUUID(%s): %v", workSessionUUID, err)
			return
		}
		item, err := l.storage.GetBacklogItem(ctx, is.BacklogItemID)
		if err != nil {
			log.ErrorLog.Printf("[BacklogLifecycle] TriggerReviewForSession GetBacklogItem session=%s item=%s: %v", workSessionUUID, is.BacklogItemID, err)
			return
		}
		if item.SkipReviewGate {
			return
		}
		log.InfoLog.Printf("[BacklogLifecycle] TriggerReviewForSession: spawning immediate review gate item=%s session=%s", item.ID, workSessionUUID)
		l.spawnReviewGate(item, is)
	}()
}

// applyVerdictsToACs updates the acceptance criteria status fields on a backlog
// item to reflect the review verdict for each criterion:
//
//	PASS  → "done"
//	PARTIAL → "in_progress"
//	FAIL / UNVERIFIABLE → unchanged (stay "pending")
//
// Best-effort: errors are logged but do not block the caller.
func applyVerdictsToACs(ctx context.Context, storage *Storage, item *BacklogItemData, acSnapshot []AcCriterion, verdicts []CriterionVerdict) {
	if len(verdicts) == 0 || len(acSnapshot) == 0 {
		return
	}

	outcomeByIdx := make(map[int]ReviewOutcome, len(verdicts))
	for _, v := range verdicts {
		outcomeByIdx[v.CriterionIndex] = v.Outcome
	}

	updated := make([]AcCriterion, len(acSnapshot))
	copy(updated, acSnapshot)
	changed := false
	for i, ac := range updated {
		outcome, ok := outcomeByIdx[ac.Index]
		if !ok {
			continue
		}
		var newStatus AcStatus
		switch outcome {
		case ReviewOutcomePass:
			newStatus = AcStatusDone
		case ReviewOutcomePartial:
			newStatus = AcStatusInProgress
		default:
			continue // FAIL / UNVERIFIABLE: leave as-is
		}
		if newStatus != ac.Status {
			updated[i].Status = newStatus
			changed = true
		}
	}

	if !changed {
		return
	}

	newJSON, err := SerializeAcCriteria(updated)
	if err != nil {
		log.ErrorLog.Printf("[BacklogLifecycle] applyVerdictsToACs serialize item=%s: %v", item.ID, err)
		return
	}
	acj := newJSON
	if _, err := storage.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{AcceptanceCriteria: &acj}, nil); err != nil {
		log.ErrorLog.Printf("[BacklogLifecycle] applyVerdictsToACs update item=%s: %v", item.ID, err)
		return
	}
	log.InfoLog.Printf("[BacklogLifecycle] applyVerdictsToACs: updated AC statuses for item=%s (%d criteria)", item.ID, len(updated))
}

// spawnReviewGate creates a one-shot review session for item, using the diff
// from the work session's worktree.
func (l *BacklogLifecycleListener) spawnReviewGate(item *BacklogItemData, is ItemSessionSummary) {
	l.runner.Run(l.shutdownCtx, item, is, l.pushAndCreatePR)
}

// reconcileStuckReviewItems notifies once per item when a review-status item
// has a review verdict on record but no active review or work session — i.e.
// it is not mid-cycle, it is simply abandoned — or when the item's only
// "active" session is confirmed dead (a zombie: pre-mortem F3). Notify-once
// dedup and "since when" are DB-backed (durable BacklogStuckState row), not
// an in-memory map, so both survive a restart. Best-effort: query/notify
// failures are logged, never returned.
func (l *BacklogLifecycleListener) reconcileStuckReviewItems(ctx context.Context, er *EntRepository) {
	seen := make(map[string]bool)

	items, err := er.FindStuckReviewItems(ctx)
	if err != nil {
		log.WarningLog.Printf("[BacklogLifecycle] reconcileStuckReviewItems query error: %v", err)
	} else {
		for _, item := range items {
			seen[item.ID.String()] = true
			l.markAbandonedReview(ctx, er, item.ID.String(), item.Title, "stuck in review with no active session")
		}
	}

	// Zombie-session review items (pre-mortem F3): items FindStuckReviewItems
	// excludes because a review/work session row still looks active, but the
	// underlying tmux/CLI process is confirmed dead.
	checker := l.getSessionLivenessChecker()
	if checker != nil {
		zombieCandidates, zErr := er.FindZombieReviewItems(ctx)
		if zErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] reconcileStuckReviewItems FindZombieReviewItems error: %v", zErr)
		} else {
			for _, item := range zombieCandidates {
				if seen[item.ID.String()] {
					continue // already flagged via the abandoned path above
				}
				allDead := len(item.Edges.ItemSessions) > 0
				for _, is := range item.Edges.ItemSessions {
					if checker(is.SessionUUID) {
						allDead = false
						break
					}
				}
				if !allDead {
					continue // at least one active session is genuinely alive
				}
				// Tombstone the confirmed-dead rows now, not just flag them. Without
				// this, AutoRespawnReview's hasActiveWorkSession/hasActiveReviewSession
				// guard (server/services/backlog_service_triage.go) still sees these
				// EndedAt-nil rows as "active" and silently skips the respawn it was
				// just dispatched to perform — the zombie detection fired for nothing.
				for _, is := range item.Edges.ItemSessions {
					if endErr := l.storage.UpdateItemSessionEnded(ctx, is.ID.String(), time.Now()); endErr != nil { //nolint:silenttransition best-effort tombstone; markAbandonedReview below still flags/notifies for this item on this same tick regardless of this specific row's outcome, and a failed tombstone here is retried on the next tick since the item still matches FindZombieReviewItems
						log.WarningLog.Printf("[BacklogLifecycle] reconcileStuckReviewItems UpdateItemSessionEnded item=%s session=%s: %v", item.ID, is.ID, endErr)
					}
				}
				seen[item.ID.String()] = true
				l.markAbandonedReview(ctx, er, item.ID.String(), item.Title, "review session process is gone (zombie)")
			}
		}
	}

	// Poll-shaped resolve (else-branch, pre-mortem F2): an item with an open
	// abandoned_review row whose condition no longer holds while it's still
	// "review" (the review gate came back in flight) must be resolved here —
	// the status-anchored self-heal sweep structurally cannot see a
	// same-status clear.
	open, openErr := er.FindOpenStuckStates(ctx)
	if openErr != nil {
		log.WarningLog.Printf("[BacklogLifecycle] reconcileStuckReviewItems FindOpenStuckStates error: %v", openErr)
		return
	}
	for _, row := range open {
		if row.Reason != domain.StuckReasonAbandonedReview {
			continue
		}
		if row.ItemStatus != BacklogStatusReview {
			continue // not this item's status anymore — self-heal sweep handles it
		}
		if seen[row.ItemID] {
			continue // still abandoned this tick
		}
		l.resolveStuckLogged(ctx, er, row.ItemID, domain.StuckReasonAbandonedReview, "reconcileStuckReviewItems")
	}
}

// reconcileUnprocessedReviewVerdicts closes the gap where a review session
// submitted its verdict (PASS/FAIL/PARTIAL/UNVERIFIABLE) but died — crash, OOM,
// server restart — before its exit event ever reached handleReviewSessionExited,
// the one place that acts on a verdict (push+PR on PASS, auto-reopen otherwise).
// The item is left stuck in "review" with a recorded verdict nothing ever
// processes.
//
// This is deliberately separate from reconcileStuckReviewItems' zombie detection:
// that path requires EVERY open review-or-work session on the item to be
// confirmed dead, but AutoReopenAfterFailedReview intentionally leaves a work
// session alive polling for the verdict once the item is back in "review" (see
// docs/tasks/backlog-feature-improvement.md's "WIP limit now undercounts live
// sessions" finding) — so the item never looks like a full zombie even though
// the review session itself is the one that died with unactioned output. Found
// live: a work session correctly detected its own item had an already-recorded
// PASS verdict and all criteria done, but had no way to force the review→done
// transition itself (by design — that's this function's job, not a work
// session's), so it looped forever re-requesting a review the backlog system
// correctly rejected (item already past "in_progress").
//
// Acts on the most recent review-role session only, once it is confirmed not
// still wrapping up on its own (EndedAt already set, or the liveness checker
// says it's dead) — a session that's merely slow to exit is left alone.
//
// latest is deliberately NOT required to carry its own ReviewVerdict. The
// query's HasReviewVerdict() filter only guarantees the ITEM has some
// review-role session with a verdict somewhere in its history — it says
// nothing about whether the newest one does. Live 2026-07-22 on backlog item
// 9264efe7 (PR #173): an older review session recorded a FAIL verdict and
// died, then two further re-review attempts were created (also dying, never
// writing a verdict of their own) before the item was ever unstuck. Bailing
// out here whenever latest lacked a verdict ("defensive: query already
// filters on HasReviewVerdict()" — that comment was wrong, the filter is
// item-scoped, not latest-scoped) skipped the item entirely on every tick,
// because the newest session is what this sweep always inspects. The correct
// behavior for a dead, verdict-less latest session is exactly
// handleReviewSessionExited's existing "review session exited without a
// verdict" branch (auto-reopen for rework) — so let it flow through instead
// of returning early; handleReviewSessionExited already looks the session's
// own verdict up again by SessionUUID and handles both shapes correctly.
// Best-effort: query/tombstone failures are logged, never returned.
func (l *BacklogLifecycleListener) reconcileUnprocessedReviewVerdicts(ctx context.Context, er *EntRepository) {
	items, err := er.FindReviewItemsWithUnprocessedVerdict(ctx)
	if err != nil {
		log.WarningLog.Printf("[BacklogLifecycle] reconcileUnprocessedReviewVerdicts query error: %v", err)
		return
	}
	checker := l.getSessionLivenessChecker()
	for _, item := range items {
		if len(item.Edges.ItemSessions) == 0 {
			continue
		}
		latest := item.Edges.ItemSessions[0] // most recent review-role session (query orders desc)

		dead := latest.EndedAt != nil
		if !dead && checker != nil {
			dead = !checker(latest.SessionUUID)
		}
		if !dead && latest.Edges.ReviewVerdict != nil && time.Since(latest.Edges.ReviewVerdict.CreatedAt) > reviewVerdictIdleThreshold {
			// A reviewer that submitted a verdict and then simply never exited
			// (process alive, no further output) reads as alive forever per the
			// checks above — submitReviewVerdict's eager review->in_progress
			// transition (server/mcp/tools_backlog.go) is the primary fix for
			// this shape on FAIL/PARTIAL/UNVERIFIABLE, but this sweep is what
			// still needs to catch PASS verdicts (deferred to session-exit by
			// design) and any case the eager path didn't reach (e.g. no
			// AutoReopenSpawner wired, or the process crashed between saving
			// the verdict and running the eager transition). Age the verdict
			// itself instead of the session, independent of whatever the
			// liveness checker (or its absence, see getSessionLivenessChecker's
			// doc comment) reports.
			dead = true
		}
		if !dead {
			continue // still plausibly wrapping up on its own — leave it alone
		}

		// latest is only a genuinely *unprocessed* verdict if it belongs to the
		// item's current stay in "review" — i.e. it was created at or after the
		// most recent transition into "review". FindReviewItemsWithUnprocessedVerdict
		// has no notion of "already consumed"; it matches on "most recent
		// review-role session has a dead-and-verdicted state", full stop. A
		// review session whose verdict was already correctly applied once (via
		// the normal real-time handleReviewSessionExited path) stays matchable
		// by that query forever, because nothing marks the verdict as consumed
		// — so if the item later re-enters "review" for any other reason (a new
		// review cycle not yet represented by a new review-role session, or,
		// live 2026-07-20 on item 0fd4a940 (PR #176), a bug elsewhere that
		// force-reopened an already-"done" item), this sweep would treat that
		// stale, already-shipped verdict as fresh and reprocess it — reshipping
		// or reopening an item nothing here should be touching. Comparing
		// against the current review-entry timestamp catches exactly that: a
		// session created before the item's current review stay began cannot
		// be what that stay's outcome will be judged on.
		if reviewAt, found, evErr := er.GetMostRecentStatusEventAt(ctx, item.ID.String(), BacklogStatusReview); evErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] reconcileUnprocessedReviewVerdicts GetMostRecentStatusEventAt item=%s: %v", item.ID, evErr)
		} else if found && latest.CreatedAt.Before(reviewAt) {
			continue // verdict belongs to a prior, already-concluded review cycle
		}

		if latest.EndedAt == nil {
			if endErr := l.storage.UpdateItemSessionEnded(ctx, latest.ID.String(), time.Now()); endErr != nil { //nolint:silenttransition best-effort bookkeeping; the verdict processing below (handleReviewSessionExited) runs regardless of this write's outcome
				log.WarningLog.Printf("[BacklogLifecycle] reconcileUnprocessedReviewVerdicts tombstone item=%s session=%s: %v", item.ID, latest.ID, endErr)
			}
		}

		if latest.Edges.ReviewVerdict != nil {
			log.WarningLog.Printf("[BacklogLifecycle] item %s: review session %s has an unprocessed %s verdict — applying it now",
				item.ID, latest.SessionUUID, latest.Edges.ReviewVerdict.OverallOutcome)
		} else {
			log.WarningLog.Printf("[BacklogLifecycle] item %s: review session %s (the most recent review attempt) exited without ever writing a verdict — processing as a failed review now",
				item.ID, latest.SessionUUID)
		}
		// forcePush=true: this is the crash-recovery sweep for a review session that
		// died before its exit event ever reached handleReviewSessionExited normally
		// — it cannot tell a genuinely-live work session apart from a zombie that will
		// never poll again, so it must make forward progress regardless. See
		// handleReviewSessionExited's doc comment and
		// TestReconcileUnprocessedReviewVerdicts_should_applyPassVerdict_When_ReviewSessionDiedButWorkSessionStillAlive.
		l.handleReviewSessionExited(ctx, ItemSessionSummary{
			ID:            latest.ID.String(),
			BacklogItemID: item.ID.String(),
			SessionUUID:   latest.SessionUUID,
			Role:          string(SessionRoleReview),
		}, true)
	}
}

// markAbandonedReview writes/refreshes the durable abandoned_review row for
// itemID and, once the condition has held past the 15-minute grace
// (abandonedReview pure fn, Story 2.1.0), notifies AND auto-respawns a review
// pass via the injected ReviewRespawner (if wired) — gives the 60s reconcile
// one or more ticks to re-spawn a review gate before flagging, avoiding a
// false positive on an item that just entered review. The row itself is
// mark/refreshed unconditionally so first_detected_at tracks the true onset
// even before the grace elapses. Respawn shares the exact same "notify once"
// gate as the notification (row.NotifiedAt IS NULL): it fires exactly once
// per stuck-row lifetime, not on every tick, so a genuinely-failing item
// doesn't spin the reconciler on repeated re-review attempts — the
// respawned call's own internal iteration cap (see
// BacklogService.AutoRespawnReview) is what actually stops runaway retries
// across separate abandoned_review occurrences. Best-effort: errors are
// logged, never returned.
func (l *BacklogLifecycleListener) markAbandonedReview(ctx context.Context, er *EntRepository, itemID, itemTitle, contextDesc string) {
	applied, err := er.MarkStuck(ctx, itemID, domain.StuckReasonAbandonedReview, BacklogStatusReview, contextDesc)
	if err != nil {
		log.WarningLog.Printf("[BacklogLifecycle] markAbandonedReview MarkStuck item=%s: %v", itemID, err)
		return
	}
	if !applied {
		return
	}

	// 15-minute grace, keyed off the most recent to_status="review" transition
	// (falls back to the row's own first_detected_at if no event is on record,
	// e.g. an item seeded directly into review by a test or migration).
	lastReviewAt, found, evErr := er.GetMostRecentStatusEventAt(ctx, itemID, BacklogStatusReview)
	if evErr != nil {
		log.WarningLog.Printf("[BacklogLifecycle] markAbandonedReview GetMostRecentStatusEventAt item=%s: %v", itemID, evErr)
	}

	rows, findErr := er.FindOpenStuckStates(ctx)
	if findErr != nil {
		log.WarningLog.Printf("[BacklogLifecycle] markAbandonedReview FindOpenStuckStates item=%s: %v", itemID, findErr)
		return
	}
	row, ok := findOpenStuckStateFor(rows, itemID, domain.StuckReasonAbandonedReview)
	if !ok {
		return
	}
	if !found {
		lastReviewAt = row.FirstDetectedAt
	}
	if !abandonedReview(lastReviewAt, time.Now()) {
		return
	}

	// Notify-once dedup: the operator notification itself still fires exactly
	// once per stuck-row lifetime (row.NotifiedAt), independent of the
	// backoff-gated respawn below — otherwise every subsequent automated retry
	// (per the exponential schedule) would also re-notify, which would be
	// spam, not signal.
	if row.NotifiedAt == nil {
		log.WarningLog.Printf("[BacklogLifecycle] item %s stuck in review with nothing in flight (%s)", itemID, contextDesc)
		l.notify(itemID,
			"Review item needs attention",
			fmt.Sprintf("%s — stuck in review with no active session (%s). It may need manual re-review or rework.", itemTitle, contextDesc),
			8, // sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING
			2, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM
		)
		if _, notifyErr := er.MarkStuckNotified(ctx, itemID, domain.StuckReasonAbandonedReview); notifyErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] markAbandonedReview MarkStuckNotified item=%s: %v", itemID, notifyErr)
			// Do NOT proceed to dispatch a respawn below on this tick: a sustained
			// MarkStuckNotified failure would otherwise re-notify (not just
			// respawn) every ~60s tick, breaking the "exactly once per
			// stuck-row lifetime" notification guarantee. The backoff gate below
			// gets its own chance on the NEXT tick regardless.
			return
		}
	}

	// Close the loop: a notification alone leaves the item stuck until a human
	// notices — the exact gap that let 4 real backlog items go stale, some for
	// multiple days (docs/tasks/backlog-feature-improvement.md). Backoff-gated
	// (session/backlog_remediation.go, Phase A of
	// docs/tasks/backlog-stuck-item-auto-remediation.md): fires on this first
	// grace-elapsed tick AND, unlike the notification above, again on each
	// later tick once the exponential schedule allows — up to
	// MaxRemediationAttempts before parking. Dispatched async, bounded by
	// reviewSem (same limiter the sibling review-gate-respawn path in
	// ReconcileStuck uses): a headless re-review call can take minutes, and
	// this runs inside a synchronous detector sweep that must not block.
	respawner := l.getReviewRespawner()
	if respawner == nil {
		log.DebugLog.Printf("[BacklogLifecycle] markAbandonedReview item=%s: no ReviewRespawner configured, notification only", itemID)
		return
	}

	// BUG-043: a respawned review's FAIL/PARTIAL/UNVERIFIABLE verdict only
	// leads anywhere via handleReviewSessionExited's autoReopenWithBackoffGate,
	// which is gated on the SEPARATE StuckReasonBouncing backoff clock, not
	// this one. When bouncing's own gate is currently closed (mid-backoff or
	// parked), respawning here cannot possibly make progress — the diff hasn't
	// changed since the last respawn (nothing reopened the item for rework),
	// so a fresh headless review would just recompute the identical verdict,
	// which would then hit the exact same closed bouncing gate again. Checked
	// BEFORE RemediationDue below so a foregone-conclusion respawn never
	// consumes an abandoned_review attempt — confirmed live 2026-07-23 (three
	// real items burned their entire 5-attempt abandoned_review budget this
	// way, each attempt producing a correct FAIL verdict that a not-due
	// bouncing row silently discarded every time, until abandoned_review
	// itself parked with a "use Reset to retry" notification that never
	// mentioned bouncing was the actual blocker). Best-effort: a query error
	// fails open (proceeds with the respawn) rather than silently stalling an
	// item that might otherwise be perfectly fine to retry.
	if blocked, blockedErr := l.storage.RemediationBlocked(ctx, itemID, domain.StuckReasonBouncing); blockedErr != nil {
		log.WarningLog.Printf("[BacklogLifecycle] markAbandonedReview RemediationBlocked(bouncing) item=%s: %v", itemID, blockedErr)
	} else if blocked {
		log.WarningLog.Printf("[BacklogLifecycle] markAbandonedReview item=%s: skipping respawn — a fresh verdict would be discarded by the bouncing reopen gate, which is not due yet; not spending an abandoned_review attempt on a foregone conclusion", itemID)
		return
	}

	due, justParked, gateErr := l.storage.RemediationDue(ctx, itemID, domain.StuckReasonAbandonedReview)
	if gateErr != nil {
		log.WarningLog.Printf("[BacklogLifecycle] markAbandonedReview RemediationDue item=%s: %v", itemID, gateErr)
		due = true // fail open — see autoReopenWithBackoffGate's identical rationale
	}
	if justParked {
		l.notify(itemID,
			"Auto-rework paused",
			fmt.Sprintf("%s — automated re-review has been retried %d times over an extended period without resolving. It now needs manual attention; use Reset to try again automatically.", itemTitle, MaxRemediationAttempts),
			8, // sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING
			3, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH
		)
	}
	if !due {
		log.InfoLog.Printf("[BacklogLifecycle] markAbandonedReview item=%s: abandoned_review remediation backoff not yet due, skipping respawn", itemID)
		return
	}

	go func(id string) {
		select {
		case l.reviewSem <- struct{}{}:
		case <-l.shutdownCtx.Done():
			return
		}
		defer func() { <-l.reviewSem }()
		if respawnErr := respawner.AutoRespawnReview(l.shutdownCtx, id); respawnErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] markAbandonedReview AutoRespawnReview item=%s: %v", id, respawnErr)
		}
	}(itemID)
}

// reviewVerdictIdleThreshold bounds how long reconcileUnprocessedReviewVerdicts
// trusts SessionLivenessChecker's "alive" verdict once a review session has
// actually saved a verdict. Set to maxWorkSessionStaleness's value rather than
// abandonedReview's much shorter 15-minute grace: a reviewer doing legitimately
// slow verification (large diff, running a full test suite) must not be reaped
// mid-review, so this errs conservative — same order of magnitude as the other
// "is this session still doing real work" threshold in this file, not the much
// tighter "did anything ever start" grace period abandonedReview enforces.
const reviewVerdictIdleThreshold = maxWorkSessionStaleness

// reconcileRespawnBlockedActiveResolution is the resolve-only counterpart to
// notifyRespawnBlockedByActiveSession (server/services/backlog_service_triage.go),
// which marks StuckReasonRespawnBlockedActive but has no periodic tick of its
// own guaranteed to notice when the blocking session ends. Unlike
// reconcileReworkBlockedStaleResolution above, this closes a gap that is not
// merely "convenient" but load-bearing for one of the three guarding
// functions: AutoRespawnReview's only caller, markAbandonedReview, gates the
// respawn attempt behind Storage.RemediationDue(StuckReasonAbandonedReview)
// — once that gate exhausts its attempts and parks, markAbandonedReview never
// calls AutoRespawnReview again for that item, so MarkStuck's own
// guard-passing resolve path (the resolveRespawnBlockedActiveLogged call at
// the top of AutoRespawnAutonomousWork/AutoReopenForPRFix/AutoRespawnReview)
// would never re-run and the row would be permanently orphaned — reproducing
// the exact "silently stuck forever" bug class this whole reason exists to
// surface. AutoRespawnAutonomousWork and AutoReopenForPRFix don't strictly
// need this sweep (both are re-invoked on every reconcile tick regardless of
// backoff/parking), but StuckReasonRespawnBlockedActive is a single shared
// reason across all three call sites, so one unconditional sweep covering all
// of them is simpler and more robust than trying to prove each caller's retry
// path is unconditional.
//
// Unlike reconcileReworkBlockedStaleResolution, this needs no
// SessionStopper-backed liveness/staleness check (no Resolver interface
// indirection) — StuckReasonRespawnBlockedActive only cares whether the
// blocking work/review ItemSession has ended, a plain EndedAt-nil check
// already available in-package via hasActiveSession (see its doc comment:
// "package-local equivalent of server/services' hasActiveWorkSession/
// hasActiveReviewSession"). Best-effort: query errors are logged, never
// returned, so one item's failure can't skip the rest.
func (l *BacklogLifecycleListener) reconcileRespawnBlockedActiveResolution(ctx context.Context, er *EntRepository) {
	open, openErr := er.FindOpenStuckStates(ctx)
	if openErr != nil {
		log.WarningLog.Printf("[BacklogLifecycle] reconcileRespawnBlockedActiveResolution FindOpenStuckStates error: %v", openErr)
		return
	}
	for _, row := range open {
		if row.Reason != domain.StuckReasonRespawnBlockedActive {
			continue
		}
		sessions, sessErr := l.storage.ListItemSessions(ctx, row.ItemID)
		if sessErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] reconcileRespawnBlockedActiveResolution ListItemSessions item=%s: %v", row.ItemID, sessErr)
			continue
		}
		if hasActiveSession(sessions) {
			continue // still genuinely blocked — leave the row open
		}
		l.resolveStuckLogged(ctx, er, row.ItemID, domain.StuckReasonRespawnBlockedActive, "reconcileRespawnBlockedActiveResolution")
	}
}
