# Event Pipeline Consistency — Pitfall Analysis

## Pitfall 1: EventSessionAcknowledged Forwarding Race (G1 / R1)

**Description.** When `AcknowledgeSession` is called, the Go handler does two things in sequence:

1. `rqs.reviewQueue.Remove(req.Msg.Id)` — removes the item from the in-memory queue (synchronous, under `ReviewQueue.mu`)
2. `rqs.eventBus.Publish(events.NewSessionAcknowledgedEvent(...))` — fans the event to all subscribers

`ReactiveQueueManager` is one subscriber. It consumes the event via `processEvents()` → `handleSessionAcknowledged()` → `rq.queue.Remove()` — which is a no-op because step 1 already removed it. `OnItemRemoved` fires from step 1, which triggers `publishToClients()` and writes a `ReviewQueueItemRemovedEvent` to each `WatchReviewQueue` stream client.

If R1 is implemented (forwarding `EventSessionAcknowledged` through `WatchSessions`), a new race window opens:

- The `EventSessionAcknowledged` wire event is assigned a sequence number by the bus and fanned to `WatchSessions` subscribers.
- Simultaneously, a new `WatchReviewQueue` client connecting for the first time calls `sendInitialSnapshot()` which calls `rq.queue.List()` under `RLock`. Because `Remove()` already completed before `Publish()`, the snapshot will not include the acknowledged item — this part is safe.
- **The actual race**: if a `WatchReviewQueue` snapshot is in-flight (already dequeued from `eventCh` by a goroutine but not yet written to the network) at the moment `EventSessionAcknowledged` arrives at the `WatchSessions` stream, the frontend can receive the snapshot first and then the acknowledged event. Since the snapshot does not contain the item (the queue is already clean), this ordering is actually safe — the acknowledged event is redundant but not harmful.

**Revised risk.** The race is benign under the current `Remove → Publish` ordering because the queue is clean before the event is published. However, R1 requires that `EventSessionAcknowledged` include the session's updated proto snapshot (R2.3) so the frontend can `upsertSession`. If the snapshot is not attached (only session ID is), the frontend has no way to clear the attention-reason chip without a separate `EventSessionUpdated` — and that update may not arrive if the session has no other state change.

**Recommendation.** When forwarding `EventSessionAcknowledged`, include the full session proto (or at least a cleared `SubStatus` field) so the frontend can `upsertSession` without waiting for a subsequent `EventSessionUpdated`.

---

## Pitfall 2: Optimistic Update Divergence on Approval (R2.1)

**Description.** R2.1 proposes an optimistic `upsertSession` dispatch in the `approvalResponse` handler in `useSessionService.ts` to immediately clear "Needs Approval" on the session card. The approval flow on the server is:

1. `ResolveApproval` RPC is called → `approvalStore.Resolve()` → HTTP hook handler unblocks → Claude Code receives the decision and resumes.
2. Separately, `ApprovalHandler` (or `ApprovalService`) calls `eventBus.Publish(NewApprovalResponseEvent(...))` which reaches the frontend via `WatchSessions`.
3. Claude Code, now unblocked, transitions its session status. `EventSessionUpdated` with the new `DetectedStatus` (e.g., `StatusActive` or `StatusError`) arrives some milliseconds later.

**Divergence scenario.**
- Frontend receives `ApprovalResponseEvent` → optimistically clears "Needs Approval" → sets status to "Processing" (or whatever the optimistic value is).
- Session immediately crashes during tool execution. Server emits `EventSessionUpdated` with `StatusError`.
- Frontend receives the `EventSessionUpdated` and calls `upsertSession` — the session card should now show the error state correctly.

**Verdict.** If the optimistic update only clears the "Needs Approval" sub-status (does not invent a new overall status), and the next `EventSessionUpdated` is authoritative via `upsertSession`, divergence is self-correcting within one event cycle. The risk is transient — the session card shows "not needing approval" during the brief window between the `ApprovalResponseEvent` and the `EventSessionUpdated`. This is acceptable behavior.

**Dangerous case.** If the optimistic update sets the session status to a specific value (e.g., `"running"`) rather than only clearing the approval indicator, and the server's `EventSessionUpdated` is delayed (e.g., Claude Code takes 2–3 seconds to resume), the card will display a misleading status for those seconds. The fix is to only mutate the sub-status field (approval indicator) in the optimistic update, never the primary session status.

