# Event Pipeline Consistency — Requirements

## Project Overview

Audit and fix gaps in the event-driven pipeline from server to all frontend surfaces.
The goal is to ensure that every state change on the server produces an event that reaches
all relevant frontend views (session list, review queue, notification page, session detail),
and that every action taken in any frontend surface is reflected consistently in all others.

---

## Background: Current Architecture

Events flow through a single `EventBus` in Go (`pkg/events/`). All events are assigned sequence
numbers and forwarded to frontend clients via the `WatchSessions` ConnectRPC stream
(`server/services/event_converter.go`). The frontend dispatches Redux actions from
`useSessionService.ts` (`handleSessionEvent`).

**Event types on the wire (proto):**
| Go event | Proto message | Frontend Redux action |
|---|---|---|
| `EventSessionCreated` | `SessionCreated` | `upsertSession` |
| `EventSessionUpdated` | `SessionUpdated` | `upsertSession` |
| `EventSessionDeleted` | `SessionDeleted` | `removeSession` + `removeReviewQueueItem` |
| `EventNotification` | `Notification` | `onNotification` callback |
| `EventApprovalResponse` | `ApprovalResponse` | `onApprovalResponse` callback |

**NOT forwarded to the WatchSessions wire:**
- `EventSessionAcknowledged` — handled internally by `ReactiveQueueManager` only
- `EventUserInteraction` — not converted in `event_converter.go`

---

## Identified Gaps

### G1: `EventSessionAcknowledged` is invisible to the frontend
When a user acknowledges a session (removes it from the review queue), the
`EventSessionAcknowledged` event only reaches `ReactiveQueueManager` (in-memory queue removal)
and `analytics/subscriber.go`. It is never forwarded through `WatchSessions`.

**Consequence:** If two browser tabs are open, or the session list is visible alongside the
review queue, the session card may still display an "attention required" indicator after the
acknowledgement. The session list is not notified and can't clear its badge.

### G2: Approval action does not guarantee timely session-list update
When an approval is granted or denied via `EventApprovalResponse`, the backend calls
`inst.Approve()` which changes session status, which eventually produces `EventSessionUpdated`.
However, there is a timing window between the approval action (front-end callback fires) and the
session-list update (next `EventSessionUpdated` arrives from the new execution state).

**Consequence:** The session card may briefly show "Needs Approval" after the user clicks
Approve in the review queue. Additionally, the notification page's approval toast has to be
manually refreshed — there is no explicit signal telling it "this approval ID was resolved."

### G3: Session deletion may leave stale review-queue items on the server
`ReactiveQueueManager` subscribes to `EventUserInteraction` and `EventSessionAcknowledged`
but NOT to `EventSessionDeleted`. If a session is deleted while it is in the review queue,
the in-memory queue entry may persist until the next reactive poll cycle.

**Consequence:** The server-side review queue momentarily contains a deleted session.
Clients streaming `WatchReviewQueue` receive a stale entry until the next queue
snapshot. The frontend handles deletion via `removeReviewQueueItem` on the Redux side,
but if a second browser tab is connected to the `WatchReviewQueue` stream, it may briefly
see the deleted session in the queue.

### G4: Notification acknowledgement is not an event; no cross-surface sync
Dismissing or marking a notification as read is handled entirely client-side via
`notificationStorage.ts` (localStorage). There is no server-side event that propagates
a "notification dismissed" state to other browser tabs or other clients.

**Consequence:** If a user dismisses a notification in one tab, other open tabs still show
the same notification as unread. The notification page and any badge counts derived from
it can diverge across tabs.

### G5: Review queue item state not updated when session detection changes
`reviewQueueSlice` items are populated via the `WatchReviewQueue` stream, which is separate
from `WatchSessions`. When `EventSessionUpdated` arrives with a new `DetectedStatus`, the
`sessionsSlice` is updated via `upsertSession`, but the review queue item's `SubStatus` and
`WorkingState` are only refreshed when the server pushes a new `WatchReviewQueue` snapshot.

**Consequence:** A session in the review queue may show a stale working state (e.g., still
shows "Needs Approval" chip) while the session card and session detail panel have already
updated to a newer `DetectedStatus`.

### G6: `EventUserInteraction` not forwarded to frontend
`EventUserInteraction` is subscribed by `ReactiveQueueManager` to snap the poll interval,
but is never forwarded through `WatchSessions` (not present in `event_converter.go`).
Frontend `useSessionService.ts` no longer has a `"statusChanged"` or `"userInteraction"` case
(it was removed in the previous refactor). So when the user types into a session terminal,
no event reaches other frontend surfaces about the interaction.

