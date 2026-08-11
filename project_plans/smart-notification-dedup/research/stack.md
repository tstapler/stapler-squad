# Stack Research: Smart Notification Dedup

## Backend (Go)

### Key file: `session/tmux/fork_metrics.go`

**Global `forkMonitor` struct** (package-level var, no constructor needed):
- `totalSpawns`, `totalFailures`, `totalZombies` — `atomic.Int64`
- `spawnRing`, `failureRing`, `zombieRing` — `*timestampRing` (ring-buffer, counts events in sliding window)
- `alertMu deadlock.Mutex`, `lastAlertAt time.Time`, `alertFns []AlertFunc`

**Threshold constants:**
```go
forkPressureWindow         = 30 * time.Second
forkAlertCooldown          = 2 * time.Minute
spawnFailureAlertThreshold = 10   // failures/window → critical
spawnRateWarnThreshold     = 120  // spawns/window → warning
zombieAlertThreshold       = 10   // zombie children/window → alert
```

**`checkPressure(now time.Time)`** — called after every recorded event:
1. Snapshots current metrics
2. If level == OK, returns immediately
3. Acquires `alertMu`; if `lastAlertAt` was set and cooldown not expired → returns
4. Updates `lastAlertAt = now`, copies `alertFns`, releases lock
5. Calls all `alertFns` in a goroutine

**`ForkPressureStats`** snapshot fields (no per-alert baseline tracking):
- `ZombiesInWindow int64` — count of zombie ring entries in last 30s window
- `FailuresInWindow int64` — count of failure ring entries in last 30s window
- `Level ForkPressureLevel` — derived from above thresholds
- `LastAlertAt time.Time` — time of last alert emission

**Critical gap**: No `lastAlertZombieCount` or `lastAlertLevel` field. The cooldown timer is the only gate — once 2 minutes pass, the next `checkPressure` call fires another alert regardless of whether zombie count has changed.

### Key file: `session/tmux/zombie_detector.go`

**`StartZombieWatcher`** — goroutine scanning every 30s:
- Maintains `reported map[int]bool` — per-PID dedup (avoids re-reporting same long-lived zombie PID)
- First scan is baseline: logs but does NOT call `RecordZombieProcess`
- Subsequent scans: calls `RecordZombieProcess` for new PIDs only
- Evicts reaped PIDs from `reported`

**Important**: `RecordZombieProcess` calls `forkMonitor.zombieRing.record(now)` which adds to the ring buffer. The ring buffer is sliding-window based — old entries expire after 30s. So even if the same PID isn't re-reported (per-PID dedup in watcher), the zombie count in the window can still trigger alerts repeatedly if the ring entries are fresh enough.

### EventBus + Registration pattern

`server.go` registers a fork pressure alert callback via:
```go
tmux.RegisterForkPressureAlert(func(level tmux.ForkPressureLevel, stats tmux.ForkPressureStats) {
    event := events.NewNotificationEvent(...)
    deps.EventBus.Publish(event)
})
```

No state is tracked in this callback — it fires blindly when `checkPressure` decides to alert.

### In-memory state pattern (NFR-2 compliance)

The existing pattern uses struct fields on `forkMonitor`. Adding `lastAlertZombieCount int64` and `lastAlertLevel ForkPressureLevel` fields to `forkMonitor` matches the existing pattern exactly — no new persistence layer needed.

### Locking model

`forkMonitor.alertMu` is a `deadlock.Mutex` (from `github.com/linkdata/deadlock`). All new state fields for baseline tracking should be read/written under `alertMu` to maintain the existing single-lock model.

## Frontend

### Browser Notification API

**`web-app/src/lib/utils/notifications.ts`**:
- `showBrowserNotification(title, options?)` — creates `new Notification(title, {...})`, no close(), no reference stored, no auto-dismiss
- Returns `Promise<void>` — caller has no handle to the Notification object

**`web-app/src/lib/hooks/useReviewQueueNotifications.ts`**:
- Calls `showBrowserNotification` for Tier 1 (requireInteraction: true) and Tier 2 (tab hidden only, silent: true)
- Uses `tag:` option on Notification — browser deduplicates by tag (replaces previous with same tag), but this only works if OS respects it

**`web-app/src/lib/hooks/useSessionNotifications.ts`**:
- Handles `NotificationEvent` from the server EventBus stream
- Does NOT call `showBrowserNotification` — only creates in-app toasts via `addNotification`
- Has its own 10s dedup window (`TOAST_DEDUP_WINDOW_MS`)

### Notification Policy (`web-app/src/lib/notification-policy.ts`)

```typescript
TOAST_STALE_MS = 5 * 60 * 1000        // 5 min — non-actionable toast sweep
TOAST_DEDUP_WINDOW_MS = 10_000          // 10s — same (sessionId, type) suppressed
ACTIONABLE_TOAST_STALE_MS = 6 * 60 * 1000  // 6 min — approval/question toasts
```

`toastAutoCloseMs(type)`:
- actionable → 6 min (ACTIONABLE_TOAST_STALE_MS)
- error/task_failed → 12s
- default → 8s

`toastAutoMinimizeMs(type)`:
- actionable → 0 (never minimize)
- error/warning → 5s
- default → 3s

### Toast lifecycle (NotificationContext.tsx)

- Active toasts in `notifications: NotificationData[]`
- Stale sweep runs every 60s via `setInterval`
- `addNotification` replaces existing toast for same `sessionId` (only latest shown per session)
- Native Notification objects: created fire-and-forget with no stored reference

## Summary

- **Backend**: needs 2 new fields on `forkMonitor` (`lastAlertZombieCount`, `lastAlertLevel`) under existing `alertMu` lock; no new deps
- **Frontend**: `showBrowserNotification` needs to store and close previous `Notification` objects; a `Map<string, Notification>` keyed by `(sessionId, type)` tag would cover FR-4
- **Policy constants**: all notification timing constants are centralized in `notification-policy.ts` — new constants for native notification auto-dismiss (FR-3) should go here
