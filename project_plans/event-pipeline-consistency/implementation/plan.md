# Event Pipeline Consistency — Implementation Plan

## Summary

5 epics, 16 stories, 38 tasks. Epics are ordered by dependency; within each epic, stories may be
worked in parallel except where noted.

**Flagged decisions requiring ADRs:** 2 (see end of document).

---

## Epic 1: Server-Side Event Forwarding Gaps

**Goal:** Forward `EventSessionAcknowledged` and `EventUserInteraction` through the `WatchSessions`
wire, and subscribe `ReactiveQueueManager` to `EventSessionDeleted` for immediate queue cleanup.

**No frontend dependencies.** All three stories can land independently of any frontend work.

---

### Story 1.1 — Add `EventSessionAcknowledged` case to `event_converter.go`

**Gap fixed:** G1 — `EventSessionAcknowledged` is currently consumed only by
`ReactiveQueueManager` and `analytics/subscriber.go`. It never reaches the `WatchSessions` wire.

**Files to change:**
- `server/services/event_converter.go`

**Specific change:**

In the `switch event.Type` block (the function that maps internal events to proto `SessionEvent`
payloads), add a new `case`:

```go
case events.EventSessionAcknowledged:
    protoEvent.Event = &sessionv1.SessionEvent_SessionAcknowledged{
        SessionAcknowledged: &sessionv1.SessionAcknowledgedEvent{
            SessionId:      event.SessionID,
            AcknowledgedAt: timestamppb.New(event.Timestamp),
            Reason:         event.Context,
        },
    }
```

**Context:** The proto schema is already complete. `SessionAcknowledgedEvent` is field 7 in the
`SessionEvent` oneof (confirmed in `events.proto` lines 554–565). The generated Go wrapper struct is
`SessionEvent_SessionAcknowledged`. No `make generate-proto` is required.

**Acceptance test:**
1. Open two browser tabs, both on the session list.
2. In tab A, acknowledge a session from the review queue panel.
3. Verify that within one event cycle (< 2 seconds), tab B's session list clears any
   "Needs Approval" / attention indicator for that session — without a page reload.
4. Add a Go test in `server/services/event_converter_test.go`: call `convertEventToProto` with
   an `EventSessionAcknowledged` payload and assert the returned `SessionEvent` is non-nil and
   has a `SessionAcknowledged` variant with the correct `session_id`.

---

### Story 1.2 — Add `EventUserInteraction` case to `event_converter.go`

**Gap fixed:** G6 — `EventUserInteraction` is consumed by `ReactiveQueueManager` to snap the
poll interval, but is not forwarded through `WatchSessions`.

**Files to change:**
- `server/services/event_converter.go`

**Specific change:**

In the same `switch event.Type` block as Story 1.1, add:

```go
case events.EventUserInteraction:
    protoEvent.Event = &sessionv1.SessionEvent_UserInteraction{
        UserInteraction: &sessionv1.UserInteractionEvent{
            SessionId: event.SessionID,
            Context:   event.Context,
            // InteractionType: map event.InteractionType string to
            //   sessionv1.UserInteractionEvent_InteractionType enum value here.
            // Use INTERACTION_TYPE_UNSPECIFIED if mapping is unknown.
        },
    }
```

**Context:** `UserInteractionEvent` is field 6 in the `SessionEvent` oneof (confirmed in
`events.proto` lines 506–551). The generated wrapper struct is `SessionEvent_UserInteraction`.
No proto changes required.

**Note on InteractionType mapping:** The `events.EventUserInteraction` struct may carry an
`InteractionType` string. Check the field name in `pkg/events/events.go` and map it to the
`sessionv1.UserInteractionEvent_InteractionType` enum. If the mapping is incomplete, emit
`INTERACTION_TYPE_UNSPECIFIED` and file a follow-up to complete the enum mapping.

**Acceptance test:**
1. Add a Go test in `event_converter_test.go`: call `convertEventToProto` with an
   `EventUserInteraction` payload and assert the returned `SessionEvent` has a `UserInteraction`
   variant with the correct `session_id`.
2. Manual smoke test: type into an active session terminal; observe in a second tab that no new
   error events appear on the `WatchSessions` stream (the event is forwarded but has no frontend
   handler yet — this is acceptable for the "nice-to-have" R6 requirement).

---

### Story 1.3 — Subscribe `ReactiveQueueManager` to `EventSessionDeleted`

**Gap fixed:** G3 — `ReactiveQueueManager.handleEvent` does not handle `EventSessionDeleted`.
If a session is deleted while in the review queue, the in-memory queue entry persists until the
next poll cycle. The existing `removeFromAllPollers` in `DeleteSession` already calls
`rq.queue.Remove()` synchronously, but an async `CheckSession` goroutine (launched from
`handleApprovalResponse`) can race and re-add the session to the queue before `RemoveInstance`
completes.

**Files to change:**
- `server/review_queue_manager.go` — `handleEvent` switch

**Specific change:**

