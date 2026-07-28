# UX Research: Notifications for Headless Review/Triage Sessions

## 1. "View in Backlog" vs "View Session" branching — verified

`web-app/src/app/notifications/NotificationsPage.tsx:376-394`:

```tsx
{notification.metadata?.["item_id"] && (
  <Link
    href={`/backlog?item=${encodeURIComponent(notification.metadata["item_id"])}`}
    className={viewButton}
    onClick={() => handleNotificationClick(group.allIds, undefined, notification.sessionId)}
    data-testid="notification-view-backlog"
  >
    View in Backlog
  </Link>
)}
{!notification.metadata?.["item_id"] && notification.sessionId && (
  <Link
    href={`/?session=${encodeURIComponent(notification.sessionId)}`}
    className={viewButton}
    onClick={() => handleNotificationClick(group.allIds, notification.onView, notification.sessionId)}
  >
    View Session
  </Link>
)}
```

**Confirmed: requirements.md's claim holds.** The condition is a simple truthy check on
`notification.metadata["item_id"]`, checked first; the two `Link`s are mutually
exclusive (never both render, and if neither `item_id` nor `sessionId` is present,
no action link renders at all — not a dead link, just an absent one). No frontend
change is needed for AC2 — populating `item_id` in backend metadata is sufficient to
flip the branch. One minor gap: the "View Session" link lacks a `data-testid` (only
"View in Backlog" has `data-testid="notification-view-backlog"`) — if AC4's regression
test needs to assert the *absence* of a "View Session" link/action area for a
suppressed headless notification, it'll have to match on link text or `viewButton`
class rather than a testid; per `.claude/rules/e2e-test-conventions.md` (data-testid
or ARIA role only), consider adding a `data-testid="notification-view-session"` as a
drive-by fix if this file is touched anyway (not required by any AC, flag only).

## 2. Live-updating vs stale list on pruning (AC3)

`web-app/src/lib/hooks/useNotificationHistory.ts:99-105`: history is fetched **once on
mount** via `GetNotificationHistory` RPC. New notifications arrive incrementally
through the `watchSessions` stream and are appended to local state by
`NotificationContext.addNotification` — explicitly, per the code comment, "periodic
polling is not needed." There is no WebSocket/stream push for *removal* — no
"notification deleted" event type exists in the notification path. `refreshHistory()`
(full refetch) only runs on stream reconnect or after an `approval_response` event
(`NotificationContext.tsx`), not on a timer.

**Consequence for AC3 (pruning):** this is purely a backend GC concern the user does
not see live. If a session/instance backing an already-rendered notification is
pruned server-side while a tab is open, that notification will keep sitting in the
open tab's local state until: (a) full page reload, (b) a stream reconnect fires
`refreshHistory()`, or (c) the user manually acts on it (mark read/clear/load more,
which re-fetches). This is acceptable and matches the existing UX model — nothing in
requirements.md's acceptance criteria implies live removal, and no other notification
mutation (e.g. mark-as-read from another tab, handled via `BroadcastChannel` in
`syncChannel.subscribe`) pushes live either except within the same browser via that
channel. No frontend change is needed for AC3; it is a backend-only fix. Do not add a
polling loop or a new stream event type for this — out of proportion to the problem
(silent GC that surfaces next natural refresh is the expected pattern here).

## 3. Error/edge state when the linked backlog item is gone

Traced `/backlog?item=<id>` → `web-app/src/app/backlog/page.tsx:813-836` (renders
`BacklogItemDetail` keyed by `selectedItemId`) → `BacklogItemDetail.tsx:391-408`
(`load()` calls `getBacklogItem(itemId)`; if the result is falsy, `setError("Item not
found.")`) → render at `BacklogItemDetail.tsx:911-928`: a dedicated not-found state,
`<div className={styles.errorState} role="alert">{error ?? "Item not found."}</div>`,
inside the same detail-pane chrome (including the close button), rather than a blank
page or silent no-op.