**Consequence:** Minor — the session list doesn't receive a live "user is active" signal
outside of periodic session snapshot updates.

---

## Requirements

### R1 — Forward `EventSessionAcknowledged` through the wire

- R1.1: Add `session_acknowledged = 6` to `SessionEvent` oneof in `events.proto`
- R1.2: Add `SessionAcknowledgedEvent` proto message with `session_id` field
- R1.3: Add `EventSessionAcknowledged` case to `event_converter.go` mapping
- R1.4: Frontend `handleSessionEvent` handles `sessionAcknowledged`:
  dispatches `upsertSession` with cleared attention-reason fields, or a lightweight
  "force-refresh" action that clears review-attention state on the session entity
- R1.5: Notification page receives the event and removes any pending approval toast/badge
  for the acknowledged session ID

### R2 — Acknowledge action immediately reflects on session list

- R2.1: When `ApprovalResponseEvent` arrives and `approved == true`, the session list
  immediately clears the "Needs Approval" sub-status on that session without waiting for
  the next `EventSessionUpdated` — done by dispatching an optimistic `upsertSession`
  update in the `approvalResponse` handler in `useSessionService.ts`
- R2.2: When `ApprovalResponseEvent` arrives, notify the notifications page to mark the
  corresponding approval notification as resolved (not just refresh history)
- R2.3: `AcknowledgeSession` RPC response should include the session's current proto snapshot
  so the frontend can immediately dispatch `upsertSession` with the latest server state

### R3 — Server-side review queue cleans up on session deletion

- R3.1: `ReactiveQueueManager` subscribes to `EventSessionDeleted`
- R3.2: On `EventSessionDeleted`, calls `rqm.queue.Remove(sessionID)` immediately
- R3.3: Signals the reactive loop to push an updated snapshot to all `WatchReviewQueue` clients
- R3.4: Result: clients on the `WatchReviewQueue` stream see the deletion within the same
  cycle as clients on the `WatchSessions` stream

### R4 — Notification dismissal is persisted server-side and synced

- R4.1: Add a `DismissNotification(notificationId)` RPC (or reuse `AcknowledgeSession`)
- R4.2: Backend stores dismissed notification IDs per-workspace (lightweight, ephemeral OK)
- R4.3: `WatchSessions` or a dedicated stream sends the dismissed state to all connected clients
- R4.4: Frontend `notificationStorage` is replaced or augmented by server-state read via RPC
  on page load, with real-time updates via stream events
- R4.5: Badge counts in the notifications nav icon are derived from the server-synced state

**Alternative (simpler):** If server-side persistence is out of scope, at minimum add a
`NotificationDismissedEvent` to the local EventBus so all tabs in the same browser session
receive the dismissal via a SharedWorker or BroadcastChannel.

### R5 — Review queue items update in sync with session detection changes

- R5.1: `WatchReviewQueue` stream sends delta updates (not just full snapshots) when a session's
  `SubStatus` changes — so the review queue item stays current with the session's live state
- R5.2: Alternatively, the frontend `reviewQueueSlice` derives `workingState` from the session
  entity already in `sessionsSlice` rather than from the review queue item's own copy
  (requires joining on session ID — avoids dual-source divergence)
- R5.3: Acceptance: a session that transitions from "Needs Approval" to "Processing" shows the
  correct state in the review queue panel within one event cycle

### R6 — Forward `EventUserInteraction` as a lightweight wire event

- R6.1: Add `user_interaction = N` to `SessionEvent` oneof in `events.proto`
- R6.2: Add `UserInteractionEvent` proto message with `session_id` and `interaction_type`
- R6.3: `event_converter.go` maps `EventUserInteraction` to the new proto
- R6.4: Frontend uses the event to update a "last active" timestamp on the session entity,
  enabling the session list to show recency without a full session update

**Priority:** R6 is nice-to-have; R1–R5 are required.

---

## Acceptance Criteria

- `EventSessionAcknowledged` arrives in the frontend when any client acknowledges a session
- Approving in the review queue immediately clears the "Needs Approval" badge in the session list
- Deleting a session while it is in the review queue removes it from the `WatchReviewQueue` stream within the same event cycle
- Dismissing a notification in one view clears the unread indicator in the notifications nav badge
- A session's working state in the review queue panel matches the session card working state at all times
- All of the above: `make build`, `make lint`, and `make test` pass with no regressions

---

## Non-Goals

- Redesigning the underlying event bus or streaming transport
- Adding new detection patterns or changing PTY parsing
- Implementing multi-device push sync (only same-browser-session tab sync is required for R4)
- Persisting the full notification history server-side (session IDs of dismissed items only)