In the `switch` inside `handleEvent()` (which currently handles `EventUserInteraction`,
`EventSessionAcknowledged`, and `EventApprovalResponse`), add a new case:

```go
case events.EventSessionDeleted:
    sessionID := event.SessionID
    rqm.queue.Remove(sessionID)
    // Signal the reactive loop so WatchReviewQueue clients get an immediate
    // snapshot push if the item was present. If Remove() already fired
    // OnItemRemoved (item was present), the stream clients are already notified.
    // This call is a safety net for the async CheckSession race.
    rqm.signalActivity()  // snaps the poll interval so WatchReviewQueue clients get an immediate push
```

**Context:** `removeFromAllPollers` in `DeleteSession` calls `rq.queue.Remove(sessionID)` before
publishing `EventSessionDeleted`. The `ReactiveQueueManager`'s removal here is therefore normally
a no-op. The value is closing the race window: if an in-progress async `CheckSession` goroutine
re-adds the session after `RemoveInstance` fires, this handler removes it again. The `Remove`
call is idempotent (filter on empty set is a no-op).

**Acceptance test:**
1. Add a session to the review queue (the session must have a pending approval).
2. Delete the session while it is in the queue.
3. Verify the `WatchReviewQueue` stream clients (all connected tabs) see a
   `ReviewQueueItemRemovedEvent` within the same event cycle that `WatchSessions` delivers
   `EventSessionDeleted`. No stale entry visible in the review queue panel.
4. Add a Go unit test in `server/review_queue_manager_test.go`: publish
   `EventSessionDeleted` after adding a session to the queue; assert the queue is empty
   after `handleEvent` returns.

---

## Epic 2: Session Deletion Cleanup

**Depends on:** Epic 1 Story 1.3 (server-side deletion handling must be in place before frontend
can rely on consistent server events).

**Goal:** Remove stale approval data from the store on deletion, fix the double-remove badge count
bug, and disable approval toasts when the session is gone.

---

### Story 2.1 — Call `approvalStore.RemoveBySession(sessionID)` in `DeleteSession` RPC

**Gap fixed:** G3 / Pitfall 7 — `DeleteSession` does not cancel pending approvals in
`approvalStore`. After deletion, the notification panel still shows an active approval toast
with clickable Approve/Deny buttons; clicking them returns `CodeNotFound`.

**Files to change:**
- `server/services/session_service.go` — `DeleteSession` handler

**Specific change:**

1. Locate the `DeleteSession` handler in `server/services/session_service.go`.
2. `approvalStore` already has `CancelSession(sessionID string)` (in
   `server/services/approval_store.go` line ~202). It closes each `decisionCh` channel for
   entries keyed to `sessionID` and removes them from the store. No new method is needed.
3. Call `approvalStore.CancelSession(sessionID)` in `DeleteSession` before or immediately
   after publishing `EventSessionDeleted`. Before is preferred (deterministic cleanup before
   event propagation).

**Context:** The approval hook handler in `ApprovalHandler.ServeHTTP` is blocked in a `select`
waiting on `decisionCh` or `r.Context().Done()`. Closing `decisionCh` on deletion lets this
goroutine exit cleanly with a "session deleted" reason rather than waiting for the context
timeout.

**Acceptance test:**
1. Start a session that requires approval.
2. Open the notifications page — the approval toast is visible.
3. Delete the session without approving.
4. Verify the approval toast is removed from the notifications page (via the frontend story
   2.3 completing this end-to-end).
5. Go unit test: after calling `approvalStore.RemoveBySession(id)`, assert the store returns
   `CodeNotFound` for any approval ID previously associated with that session.

---

### Story 2.2 — Fix `reviewQueueSlice.removeItem` to derive `totalItems` from `items.length`

**Gap fixed:** Pitfall 5 + 6 — The `removeItem` reducer decrements `totalItems` as a separate
counter, so double-remove (from `WatchSessions` session-deleted event AND the concurrent
`WatchReviewQueue` item-removed event) silently corrupts the badge count.

**Files to change:**
- `web-app/src/lib/store/reviewQueueSlice.ts` — `removeItem` reducer

**Specific change:**

Replace the current decrement logic in `removeItem`:

```ts
// BEFORE (broken):
state.reviewQueue.items = (state.reviewQueue.items ?? []).filter(
  (item) => item.sessionId !== action.payload
);
const newTotal = Math.max(0, state.reviewQueue.totalItems - 1);

// AFTER (idempotent):
removeItem(state, action: PayloadAction<string>) {
  if (!state.reviewQueue) return;
  const itemsBefore = state.reviewQueue.items.length;
  state.reviewQueue.items = state.reviewQueue.items.filter(
    (item) => item.sessionId !== action.payload
  );
  const removed = itemsBefore - state.reviewQueue.items.length;
  state.reviewQueue.totalItems = Math.max(0, state.reviewQueue.totalItems - removed);
  if (state.stats) {
    state.stats.totalItems = Math.max(0, (state.stats.totalItems ?? 0) - removed);
  }
}
```

