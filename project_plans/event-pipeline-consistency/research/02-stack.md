# Stack Research: Event Pipeline Consistency

## 1. Proto `SessionEvent` oneof — Current Field Numbers and Safe Additions

### Current state (events.proto, lines 10–34)

```protobuf
message SessionEvent {
  google.protobuf.Timestamp timestamp = 1;
  oneof event {
    SessionCreatedEvent    session_created    = 2;
    SessionUpdatedEvent    session_updated    = 3;
    SessionDeletedEvent    session_deleted    = 4;
    // field 5 (status_changed) removed in Epic 4
    UserInteractionEvent   user_interaction   = 6;
    SessionAcknowledgedEvent session_acknowledged = 7;
    ApprovalResponseEvent  approval_response  = 8;
    NotificationEvent      notification       = 9;
  }
  reserved 5;
  reserved "status_changed";
  uint64 seq = 10;
}
```

**Key finding:** Both `user_interaction` (field 6) and `session_acknowledged` (field 7) are **already defined** in the proto. The requirements document (R1.1, R6.1) references adding these — but they already exist at the expected field numbers. The gap is entirely in `event_converter.go`, which has no `case events.EventSessionAcknowledged` or `case events.EventUserInteraction` branch.

### What is actually missing

The proto schema is complete. No new field numbers are needed for R1 or R6. The messages are also defined:
- `SessionAcknowledgedEvent` (lines 554–565): has `session_id`, `acknowledged_at`, `reason` ✓
- `UserInteractionEvent` (lines 506–551): has `session_id`, `type`, `context` ✓

**Next safe field number** (if a future message were ever needed in `SessionEvent.oneof`): **field 11** (field 10 is taken by `seq` outside the oneof; oneof fields 2–4, 6–9 are taken; 5 is reserved).

### Reserved fields
- Field 5 is reserved (named `status_changed`). Do not reuse.
- Fields 2–4, 6–9 are actively used. Field 10 is `seq`.

### Implementation path for R1 and R6
Both require **only** changes to `event_converter.go`. Add two new `case` branches to the `switch event.Type` block:

```go
case events.EventSessionAcknowledged:
    protoEvent.Event = &sessionv1.SessionEvent_SessionAcknowledged{
        SessionAcknowledged: &sessionv1.SessionAcknowledgedEvent{
            SessionId:        event.SessionID,
            AcknowledgedAt:   timestamppb.New(event.Timestamp),
            Reason:           event.Context,
        },
    }

case events.EventUserInteraction:
    protoEvent.Event = &sessionv1.SessionEvent_UserInteraction{
        UserInteraction: &sessionv1.UserInteractionEvent{
            SessionId: event.SessionID,
            Context:   event.Context,
            // InteractionType: requires mapping event.InteractionType string
            //   to sessionv1.UserInteractionEvent_InteractionType enum
        },
    }
```

The generated Go oneof wrapper struct names follow the pattern `SessionEvent_<CamelCasedFieldName>`. The generated code already exists in `gen/proto/go/session/v1/events.pb.go` because the proto is already defined.

---

## 2. BroadcastChannel API — Cross-Tab Notification Sync (R4 Alternative)

### API Overview

`BroadcastChannel` is a browser-native API that allows communication between same-origin browsing contexts (tabs, windows, iframes) **in the same browser**.

```ts
// Publisher (one tab)
const bc = new BroadcastChannel("notification-dismissed");
bc.postMessage({ notificationId: "abc-123", sessionId: "sess-456" });

// Subscriber (other tabs)
const bc = new BroadcastChannel("notification-dismissed");
bc.onmessage = (event) => {
  console.log(event.data); // { notificationId: "abc-123", sessionId: "sess-456" }
};
bc.close(); // cleanup in useEffect return
```

### Availability

- **MDN baseline**: Available since 2022. Supported in Chrome 54+ (2016), Firefox 38+ (2015), Safari 15.4+ (2022), Edge 79+.
- **Not available**: Node.js (SSR), Web Workers (use `MessageChannel` instead), SharedWorker (different API).
- **Next.js 15 with App Router**: Components that use `BroadcastChannel` must be Client Components (`"use client"`). `new BroadcastChannel(...)` throws during SSR — must be guarded with a `typeof window !== "undefined"` check or placed inside `useEffect`.
- **Electron / Tauri**: Supported (they embed Chromium).

### What it does NOT handle

- Tabs opened **after** the broadcast: a tab that opens 60 seconds after a notification dismissal will not receive the message. BroadcastChannel is fire-and-forget with no replay.
- Cross-device sync: only works within the same browser profile on the same machine.
- Safari prior to 15.4 (2022): not supported.

### Usage Pattern for R4 (Same-Browser Tab Sync)

