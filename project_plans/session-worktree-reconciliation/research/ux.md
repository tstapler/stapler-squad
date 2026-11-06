# UX Research: session-worktree-reconciliation

## Bottom line

**No new UI surface.** This feature should reuse the generic operator-notification
pipe (`events.NewNotificationEvent` → `EventBus` → `NotificationPanel`/`NotificationsNavBadge`)
exactly as `notifyIfActiveWorkSessionStale` does today
(`server/services/backlog_service_triage.go:1560-1601`). There is no new component,
no new badge, and no new panel to design. The candidate alternative surface — the
"Stuck Backlog Items" panel (`StuckReason` enum + `StuckItem`/`StuckItemDetail`/
`StuckItemsSection`) — does not fit, and forcing this feature into it would require
bending that system's data model in ways the requirements don't call for (see Q1).

## 1. Existing notification UX for similar flags

Two distinct "operator flag" surfaces exist in this codebase; they solve different
problems and this feature's flag maps cleanly to only one of them:

**Surface A — generic notification pipe (fits this feature).**
`events.NewNotificationEvent(sessionID, sessionName, notificationID, notificationType,
priority, title, message, metadata)` (`pkg/events/types.go:339-360`) publishes onto
the shared `EventBus`; the frontend subscribes via `NotificationContext.tsx` and
renders through `NotificationPanel.tsx` (the bell-icon dropdown) and
`NotificationsNavBadge.tsx` (unread-count badge). Nothing about this pipe is
per-feature — any backend code can call `NewNotificationEvent` and it appears in the
existing panel with zero frontend changes. `notifyIfActiveWorkSessionStale`
(`server/services/backlog_service_triage.go:1560`) is the closest existing analog:
a background reconciliation-style check (staleness of a work session) that can't be
safely auto-fixed, so it (a) durably marks state via `storage.MarkStuck`, and
(b) publishes a `NOTIFICATION_TYPE_WARNING` / urgent+important notification so the
operator sees it in the bell even if they miss the toast.