`removed` is 0 if the item was not present, so double-remove is harmless. `totalItems` stays
in sync because it is adjusted by actual array shrinkage, not by assumption.

**Alternative (simpler):** Derive `totalItems` purely from `items.length` on every reducer call
that touches `items`:

```ts
state.reviewQueue.totalItems = state.reviewQueue.items.length;
```

This is the preferred approach if `totalItems` is always intended to equal `items.length` (verify
in `ReviewQueue` proto: if `totalItems` is a separate server concept, keep the delta approach;
if it is always `items.length`, use the computed form).

**Acceptance test:**
1. Jest unit test in `reviewQueueSlice.test.ts`:
   - Dispatch `removeItem("sess-1")` twice on a state with one item having `sessionId: "sess-1"`.
   - Assert `totalItems === 0` after both dispatches (not negative).
2. Manual: delete a session that is in the review queue; verify the badge count in the nav is
   correct and does not show negative or stale numbers.

---

### Story 2.3 — Frontend: disable approval toasts for deleted sessions

**Gap fixed:** Pitfall 7 — after `EventSessionDeleted` arrives, the notification panel still
shows an active approval toast with Approve/Deny buttons.

**Files to change:**
- `web-app/src/lib/hooks/useSessionService.ts` — `handleSessionEvent` `sessionDeleted` case
- `web-app/src/lib/contexts/NotificationContext.tsx` or
  `web-app/src/lib/contexts/SessionServiceContext.tsx` — wherever `removeToastByApprovalId` /
  `removeToastBySessionId` is available

**Specific change:**

In `handleSessionEvent` in `useSessionService.ts`, find the `sessionDeleted` case (or
`"sessionDeleted"` string literal). Add a call to disable or remove any active approval toasts
associated with the deleted session ID:

```ts
case "sessionDeleted": {
  const sessionId = event.event.value.sessionId ?? "";
  dispatch(removeSession(sessionId));
  dispatch(removeReviewQueueItem(sessionId));
  // New: disable approval toasts for the deleted session
  onSessionDeletedRef.current?.(sessionId);  // new callback, or:
  removeToastBySessionIdRef.current?.(sessionId);
  break;
}
```

If `NotificationContext` does not expose a `removeToastBySessionId` function, add one:

```ts
// NotificationContext.tsx
const removeToastBySessionId = useCallback((sessionId: string) => {
  setApprovalToasts(prev =>
    prev.filter(toast => toast.sessionId !== sessionId)
  );
}, []);
```

The Approve/Deny buttons in the toast must be rendered as disabled (or the toast removed) when
the session is deleted. Removing the toast is the simpler UX.

**Acceptance test:**
1. Start a session that generates an approval toast in the notifications panel.
2. Delete the session from the session list.
3. Verify the approval toast disappears (or its Approve/Deny buttons become disabled) within
   one event cycle after deletion.

---

## Epic 3: Approval Action Cross-Surface Update

**Depends on:** Epic 1 Story 1.1 (the `EventSessionAcknowledged` wire event must be forwarded
before frontend handlers for it are meaningful).

**Goal:** When an approval fires or a session is acknowledged, immediately clear the "Needs
Approval" indicator from the session card without waiting for the next `EventSessionUpdated`.

---

### Story 3.1 — Add `removeDetectedStatus(sessionId)` action to `sessionsSlice`

**Gap fixed:** G2 — no scoped action exists to clear only the approval-indicator sub-status
without touching the session's primary `status`, `detectedStatus`, or `workingState`.

**Files to change:**
- `web-app/src/lib/store/sessionsSlice.ts`

**Specific change:**

Add a new reducer action to `sessionsSlice`:

```ts
removeDetectedStatus(state, action: PayloadAction<string>) {
  // Clear only the detectedStatusMap entry (approval/attention indicator).
  // Do NOT modify the entity itself (status, detectedStatus, subStatus fields are
  // authoritative from the server — the next EventSessionUpdated will write them).
  delete state.detectedStatusMap[action.payload];
}
```

**Critical constraint (Pitfall 2):** This action must NOT set `status`, `detectedStatus`, or
`workingState`. Those fields remain authoritative from the server. Only the "Needs Approval"
chip / sub-status indicator is cleared optimistically.

**Acceptance test:**
1. Jest unit test in `sessionsSlice.test.ts`:
   - Create state with a session having `subStatus: "needs_approval"` and `status: "running"`.
   - Dispatch `removeDetectedStatus(sessionId)`.
   - Assert `subStatus` is cleared AND `status` is still `"running"` (unchanged).

---

### Story 3.2 — Dispatch `removeDetectedStatus` in `approvalResponse` case

**Gap fixed:** G2 — when an approval response arrives with `approved === true`, the session
card continues to show "Needs Approval" until the next `EventSessionUpdated` (which may be
100–500ms later). This produces a visible flash.

**Files to change:**
- `web-app/src/lib/hooks/useSessionService.ts` — `handleSessionEvent` `approvalResponse` case

