# ADR-007: Extract and Evolve the In-Repo Toast Stack; No Toast or Gesture Library

**Status**: Proposed
**Date**: 2026-10-07
**Project**: notification-tray-and-hidden-session-gate

## Context

Toasts are rendered inline by `NotificationProvider`
(`web-app/src/lib/contexts/NotificationContext.tsx`, ~L483-501), 532 lines,
5 commits in 90 days. `clearAll()` is `setNotifications([])` (L263), which would
clear approvals if wired to a button. `showUndoToast`/`showActionToast` use bare
`setTimeout` (L296, L322), so pause/cancel is impossible. The client
`isActionable` (`web-app/src/lib/notification-policy.ts:29`) covers
approval_needed and question only, while the server
`IsActionableType` (`server/notifications/store.go:55`) also covers
confirmation, error, failure and warning.

## Decision

1. Refactor first: extract `ToastStack` and a per-toast timer registry
   (`Map<id, timeoutId>` with pause/resume) out of `NotificationContext.tsx`
   with characterization tests before behavior changes.
2. Policy lives in pure functions in `notification-policy.ts`: `isPinned`, cap
   partition (`partitionToasts`). **The client keeps no notification-type list and
   no parity fixture** (Phase 4 consistency fix; the earlier text defined a client
   `isPinned` type list and a parity test, which contradicted plan.md and
   ADR-008): `isPinned(toast)` reads the server-sent `is_pending_decision` field
   that ADR-008 stamps on both the live `NotificationEvent` and the history
   record, and is presentational only (never used to choose rows to delete). The
   pinned set is therefore the server's `IsPendingDecision` by construction
   (`IsActionableType` at `server/notifications/store.go:55-62`: approval_needed,
   input_required, confirmation_needed, error, failure, warning, minus a WARNING
   whose producer stamped `auto_remediating=true`, ADR-008). A grep test fails if an
   `ACTIONABLE`-style type set appears in `web-app/src/lib`. `task_failed` is an
   `ssq-notify` hook name, not a proto `NotificationType`, and is not used.
3. Cap 3 visible; pinned toasts always render and sort first; overflow
   (including pinned beyond 3) collapses into a "+N more" chip that opens the
   tray, and the tray handle shows a pinned count. Routine types demote
   straight to the tray (existing `HISTORY_ONLY_TYPES`). Placement and the
   per-viewport cap (mobile top dock, desktop bottom-right) are in ADR-009.
   The deck header holds one control, "Move all to tray (N)", for every deck
   content; it only demotes (never dismisses or marks read). The earlier
   "Dismiss all" is withdrawn (ADR-008 decision 1).
4. `clearAll` is kept as a pinned-safe alias (it filters out pinned toasts) so
   the flag-off fallback path stops being able to clear approvals; there is no
   bulk dismiss on the deck, only per-toast `dismissToast(id)`. Live regions are
   owned by one `Announcer` (plan Story 3.6), not by the stack.
5. Swipe-to-dismiss is a custom hook (`useSwipeToDismiss`) modeled on
   `lib/window/useWindowSwipe.ts` constants, row-scoped, `touch-action: pan-y`,
   `stopPropagation`; every swipe action also has a 44px button.
6. Zero new dependencies. `@use-gesture/react` is the documented fallback if
   the hook proves flaky on a real device.

## Alternatives Considered

- sonner / @radix-ui/react-toast: rejected; owns its own queue, duplicates
  `NotificationContext` state, no pinned or "never clear a decision" concept.

## Consequences

- Cross-tab bulk dismissal needs a new idempotent id-set message (ADR-008).
- jscpd threshold is 0.14 (`web-app/.jscpd.json`); a single row component and
  single predicate set are required to stay under it.
