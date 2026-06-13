# Features Research: Smart Notification Dedup

## Existing Notification Surfaces

### 1. In-App Toast (NotificationContext + NotificationToast)
- Managed by `NotificationContext.tsx` `notifications: NotificationData[]` state
- One toast per `sessionId` max (older replaced by newer for same session)
- Auto-close driven by `toastAutoCloseMs()` in `notification-policy.ts`
- Auto-minimize driven by `toastAutoMinimizeMs()`
- Stale sweep every 60s removes toasts older than TOAST_STALE_MS / ACTIONABLE_TOAST_STALE_MS

### 2. Native Browser Notification (Web Notification API)
- Created in `showBrowserNotification()` in `notifications.ts`
- Fire-and-forget: `new Notification(title, options)` — no reference stored, no auto-close
- Uses `tag:` option in some call sites (review queue) for OS-level dedup
- Fork pressure path (`useSessionNotifications.ts`) does NOT call showBrowserNotification at all — only toast

### 3. Notification History Panel
- Persisted in `NotificationHistoryStore` (JSON file on disk)
- Backend-authoritative: frontend hydrates from server on load/reconnect
- `useNotificationHistory` hook fetches via ConnectRPC
- All notifications (toast-suppressed or not) reach history via server-side store

### 4. Push Notifications (separate path)
- `push.StartPushSubscriber` subscribes to EventBus and sends to registered push endpoints
- Separate from browser Notification API — uses service worker / push protocol
- Out of scope for this feature

## Notification Creation Flows

### Flow A: Fork Pressure Alert
```
checkPressure() → alertFns[] → server.go callback → EventBus.Publish(NotificationEvent)
    → [frontend] useSessionNotifications.handleNotification()
        → addNotification() (toast only)
        → (no native browser notification created)
```

### Flow B: Review Queue (Approval/Input Required)
```
ReviewQueue polling → useReviewQueueNotifications effect
    → showSessionNotification() (toast)
    → showBrowserNotification() (native, with tag)
```

### Flow C: Tmux Server Recovery
```
tmux.SetServerRecoveryCallback → EventBus.Publish(NotificationEvent)
    → useSessionNotifications.handleNotification()
        → addNotification() (toast only)
```

## Edge Cases & Failure Modes

### Edge Case 1: Zombie count stable but non-zero
Current behavior: every 2 minutes, `checkPressure` fires again because cooldown expired.
Required: compare current `ZombiesInWindow` to `lastAlertZombieCount`; suppress if equal.

### Edge Case 2: Level escalation mid-stable-count
E.g. zombie count stays at 10 but spawn failures newly cross 10 (warning→critical).
Required: a level upgrade must always fire, even if count unchanged (FR-1 "worsened").

### Edge Case 3: Count drops below threshold then rises again (FR-5 re-arm)
Zombie count: 12 → 0 → 15. The 0-crossing should reset `lastAlertZombieCount = 0` and `lastAlertLevel = OK`.
Current code: `checkPressure` returns early if `level == OK`, so the reset must happen in that early-return branch.

### Edge Case 4: Multiple browser tabs open
Each tab runs its own `useSessionNotifications` and `useReviewQueueNotifications`. Each creates its own `Notification` objects. The `tag:` option causes the browser to replace earlier notifications with the same tag, but only within the same origin/registration.

### Edge Case 5: Notification permission denied mid-session
`showBrowserNotification` silently returns — no error propagated. The in-app toast still fires. Mixed suppression is acceptable per NFR-1 (approvals must never be suppressed — toast side is independent of native).

### Edge Case 6: Mobile Safari
`Notification` API requires user gesture on iOS Safari; `Notification.requestPermission()` silently fails or returns "denied" without a prompt if not initiated by click. The current implementation calls it on first notification event, not on a button click. This is a pre-existing limitation that dedup changes don't worsen.

### Edge Case 7: Approval notification accidentally deduplicated
Current `useSessionNotifications` already guards against this:
```ts
const isApproval = event.notificationType === APPROVAL_NEEDED || INPUT_REQUIRED;
if (!isApproval && lastShown && now - lastShown < DEDUP_WINDOW_MS) { return; }
```
The same guard pattern must be applied to any new backend-side condition-change gating.

## Feature Gaps vs Requirements

| Requirement | Current State | Gap |
|---|---|---|
| FR-1: Condition-change gating | Only cooldown-based | Need zombie/level baseline in `forkMonitor` |
| FR-2: Count baseline after notification | Not tracked | Add `lastAlertZombieCount`, `lastAlertFailureCount` fields |
| FR-3: Native notification auto-dismiss | Never closes | Need `setTimeout(() => n.close(), ms)` after creation |
| FR-4: Native notification dedup (close old before new) | OS `tag:` dedup only (best-effort) | Need explicit `Notification` handle map + `close()` |
| FR-5: Re-arm on clear | Not implemented | Reset baseline in `checkPressure` OK branch |
| FR-6: Consistent toast + native behavior | Fork pressure: toast only, no native; review queue: both | Fork pressure path needs native notification wired in via useSessionNotifications |

## Unstated User Needs

1. **Visual indication that dedup is active**: if the user sees no new notifications for 10 minutes while still in a critical state, they may assume the app is broken. A persistent badge or header indicator showing "Fork Pressure: Critical" would complement dedup.

2. **Manual dismiss clears both surfaces**: dismissing the in-app toast should also close the native notification. Currently no coordination between the two exists.

3. **Audit trail**: the notification history panel currently de-dupes by `(sessionId, type)` which means re-occurrences after a clear (FR-5) should show as a new entry, not update the old one. This requires verifying that `notifications.json` handles re-arming correctly.