**Specific change:**

In the `approvalResponse` case of `handleSessionEvent`:

```ts
case "approvalResponse": {
  const approvalId = event.event.value.context ?? "";
  const sessionId = event.event.value.sessionId ?? "";

  // Optimistic clear: only fire when approved === true.
  // On denial, leave the sub-status in place — server will update.
  if (event.event.value.approved && sessionId) {
    dispatch(removeDetectedStatus(sessionId));
  }

  if (approvalId) {
    onApprovalResponseRef.current?.(approvalId, sessionId);
  }
  break;
}
```

**Why only on `approved === true`:** If denied, the session may re-enter the review queue or
display a different attention reason. The server's `EventSessionUpdated` (which follows within
~500ms regardless of outcome) is the authoritative update. Clearing optimistically only on
approval avoids showing a misleading "no issue" state when the session is actually denied.

**Acceptance test:**
1. Manual test: click "Approve" in the review queue panel; verify the "Needs Approval" chip
   vanishes from the session card immediately (same frame), not after the next server event.
2. Jest test in `useSessionService.test.ts` (or new test file): simulate a `approvalResponse`
   stream event with `approved: true`; assert `removeDetectedStatus` was dispatched with the
   correct `sessionId`.

---

### Story 3.3 — Handle `sessionAcknowledged` in `handleSessionEvent`

**Gap fixed:** G1 / R1.4 — after Story 1.1 forwards the event to the wire, the frontend
`handleSessionEvent` has no `sessionAcknowledged` case. The event is silently dropped.

**Files to change:**
- `web-app/src/lib/hooks/useSessionService.ts` — `handleSessionEvent` switch

**Specific change:**

Add a new case to `handleSessionEvent`:

```ts
case "sessionAcknowledged": {
  const sessionId = event.event.value.sessionId ?? "";
  if (sessionId) {
    // Clear the attention/approval indicator on the session card
    dispatch(removeDetectedStatus(sessionId));
    // Remove from review queue (idempotent — the WatchReviewQueue ItemRemoved event
    // may arrive before or after this, but removeItem is safe to call twice)
    dispatch(removeReviewQueueItem(sessionId));
  }
  break;
}
```

**Proto field name check:** The frontend proto wrapper for field 7 of `SessionEvent` oneof will
be either `sessionAcknowledged` or `session_acknowledged` depending on the generated TypeScript.
Verify the generated field name in `web-app/src/gen/session/v1/events_pb.ts` before writing
the `case` string.

**Context on double-remove:** `removeReviewQueueItem` dispatched here, combined with the
`ReviewQueueItemRemovedEvent` from `WatchReviewQueue`, creates a double-remove. Story 2.2 makes
this safe by deriving `totalItems` from `items.length` rather than decrementing a counter.
Story 2.2 must land before or alongside this story.

**Acceptance test:**
1. Two tabs open. Tab A acknowledges a session.
2. In tab B (which did not initiate the acknowledgement), verify:
   - The session card's "Needs Approval" chip is cleared.
   - The session is removed from the review queue panel.
   - Both happen within one event cycle, without a page reload.
3. Jest test: simulate a `sessionAcknowledged` stream event; assert both `removeDetectedStatus`
   and `removeReviewQueueItem` were dispatched.

---

## Epic 4: Review Queue Item State Consistency

**Depends on:** Epic 3 (the `removeDetectedStatus` action and `sessionsSlice` normalization
must exist before the joined selector can reference them meaningfully).

**Goal:** Ensure review queue items display the same working state as the session card at all
times, and that the `WatchReviewQueue` stream reconnects automatically on disconnect.

---

### Story 4.1 — Add memoized joined selector `selectReviewQueueItemWithLiveStatus`

**Gap fixed:** G5 — `reviewQueueSlice` items carry their own `workingState`/`subStatus` copy
populated from `WatchReviewQueue`, which diverges from the live `detectedStatusMap` in
`sessionsSlice` after a `SessionUpdatedEvent` arrives via `WatchSessions`.

**Files to change:**
- `web-app/src/lib/store/reviewQueueSlice.ts` (or a new
  `web-app/src/lib/store/reviewQueueSelectors.ts` file — prefer colocation with the slice)

**Specific change:**

Add a memoized `createSelector` that joins review queue items with live session state:

```ts
import { createSelector } from "@reduxjs/toolkit";
import { selectAllSessionEntities } from "./sessionsSlice";  // adjust import path

export const selectReviewQueueItemsWithLiveStatus = createSelector(
  [
    (state: RootState) => state.reviewQueue.reviewQueue?.items ?? [],
    selectAllSessionEntities,
  ],
  (items, sessionEntities) =>
    items.map(item => {
      const liveSession = sessionEntities[item.sessionId];
      return {
        ...item,
        // Override stale queue-item state with live session state.
        // Falls back to item's own value if session is not in sessionsSlice yet.
        workingState: liveSession?.detectedStatus ?? item.workingState,
        subStatus: liveSession?.subStatus ?? item.subStatus,
      };
    })
);
```

