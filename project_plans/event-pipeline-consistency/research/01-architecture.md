# Architecture Map: Event Pipeline Consistency

## 1. ReactiveQueueManager Subscriptions

**File:** `server/review_queue_manager.go`

`ReactiveQueueManager` subscribes to the global `EventBus` via a single channel and handles exactly three event types in `handleEvent()`:

| Event | Handler | Action |
|---|---|---|
| `EventUserInteraction` | `handleUserInteraction` | Calls `poller.CheckSession(inst)`, signals activityCh to snap poll interval |
| `EventSessionAcknowledged` | `handleSessionAcknowledged` | Calls `rqm.queue.Remove(sessionID)` immediately |
| `EventApprovalResponse` | `handleApprovalResponse` | Calls `inst.Approve()` if approved, then `rqm.queue.Remove(sessionID)` |

**Critical gap (G3):** `EventSessionDeleted` is NOT handled. If a session is deleted while in the review queue, the in-memory queue entry persists until the next poll cycle. The `queue.Remove()` is never called on deletion.

The manager also implements `ReviewQueueObserver` interface:
- `OnItemAdded`: publishes `ReviewQueueEvent_ItemAdded` to all stream clients. Also publishes `EventNotification` to EventBus (skipping APPROVAL_PENDING to avoid duplicates from ApprovalHandler).
- `OnItemRemoved`: publishes `ReviewQueueEvent_ItemRemoved` to all stream clients.
- `OnQueueUpdated`: publishes statistics event to stream clients.

## 2. WatchReviewQueue Stream

**File:** `server/services/review_queue_service.go` lines 211–244

The `WatchReviewQueue` RPC handler:
1. Registers a new `reviewQueueStreamClient` with `ReactiveQueueManager.AddStreamClient()`
2. If `InitialSnapshot: true` in the request, immediately sends all current queue items as `ItemAdded` events with `IsSnapshot: true` flag (frontend should NOT fire notifications for these)
3. Then streams subsequent real-time delta events: `ItemAdded`, `ItemRemoved`, `ItemUpdated`, `Statistics`
4. This is **delta-based, not snapshot-only** — individual item add/remove events are streamed as they occur

**No sequence-number replay mechanism**: Unlike `WatchSessions` which has `afterSeq` for targeted replay, `WatchReviewQueue` has no reconnect-replay. On reconnect, the client requests a fresh `initialSnapshot: true` to re-hydrate. There is no catch-up for events missed during a disconnect window.

**Stream push mechanism**: `publishToClients()` uses non-blocking channel sends. If a client's buffered channel (capacity 100) is full, events are silently dropped (`default:` case). This is a potential loss-of-event path under high-frequency queue churn.

## 3. Frontend Review Queue Data Source

**File:** `web-app/src/lib/hooks/useReviewQueue.ts`

The frontend uses a **hybrid approach**:
- **Primary**: `WatchReviewQueue` WebSocket stream (`useWebSocketPush: true` by default)
- **Fallback**: `GetReviewQueue` REST RPC poll every 30 seconds

**Critical Phase 1 shortcut (G5 root cause):** When any stream event arrives (`itemAdded`, `itemRemoved`, `itemUpdated`), the handler **does not apply the delta in-place**. Instead it calls `refreshRef.current()` which triggers a full `GetReviewQueue` REST fetch. Comment in code: "For Phase 1 of the RTK migration, incremental WebSocket events trigger a full re-fetch rather than in-place mutation."

This means:
- Queue updates arrive fast via WebSocket stream
- But the state applied is from a subsequent REST call, creating a brief delay
- `reviewQueueSlice` is a flat `ReviewQueue | null` with items array — no normalized entity adapter

**Optimistic acknowledge:** `acknowledgeSession` in `useReviewQueue` dispatches `removeItem(sessionId)` from `reviewQueueSlice` immediately before the RPC call, with rollback to a full refresh on error.

## 4. Notification Page Data Source

**File:** `web-app/src/app/notifications/NotificationsPage.tsx`

The notifications page uses `useNotifications()` from `NotificationContext`. The notification history comes from `useNotificationHistory()` which:
- Fetches from the `GetNotificationHistory` RPC on mount (once)
- Does **not** subscribe to any real-time stream
- History is refreshed by calling `refreshHistory()` explicitly

When stream events arrive:
- `onNotification` callback in `SessionServiceContext` routes new events to `handleNotification` (from `useSessionNotifications`)
- `onApprovalResponse` callback calls `refreshHistory()` immediately after removing the approval toast
- `onReconnect` also triggers `refreshHistory()`

So the notification page is **not stream-driven for real-time updates** — it relies on explicit `refreshHistory()` calls triggered by stream events. The `NotificationContext` merges backend history with local state on each `refreshHistory()` call.

**NotificationContext authority model:** Backend is authoritative. On each `refreshHistory()`, backend items are merged into local state using both `id` and `sessionId:notificationType` dedup keys. Local callbacks (onView, onApprove) are preserved from local state since they're not persisted server-side.

## 5. AcknowledgeSession RPC

**File:** `server/services/review_queue_service.go` lines 140–208

The `AcknowledgeSession` handler:
1. Immediately removes the session from the in-memory `reviewQueue` via `rqs.reviewQueue.Remove(req.Msg.Id)`
2. Looks up the live instance in `reviewQueuePoller.FindInstance()` (preferred, avoids loading stale instances from DB)
3. Falls back to storage lookup if instance not in poller
4. Calls `instance.MarkAcknowledged()` and persists via `storage.UpdateInstance()`
5. Publishes `events.NewSessionAcknowledgedEvent()` to EventBus
6. Returns `AcknowledgeSessionResponse{Success: true, Message: "..."}` — **no session snapshot included**