**Recommendation.** The optimistic `upsertSession` payload in R2.1 must only clear `subStatus` (or the equivalent field for the "Needs Approval" chip). It must not set `status`, `detectedStatus`, or `workingState` — those fields must remain as they were until the next authoritative `EventSessionUpdated` arrives.

---

## Pitfall 3: Review Queue Snapshot vs Deletion Race (G3 / R3)

**Description.** `sendInitialSnapshot` (called when a new `WatchReviewQueue` client connects) takes a snapshot by calling `rq.queue.List()`, which acquires `rq.mu.RLock()`. This is correct. The key question is: can a concurrently deleted session appear in the snapshot?

Looking at `DeleteSession`:
1. `s.removeFromAllPollers(sessionTitle)` → calls `s.reviewQueueSvc.GetQueue().Remove(id)` → acquires `rq.mu.Lock()`, deletes the item, releases the lock, fires `OnItemRemoved` → `publishToClients` sends a `ReviewQueueItemRemovedEvent` to existing stream clients.
2. `s.eventBus.Publish(events.NewSessionDeletedEvent(sessionUUID))` — this is the WatchSessions event.

**Race window.** Step 1 removes from the queue before step 2 publishes the deleted event. A new `WatchReviewQueue` client that connects between steps 1 and 2 will receive a snapshot without the deleted session — which is correct.

However, `ReactiveQueueManager.handleEvent` does NOT currently handle `EventSessionDeleted` (the `switch` in `handleEvent` only handles `EventUserInteraction`, `EventSessionAcknowledged`, and `EventApprovalResponse`). So:

- If R3 is implemented (R3.1: subscribe to `EventSessionDeleted`), and the implementation calls `rqm.queue.Remove(sessionID)`, this is a no-op because `removeFromAllPollers` already removed it in step 1. The `OnItemRemoved` callback would NOT fire again for the event-bus-driven removal (since the item is already gone), so no duplicate `ReviewQueueItemRemovedEvent` would be sent to stream clients. This is safe.

- **The real G3 risk** is not the snapshot race but the **periodic poller**: `ReviewQueuePoller` periodically re-evaluates all sessions. If `removeFromAllPollers` removes the session from the poller's instance list (`reviewQueuePoller.RemoveInstance(id)`) before the poller's next cycle, the deleted session will not be re-added. But if there is a race between `RemoveInstance` and an in-progress `CheckSession` goroutine (note that `handleApprovalResponse` launches `go rqm.poller.CheckSession(inst)` asynchronously), the in-progress check could re-add the deleted session to the queue before `RemoveInstance` completes.

**Recommendation.** R3.2's `rqm.queue.Remove(sessionID)` in `handleEvent(EventSessionDeleted)` is the right safety net. Even if `removeFromAllPollers` already removed it, the explicit removal in the event handler closes the race window from the async `CheckSession` goroutine.

---

## Pitfall 4: Notification Dismissal and Seq-Based Event Replay (G4 / R4)

**Description.** Notification dismissal is currently entirely client-side (`localStorage`). On stream reconnect, the client uses `afterSeq: lastSeqRef.current` to replay missed events from the ring buffer. Events in the ring buffer are all event types that pass through `convertEventToProto` — currently: `EventSessionCreated`, `EventSessionUpdated`, `EventSessionDeleted`, `EventNotification`, `EventApprovalResponse`.

If R4 is implemented by adding a `NotificationDismissedEvent` to the wire, the following replay issue exists:

- User dismisses notification N on tab A at time T₀. `NotificationDismissedEvent` is published to the bus (seq=X) and written to the ring buffer.
- Tab B disconnects at T₁ < T₀ (before the dismissal) and reconnects at T₂ > T₀.
- Tab B reconnects with `afterSeq = lastSeq_B < X`, so it replays the `NotificationDismissedEvent` and correctly dismisses N.
- **Scenario 2:** Tab C connects fresh (afterSeq=0) at T₂. It receives the initial session snapshot but does NOT receive the `NotificationDismissedEvent` because `afterSeq=0` skips the ring buffer replay. It will show N as unread.

This is the fundamental limitation of ephemeral event buses for "state" events (dismissals). The fix for fresh connections is to include dismissed notification IDs in the initial WatchSessions response or in a separate `GetNotificationDismissals` RPC call made at page load.