**Notes:**
- `selectAllSessionEntities` should return the normalized entity map keyed by session ID.
  Adjust to match the actual selector name in `sessionsSlice.ts`.
- The `workingState` field name on `ReviewQueueItem` must match the actual proto-generated
  TypeScript field name. Verify in the generated `ReviewQueueItem` type.
- `createSelector` memoizes on input reference equality. Since `sessionsSlice.entities` is
  updated on every `upsertSession` dispatch, the selector will recompute when either the
  review queue items or the live sessions change — which is the desired behavior.

**Acceptance test:**
1. Jest test in `reviewQueueSlice.test.ts` or `reviewQueueSelectors.test.ts`:
   - Set up state with a review queue item having `workingState: "needs_approval"`.
   - Set up `sessionsSlice.entities` for the same session with
     `detectedStatus: DetectedStatus.UNSPECIFIED` (post-approval).
   - Call `selectReviewQueueItemsWithLiveStatus(state)`.
   - Assert the returned item has the live `workingState` (not the stale queue-item value).

---

### Story 4.2 — Update `ReviewQueuePanel.tsx` to use the joined selector

**Gap fixed:** G5 — review queue panel currently reads `item.workingState` directly from
`reviewQueueSlice`, bypassing the live session state.

**Files to change:**
- `web-app/src/app/review-queue/ReviewQueuePanel.tsx` (or equivalent panel component)
- Any other component that reads review queue items and displays working state / sub-status

**Specific change:**

Replace direct `reviewQueueSlice` item selectors with `selectReviewQueueItemsWithLiveStatus`:

```ts
// BEFORE:
const items = useAppSelector(state => state.reviewQueue.reviewQueue?.items ?? []);

// AFTER:
const items = useAppSelector(selectReviewQueueItemsWithLiveStatus);
```

Audit all other selectors in the panel (badge counts, "Needs Approval" chip rendering,
`workingState` display) and ensure they read from the joined selector, not the raw
`reviewQueue.items`.

**Acceptance test:**
1. Open the review queue panel with a session that needs approval.
2. From a separate tab, approve the session via the omnibar or session detail view.
3. Observe that the review queue panel shows the updated state (e.g., "Processing" or no chip)
   within one `WatchSessions` event cycle — without waiting for the `WatchReviewQueue` stream
   to push a new snapshot.

---

### Story 4.3 — Add reconnect loop to `useReviewQueue` hook

**Gap fixed:** Architecture research finding — `WatchReviewQueue` has no reconnect logic.
On `AbortError` (stream disconnect), the stream silently stops. The 30-second fallback poll
provides eventual consistency, but any delta events missed during the disconnect window are lost.

**Files to change:**
- `web-app/src/lib/hooks/useReviewQueue.ts`

**Specific change:**

Wrap the `WatchReviewQueue` stream subscription in the same `while (true)` / exponential
backoff reconnect pattern used by `WatchSessions` in `useSessionService.ts`:

```ts
// Pseudo-code — adapt to match the actual WatchSessions reconnect pattern:
const reconnectWithBackoff = async () => {
  let delay = 1000; // 1 second initial
  while (!abortController.signal.aborted) {
    try {
      // Request initial snapshot on (re)connect
      await subscribeToWatchReviewQueue({ initialSnapshot: true, signal: abortController.signal });
      delay = 1000; // reset on clean disconnect
    } catch (err) {
      if (isAbortError(err)) break; // intentional shutdown
      await sleep(delay);
      delay = Math.min(delay * 2, 30_000); // cap at 30 seconds
    }
  }
};
```

`initialSnapshot: true` re-hydrates current state on reconnect. The in-place delta application
(Story 4.4) ensures that a brief reconnect gap is healed by the snapshot.

**Acceptance test:**
1. Open the review queue panel.
2. Simulate a network disconnect (DevTools → Offline → Online).
3. Verify the review queue reconnects and re-hydrates without a page reload.
4. Add a session to the queue during the offline window; verify it appears after reconnect
   (because the initial snapshot on reconnect includes it).

---

### Story 4.4 — Fix `useReviewQueue` to apply delta events in-place

**Gap fixed:** G5 / Architecture research — stream events from `WatchReviewQueue` currently
trigger a full `GetReviewQueue` REST re-fetch ("Phase 1 shortcut" in `useReviewQueue.ts`).
This adds a REST round-trip delay to every queue update and makes the delta stream purely
advisory.

**Files to change:**
- `web-app/src/lib/hooks/useReviewQueue.ts`
- `web-app/src/lib/store/reviewQueueSlice.ts` — add `addItem` and `updateItem` actions if
  they do not already exist

**Specific change:**

