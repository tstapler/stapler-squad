# Features Research: Cross-View State Consistency and Notification Sync

Research for the `event-pipeline-consistency` project. Covers patterns used by real-time collaborative
apps (Linear, GitHub, Slack, Phabricator) to keep multi-view state consistent and propagate
acknowledgement/dismissal events across surfaces.

---

## 1. Real-Time Review Queue Patterns

### Dominant Pattern

Apps like Linear and GitHub use a **single authoritative event stream** where every state mutation
produces a typed event. All views (sidebar, main panel, badges) subscribe to the same stream — they
never poll independently for their own snapshot.

**Linear**: Uses GraphQL Subscriptions (WebSocket) where the sidebar issue list and detail panel
share the same subscription client. A mutation (`updateIssue`) returns an optimistic result
immediately, and the subscription fires the canonical server event within milliseconds, reconciling
both views simultaneously.

**GitHub PR Review**: Uses a combination of SSE (for notification badges) and full-page subscription
channels (Action Cable in Rails for some parts, REST polling for others). The review queue (PR list)
and the PR detail view are on separate pages, so "sync" is achieved by: (a) server-side state being
authoritative, (b) navigation between pages doing a fresh fetch.

**Phabricator** (Differential): Uses long-polling. The sidebar "Revision Queue" is a server-rendered
snapshot; review panel does its own AJAX poll. No real-time sync between them — this is a known
weakness of Phabricator's architecture.

### Mechanism

The critical insight from Linear's architecture (published in their engineering blog):
- Every mutation fires an event on the backend event bus
- The subscription stream delivers events to **all** connected clients — not just the initiator
- The frontend Redux (or equivalent) store updates all derived views from the single event

```
Mutation → Backend Event Bus → Subscription stream → Redux store → All views re-render
```

For a ConnectRPC streaming app: the `WatchSessions` stream serves exactly this role. The gap is
events that bypass the stream (like `EventSessionAcknowledged` going only to `ReactiveQueueManager`).

### Key Tradeoffs

| Approach | Consistency | Complexity | Suitable for |
|---|---|---|---|
| Shared event stream → unified store | Strong | Medium | Single-user, few tabs |
| Per-view subscriptions | Weaker (can diverge) | Low | Simple apps |
| Server-side polling per view | Weak | Low | Legacy/simple apps |
| CRDT / operational transform | Very strong | High | Multi-user collaborative |

### Recommendation for This Project

The existing architecture (single `WatchSessions` stream → Redux) is correct. The fix is to ensure
**every** state-changing event is forwarded through the stream. Missing events (`EventSessionAcknowledged`,
`EventUserInteraction`) should be added to `event_converter.go`. The review queue (`WatchReviewQueue`)
should either be merged into the same stream or subscribe to session events and derive its state from
the shared `sessionsSlice`.

---

## 2. Notification Acknowledgement Sync Across Browser Tabs

### Dominant Pattern

Production apps use a **layered approach**:

1. **Server-side persistence** — read state stored per (user, notification-id) in the database
2. **WebSocket/SSE event** — server broadcasts a `notification_read` event to all connections for that user
3. **BroadcastChannel** — used as a fast local shortcut within the same browser (avoids round-trip)

**Slack**: Server-side. When you mark a channel as read, Slack sends an API call that persists the
`last_read` timestamp. The server then pushes a `channel_marked` event on the RTM (WebSocket) stream.
All other tabs receive this event and update their badge counts. BroadcastChannel is NOT used — Slack
relies on the WebSocket connection in each tab receiving the server event.

**Linear**: Server-side + GraphQL subscription. Marking a notification as done sends a mutation;
the subscription fires `NotificationUpdate` to all subscribers. Linear also has a "notification_count"
subscription that triggers badge updates independently.

**GitHub**: Server-side only (no real-time for notifications). You mark a notification as read via
REST; the thread count in the nav badge updates on the next page navigation or SSE poll (GitHub uses
SSE for some live-count updates). There is no BroadcastChannel usage in GitHub's notification system.

### Mechanism: BroadcastChannel API

The `BroadcastChannel` API lets same-origin tabs communicate without a server round-trip:

```typescript
// In the tab that dismisses the notification
const channel = new BroadcastChannel('notifications');
channel.postMessage({ type: 'notification_dismissed', id: notificationId });

// In every other tab
const channel = new BroadcastChannel('notifications');
channel.onmessage = (event) => {
  if (event.data.type === 'notification_dismissed') {
    dispatch(dismissNotification(event.data.id));
  }
};
```

