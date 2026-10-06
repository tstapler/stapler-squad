# Requirements: notification-revamp

**Date**: 2026-09-04
**Type**: feature addition (cross-cutting: backend signal quality + frontend IA reboot)
**Complexity**: 3 — system design, multiple epics, consolidates prior stalled plans

## Problem Statement

The Notifications page and Review Queue page (mobile screenshots, 2026-09-04:
152 notifications, 107 review-queue items, "47 idle, 39 stale") have stopped
functioning as an attention signal. Two concrete failure modes, confirmed
against the current codebase:

1. **Notifications never resolve.** `server/notifications/store.go`'s
   `Append()` only collapses a repeat `(sessionID, notificationType)` event
   into an existing row while that row is unread (store.go:149-165, per the
   in-file ADR-003 comment at L21-27). Once a notification is read, every
   later recurrence of the same event forks a brand-new row instead of
   bumping a counter — unbounded growth of low-value rows ("Claude turn
   complete") that never reflects a resolved state.
2. **Idle sessions clutter the review queue and don't stay dismissed.**
   `session/review_queue_determiner.go` treats "idle" (5s since last update,
   L258-265) as a review-queue reason with no acknowledgment suppression
   (unlike Stale, which has it at L281-288, and unlike the working-state
   distinction this same repo already scoped and shelved — see Prior Art).
   Skipping/acknowledging an idle item doesn't stick; it reappears on the
   next state change.
3. **Auto-approval doesn't reconcile the backlog it should clear.** When the
   classifier (`pkg/classifier/classifier.go`) auto-decides
   (`AutoAllow`/`AutoDeny`), `approval_handler.go:388-419` returns before a
   review item is ever created — correct going forward, but nothing
   re-evaluates *already-escalated* pending approvals when a rule is added
   or edited later. A pending approval that a new rule now covers sits
   forever until a human clicks it, which is the exact complaint that
   triggered this project ("a rule auto-approves something and it still
   needs my attention").
4. **Both pages are flat, ungrouped, reverse-chron lists.** Severity/priority
   fields already exist (see Prior Art) but the UI doesn't use them to
   separate "needs a decision" from "FYI" — every row gets equal visual
   weight.

## Prior Art (read before planning — do not re-derive)

This exact problem space has been planned before in `project_plans/`. Phase 2
research MUST read the following instead of re-investigating from scratch:

**Already shipped — do not re-propose:**
- `dynamic-rule-reload` (merged #538) — rules reload at runtime without
  restart. Relevant: a reconciliation hook can piggyback on the existing
  reload path rather than inventing a new trigger.
- `review-queue-severity` (merged #411) — `RiskLevel` (LOW/MEDIUM/HIGH) is
  already threaded from the classifier through `PendingApproval` into
  `ReviewQueuePanel.tsx` and `ApprovalCard`/`ApprovalRulesPanel`. The
  determiner also already has a separate `Priority` enum
  (Low/Medium/High/Urgent, `review_queue_determiner.go:118+`). **The gap is
  not "no severity model" — it's that idle items are hardcoded
  `PriorityLow` (L160, L235) and the UI doesn't visually separate priority
  tiers**; it lists everything with equal weight.
- `review-session-notification-cleanup` (merged #227) — headless
  review/triage sessions (`Instance.Hidden`) are already suppressed from
  notifications.
- `stale-session-detection` (merged #515) — the `StaleSessionNotifier`
  mechanism (server/services/stale_session_notifier.go) is the shipped
  result of this; it only *adds* stale notifications, confirmed no removal
  path exists.

**Planned, never implemented (plan.md exists, no trace in code) — fold in,
don't restart:**
- `review-queue-state-detection` (requirements dated 2026-05-02) — same root
  problem as Failure Mode 2 above: idle/stale misclassifies sessions where
  Claude is actively mid-turn. Its FR-1–FR-3 (multi-signal working-state
  detection, queue filtering, structured state-change events) are still the
  right shape for that sub-problem. **Its FR-4 (golden-state capture
  corpus/labeling tooling) is scope creep for this project — see Out of
  Scope.** Also check [[instinct_detection_status_ready_dead_code]]-adjacent
  reality: a `detection` package with `StatusIdle`/`StatusContext` already
  exists for the unrelated unattended-PTY-write feature — research must
  determine whether that state machine can be reused instead of building new
  regex heuristics from scratch.
- `review-queue-event-driven` — replaces the 2s poller with event-driven
  status-change callbacks to cut queue latency from ~2s to ~1s. This is a
  latency/architecture concern, not a signal-quality concern, and doesn't
  address why the page feels unusable. **Deferred — see Out of Scope.**
- `smart-notification-dedup` — a *different* dedup bug than Failure Mode 1:
  covers health/system alerts (fork-pressure, tmux recovery) re-firing on an
  unchanged condition, plus native OS/browser notification auto-dismiss.
  Different subsystem (health-alert service, not
  `NotificationHistoryStore`). Worth doing for consistency but not the
  source of the complaint that triggered this project — include only if
  appetite allows after the core fixes.

## Baseline

Today: notifications accumulate without bound past the first read of each
type; the review queue is dominated by idle items that reappear after being
skipped; approvals a new rule would now auto-decide sit unresolved
indefinitely; both pages present everything with equal visual priority. The
user has stopped trusting either page to tell them what actually needs
action.

## Users / Consumers

Single operator (Tyler) via the stapler-squad web UI (desktop + mobile, per
`feedback_mobile_desktop_ux` memory — both form factors must be considered).

## Success Metrics

- A repeated informational event (e.g. "Claude turn complete") for the same
  session produces one row with an occurrence count, not N rows, regardless
  of read state.
- Skipping/acknowledging any review-queue item (including idle-class
  reasons) keeps it out of the queue until its underlying state materially
  changes — it does not reappear on the next poll tick alone.
- A session that is actively generating output/mid-turn does not appear as
  "idle" or "stale" in the review queue.
- Adding or editing an approval rule auto-resolves any currently-pending
  escalated approval that the new rule would now auto-decide, with a visible
  "Auto-resolved by rule: <name>" note on the record (never silent — per
  [[feedback_document_ai_decisions_in_edge_cases]]).
- Both pages default to showing "needs a decision" (pending approvals,
  errors, genuinely stuck sessions) above the fold, with informational/
  completed activity grouped and collapsed rather than interleaved 1:1.
- No bulk action (e.g. "mark all read," "skip all") ever silently marks
  read, dismisses, or otherwise clears an item that still needs a decision.
  An item leaves the needs-a-decision view only by being resolved (approved,
  denied, opened and acted on) or because the underlying state that put it
  there changes — never as a side effect of a bulk action scoped to
  informational activity (per
  [[feedback_document_ai_decisions_in_edge_cases]] and this project's own
  founding complaint that items don't stay resolved).

## Appetite

Medium (1–2 weeks equivalent). Scope is deliberately trimmed from the full
ambition of the stalled `review-queue-state-detection` /
`review-queue-event-driven` plans to fit this — see Out of Scope.

## Constraints

- No new persistence layer beyond what already exists
  (`notifications.json`-backed store, in-memory review queue) unless
  research shows it's unavoidable for the reconciliation feature.
- Must not regress `TestReviewQueue*`/`TestReviewQueuePoller*` or existing
  e2e specs (`tests/e2e/`, per `e2e-test-conventions` skill).
- Detection-heuristic changes carry real regression risk (recall
  `review-gate-stale-session-rework` PR #219's threshold recalibration) —
  changes to idle/stale/working-state logic need a fix + a check (test or
  golden fixture), not just a threshold tweak.

## Non-functional Requirements

- **Performance SLO**: not specified — this is a single-operator internal
  tool; the ~2s poll latency is explicitly out of scope (see below).
- **Scalability**: dozens of concurrent sessions, single user — not a
  driver of design decisions here.
- **Security classification**: internal.
- **Data residency**: not applicable.

## Scope

### In Scope

1. Fix `NotificationHistoryStore.Append()` to dedup regardless of read
   state (bump `OccurrenceCount`, flip to unread) instead of forking new
   rows after first read. Exception: `AUTO_APPROVED` records still collapse
   into one row on recurrence, but never flip back to unread — they're
   written pre-read by design, and recurrence shouldn't manufacture an
   unread state that never existed for that type (see ADR-001 in the
   implementation plan).
2. Add acknowledgment suppression for idle-class review-queue reasons,
   matching the existing Stale pattern.
3. Working-state detection: prevent sessions that are actively producing
   output/mid-turn from being classified idle/stale — reusing existing
   signals (`LastMeaningfulOutput`, existing `detection` package states,
   PTY activity) rather than building new golden-corpus tooling.
4. Rule-reconciliation: on rule upsert/reload, re-run the classifier against
   pending `Escalate`-created review items and auto-resolve any that now
   auto-decide, with a visible audit note + notification.
5. Notifications page IA reboot: "Needs a decision" (unread actionable)
   section always on top; recent/completed activity grouped per-session and
   collapsed instead of one row per event; keep the existing "Auto-handled"
   section, extend it to include rule-reconciled items.
6. Review Queue page IA reboot: use the existing `Priority`/`RiskLevel`
   fields to visually separate genuinely-actionable items from low-priority
   noise instead of listing everything with equal weight; idle-only "ready
   for next task" sessions move to a status chip on the Sessions list rather
   than occupying review-queue slots.

### Out of Scope

- Golden-state capture/labeling corpus tooling (`review-queue-state-detection`
  FR-4) — disproportionate infra for a single-operator tool; revisit only if
  heuristic accuracy proves impossible to validate otherwise.
- Full event-driven poller rearchitecture (`review-queue-event-driven`) —
  addresses ~1s of latency, not the signal-quality complaint that triggered
  this project. Candidate follow-up project.
- Health/system alert dedup and native OS notification auto-dismiss
  (`smart-notification-dedup`'s fork-pressure/tmux-recovery scope) — separate
  subsystem; fold in only if Phase 3 planning finds it's cheap to piggyback
  on the notification-store dedup fix, otherwise a separate follow-up.
- Rules page visual builder (`rules-ux-redesign`) — tangential; not a
  blocker for the signal-quality problem here.
- Any change to `WatchReviewQueue`'s wire protocol beyond what's needed for
  the new reconciliation event.

## Rabbit Holes

- Working-state detection heuristics (spinner text, "esc to interrupt",
  tool-call patterns) are inherently fragile against Claude Code UI changes.
  Cap this at reusing/extending existing signals; do not chase perfect
  detection.
- Rule reconciliation touching classifier internals could balloon into a
  classifier refactor — scope it strictly to "re-run existing `Classify()`
  against pending items," not a classifier redesign.
- "Notifications page IA reboot" could expand into a full activity-feed
  product (search, saved filters, etc.) — cap at the grouping/prioritization
  described in Scope; anything fancier is a follow-up.

## Alternatives Considered

- Pure frontend fix (client-side grouping/hiding only, no backend changes):
  rejected — the dedup bug and idle-reappearance bug are server-side state
  problems; a frontend band-aid would re-break on refresh/other clients
  (mobile web, per the screenshots).
- Full adoption of the stalled `review-queue-event-driven` architecture now:
  rejected for this pass — it's a latency optimization orthogonal to why the
  page feels unusable today; see Out of Scope.

## Feasibility Risks

- Reusing the existing `detection` package's `StatusIdle`/`StatusContext`
  state machine for review-queue working-state filtering may not map
  cleanly (it was built for the unattended-PTY-write gating use case) —
  research must confirm reuse is viable before planning commits to it.
- Rule-reconciliation needs to run the classifier against historical pending
  items without re-triggering side effects meant for live requests (e.g. no
  double-notification, no interference with a request the user is mid-way
  through answering).

## Observability Requirements

Log a structured event whenever the reconciliation pass auto-resolves a
previously-pending approval (rule name, item ID, before/after decision) —
this is the one place silent state change would violate house policy on
visible AI-driven decisions. Standard request logging otherwise sufficient.

## Risk Control

No feature flag — this is a single-operator internal tool behind no public
traffic. Rollback is a straightforward revert (JSON-file-backed state, no
migration). Land behind a branch/PR reviewed before merging to `main`, per
normal repo flow.

## Open Questions

All three resolved during planning — kept here for the record rather than deleted, per the project's audit trail:

1. **Resolved (Phase 2 research)**: the existing `detection` package's
   `StatusIdle`/`IdleState` enums are already reused by the review queue's
   controller-active branch — no separate state machine to integrate. The
   actual gap was `StatusWaitingForAgent` going unhandled in that branch,
   closed by plan.md's Epic 1.3.
2. **Resolved (Phase 3 planning, ADR-004)**: rule-reconciliation hooks into
   `RulesService`'s existing `rebuildClassifier()`/`onReload` path (the
   `dynamic-rule-reload` mechanism), serialized via the existing `rebuildMu`,
   running as a goroutine spawned after `rebuildMu.Unlock()`.
3. **Resolved (Phase 3 planning, ADR-002)**: "idle, ready for next task" is
   removed from the Review Queue entirely — it duplicates the Sessions
   list's status display, which now surfaces it as a status chip instead
   (plan.md Epic 3.2.2b). Confirmed by UX design (`design/ux.md` Surface 10)
   with no dissent from either artifact.