**Surface B — "Stuck Backlog Items" panel (does not fit).**
`domain.StuckReason` (`session/ent/schema/backlog_stuck_state.go:38+`, 19 values
including `StuckReasonReworkBlockedStale`, `StuckReasonStaleWork`, etc.) backs a
dedicated board section: `StuckItemsSection.tsx` → `StuckItem.tsx` (chip, duration,
retry/snooze controls) → `StuckItemDetail.tsx` (expanded detail, rework-cap override,
plan-approval). This is a much richer, purpose-built UI (icons, priority ordering via
`STUCK_REASON_PRIORITY`, snooze, "Retry now", cross-reference badges for
multi-reason items — `web-app/src/components/backlog-stuck/stuckReason.ts:15-133`).
It looks like the natural home for "flag for operator attention," but it is
structurally **item-status-scoped**: `MarkStuck(ctx, itemID, reason, expectedStatus,
context)` (`session/ent_repository_backlog.go:2126`) requires both a real backlog
`item_id` and a single `expectedStatus` precondition ("only write if the item is
still in exactly this status") — see the doc comment at
`session/ent_repository_backlog.go:2100-2125`. "Worktree tracking broken" is
orthogonal to backlog status (it can occur at any status, or on a session with
**no** linked backlog item at all — the requirements doc's "orphaned/leaked
sessions" framing explicitly includes sessions whose item may already be terminal
or absent). Bending Surface B to accept an optional item + no status precondition
would be a schema/semantics change to a system this project's requirements never
ask to touch. **Recommendation for the planning phase: do not add a new
`StuckReason` value; publish via Surface A instead**, and call `storage.MarkStuck`
only in the (frequent but non-universal) case where the broken session does have a
live, non-terminal-status backlog item — optional, best-effort, exactly like
`notifyIfActiveWorkSessionStale`'s own `MarkStuck` call.

## 2. Operator mental model / message shape

The operator here is the developer running stapler-squad locally (per
`docs/reference/state-isolation.md`), not an end user — same audience the
`docs/how-to/debug-with-logs.md` log-format conventions target. The existing
notification-emitting functions in `backlog_service_triage.go` establish a
consistent message template worth copying verbatim in shape:

- `notifyIfActiveWorkSessionStale` (`:1560`): title `"Rework blocked by a
  stale-but-alive session"`, body `"<item> — a failed review can't reopen ...
  because its active work session hasn't produced output in over <duration>. The
  session is still running, so it will not be stopped automatically; check it
  manually, or use \"Reopen for Revision\" once you've confirmed it's actually
  stuck."`
- `notifyReworkCapHit` (`:169-197`) and `notifyRepeatedFailure` (`:207-232`) follow
  the identical shape: **what** happened, **why it wasn't auto-fixed**, and **what
  action the operator can take**.

Every one of these three fields is present in every existing message. For this
feature's flag, the equivalent triad is:

1. **What's broken** — which session (title/UUID), which worktree field
   (`repo_path`/`worktree_path`/`base_commit_sha`), and the specific inconsistency
   found (missing row vs. unresolvable path vs. unresolvable SHA).
2. **What the sweep already tried** — e.g. "could not derive `repo_path`
   unambiguously from the live git worktree list" or "N candidate worktrees
   matched, could not pick one" — the same "why it wasn't auto-fixed" slot the
   existing messages fill, so the operator doesn't manually redo the sweep's own
   diagnostic work.
3. **What to do** — since there's no UI action button on a plain notification
   (unlike the Backlog-stuck panel's Retry/Snooze), the message body itself needs
   the actionable next step in prose (e.g. which DB fields to check/edit, or "the
   session's worktree could not be repaired automatically — see logs for the sweep
   run" pointing at `docs/how-to/debug-with-logs.md`).

`map[string]string{"item_id": itemID}` in metadata is the mechanism that makes a
notification click-through to `/backlog?item=<id>` ("View in Backlog" link,
`web-app/src/components/ui/NotificationItem.tsx:311-319`); passing the actual
session UUID as `sessionID` (not substituting `itemID` the way
`notifyReworkCapHit` does) drives "View Session" (`:321-328`, falls back to the
durable `/sessions/summary` route once the tmux session is gone —
`getSessionHref`, `NotificationItem.tsx:110-119`). **Note the two links are
mutually exclusive in the current rendering logic** — if `metadata["item_id"]` is
set, "View Session" is suppressed even when `sessionId` is also populated
(`NotificationItem.tsx:311` vs `:321`). For a worktree-tracking flag, the session
itself (not the backlog item) is almost always the more useful click-through
target, since the defect is in the session's own DB/git state — planning should
decide deliberately whether to set `item_id` in metadata at all, rather than
copying the `notifyReworkCapHit` pattern by default and losing the session link.

## 3. Accessibility / keyboard nav

No new component is planned (see Bottom line and Q1) — this reuses
`NotificationPanel.tsx`/`NotificationItem.tsx`/`NotificationsNavBadge.tsx`
unchanged. Those already have their own accessibility surface (`role="dialog"`,
`aria-modal`, `aria-label`s throughout, keyboard-dismissible) which this feature
does not add to or need to re-audit. **Explicitly skipped per the task's own
instruction: no new component, no new a11y surface.**

## 4. Error states — repair itself fails

The codebase already has a two-tier severity convention across `NotificationType`
that maps directly onto "flag" vs. "flag, and we couldn't even try":

- `NOTIFICATION_TYPE_WARNING` (used by `notifyIfActiveWorkSessionStale`,
  `notifyReworkCapHit`, `notifyRepeatedFailure`) — "surfaced for manual review, but
  the system is not broken, just needs a human decision."
- `NOTIFICATION_TYPE_ERROR` / `NOTIFICATION_TYPE_FAILURE` — reserved in
  `server/review_queue_manager.go:955-958` for `ReasonErrorState`
  (`NOTIFICATION_TYPE_ERROR`) and `ReasonTestsFailing`
  (`NOTIFICATION_TYPE_FAILURE`) respectively — i.e. an actual failure condition,
  not just an unresolved decision.

Recommendation: **same flag path, escalated severity — not a distinct UI state.**
"Detected an inconsistency, auto-repair not attempted (ambiguous)" is a `WARNING`
at `derivePriority(urgent=true, important=true)` (matching the existing
manual-review notifications). "Attempted repair and the derived value(s) also
don't resolve" is a genuinely worse case — the sweep tried and failed, not just
declined — and should use `NOTIFICATION_TYPE_ERROR` (or `FAILURE`, if the
"repair" is modeled as an operation with a pass/fail outcome) to let the operator
visually distinguish "needs your judgment call" from "the sweep itself hit a wall
and this needs deeper investigation," consistent with the existing type taxonomy.
Both cases go through the identical publish path — no second component, no
separate escalation UI.

## Files referenced

- `pkg/events/types.go:339-360` — `NewNotificationEvent` signature
- `server/services/backlog_service_triage.go:159-232,1516-1601` — message-shape
  template (`notifyReworkCapHit`, `notifyRepeatedFailure`,
  `notifyIfActiveWorkSessionStale`)
- `server/services/notification_priority.go:11-22` — `derivePriority`
- `web-app/src/components/ui/NotificationPanel.tsx`,
  `NotificationsNavBadge.tsx`, `NotificationItem.tsx` — rendering pipe (unchanged)
- `session/ent_repository_backlog.go:2100-2126` — `MarkStuck` precondition model
  (why Surface B doesn't fit)
- `web-app/src/components/backlog-stuck/stuckReason.ts`,
  `StuckItem.tsx` — Surface B, considered and rejected as the home for this flag
- `server/review_queue_manager.go:948-967` — `NotificationType` severity taxonomy
  (`ERROR`/`FAILURE` vs `WARNING`)