BroadcastChannel is **same-origin, same-browser** only. It does NOT persist across browser restarts
and does NOT sync across devices. For a local-first single-user app, this is exactly what you need.

### Mechanism: SharedWorker

A `SharedWorker` acts as a shared in-memory hub for all tabs:

```typescript
// shared-worker.js
const tabs = new Set<MessagePort>();
self.onconnect = (e) => {
  const port = e.ports[0];
  tabs.add(port);
  port.onmessage = (msg) => {
    tabs.forEach(t => { if (t !== port) t.postMessage(msg.data); });
  };
};
```

SharedWorker is more powerful than BroadcastChannel (can maintain shared state), but adds complexity.
Overkill for simple notification dismissal sync.

### Key Tradeoffs

| Mechanism | Persistence | Cross-tab | Cross-device | Complexity |
|---|---|---|---|---|
| localStorage only | Browser restart | Only via storage event | No | Minimal |
| BroadcastChannel | No | Yes (same browser) | No | Low |
| SharedWorker | In-memory only | Yes | No | Medium |
| Server-side + stream | Yes | Yes | Yes (multi-device) | High |

### Recommendation for This Project

For R4 (notification dismissal), the **BroadcastChannel approach** is the right fit:
- Single-user, local-first app — no multi-device requirement
- Same-browser tab sync is the stated requirement
- No additional server infrastructure needed
- Can be layered on top of existing localStorage without a full rewrite

Implementation: When `dismissNotification` is called, (a) write to localStorage as before, (b) post
to BroadcastChannel. All tabs listen and dispatch the Redux action. If later the server-side approach
(R4.1–R4.5) is implemented, BroadcastChannel can be removed since the stream will handle sync.

---

## 3. Optimistic UI Patterns for Approval Flows

### Dominant Pattern

The industry-standard pattern for approval actions is **optimistic UI with rollback**:

1. User clicks "Approve" → UI **immediately** moves/updates the item (optimistic)
2. API call fires in background
3. On success: server confirmation reconciles state (usually no-op if optimistic was correct)
4. On failure: rollback to previous state + show error toast

**GitHub PR Review**: When you submit a review (Approve/Request Changes), GitHub shows an immediate
state change in the PR. The page updates optimistically — the review appears, the merge button state
changes. If the API call fails, GitHub shows an error and reverts. This is a full-page approach, not
a single-field optimistic update.

**Linear**: Full optimistic UI. When you move an issue to "Done", it disappears from the active queue
immediately. If the mutation fails (rare), the item reappears with an error toast. Linear's
architecture paper describes this as: "Apply the mutation locally first, then sync to server."

**Notion**: Locally-first approach — changes are applied immediately to the local store and synced
to the server. Server conflicts are resolved by CRDT merge.

### Standard Implementation Pattern

```typescript
// Redux approach (RTK)
const approveSession = createAsyncThunk('sessions/approve', async (sessionId, { dispatch, getState }) => {
  // 1. Capture previous state for rollback
  const previousSession = selectSession(getState(), sessionId);
  
  // 2. Optimistic update — dispatch immediately
  dispatch(upsertSession({ ...previousSession, subStatus: null, needsApproval: false }));
  
  try {
    // 3. Server call
    await api.approveSession(sessionId);
  } catch (err) {
    // 4. Rollback on failure
    dispatch(upsertSession(previousSession));
    dispatch(showErrorToast('Approval failed'));
    throw err;
  }
});
```

### "Pending" State Variant

Some UIs use a transient "pending" state (spinner, disabled button) instead of full optimistic
remove. This is appropriate when:
- The action has significant side effects (sending emails, billing)
- The success state is substantially different from the pending state
- Rollback would be confusing

For approval of AI agent sessions (no external side effects, clear success state), **full optimistic
UI** (R2.1) is the right call.

### Key Tradeoffs

| Pattern | UX Feel | Complexity | Risk |
|---|---|---|---|
| Optimistic (immediate) | Snappy, responsive | Medium (needs rollback) | Brief flicker on failure |
| Pending state | Honest, safe | Low | Feels slow (100-300ms stall) |
| Wait for server | Sluggish | Lowest | Always accurate |

### Recommendation for This Project