**Gap (R2.3):** The response does NOT include the current session proto snapshot. The frontend cannot dispatch `upsertSession` with the latest server state from this response alone.

**Gap (G1):** The `EventSessionAcknowledged` event published in step 5 is consumed by `ReactiveQueueManager.handleSessionAcknowledged()` (removes from queue) and `analytics/subscriber.go`. It is NOT forwarded through `WatchSessions` via `event_converter.go` (not present in the switch statement). So tabs connected to `WatchSessions` never learn that a session was acknowledged.

## 6. Cross-Tab State

**File search result:** Only `web-app/src/lib/hooks/usePushNotifications.ts` references browser-level inter-tab APIs.

There is **no BroadcastChannel, SharedWorker, or postMessage usage** for state synchronization in the review queue or session list. Cross-tab sync relies entirely on each tab independently connecting to the server streams (`WatchSessions`, `WatchReviewQueue`). Notification dismissal state is stored in `localStorage` (via `notificationStorage.ts`) which is tab-local and not broadcast to other tabs.

## 7. Optimistic Update Patterns

| Action | Optimistic Update | Source |
|---|---|---|
| `acknowledgeSession` (review queue) | Yes — dispatches `removeItem(sessionId)` before RPC | `useReviewQueue.ts` line 361 |
| `deleteSession` | Yes — dispatches `removeSession(id)` | `useSessionService.ts` line 307 |
| `createSession` | Yes — dispatches `upsertSession(session)` from response | `useSessionService.ts` line 243 |
| `updateSession` | Yes — dispatches `upsertSession(session)` from response | `useSessionService.ts` line 280 |
| `approvalResponse` received | No optimistic — calls `removeToastByApprovalId` then `refreshHistory()` | `SessionServiceContext.tsx` lines 84–89 |
| `acknowledgeSession` in session list | No optimistic — no dispatch in `useSessionService.acknowledgeSession` | `useSessionService.ts` lines 502–517 |

**G2 confirmation:** When `approvalResponse` arrives, the session list is NOT optimistically updated. The session card will continue showing "Needs Approval" until the next `EventSessionUpdated` arrives from the backend after `inst.Approve()` changes the execution state. The `onApprovalResponse` callback does not dispatch any Redux action to `sessionsSlice`.

## 8. Stream Reconnect and Replay

**WatchSessions (has replay):**
- Maintains `lastSeqRef` tracking the last received event sequence number
- On reconnect, passes `afterSeq: lastSeqRef.current` to the new stream request
- Server replays events with seq > afterSeq (up to 1 hour buffer)
- Before reconnect, calls `listSessions()` to flush any state missed during the window

**WatchReviewQueue (no replay):**
- No sequence number tracking in `useReviewQueue.ts`
- On reconnect (AbortError), the stream silently stops — no reconnect logic is implemented in `useReviewQueue.ts`
- The 30-second fallback poll provides eventual consistency, but any events missed during a disconnect (itemAdded/itemRemoved) are not replayed
- A new connection with `initialSnapshot: true` would re-hydrate current state, but the reconnect is not automatic in the current implementation

## Gap Summary

| Gap | Location | Root Cause |
|---|---|---|
| **G1**: EventSessionAcknowledged invisible to frontend | `event_converter.go` switch | Missing `EventSessionAcknowledged` case — not converted to proto |
| **G2**: Approval action doesn't update session list | `SessionServiceContext.tsx` `onApprovalResponse` | No Redux dispatch on `approvalResponse` event; waits for `EventSessionUpdated` |
| **G3**: Session deletion leaves stale review queue entries | `review_queue_manager.go` `handleEvent` | Missing `EventSessionDeleted` case in event switch |
| **G4**: Notification dismissal not cross-tab | `notificationStorage.ts` | localStorage only; no BroadcastChannel or server-side sync |
| **G5**: Review queue item state diverges from session state | `useReviewQueue.ts` Phase 1 shortcut | Stream events trigger full REST re-fetch instead of in-place delta; no normalization from `sessionsSlice` |
| **G6**: EventUserInteraction not on wire | `event_converter.go` switch | Missing `EventUserInteraction` case |

## Key Files Reference

| File | Role |
|---|---|
| `server/review_queue_manager.go` | Subscribes to EventBus, manages stream clients, publishes delta events |
| `server/services/review_queue_service.go` | WatchReviewQueue RPC handler, AcknowledgeSession RPC |
| `server/services/event_converter.go` | Converts internal events to proto for WatchSessions wire |
| `web-app/src/lib/hooks/useReviewQueue.ts` | Subscribes to WatchReviewQueue stream, manages Redux state |
| `web-app/src/lib/hooks/useSessionService.ts` | Subscribes to WatchSessions stream, handles session events |
| `web-app/src/lib/contexts/SessionServiceContext.tsx` | Wires onApprovalResponse/onReconnect to NotificationContext |
| `web-app/src/lib/contexts/NotificationContext.tsx` | Manages notification toast and history state |
| `web-app/src/lib/hooks/useNotificationHistory.ts` | Fetches persisted notification history from server |
| `web-app/src/lib/utils/notificationStorage.ts` | localStorage-only notification dedup/acknowledgment |
| `web-app/src/lib/store/reviewQueueSlice.ts` | Redux slice for review queue (flat ReviewQueue, not normalized) |
| `web-app/src/app/notifications/NotificationsPage.tsx` | Notification page — uses NotificationContext, no direct stream |
