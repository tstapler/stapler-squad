# Architecture Research: Smart Notification Dedup

## System Overview

```
[Backend: Go]
  forkMonitor (package-level state)
    └─ checkPressure() → alertFns[] (registered callbacks)
         └─ server.go callback → EventBus.Publish(NotificationEvent)

[Frontend: React]
  EventBus SSE/WebSocket stream
    └─ useSessionNotifications.handleNotification()
         ├─ addNotification() → NotificationContext (toast)
         └─ (fork pressure path does NOT call showBrowserNotification)

  useReviewQueueNotifications (review queue polling)
    ├─ showSessionNotification() → toast
    └─ showBrowserNotification() → native Notification (fire-and-forget)
```

## Integration Points

### Backend: `forkMonitor` struct (`session/tmux/fork_metrics.go`)

The global `forkMonitor` struct is the single source of truth for fork pressure state. It already holds:
- `lastAlertAt time.Time` — time of last alert
- `alertMu deadlock.Mutex` — guards both `lastAlertAt` and `alertFns`

**Proposed additions** (all under `alertMu`):
```go
lastAlertZombieCount   int64             // zombie count at last alert emission
lastAlertFailureCount  int64             // failure count at last alert emission
lastAlertLevel         ForkPressureLevel // level at last alert emission
```

**Modified `checkPressure` logic**:
```
1. snapshot → if level == OK:
      if lastAlertLevel != OK:
          reset baseline (lastAlertZombieCount=0, lastAlertFailureCount=0, lastAlertLevel=OK)
      return  ← (no alert)

2. acquire alertMu
3. cooldown check (existing)
4. NEW: condition-change check:
      if level == lastAlertLevel
         && stats.ZombiesInWindow <= lastAlertZombieCount
         && stats.FailuresInWindow <= lastAlertFailureCount:
          release lock; return  ← suppress
5. update baseline + lastAlertAt
6. release lock; fire alertFns
```

This puts ALL gating logic in one place — the callback in `server.go` does not change.

### Frontend: `showBrowserNotification` (`web-app/src/lib/utils/notifications.ts`)

**Current signature**: `async function showBrowserNotification(title, options?): Promise<void>`

**Problem**: returns nothing — no handle to close the notification.

**Proposed change**: introduce a module-level `Map<string, Notification>` keyed by notification tag. When creating a new notification with a tag, close the previous one first:

```typescript
const activeNativeNotifications = new Map<string, Notification>();

export async function showBrowserNotification(title, options?): Promise<void> {
  // ... permission check unchanged ...
  const tag = options?.tag ?? "__untagged__";
  // Close previous notification with same tag
  activeNativeNotifications.get(tag)?.close();
  const notif = new Notification(title, { icon: ..., ...options });
  activeNativeNotifications.set(tag, notif);
  // Auto-dismiss
  const ttl = options?.autoClosMs ?? DEFAULT_NATIVE_TTL_MS;
  setTimeout(() => {
    notif.close();
    if (activeNativeNotifications.get(tag) === notif) {
      activeNativeNotifications.delete(tag);
    }
  }, ttl);
  notif.onclose = () => activeNativeNotifications.delete(tag);
}
```

**Tag convention for fork pressure** (new — currently fork pressure never calls showBrowserNotification):
- Tag: `fork-pressure` (single slot; replaced on each new alert)

**Tag convention for review queue** (existing):
- Tier 1: `review-queue-tier1-${sessionId}` — per-session slot
- Tier 2: `review-queue-tier2` — shared slot

### Notification Policy constants (`web-app/src/lib/notification-policy.ts`)

New constants to add (FR-3):
```typescript
/** Auto-close delay for high/urgent native notifications (FR-3) */
export const NATIVE_HIGH_TTL_MS = 30_000;
/** Auto-close delay for medium/low native notifications (FR-3) */
export const NATIVE_MEDIUM_TTL_MS = 15_000;
/** Auto-close delay for actionable native notifications (FR-3) */
export const NATIVE_ACTIONABLE_TTL_MS = 5 * 60 * 1000;
```

`showBrowserNotification` should accept an optional `autoCloseMs` parameter, defaulting to `NATIVE_MEDIUM_TTL_MS`.

### Wiring fork pressure → native notification (FR-6 gap)

`useSessionNotifications.ts` handles `NotificationEvent` from the backend but does not call `showBrowserNotification`. To satisfy FR-6 (consistent toast + native behavior), add native notification in `handleNotification`:

```typescript
// After addNotification() call:
if (event.sessionId === "fork-pressure" || event.notificationType === NotificationType.ERROR) {
  void showBrowserNotification(event.title, {
    body: event.message,
    tag: `system-${event.sessionId}`,
    autoCloseMs: NATIVE_HIGH_TTL_MS,
  });
}
```

A cleaner approach: map `NotificationPriority` to auto-close TTL in `handleNotification`.

## Data Flow After Changes

```
[Backend]
checkPressure() call (triggered by RecordZombieProcess or recordFailure):
  1. snapshot metrics
  2. if level == OK and lastAlertLevel != OK → reset baseline → return
  3. acquire alertMu
  4. cooldown check (existing, 2 min)
  5. condition-change check (NEW):
      suppress if level unchanged AND counts not higher
  6. update lastAlertAt, lastAlertZombieCount, lastAlertFailureCount, lastAlertLevel
  7. fire alertFns → EventBus.Publish(NotificationEvent)

[Frontend]
useSessionNotifications.handleNotification(event):
  1. dedup check (existing, 10s TOAST_DEDUP_WINDOW_MS) ← secondary guard, mostly redundant now
  2. addNotification() → toast
  3. showBrowserNotification() [NEW] → native notif with auto-close + dedup by tag

showBrowserNotification():
  1. close previous native notif with same tag
  2. create new Notification
  3. schedule setTimeout(notif.close(), ttl)
  4. store in activeNativeNotifications map
```

## Consistency Guarantee (FR-6)

The `handleNotification` callback is the single point where both toast and native notification fire. Adding `showBrowserNotification` there ensures they always fire together and are suppressed together (the existing `TOAST_DEDUP_WINDOW_MS` check gates both).

## Non-Regression Design (NFR-1)

The `isApproval` guard in `useSessionNotifications` already bypasses the dedup check. The new `showBrowserNotification` call in that path must also bypass dedup — approval tags should use unique IDs (e.g. `approval_id` from metadata) so each one opens a fresh notification slot.

## Backward Compatibility (NFR-3)

- No proto changes needed (all changes are backend state tracking + frontend lifecycle)
- `notifications.ts` API is internal to the frontend — no external contract
- `forkMonitor` struct change is internal to the `tmux` package — no exported API changes beyond `ForkPressureStats` (could optionally add `LastAlertLevel` to the snapshot for observability)
