# ADR-008: Bulk Actions Are Scoped to Informational Rows, Guarded Server-Side, Synced by Id Set

**Status**: Proposed (revised in plan repair iteration 3: the field is also stamped on the live event; Phase 4: shipped label "Mark activity read", `kept` handling on the Notifications page, file renamed to match the title; triad iteration 1: one deck bulk control, "Move all to tray"; the predicate excludes an auto-remediating WARNING; the pinned group is "Needs attention"; the undo window is configurable)
**Date**: 2026-10-07
**Project**: notification-tray-and-hidden-session-gate

## Context

#738 requires that no bulk action clears an item needing a decision.
`NotificationHistoryStore.Clear` already never removes an unread actionable
record (`server/notifications/store.go:419-430`), but
`ClearNotificationHistoryRequest` has only `before_timestamp` (proto line
1785), so "Clear informational" cannot be expressed. Cross-tab sync handles
only `NOTIFICATION_DISMISSED` (`NotificationContext.tsx`), and a "clear
everything now" message would wipe items that arrived in the other tab after
the click.

## Decision

1. Verb discipline: the toast deck has **one** bulk control, **Move all to
   tray (N)** (client-only, demote-only, no RPC; the earlier "Dismiss all" is
   withdrawn because routine types never toast and nearly every toast is
   pinned, so it was a near-dead path and its slot flipped label with the deck's
   contents, a mode error); dismissal is per toast (the x or a swipe, informational
   only). **Mark activity read** (tray header icon, visual only; the shipped label at
   `NotificationsPage.tsx:304`, not "Mark all read", because it excludes unread
   decisions), **Clear
   informational (N)** and **Clear history** (tray overflow menu, with a
   confirm line listing what is kept and an undo toast).
2. **One definition of "pending decision", computed on the server.** Add an
   additive `is_pending_decision` bool to `NotificationHistoryRecord`
   (`IsPendingDecision(type, metadata, read)` = `IsActionableType(type)`
   (`server/notifications/store.go:55`) AND NOT `auto_remediating=true` AND
   unread), **and the same bool on the live `NotificationEvent`** (`events.proto`
   field 10, computed at creation, stamped in
   `server/services/event_converter.go:72`). **Auto-remediating WARNING (triad
   iteration 1)**: a producer whose WARNING only reports that automation is
   already acting stamps `events.MetadataKeyAutoRemediating` (known:
   `session/backlog_lifecycle_pr.go:1317`, "PR needs attention ... An automated
   fix attempt will run"). Such an event is not a pending decision: it does not
   pin, does not count, and is dismissible and clearable; if the automation fails
   the producer escalates with an unstamped ERROR/FAILURE, which pins. `Clear`,
   `ClearByIDs` and the prune all call the one predicate, which narrows what
   "Clear history" keeps (flag-independent, listed in the contract PR 1
   description). The tray group is named "Needs attention" because the pinned set
   includes errors and failures. A toast is shown from the live event
   before any history record exists, so without this the client would need its
   own type list for `isPinned` (a fourth predicate; architecture C6,
   adversarial N5).
   Today three predicates disagree (server `IsActionableType` includes WARNING;
   TS `ACTIONABLE_TYPES` includes it; TS `isActionable` does not); the client
   predicates are removed or reduced to reading the field. `isPinned` for
   toasts reads the event/record field only (no type list) and is presentational
   only; it is never used to choose rows to delete.
3. Add optional `repeated string notification_ids = 2` to
   `ClearNotificationHistoryRequest` and a store method `ClearByIDs` that
   applies the same unconditional "never delete an unread actionable record"
   guard and returns `(deleted, kept)`. The client selects rows with
   `is_pending_decision == false`; the server remains the safety net, and the
   client restores any row listed in `kept`. The tray and the Notifications page
   handle a `kept` response identically through one shared helper (plan Task
   4.4f, ux.md TM-10): kept rows are restored in place and a status line "N kept:
   needs a decision" is shown.
4. New BroadcastChannel message `NOTIFICATIONS_BULK_DISMISSED { ids: string[] }`
   (idempotent, id-set based; not synced: per-session collapse state). Receivers
   remove exactly those ids. `storage`-event fallback where BroadcastChannel is
   unavailable.
5. Optimistic updates roll back with a visible message on RPC failure.

## Alternatives Considered

- N `MarkRead`/delete RPCs: rejected; N round trips and partial failure.
- Type-filter on the server (`types` field): rejected; no id receipt for undo.
- A client predicate kept in parity with the server by a shared fixture
  (earlier draft): rejected; a fourth predicate plus a parity test still lets
  an unread WARNING count as informational client-side while the server keeps it.
- The history prune (plan Story 2.7) is *outside* this safety net by design:
  it is operator-triggered, dry-run first, and never deletes an unread pending
  decision unless the operator passes `include_unread_actionable`.

## Consequences

- One proto field plus `make proto-gen`; `make registry-generate` for the RPC
  change.
- Undo re-creates nothing server-side: undo is limited to the undo-toast window
  and restores client state only when the RPC has not yet been issued
  (clear is debounced behind the undo control for the `UndoWindow`, default 8s,
  per-device 5/8/15/30s, paused while the control is hovered or focused; if the
  page is hidden or closed during the window the RPC is sent at once with
  `keepalive`, or cancelled if the transport cannot honor it). Decision recorded here to avoid a
  later "undo after delete" expectation.