1. In `reviewQueueSlice.ts`, add `addItem` and `updateItem` reducers if missing:
   ```ts
   addItem(state, action: PayloadAction<ReviewQueueItemProto>) {
     if (!state.reviewQueue) return;
     const exists = state.reviewQueue.items.some(i => i.sessionId === action.payload.sessionId);
     if (!exists) {
       state.reviewQueue.items.push(action.payload);
       state.reviewQueue.totalItems = state.reviewQueue.items.length;
     }
   },
   updateItem(state, action: PayloadAction<ReviewQueueItemProto>) {
     if (!state.reviewQueue) return;
     const idx = state.reviewQueue.items.findIndex(i => i.sessionId === action.payload.sessionId);
     if (idx >= 0) {
       state.reviewQueue.items[idx] = { ...state.reviewQueue.items[idx], ...action.payload };
     }
   },
   ```

2. In `useReviewQueue.ts`, replace the `refreshRef.current()` call inside the stream event
   handler with direct Redux dispatches:

   ```ts
   // BEFORE (Phase 1 shortcut):
   case "itemAdded":   refreshRef.current(); break;
   case "itemRemoved": refreshRef.current(); break;
   case "itemUpdated": refreshRef.current(); break;

   // AFTER (in-place delta):
   case "itemAdded":
     if (!event.isSnapshot) {
       dispatch(addItem(event.item));
     }
     break;
   case "itemRemoved":
     dispatch(removeItem(event.sessionId));
     break;
   case "itemUpdated":
     dispatch(updateItem(event.item));
     break;
   ```

   Keep the `isSnapshot` guard: snapshot items (sent on initial connect) should NOT trigger
   the `onNotification` callback that fires new notification toasts.

3. Retain the 30-second polling fallback (`GetReviewQueue`) as a recovery mechanism, but
   its sole purpose is now resilience — it should call `setReviewQueue(data)` to replace
   state wholesale, not trigger per-item updates.

**Acceptance test:**
1. Jest test: mock the `WatchReviewQueue` stream to emit an `itemAdded` event; assert
   `addItem` was dispatched and `refreshRef.current` was NOT called.
2. Manual: with DevTools network throttled, add a session to the queue; verify the panel
   updates immediately from the WebSocket delta without an additional REST round-trip
   (observable in the Network tab: only one REST call on page load, not one per stream event).

---

## Epic 5: Notification Cross-Tab Sync

**Independent — no dependency on Epics 1–4.** Can be developed in parallel with other epics.

**Goal:** When a user dismisses a notification in one browser tab, all other tabs in the same
browser session clear the unread indicator and update badge counts.

**Implementation approach:** BroadcastChannel + localStorage (the "simpler alternative" from
requirements R4, confirmed as the right fit for a local-first single-user app by the features
and pitfalls research). The server-in-memory + stream approach (R4.1–R4.4) is the ADR-flagged
alternative and should be considered if cross-device or server-restart persistence is required
in future.

---

### Story 5.1 — Add typed `BroadcastChannel` wrapper utility

**Files to create/change:**
- `web-app/src/lib/utils/broadcastChannel.ts` (new file — only file that should be created
  in this project; justified because it is a shared utility with no existing equivalent)

**Specific change:**

```ts
// web-app/src/lib/utils/broadcastChannel.ts

export type NotificationSyncMessage =
  | { type: "NOTIFICATION_DISMISSED"; notificationId: string }
  | { type: "NOTIFICATION_ACKNOWLEDGED"; sessionId: string };

const CHANNEL_NAME = "stapler-squad:notification-sync";

export function createNotificationSyncChannel(): {
  broadcast: (message: NotificationSyncMessage) => void;
  subscribe: (handler: (message: NotificationSyncMessage) => void) => () => void;
} {
  // SSR guard: BroadcastChannel is not available in Node.js / during SSR
  if (typeof window === "undefined") {
    return {
      broadcast: () => {},
      subscribe: () => () => {},
    };
  }

  const channel = new BroadcastChannel(CHANNEL_NAME);

  return {
    broadcast: (message) => channel.postMessage(message),
    subscribe: (handler) => {
      const listener = (event: MessageEvent<NotificationSyncMessage>) => {
        handler(event.data);
      };
      channel.addEventListener("message", listener);
      return () => {
        channel.removeEventListener("message", listener);
        channel.close();
      };
    },
  };
}
```

**Notes:**
- `BroadcastChannel` is available in Chrome 54+, Firefox 38+, Safari 15.4+. The codebase
  targets modern browsers; no polyfill is required.
- The `"use client"` directive in Next.js App Router is not applicable here (this is a
  plain React + Vite/CRA app based on the project structure). The `typeof window` guard
  handles any SSR-like environment.
- `channel.close()` is called by the subscriber cleanup function. If multiple subscribers
  are needed in the future, extract `channel` to a singleton. For now, one subscriber per
  mount is sufficient.

**Acceptance test:**
1. Jest test using `jest-environment-jsdom`: mock `BroadcastChannel`; call `broadcast`;
   assert `postMessage` was called with the correct payload.
2. Jest test: call `subscribe` with a handler; simulate a `MessageEvent`; assert the handler
   fires with the correct typed payload.