```ts
// useCrossTabNotificationSync.ts
import { useEffect, useRef } from "react";

const CHANNEL_NAME = "stapler-squad-notification-sync";

export function useCrossTabNotificationSync(
  onDismiss: (notificationId: string) => void
) {
  const bcRef = useRef<BroadcastChannel | null>(null);

  useEffect(() => {
    if (typeof window === "undefined") return;
    bcRef.current = new BroadcastChannel(CHANNEL_NAME);
    bcRef.current.onmessage = (event) => {
      if (event.data?.type === "dismiss") {
        onDismiss(event.data.notificationId);
      }
    };
    return () => bcRef.current?.close();
  }, [onDismiss]);

  const broadcastDismiss = (notificationId: string) => {
    bcRef.current?.postMessage({ type: "dismiss", notificationId });
  };

  return { broadcastDismiss };
}
```

### Recommendation for R4

BroadcastChannel is the correct minimal implementation for the "simpler alternative" in R4: same-browser tab sync without server-side persistence. It requires no backend changes and integrates directly with the existing `notificationStorage.ts` localStorage pattern. The limitation (tabs opened after broadcast won't receive replay) is acceptable because those tabs will re-read localStorage state on mount, which already tracks dismissed notifications.

Implementation approach:
1. When user dismisses notification: call `markAcknowledged(sessionId)` (localStorage) **and** `broadcastDismiss(notificationId)` (BroadcastChannel).
2. On receiving `onmessage`: call `markAcknowledged(receivedSessionId)` and re-derive badge count from the updated localStorage state.

---

## 3. ConnectRPC Delta Streaming vs. Full Snapshot Pattern

### Current WatchReviewQueue behavior

`ReactiveQueueManager` already sends **delta events**, not full snapshots, during its normal operation:
- `ReviewQueueItemAddedEvent` — when item enters queue
- `ReviewQueueItemRemovedEvent` — when item leaves queue
- `ReviewQueueItemUpdatedEvent` — when item properties change
- `ReviewQueueStatisticsEvent` — aggregate stats

The only full-snapshot path is `sendInitialSnapshot()`, which sends one `ItemAddedEvent` per item with `is_snapshot: true` when a new client connects.

### The actual gap (G5)

The `WatchReviewQueue` stream sends `ItemUpdatedEvent` (`ReviewQueueItemUpdatedEvent`) when a queue item's `priority`, `context`, or `reason` changes. However, the `ReactiveQueueManager` only calls `OnItemUpdated` indirectly through the queue observer — and the `DetectedStatus` / `WorkingState` that the frontend uses to determine if a session is "Needs Approval" vs "Processing" comes from the `session.detectedStatus` proto field, which is updated via `WatchSessions` (`SessionUpdatedEvent`), not `WatchReviewQueue`.

### Idiomatic ConnectRPC server streaming pattern for delta updates

ConnectRPC server streaming is semantically identical to gRPC server streaming. The idiomatic pattern for delta updates is the one already used: send discriminated union events (oneof) representing atomic changes. The `ReviewQueueEvent` oneof is already designed correctly for this.

For R5, the two approaches are:

**Option A — Frontend derives `workingState` from `sessionsSlice`** (recommended for R5.2):
- `reviewQueueSlice` items keep their `sessionId`.
- A selector joins `reviewQueueSlice.items` with `sessionsSlice.entities` on `sessionId`, deriving `workingState` from the session's live `detectedStatus` rather than the queue item's own stale copy.
- No backend change needed. No new proto messages.
- Con: introduces a cross-slice join in the selector.

```ts
export const selectWaitingItemsWithLiveState = createSelector(
  selectReviewQueueItems,
  selectAllSessionEntities,        // from sessionsSlice
  (items, sessionMap) =>
    items.map(item => ({
      ...item,
      _liveDetectedStatus: sessionMap[item.sessionId]?.detectedStatus,
    }))
);
```

**Option B — Backend sends `ItemUpdatedEvent` when `DetectedStatus` changes**:
- `ReactiveQueueManager.OnControllerStatusChange` already calls `poller.CheckSession(inst)`. When status changes, `handleEvent` could dispatch a `ReviewQueueItemUpdatedEvent`.
- Requires wiring detection events to `OnItemUpdated` callbacks.
- More correct but more complex.

Option A (frontend join) is the lower-risk path and eliminates the dual-source divergence structurally.

---

## 4. RTK Optimistic Update + Rollback Pattern (R2.1)

### RTK version in use: `^2.11.2`

RTK 2.x ships `createAsyncThunk` with `{optimistic}` patterns and RTK Query with `onQueryStarted`. The project does **not** use RTK Query — it uses manual dispatch with `useSessionService.ts`. This narrows the pattern.

### Manual optimistic update pattern (no RTK Query)

Since `handleSessionEvent` in `useSessionService.ts` already dispatches Redux actions directly, the optimistic update for R2.1 is straightforward — dispatch an optimistic `upsertSession` in the `approvalResponse` case before the server `SessionUpdatedEvent` arrives:

```ts
case "approvalResponse": {
  const approvalId = event.event.value.context ?? "";
  const sessionId = event.event.value.sessionId ?? "";

  // R2.1: Optimistic clear of "Needs Approval" sub-status.
  // The server will emit EventSessionUpdated shortly, which will overwrite this.
  // If approved is false (denied), don't clear — session may re-enter queue.
  if (event.event.value.approved && sessionId) {
    const existingSession = store.getState().sessions.entities[sessionId];
    if (existingSession) {
      dispatch(upsertSession({
        ...existingSession,
        detectedStatus: DetectedStatus.UNSPECIFIED,
        detectedContext: "",
      }));
    }
  }

  if (approvalId) {
    onApprovalResponseRef.current?.(approvalId, sessionId);
  }
  break;
}
```

**Rollback mechanism**: No explicit rollback is needed. When the server's `EventSessionUpdated` arrives (which happens immediately after `inst.Approve()` in `handleApprovalResponse`), it dispatches `upsertSession` with authoritative state, overwriting the optimistic update. If approval is **denied**, the server will emit an updated session status without clearing the attention state, and the optimistic patch (which only fires on `approved == true`) is not applied.

### Why no formal "undo thunk" is needed

The stream-driven architecture acts as a self-correcting rollback: any divergence between the optimistic update and server truth is corrected by the next `SessionUpdatedEvent`. The window is typically <500ms (time for `inst.Approve()` to complete and the backend to publish `EventSessionUpdated`).

### Access to Redux store from the hook

`useSessionService.ts` already has access to `dispatch` via `useAppDispatch()` and to session entities via `useAppSelector(selectAllSessions)`. To read a single session by ID for the optimistic patch, add `useAppSelector` usage or accept that the optimistic update reconstructs a partial session object from what `event.event.value` provides (session ID only). 

The cleaner approach: instead of patching the session entity (which requires knowing all its fields), dispatch `removeDetectedStatus(sessionId)` — a new action on `sessionsSlice` that specifically clears `detectedStatusMap[sessionId]`:

```ts
// sessionsSlice.ts — new action
removeDetectedStatus(state, action: PayloadAction<string>) {
  delete state.detectedStatusMap[action.payload];
}
```

This avoids needing to read the full session entity and is idiomatic for targeting only the relevant state.

---

## 5. Proto Field Numbers — Complete Inventory

### `SessionEvent.oneof event` (events.proto, lines 15–24)

| Field number | Name | Status |
|---|---|---|
| 1 | `timestamp` | Used (outside oneof) |
| 2 | `session_created` | Used (SessionCreatedEvent) |
| 3 | `session_updated` | Used (SessionUpdatedEvent) |
| 4 | `session_deleted` | Used (SessionDeletedEvent) |
| 5 | `status_changed` | **Reserved** (removed Epic 4) |
| 6 | `user_interaction` | Used (UserInteractionEvent) |
| 7 | `session_acknowledged` | Used (SessionAcknowledgedEvent) |
| 8 | `approval_response` | Used (ApprovalResponseEvent) |
| 9 | `notification` | Used (NotificationEvent) |
| 10 | `seq` | Used (outside oneof, uint64) |
| 11+ | — | **Safe to use** |

### `ReviewQueueEvent.oneof event` (events.proto, lines 592–599)

| Field number | Name | Status |
|---|---|---|
| 1 | `timestamp` | Used (outside oneof) |
| 2 | `item_added` | Used (ReviewQueueItemAddedEvent) |
| 3 | `item_removed` | Used (ReviewQueueItemRemovedEvent) |
| 4 | `item_updated` | Used (ReviewQueueItemUpdatedEvent) |
| 5 | `statistics` | Used (ReviewQueueStatisticsEvent) |
| 6+ | — | **Safe to use** |

### Summary

No proto schema changes are required for R1 or R6. All required messages and field numbers already exist. The only changes needed are in `event_converter.go` (backend) and `handleSessionEvent` (frontend).

---

## Key Findings Summary

1. **Proto is already complete** — `session_acknowledged` (field 7) and `user_interaction` (field 6) are defined in `SessionEvent` with full message bodies. The gap is exclusively in `event_converter.go` missing two `case` branches. No `make generate-proto` needed for R1/R6.

2. **BroadcastChannel is the right R4 alternative** — available in all modern browsers (Safari 15.4+, Chrome 54+), fire-and-forget within same-browser tabs, SSR-safe with `useEffect` guard. Works with existing `notificationStorage.ts` localStorage without backend changes. Does not replay to late-joining tabs, but those tabs read localStorage on mount.

3. **RTK optimistic update for R2.1 should target `detectedStatusMap` only** — rather than cloning a full session entity, dispatch a new `removeDetectedStatus(sessionId)` action on `sessionsSlice` from the `approvalResponse` case in `handleSessionEvent`. The server's `EventSessionUpdated` (which follows within ~500ms) acts as the natural rollback/confirmation.