R2.1 is correct: when `ApprovalResponseEvent` arrives with `approved == true`, dispatch an optimistic
`upsertSession` immediately clearing `needsApproval`/`subStatus`. This fires **before** the follow-up
`EventSessionUpdated` arrives. The subsequent `EventSessionUpdated` will be a no-op (same state) or
will add additional fields — either way, no flicker.

The key: clear the "Needs Approval" indicator on the `ApprovalResponse` event, not on the next
`SessionUpdated`. The approval response **is** the signal that the state changed.

---

## 4. Server-Sent Event Ordering for Multi-View Consistency

### The Divergence Problem

When two views subscribe to the same stream but have different processing pipelines, they can diverge:

```
Stream event sequence: [E1, E2, E3]

View A (session list): receives E1, E2, E3 → current state: S3
View B (review queue): receives E1, processes slowly, shows E1 state while A shows E3
```

### Dominant Pattern: Sequence Numbers + Client-Side Buffer

**Linear**: Every event has a `sequenceNumber` (monotonically increasing). The frontend buffers
events that arrive out of order and applies them in sequence. If a gap is detected, it requests a
full snapshot (reconnect to subscription).

**Notion**: Uses vector clocks for CRDT operations. Each operation has a logical timestamp. The store
applies operations in causal order.

**Firebase Realtime Database**: Uses a `timestamp` field plus a "server offset" calibration. Events
are ordered by server-assigned timestamp. Clients queue events until the timestamp is confirmed.

### Mechanism: Sequence Number + Redux Middleware

```typescript
// Redux middleware for event ordering
const eventOrderingMiddleware: Middleware = (store) => (next) => (action) => {
  if (action.type === 'SESSION_EVENT') {
    const { sequenceNumber, payload } = action;
    const expectedSeq = store.getState().events.lastSequence + 1;
    
    if (sequenceNumber === expectedSeq) {
      next(action);
    } else if (sequenceNumber > expectedSeq) {
      // Buffer and wait for missing events (or request resync)
      eventBuffer.push(action);
      scheduleResync();
    }
    // sequenceNumber < expectedSeq: duplicate, discard
  } else {
    next(action);
  }
};
```

The stapler-squad backend already assigns sequence numbers to events (`EventBus` assigns seq).
The existing `WatchSessions` proto messages should carry this sequence number.

### Mechanism: Single Redux Store as Single Source of Truth

The most pragmatic solution for preventing view divergence in a React/Redux app:

1. **Both views read from the same Redux slice** — not separate data sources
2. **Only one place applies events** — `handleSessionEvent` in `useSessionService.ts`
3. **Derived views compute from the unified store**

This is the "normalize your state" pattern from Redux's official documentation. If `reviewQueueSlice`
derives `workingState` from `sessionsSlice` data (join on session ID), the two slices cannot diverge
because they share the same underlying data.

### Key Tradeoffs

| Pattern | Consistency | Complexity | For This App |
|---|---|---|---|
| Sequence numbers + buffer | Strong, handles gaps | Medium | Good if seq numbers exposed |
| Single Redux store, derived views | Strong for same-tab | Low | Best fit for R5 |
| Per-view subscriptions | Weak (can diverge) | Low | Avoid |
| CRDT | Very strong | High | Overkill |

### Recommendation for This Project

R5.2 is the correct approach: have `reviewQueueSlice` derive `workingState` from `sessionsSlice`
rather than maintaining its own copy. This eliminates the dual-source divergence problem entirely
without needing sequence buffering:

```typescript
// reviewQueueSelectors.ts
export const selectReviewQueueItemWithLiveState = createSelector(
  [selectReviewQueueItem, selectSession],
  (queueItem, session) => ({
    ...queueItem,
    // Override queue item's stale working state with live session state
    workingState: session?.detectedStatus ?? queueItem.workingState,
    subStatus: session?.subStatus ?? queueItem.subStatus,
  })
);
```

---

## 5. Notification Dismissal: localStorage vs Server-Side for Local-First Apps

### What Real Apps Do

**Local-first apps (Obsidian, Logseq)**: All state in local files. No server-side notification state.
Cross-tab sync achieved via file system watchers or BroadcastChannel.

**Partially local-first apps (Notion offline mode)**: Server-side is authoritative. Local state is a
cache. On reconnect, server state wins. Notification read state synced via WebSocket.

**Linear**: Fully server-side. No localStorage for notification state. All read/unread state is in
the database, delivered via subscription.

**GitHub**: Fully server-side. Notification read state stored in the database. No BroadcastChannel
usage.