**Compare to today's "View Session" dead-link behavior**, traced in
`web-app/src/app/page.tsx:164-197`: if `findSessionById(sessionId)` returns nothing,
the effect just does `console.warn(...)` and returns — **no user-visible error at
all**, the picker/main page is left exactly as it was, with zero indication anything
was clicked. This is the actual "dead session view" the backlog item's problem
statement complains about: not a crash, but a silent no-op.

**Verdict: AC2's fix is a strict improvement, not a lateral trade of one dead-link
class for another.** Today: click → nothing visible happens (console-only warning).
After AC2: click → detail pane opens with an explicit `role="alert"` "Item not found."
message. Both are "dead" in the sense that the destination is gone, but one informs
the user and one doesn't. No additional frontend work is required to make AC2 safe;
`BacklogItemDetail`'s not-found path was already built (used for direct navigation to
a deleted item's URL, not new for this feature).

## 4. Accessibility of the swapped link

The two `Link`s are separate elements gated by mutually exclusive conditions, not one
element whose `href`/label update independently — so there is no risk of a stale
accessible name pointing at a new destination (a common ARIA pitfall when only a
`href` changes but cached/memoized text doesn't). Each `Link`'s accessible name comes
from its own visible text content ("View in Backlog" vs "View Session"), no
`aria-label` override on either, so a screen reader announces exactly the text that's
visible — correct and destination-accurate by construction. No ARIA changes needed.

The only latent accessibility/testability gap (not a blocker, not required by any AC):
the "View Session" link has no `data-testid`, unlike its sibling. Flagged in section 1
only as a drive-by-fix candidate if this file is touched for AC4's test, per
`feedback_fix_collateral_debt` — not a required part of this feature.

## 5. Job-to-be-done: full suppression vs. quieter/collapsed notification

requirements.md's AC1 says these sessions should "no longer generate generic
TASK_COMPLETE / Session-idle / Stale notifications" (full suppression at the
generation source), and the Problem section states plainly these notifications "add
no value" because the underlying process has already exited by the time anyone reads
it, and the one click target is dead. This is a considered, not accidental, choice —
worth cross-checking against the one precedent in this codebase for a *quieter*
alternative: `NotificationsPage.tsx`'s "Auto-handled" collapsed section
(lines 412-449), which shows auto-approved/denied tool-use notifications in a
low-visibility, collapsed-by-default list rather than fully hiding them.

**That precedent does not apply here and full suppression is the right call**, for a
reason distinct from but reinforcing AC1's literal wording: the "Auto-handled" section
exists for notifications where an *automated decision was made on the user's behalf*
that they might want to audit later (did the classifier approve the right command?).
A headless review/triage session's TASK_COMPLETE/Idle/Stale lifecycle notification
carries no comparable decision to audit — it's pure infrastructure noise about a
scratch session the user never directly interacted with and whose outcome (the
review verdict) is *already* surfaced through the backlog item itself, not through
this notification. There is no unstated need for a "quieter" tier: the job the user
actually wants done when a headless review/triage session finishes is to see the
*result* show up on the backlog item (status change, review verdict, comment) — which
happens through a different mechanism entirely (the backlog UI, not the notification
feed) — not to receive any trace, however muted, in the notification feed for the
scratch session's mere existence. Full suppression (AC1) is correct as scoped; do not
introduce a collapsed/quiet tier as an alternative.

## Summary of frontend-touching implications

- **No frontend code changes required** for AC1, AC2, or AC3. All three are backend-
  only fixes given the current frontend architecture (metadata-driven conditional
  link, fetch-once-on-mount history, existing graceful not-found state on the backlog
  detail pane).
- AC4's regression test is backend-only in nature (assert no notification record is
  created/stored for a headless review/triage session reaching TASK_COMPLETE/
  Idle/Stale) — no new frontend test is required by the acceptance criteria as
  written, though a frontend test *could* assert the "View Session" link never
  renders for a suppressed-category notification if one somehow leaked through, using
  link text (or a newly added testid, see section 1) since none currently exists for
  that link.
- One optional, non-blocking drive-by-fix candidate: add
  `data-testid="notification-view-session"` to the "View Session" `Link` in
  `NotificationsPage.tsx:387-394` for testability symmetry with its "View in Backlog"
  sibling — flag only, not required by any AC.
