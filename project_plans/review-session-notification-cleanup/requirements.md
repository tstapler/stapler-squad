# Requirements: Notifications for Headless Review/Triage Sessions

Backlog item: `153f8eac-c454-4fa3-a8f4-83b070b9a035`
Priority: 2 | Pipeline: sdd

## Problem

The Notifications page fills with lifecycle notifications ("TASK COMPLETE —
Session idle - ready for next task", "WARNING — No activity for Nh Nm —
session may be stuck or waiting") generated for ephemeral, one-shot
`review:<hash>` / `headless-<uuid>` sessions spawned by the backlog
review-gate/triage flow. These add no value: the underlying process has
already exited by the time the notification is read, and the only click
target ("View Session") leads to a dead session view.

## Acceptance Criteria

1. Headless/ephemeral review and triage sessions are suppressed via
   `Instance.Hidden == true` as the sole necessary-and-sufficient trigger
   (verified to always co-occur with `ItemSession.SessionRole == review` for
   every real call site today, and `SessionRole == triage` sessions never
   construct an `Instance` at all — see Design Decision 1 in
   `implementation/plan.md` for the full justification), so that they no
   longer generate generic TASK_COMPLETE / Session-idle / Stale
   notifications. `SessionRole` is additionally resolved and used for
   `item_id` enrichment/corroboration but is never an independent
   suppression trigger.
2. Any notification tied to a backlog-linked session includes `item_id` in
   its metadata so the frontend links to "View in Backlog" (the review
   verdict) instead of the dead "View Session" link.
3. Notifications referencing a session/instance that no longer exists are
   pruned rather than sitting untouched for up to 7 days.
4. A regression test confirms a headless review session completing/going
   stale does not produce a Notifications-page entry.

## Current State (verified by code inspection)

- `session/instance.go:220-223` — `Instance.Hidden bool`: "excludes this
  session from the default session list and review queue." Set by
  `SpawnReviewSession` (`server/services/session_service.go:826-833`) via
  `CreateDirectorySession(..., oneShot=true, hidden=true)`.
- `session/review_queue_poller.go:627-636` (`shouldSkipSession`) is the
  *only* place `Hidden` gates anything in the notification path: it's
  checked before `checkSession` calls `Determine()`
  (`session/review_queue_determiner.go`) and before an item ever reaches
  `rqp.queue.Add()`. `Determine()` itself has no knowledge of Hidden,
  OneShot, or SessionRole — it only inspects `Instance`/`InstanceStatusInfo`
  (idle state, controller status, staleness via
  `GetTimeSinceLastMeaningfulOutput()`).
- `server/review_queue_manager.go:319-373` (`OnItemAdded`) is the sole
  place a `ReviewItem` becomes a notification: title is
  `"<Reason>: <SessionName>"`, message is `item.Context` (e.g. "Session idle
  - ready for next task", "No activity for Nh Nm..."). It fires for every
  `ReviewQueue.Add` except `ReasonApprovalPending` (deduped against the hook
  notification). It does not check `Hidden` or `SessionRole` itself — it
  trusts the queue only contains what the poller decided to add.
- `item.Metadata` is populated in `checkSession` only for
  `ReasonApprovalPending` (`session/review_queue_poller.go:807-830`,
  `tool_name`, `pending_approval_id`, etc.) — never `item_id`. Separately,
  `session/review_queue_poller.go:809-826`-area metadata building for the
  approval path is the only metadata producer; TASK_COMPLETE/IDLE/STALE
  items get `nil` Metadata today.
- `ItemSession` (`session/ent/schema/item_session.go`) has `session_uuid`
  (loose FK, not an ent edge) and a required `backlog_item` edge — so
  resolving `item_id` for a given `Instance.UUID` means looking up the
  `ItemSession` row by `session_uuid` and following the edge, not reading a
  field directly off `Instance`.
- `web-app/src/app/notifications/NotificationsPage.tsx:377-393` already
  prefers "View in Backlog" over "View Session" whenever
  `notification.metadata["item_id"]` is present — the frontend half of AC2
  requires no change; only the backend metadata producer needs to set it.
- `server/notifications/store.go` prunes by age (`MaxNotificationAge = 7 *
  24h`) and count (`MaxNotifications = 500`, `enforceRetention()` at
  `:437`) only. No existing check ties a stored notification back to
  whether its session/instance still exists.
- **Open question for research phase**: `server/services/backlog_service_triage.go:1781-1796`
  shows headless triage sessions are spawned with a **synthetic**
  `SessionUUID` (`headlessTriageUUIDPrefix + uuid.New()`) run through a
  `headlessPool` — there is no corresponding `session.Instance`/tmux
  session for these at all. Since `Determine()`/`shouldSkipSession` only
  operate on `*Instance`, the mechanism (if any) by which a completed or
  stale *headless-triage* session produces a TASK_COMPLETE/Idle/Stale
  notification is not yet located and must be confirmed in Phase 2
  research — it is likely a separate code path from the `review:<hash>`
  case (which does have a real, `Hidden=true` `Instance`).

## Scope

### In scope
- Suppress `ReasonTaskComplete` / `ReasonIdle` / `ReasonStale` notification
  generation (not just review-queue visibility) for sessions that are
  `Hidden` and/or backed by an `ItemSession` with `SessionRole` `review` or
  `triage`, at the point notifications are actually published
  (`OnItemAdded` and/or `Determine`/`checkSession`), independent of the
  poller's existing `shouldSkipSession` queue-visibility guard.
- Thread `item_id` into notification `Metadata` for any notification tied
  to a backlog-linked session (via `ItemSession.session_uuid` →
  `backlog_item` lookup), so any notification that *does* legitimately fire
  for a backlog-linked session (e.g. a real interactive work session going
  stale) routes to "View in Backlog" rather than "View Session".
- Add pruning of notifications whose referenced session/instance no longer
  exists, independent of the 7-day age-based retention.
- Cover whichever code path headless-triage notifications actually
  originate from (per the open question above) with the same suppression.
- Regression test(s) proving a headless review/triage session reaching
  TASK_COMPLETE/Idle/Stale does not produce a Notifications-page entry.

### Out of scope
- Changing behavior for real, non-headless interactive sessions — those
  should keep generating TASK_COMPLETE/Idle/Stale notifications exactly as
  today.
- Changing the "View in Backlog" frontend routing logic itself (already
  correct, confirmed above) — only the backend metadata producer changes.
- Any change to `ReasonApprovalPending` notification handling (already
  deduped/enriched separately and out of this item's scope).
- Broader review-queue UX changes (item ordering, priority tuning, etc.)
  not called out in the acceptance criteria.

## Constraints / Notes

- Existing rule `.claude/rules/prefer-go-git-over-subshells.md` — n/a here,
  no git shelling involved.
- `.claude/rules/ent-schema-generation.md` applies if any ent schema field
  needs to change (e.g. if `item_id` needs to be queryable more cheaply) —
  use `--feature sql/upsert` on regenerate.
- Must not regress `Instance.Hidden`'s existing effect (still excluded from
  default session list / review queue UI).
- Fix must be defense-in-depth at the notification-publishing source(s), not
  solely a second reliance on the poller's `shouldSkipSession`, per AC1's
  intent (matches the backlog item's explicit call-out that Hidden/SessionRole
  checks are currently siloed in one call path).