**VS Code (server-based extensions)**: Extensions can use `globalState` (persisted to disk by VS
Code) or `workspaceState`. This is the closest analogue to "local-first server app" — state is on
the local server, not a remote cloud.

### The localStorage Problem

localStorage has two failure modes for multi-tab consistency:
1. **No real-time sync**: Tab B reads stale state until it polls or navigates
2. **`storage` event workaround**: The `window.storage` event fires when localStorage is changed from
   **another tab** (not the current one). This is commonly used for tab sync:

```typescript
window.addEventListener('storage', (event) => {
  if (event.key === 'dismissed_notifications') {
    const dismissed = JSON.parse(event.newValue ?? '[]');
    dispatch(syncDismissedNotifications(dismissed));
  }
});
```

This works but is a side channel — easy to miss edge cases (same-tab writes don't fire the event).

### BroadcastChannel as the Right Middle Ground

For a single-user local-first app needing same-browser tab sync, BroadcastChannel hits the sweet spot:

```typescript
class NotificationSyncService {
  private channel = new BroadcastChannel('stapler-squad:notifications');
  
  dismiss(notificationId: string) {
    // 1. Persist locally
    notificationStorage.dismiss(notificationId);
    
    // 2. Notify other tabs
    this.channel.postMessage({ type: 'dismissed', id: notificationId });
    
    // 3. Update local Redux state
    store.dispatch(dismissNotification(notificationId));
  }
  
  init() {
    this.channel.onmessage = (event) => {
      if (event.data.type === 'dismissed') {
        // Another tab dismissed — sync local state
        notificationStorage.dismiss(event.data.id);
        store.dispatch(dismissNotification(event.data.id));
      }
    };
  }
}
```

### Server-Side Option (R4.1–R4.4)

If the server persists dismissed notification IDs, the existing `WatchSessions` stream can carry
a `NotificationDismissedEvent`. All connected tabs (which each have their own stream connection)
receive the event simultaneously. This is architecturally cleaner and consistent with how other
events are handled in the codebase.

**Minimal server-side implementation**:
- In-memory map `map[string]bool` of dismissed notification IDs (keyed by notification ID)
- No persistence required — resets on server restart (acceptable for notifications)
- New `DismissNotification` RPC writes to the map and emits `EventNotificationDismissed`
- `event_converter.go` forwards to `WatchSessions` stream
- All tabs receive the event and update their Redux state

### Key Tradeoffs

| Approach | Persistence | Cross-tab | Restart survival | Complexity | Fits This App |
|---|---|---|---|---|---|
| localStorage only | Browser | No real-time | Yes | Minimal | Current state (broken) |
| localStorage + `storage` event | Browser | Yes | Yes | Low | Acceptable hack |
| BroadcastChannel + localStorage | Browser | Yes (same browser) | Yes | Low | Good |
| Server in-memory + stream | Session | Yes | No (ephemeral) | Medium | Best fit |
| Server DB + stream | Permanent | Yes | Yes | High | Overkill for notifications |

### Recommendation for This Project

**Server in-memory + stream** is the right call for this codebase:
- Consistent with the existing event-driven architecture
- All tabs receive dismissal via the existing `WatchSessions` stream (no BroadcastChannel needed)
- In-memory is fine — dismissed notification state doesn't need to survive server restarts
- Aligns with R4.1–R4.4 requirements
- Removes the architectural inconsistency of localStorage being the source of truth for some state
  while the server stream is authoritative for everything else

If server-side is deferred, BroadcastChannel + localStorage is the minimal viable fix (R4 alternative
in requirements).

---

## Summary: Most Relevant Patterns for This Project

1. **Forward every state-changing event through the single shared stream** (Linear/Slack pattern):
   The fix for G1, G3, G6 is not architectural change — it's ensuring `event_converter.go` covers
   every event type. All views that consume Redux state automatically get consistent updates.

2. **Optimistic UI on the approval/acknowledgement response event, not on the follow-up session update**:
   For G2 (approval race condition), dispatch the optimistic `upsertSession` when `ApprovalResponseEvent`
   arrives. The response IS the signal — don't wait for the consequent `EventSessionUpdated`. This is
   the standard Linear/GitHub pattern for approval UX.

3. **Derive review queue item state from the session slice, not from queue-item's own copy** (single-store
   normalization): For G5 (review queue divergence), use a selector that joins `reviewQueueSlice` with
   `sessionsSlice`. This eliminates dual-source divergence without requiring any backend changes or
   sequence buffering.