---

### Story 5.2 — Broadcast `NOTIFICATION_DISMISSED` when `markAcknowledged` is called

**Files to change:**
- `web-app/src/lib/utils/notificationStorage.ts` — the function that marks a notification
  as acknowledged/dismissed (likely `markAcknowledged` or `dismissNotification`)
- OR: the call site in `NotificationContext.tsx` or `useNotifications.ts` — wherever
  `notificationStorage.markAcknowledged(id)` is called

**Specific change:**

At the point where a notification is dismissed locally (localStorage write), also broadcast
the event to other tabs:

```ts
// In notificationStorage.ts or the call-site context:
import { createNotificationSyncChannel } from "../utils/broadcastChannel";

const syncChannel = createNotificationSyncChannel();

export function dismissNotification(notificationId: string): void {
  // 1. Existing localStorage write
  markAcknowledgedInStorage(notificationId);

  // 2. Notify other tabs (fire-and-forget)
  syncChannel.broadcast({ type: "NOTIFICATION_DISMISSED", notificationId });
}
```

**Important:** The `broadcast` call must NOT fire when the local tab is receiving an event
from another tab (Stories 5.3). Only the initiating tab broadcasts. The subscriber handler
in 5.3 calls only `markAcknowledgedInStorage` (no re-broadcast).

**Acceptance test:**
1. Jest test: mock `BroadcastChannel.postMessage`; call `dismissNotification`; assert
   `postMessage` was called once with `{ type: "NOTIFICATION_DISMISSED", notificationId }`.

---

### Story 5.3 — All tabs listen on the channel and sync local state on receipt

**Files to change:**
- `web-app/src/lib/contexts/NotificationContext.tsx` or
  `web-app/src/lib/hooks/useNotifications.ts` — wherever the notification store is hydrated
  and badge counts are derived

**Specific change:**

Subscribe to the `BroadcastChannel` on mount and update local state on receipt:

```ts
// In NotificationContext.tsx (or useNotifications hook):
useEffect(() => {
  const syncChannel = createNotificationSyncChannel();
  const unsubscribe = syncChannel.subscribe((message) => {
    if (message.type === "NOTIFICATION_DISMISSED") {
      // 1. Persist locally (idempotent — may already be in localStorage if this tab
      //    initiated the dismiss, but in that case the broadcast did not fire for this tab)
      markAcknowledgedInStorage(message.notificationId);
      // 2. Update Redux / context state to trigger badge re-calculation
      dispatch(dismissNotification(message.notificationId));
      // OR: setLocalDismissed(prev => new Set([...prev, message.notificationId]));
    }
  });
  return unsubscribe;
}, [dispatch]);
```

**Note on same-tab behavior:** `BroadcastChannel.postMessage` does NOT deliver to the same
tab that sent the message (this is by spec). So the subscribing tab only receives messages
from OTHER tabs — no infinite loop risk.

**Acceptance test:**
1. Open two browser tabs (tab A and tab B) on the notifications page.
2. In tab A, dismiss a notification.
3. In tab B (without page reload), observe the notification's unread indicator clears and the
   nav badge count decrements.
4. Jest test using mocked `BroadcastChannel`: simulate receiving a `NOTIFICATION_DISMISSED`
   message; assert `dismissNotification` was dispatched and the badge count recalculated.

---

### Story 5.4 — Add `DismissNotification` RPC stub (server-side, optional phase)

**Context:** This story implements the server-in-memory alternative to BroadcastChannel.
Stories 5.1–5.3 provide the minimal same-browser solution. Story 5.4 is an optional follow-up
that allows cross-device dismissal and survived server restarts if later required.

**Files to change:**
- `proto/session/v1/session.proto` — add `DismissNotification` RPC
- `server/services/review_queue_service.go` or a new `notification_service.go` — handler
- `server/services/event_converter.go` — forward `EventNotificationDismissed` (new event type)
  OR reuse `EventSessionAcknowledged` with a `notificationId` field in `Context`

**Specific change:**

1. Add to `session.proto`:
   ```protobuf
   rpc DismissNotification(DismissNotificationRequest) returns (DismissNotificationResponse) {}

   message DismissNotificationRequest {
     string notification_id = 1;
   }
   message DismissNotificationResponse {
     bool success = 1;
   }
   ```

2. Implement the handler:
   ```go
   func (s *NotificationService) DismissNotification(
     ctx context.Context,
     req *connect.Request[sessionv1.DismissNotificationRequest],
   ) (*connect.Response[sessionv1.DismissNotificationResponse], error) {
     s.dismissedMu.Lock()
     s.dismissed[req.Msg.NotificationId] = true
     s.dismissedMu.Unlock()
     s.eventBus.Publish(events.NewNotificationDismissedEvent(req.Msg.NotificationId))
     return connect.NewResponse(&sessionv1.DismissNotificationResponse{Success: true}), nil
   }
   ```

