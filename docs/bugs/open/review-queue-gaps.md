# Review Queue Architecture Gaps

Identified during architecture review on 2026-03-30.

---

## BUG-001 (HIGH): Duplicate Notifications for APPROVAL_PENDING Items

### Status: Fixed in the same session (see commit)

### Root Causes

Two independent code paths both publish `NOTIFICATION_TYPE_APPROVAL_NEEDED` events to
the EventBus for the same underlying approval request:

1. **`ApprovalHandler.broadcastApprovalNotification()`** (`server/services/approval_handler.go:292`)
   Published when the HTTP hook fires. Carries the real approval UUID as the notification ID,
   tool name, command preview, and cwd.

2. **`ReactiveQueueManager.OnItemAdded()`** (`server/review_queue_manager.go:241`)
   Published when the review queue item is added. Uses a timestamp-based ID
   (`"review-queue-<sessionID>-<ms>"`), a different session ID value (canonical title from
   queue vs. raw `X-CS-Session-ID` header), and a different message format ("Approval
   Pending: …" vs. "Permission Required: Bash").

Because `APPROVAL_NEEDED` is excluded from server-side deduplication in
`store.go:125`, both records are persisted, resulting in two notification cards for
one approval event.

### Fix

Skip EventBus publication in `OnItemAdded` when `item.Reason == ReasonApprovalPending`.
The `ApprovalHandler` is already the authoritative source for approval notifications.

---

## BUG-002 (MEDIUM): Session ID Inconsistency in Approval Notifications

### Status: Fixed in the same session (see commit)

### Root Cause

`ApprovalHandler.HandlePermissionRequest()` uses the `X-CS-Session-ID` header value
verbatim as the session ID for notifications. The hook config injected by stapler-squad
(`InjectHookConfig`, `approval_handler.go:437`) correctly uses `sessionTitle` (canonical
name). However, if:

- A session was created externally and the user configured the hook manually with the
  tmux session name (e.g., `staplersquad_eheckc-for-compression` instead of
  `eheckc-for-compression`), or
- A legacy hook config using the tmux-prefixed name is still present

…then approval notifications end up under a different session ID than all other
notification types (which use the canonical title from the review queue).

This causes the same session to appear twice in the notification panel under
different names.

### Fix

Add `normalizeSessionID()` to `ApprovalHandler` that:
1. Tries an exact title match in storage.
2. Falls back to stripping a `<prefix>_` from the header value and matching
   the remainder against instance titles.
3. Returns the canonical title on match, original value otherwise.

---

## BUG-003 (MEDIUM): APPROVAL_NEEDED Notifications Not Deduplicated Server-Side

### Status: Fixed in the same session (see commit)

### Root Cause

`store.go:125` explicitly excludes `NOTIFICATION_TYPE_APPROVAL_NEEDED` from the
`(sessionID, notificationType)` deduplication logic. The original reason:

> Each approval has a unique `approval_id` that must equal the notification record ID
> so that `SetMetadata` can stamp outcomes after resolution.

This means each approval request (even sequential ones for the same session) creates
a new persisted record. After 3 bash commands need approval for the same session, the
panel shows an "x3" group count — but each is tracked as a separate record.

### Fix

Allow deduplication for `APPROVAL_NEEDED` by updating `existing.ID` to the incoming
`record.ID` when collapsing into an existing unread record. This preserves the
`SetMetadata` correlation requirement (it can now stamp the latest approval UUID)
while removing the visual clutter of multiple records per session.

---

## GAP-001 (LOW): Approval Timeout UX Degrades Silently

### Status: Open

### Description

When the 4-minute server-side approval timeout fires (`approval_handler.go:262`), the
backend returns an empty HTTP 200 with no body. Claude Code falls back to its native
terminal permission dialog. The web UI shows an "Expired" badge on the review queue
item, but there is no proactive notification to the user that the approval timed out
and moved to the terminal.

Users watching the notification panel may not notice the badge change, especially if
the session detail modal is not open.

### Suggested Fix

Publish a separate `NOTIFICATION_TYPE_APPROVAL_NEEDED` event (or a new INFO event)
when timeout occurs, stamped with `"approval_decision": "timeout"` in metadata, so
the notification card shows a persistent "Timed out — check terminal" message.

---

## GAP-002 (LOW): No Risk-Weighted Sorting Within APPROVAL_PENDING Tier

### Status: Open / Won't Fix (MVP)

### Description

All `ReasonApprovalPending` items share `PriorityHigh` (value 2). Within the HIGH tier,
items are sorted by `LastActivity` (most recent first). A destructive command like
`sudo rm -rf /tmp/work` and a benign command like `ls` receive identical queue position.

Users must visually scan the command preview `<pre>` block to assess risk.

### Suggested Fix

After the rule-based classifier escalates an approval, include the classifier's
`RiskLevel` in the `ReviewItem.Metadata`. The queue sorter can use this as a
tiebreaker within the same priority tier.

---

## GAP-003 (LOW): WebSocket Reconnect Falls Back to 30s Poll

### Status: Open / Low Priority

### Description

If the `WatchReviewQueue` WebSocket stream fails, the frontend falls back to 30-second
polling (`useReviewQueue.ts`). There is no exponential backoff reconnect logic. During
the reconnect window, a new approval event would be delayed up to 30 seconds.

### Suggested Fix

Add exponential backoff reconnect in `useReviewQueue.ts` with a cap of ~5 seconds.
This is low priority because the 30s fallback provides eventual consistency and approval
events are also surfaced via the `useApprovals` polling hook.

---

## GAP-004 (VERY LOW): Multi-Approval Session Shows Only First Approval

### Status: Open / Edge Case

### Description

`review_queue_service.go:121` only grabs the first pending approval for a session when
enriching the queue item with `pending_approval_id`. If a session has two concurrent
approvals (two parallel Claude instances requesting permission simultaneously), only
the first is surfaced in the queue UI. The second is accessible only via the
`ApprovalPanel` in the session detail modal.

### Suggested Fix

If multiple approvals exist, surface the oldest (most urgent) one. Consider showing
a count badge ("2 approvals") on the queue item.