**BroadcastChannel alternative (for R4 simpler path).** If only same-browser tab sync is required (not server persistence), `BroadcastChannel` is safe and avoids the replay problem entirely — each tab's channel listener fires on dismissal in any other tab in the same browser session. No replay needed, no ring buffer concern.

**Recommendation.** If server-side persistence for dismissed IDs is added (R4.2), the initial WatchSessions snapshot should include dismissed notification IDs. If using `BroadcastChannel`, no seq replay concerns exist. Do not emit `NotificationDismissedEvent` as a seq-tracked bus event without also providing a way for fresh connections to learn the current dismissed set.

---

## Pitfall 5: Double-Remove in Multi-Tab EventSessionAcknowledged (R1)

**Description.** If two tabs (A and B) are open and tab A acknowledges session S:

1. Tab A's `WatchSessions` receives `EventSessionAcknowledged` → dispatches `removeReviewQueueItem(S)` (if R1.4 dispatches this action).
2. Tab B's `WatchSessions` also receives `EventSessionAcknowledged` → dispatches `removeReviewQueueItem(S)`.
3. Simultaneously, `ReactiveQueueManager`'s `OnItemRemoved` fires a `ReviewQueueItemRemovedEvent` on the `WatchReviewQueue` stream → tab A's `WatchReviewQueue` handler calls `removeItem(S)` → tab B's `WatchReviewQueue` handler also calls `removeItem(S)`.

**Is double-remove safe?** The `removeItem` reducer in `reviewQueueSlice.ts` does:
```ts
state.reviewQueue.items = (state.reviewQueue.items ?? []).filter(
  (item) => item.sessionId !== action.payload
);
const newTotal = Math.max(0, state.reviewQueue.totalItems - 1);
```

The `totalItems` decrement fires for each removal. If `removeItem(S)` is dispatched twice, `totalItems` will be decremented twice even though S was only in the queue once. This will make the badge count go negative (clamped to 0 by `Math.max`) but then show 0 prematurely if other items are still in the queue.

**Verdict.** This is a real bug risk. If both `EventSessionAcknowledged` (via `WatchSessions`) and `ReviewQueueItemRemovedEvent` (via `WatchReviewQueue`) both trigger a `removeItem` dispatch, the totalItems counter can drift.

**Recommendation.** `removeItem` should check whether the item exists before decrementing `totalItems`, or derive `totalItems` from `items.length` rather than maintaining it as a separate counter:
```ts
removeItem(state, action: PayloadAction<string>) {
  if (!state.reviewQueue) return;
  const before = state.reviewQueue.items.length;
  state.reviewQueue.items = state.reviewQueue.items.filter(
    (item) => item.sessionId !== action.payload
  );
  const removed = before - state.reviewQueue.items.length;
  state.reviewQueue.totalItems = Math.max(0, state.reviewQueue.totalItems - removed);
  state.stats.totalItems = Math.max(0, state.stats.totalItems - removed);
}
```

---

## Pitfall 6: WatchReviewQueue Snapshot vs WatchSessions EventSessionDeleted Ordering (G5 / R5)

**Description.** Both `WatchSessions` and `WatchReviewQueue` are independent streams, each with their own ordering guarantees. Consider:

1. Session S is deleted. `removeFromAllPollers(S)` removes S from the review queue (synchronously).
2. `eventBus.Publish(NewSessionDeletedEvent(S))` is called — this fans out to all `WatchSessions` subscribers.
3. Frontend tab receives `EventSessionDeleted` → dispatches `removeSession(S)` and `removeReviewQueueItem(S)`.
4. Separately, `WatchReviewQueue`'s `OnItemRemoved` fired at step 1 and the `ReviewQueueItemRemovedEvent` was queued to stream clients. Due to buffered channel (`eventCh` capacity=100) and goroutine scheduling, this event may arrive at the frontend after the `sessionDeleted` from step 3, or it may arrive first.

If it arrives first, the frontend removes S from `reviewQueueSlice` (correct). Then `sessionDeleted` arrives and removes S from `sessionsSlice` AND calls `removeReviewQueueItem(S)` again — triggering the double-remove bug described in Pitfall 5.

If it arrives second, `sessionDeleted` first removes S from both slices (correct state), then `ReviewQueueItemRemovedEvent` arrives and calls `removeItem(S)` which is already gone — `totalItems` will be decremented again (bug).