3. Add `EventNotificationDismissed` case to `event_converter.go` (new event type — add to
   `SessionEvent` oneof at the next safe field number, which is 11).

4. Frontend: on receiving `notificationDismissed` in `handleSessionEvent`, call
   `markAcknowledgedInStorage(event.notificationId)` and dispatch `dismissNotification`.

5. On `WatchSessions` reconnect, call `GetDismissedNotifications` (a new list RPC) to
   re-hydrate dismissed state for fresh connections (resolves Pitfall 4's fresh-tab problem).

**Note:** If Story 5.4 is implemented, Stories 5.1–5.3 (BroadcastChannel) can be removed.
Do not ship both — they would double-dismiss.

**Acceptance test:**
1. `make generate-proto` completes without error.
2. Call `DismissNotification` RPC; verify the dismissed ID is returned by a subsequent
   `GetDismissedNotifications` call.
3. All tabs connected to `WatchSessions` receive the `notificationDismissed` event and
   update badge counts.

---

## Acceptance Criteria (Full Plan)

| Criterion | Stories |
|---|---|
| `make build`, `make lint`, `make test` pass | All |
| Acknowledging a session in one tab removes the review queue badge in all tabs | 1.1, 3.3 |
| Deleting a session in review queue removes it immediately, no stale entry | 1.3, 2.1, 2.2, 2.3 |
| Approval action immediately clears "Needs Approval" from session card (no flash) | 3.1, 3.2 |
| Review queue item working state matches session card working state at all times | 4.1, 4.2 |
| Notification dismissed in one tab clears badge in all same-browser tabs | 5.1, 5.2, 5.3 |

---

## Epic and Story Count

| Epic | Stories | Tasks (estimated) |
|---|---|---|
| 1: Server-side event forwarding gaps | 3 | 9 |
| 2: Session deletion cleanup | 3 | 8 |
| 3: Approval action cross-surface update | 3 | 8 |
| 4: Review queue item state consistency | 4 | 9 |
| 5: Notification cross-tab sync | 4 | 8 |
| **Total** | **17** | **~42** |

(Story 5.4 is optional; if excluded: 16 stories, ~34 tasks.)

---

## Flagged Decisions Requiring ADRs

### ADR Decision 1: Notification dismissal mechanism (BroadcastChannel vs. server in-memory)

**Decision required:** Stories 5.1–5.3 implement BroadcastChannel (same-browser tab sync,
no backend changes). Story 5.4 implements server-in-memory + stream (all tabs via existing
`WatchSessions` stream, survives within a server session).

**Trade-off:**
- BroadcastChannel: no backend change, fires instantly, but does not survive if a tab opens
  after the dismissal (late-joining tabs read localStorage on mount, which covers most cases).
- Server in-memory: consistent with the codebase's event-driven architecture, handles tabs
  that open after dismissal (via seq replay from ring buffer), but adds an RPC and a new
  event type. Does NOT survive server restarts (acceptable per requirements: "ephemeral OK").

**Recommendation:** Implement 5.1–5.3 now (simpler, unblocks the requirement). File ADR to
document the trade-off and the upgrade path to 5.4. Do not ship both simultaneously.

### ADR Decision 2: Review queue delta vs. full-snapshot update model

**Decision required:** Story 4.4 removes the "Phase 1 shortcut" (full `GetReviewQueue`
re-fetch on every stream event) in favor of in-place delta application. This is a behavioral
change that affects cache invalidation logic.

**Trade-off:**
- Delta (in-place): lower latency, no extra REST calls, matches the server's stream design.
  Risk: if a delta is missed during a brief disconnect, state diverges until the next snapshot
  (mitigated by Story 4.3's reconnect loop which requests `initialSnapshot: true`).
- Full re-fetch on event: always consistent (fetches authoritative state), but adds a REST
  round-trip for every queue change (100–300ms latency visible on every update, high churn
  during batch operations).

**Recommendation:** Ship delta (4.4). The reconnect + initial snapshot covers the missed-event
case. Document the decision in an ADR noting that the 30-second fallback poll remains as a
residual safety net.

---

## Dependency Graph

```
Epic 1 (no deps)
  ├── 1.1 → Epic 3 (3.1, 3.2, 3.3)
  │             └── Epic 4 (4.1, 4.2, 4.3, 4.4)
  ├── 1.2 (independent, nice-to-have)
  └── 1.3 → Epic 2 (2.1, 2.2, 2.3)

Epic 5 (independent, no deps on 1–4)
```

**Recommended merge order:**
1. Epic 1 stories (all can land in a single PR — small, focused on `event_converter.go` and
   `review_queue_manager.go`)
2. Epic 2 + Epic 3 in parallel (no conflict: 2.x touches Go backend + review queue Redux;
   3.x touches session Redux + `useSessionService.ts`)
3. Epic 4 after Epics 2 and 3 (selector joins both slices, in-place delta is safer once
   the idempotent remove is in place)
4. Epic 5 in any order (no shared files with Epics 1–4 except notification context)