**Verdict.** The ordering between `WatchSessions` and `WatchReviewQueue` is fundamentally non-deterministic. Both streams independently remove the session from the review queue slice, creating the double-remove problem. This interacts directly with Pitfall 5 and will manifest every time a session is deleted while in the review queue.

**Recommendation.** The root fix is to make `removeItem` idempotent on `totalItems` (see Pitfall 5 recommendation). Additionally, `removeReviewQueueItem` in the `sessionDeleted` case handler should check if the item is actually in the queue before decrementing.

---

## Pitfall 7: Session Deletion + In-Flight Approval RPC (G3 / R3)

**Description.** If a user clicks "Approve" and the `ResolveApproval` RPC is in-flight when the session is simultaneously deleted:

1. `DeleteSession` is called → `removeFromAllPollers(sessionTitle)` → `approvalStore` items for S are NOT explicitly removed. `removeFromAllPollers` does not call `approvalStore.Remove()` or `approvalStore.Resolve()`.
2. `ResolveApproval` RPC completes on the server: `approvalStore.Resolve(approvalId, decision)` calls `Resolve()` which sends to `approval.decisionCh`. The HTTP hook handler at `ApprovalHandler.ServeHTTP` is blocked in `select { case decision = <-approval.decisionCh: ... }`.
3. However, after `DeleteSession`, the `r.Context()` for the approval HTTP hook may have been cancelled (if the session's context is tied to the tmux process context). If so, the `case <-r.Context().Done()` arm fires, stamping "canceled" and writing nothing to the deleted session's process — which is harmless since the session is gone.
4. If the hook context is NOT cancelled (the session was deleted but the hook goroutine is still alive), `writeDecision(w, "allow", "")` is called on an HTTP response writer for a deleted session. This is a no-op from Claude Code's perspective (it may have already restarted or exited).

**Verdict.** The in-flight approval RPC itself does not panic — `approvalStore.Resolve()` returns `CodeNotFound` only if the approval ID was already removed from the store. The risk is a benign race where the decision is written to a dead process, or the hook context fires "canceled" and stamps a misleading reason. No crash or data corruption occurs.

**Edge case.** If `DeleteSession` is called, then the `EventSessionDeleted` arrives at the frontend, which dispatches `removeReviewQueueItem(S)` — but the approval toast from the notifications page may still be visible with an active "Approve/Deny" button. The user can click it again. The second `ResolveApproval` call will hit `approvalStore.Resolve(approvalId)` which returns `CodeNotFound` (since the approval was already resolved or the store entry removed). The frontend should handle this gracefully with an error toast.

**Recommendation.** `DeleteSession` should call `approvalStore.RemoveBySession(sessionID)` to cancel any pending approvals, and the approval context channel should be closed on removal. The UI should disable the Approve/Deny buttons when `EventSessionDeleted` is received for the session.

---

## Summary: Highest-Risk Pitfalls

### Risk 1 (CRITICAL): Double-remove corrupts `totalItems` badge count (Pitfall 5 + 6)
The `removeItem` reducer decrements `totalItems` without checking if the item was actually present. With two independent streams (`WatchSessions` + `WatchReviewQueue`) both firing removal actions for the same session deletion or acknowledgement, the badge count will silently drift negative (clamped to 0). This happens on every session deletion while in the review queue — already a common user action. Fix: make the reducer check existence before decrementing, or derive the count from `items.length`.

### Risk 2 (HIGH): Optimistic approval update must not invent session status (Pitfall 2)
R2.1's optimistic `upsertSession` will cause a misleading session state if it sets `status` or `workingState` directly. If the session crashes during the approval execution window, the card will show "running" while the server's `EventSessionUpdated` with `StatusError` is in transit. Only `subStatus` (the "Needs Approval" chip) must be cleared optimistically — all other fields must remain authoritative from the server.

### Risk 3 (HIGH): In-flight approval + session deletion leaves stale approval toasts (Pitfall 7)
`DeleteSession` does not remove or cancel pending approvals in `approvalStore`. The notification panel will continue to show an active approval toast with clickable Approve/Deny buttons after the session is deleted. Clicking those buttons returns `CodeNotFound` — the UI must handle this gracefully and suppress the buttons when `EventSessionDeleted` is received for the associated session.
